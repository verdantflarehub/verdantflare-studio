package web

import (
	"context"
	"io"
	"io/fs"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/verdantflarehub/verdantflare-studio/internal/application"
	"github.com/verdantflarehub/verdantflare-studio/internal/mcp"
	clientv3 "go.etcd.io/etcd/client/v3"
)

const cookieName = "vf_studio_session"
const defaultSessionTTL = 72 * time.Hour
const defaultSessionMaxAge = int(defaultSessionTTL / time.Second) // 259200 (3 days)
const sessionEtcdPrefix = "/verdantflare/studio/sessions/"

type session struct {
	token   string
	expires time.Time
}
type Server struct {
	station    *application.Station
	origin     string
	secure     bool
	mu         sync.Mutex
	sessions   map[string]session
	etcd       *clientv3.Client
	mcpGateway *mcp.Gateway
}

func (s *Server) SetEtcdClient(cli *clientv3.Client) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.etcd = cli
}

func (s *Server) getSession(ctx context.Context, sid string) (string, bool) {
	if sid == "" {
		return "", false
	}
	s.mu.Lock()
	now := time.Now()
	ss, ok := s.sessions[sid]
	if ok && ss.expires.After(now) && ss.token != "" {
		s.mu.Unlock()
		return ss.token, true
	}
	s.mu.Unlock()

	if s.etcd != nil {
		getCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		resp, err := s.etcd.Get(getCtx, sessionEtcdPrefix+sid)
		if err == nil && len(resp.Kvs) > 0 {
			tok := string(resp.Kvs[0].Value)
			if tok != "" {
				s.mu.Lock()
				s.sessions[sid] = session{tok, time.Now().Add(defaultSessionTTL)}
				s.mu.Unlock()
				return tok, true
			}
		}
	}
	return "", false
}

func (s *Server) saveSession(ctx context.Context, sid, token string) {
	s.mu.Lock()
	s.sessions[sid] = session{token, time.Now().Add(defaultSessionTTL)}
	s.mu.Unlock()

	if s.etcd != nil {
		go func() {
			leaseCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			lease, err := s.etcd.Grant(leaseCtx, int64(defaultSessionMaxAge))
			putCtx, pcancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer pcancel()
			if err == nil {
				_, _ = s.etcd.Put(putCtx, sessionEtcdPrefix+sid, token, clientv3.WithLease(lease.ID))
			} else {
				_, _ = s.etcd.Put(putCtx, sessionEtcdPrefix+sid, token)
			}
		}()
	}
}

func (s *Server) deleteSession(ctx context.Context, sid string) {
	if sid == "" {
		return
	}
	s.mu.Lock()
	delete(s.sessions, sid)
	s.mu.Unlock()

	if s.etcd != nil {
		go func() {
			delCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_, _ = s.etcd.Delete(delCtx, sessionEtcdPrefix+sid)
		}()
	}
}

func New(station *application.Station, origin string, assets fs.FS, video ...VideoConfig) *gin.Engine {
	engine, _ := NewServer(station, origin, assets, video...)
	return engine
}

func NewServer(station *application.Station, origin string, assets fs.FS, video ...VideoConfig) (*gin.Engine, *Server) {
	s := &Server{station: station, origin: origin, secure: strings.HasPrefix(origin, "https://"), sessions: map[string]session{}}
	r := gin.New()
	r.Use(gin.Recovery())
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok", "service": "verdantflare-studio", "entrypoint": "gin", "version": "0.5.2"})
	})
	r.POST("/mcp", s.mcpHandler)
	r.POST("/studio/mcp", s.mcpHandler)
	r.Any("/api/*path", s.api)
	r.Any("/studio/api/*path", s.api)
	if len(video) > 0 {
		r.Any("/apps/video/*path", func(c *gin.Context) { s.video(c, video[0]) })
		r.Any("/studio/apps/video/*path", func(c *gin.Context) { s.video(c, video[0]) })
	}
	rootFiles := http.FileServer(http.FS(assets))
	studioFiles := http.StripPrefix("/studio/", rootFiles)

	// 1. Immutable hashed static assets: 7-day cache
	serveAssets := func(h http.Handler) gin.HandlerFunc {
		return func(c *gin.Context) {
			c.Header("Cache-Control", "public, max-age=604800, immutable")
			c.Header("X-Content-Type-Options", "nosniff")
			h.ServeHTTP(c.Writer, c.Request)
		}
	}
	r.GET("/assets/*path", serveAssets(rootFiles))
	r.GET("/studio/assets/*path", serveAssets(studioFiles))

	// 2. Zero-cache for HTML entrypoint and redirect routes
	serveIndex := func(c *gin.Context) {
		c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
		c.Header("Pragma", "no-cache")
		c.Header("Expires", "0")
		c.Header("X-Content-Type-Options", "nosniff")
		data, err := fs.ReadFile(assets, "index.html")
		if err != nil {
			c.String(http.StatusInternalServerError, "Internal Server Error")
			return
		}
		c.Data(http.StatusOK, "text/html; charset=utf-8", data)
	}

	r.GET("/", serveIndex)
	r.GET("/index.html", serveIndex)
	r.GET("/studio", func(c *gin.Context) { c.Redirect(http.StatusMovedPermanently, "/") })
	r.GET("/studio/", func(c *gin.Context) { c.Redirect(http.StatusMovedPermanently, "/") })
	return r, s
}
func (s *Server) api(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	if c.Request.Method != "GET" && c.GetHeader("Origin") != s.origin {
		c.JSON(403, gin.H{"code": "PERMISSION_DENIED"})
		return
	}
	path := strings.TrimPrefix(c.Param("path"), "/")
	var body []byte
	if c.Request.Method == "POST" {
		if path != "logout" && strings.Split(c.GetHeader("Content-Type"), ";")[0] != "application/json" {
			c.JSON(400, gin.H{"code": "INVALID_ARGUMENT"})
			return
		}
		var err error
		body, err = io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, 16384))
		if err != nil {
			c.JSON(413, gin.H{"code": "INVALID_ARGUMENT"})
			return
		}
	}
	sid, _ := c.Cookie(cookieName)
	token, _ := s.getSession(c.Request.Context(), sid)
	out := s.station.Call(c.Request.Context(), token, application.Request{Path: path, Method: c.Request.Method, Body: body})
	c.Header("X-Request-ID", out.RequestID)
	if path == "login" && out.Status == 201 {
		if sid != "" {
			s.deleteSession(c.Request.Context(), sid)
		}
		s.mu.Lock()
		if len(s.sessions) >= 10000 {
			s.mu.Unlock()
			c.JSON(503, gin.H{"code": "SERVICE_UNAVAILABLE"})
			return
		}
		s.mu.Unlock()
		sid = application.ID()
		s.saveSession(c.Request.Context(), sid, out.Token)
		http.SetCookie(c.Writer, &http.Cookie{Name: cookieName, Value: sid, Path: "/", HttpOnly: true, Secure: s.secure, SameSite: http.SameSiteStrictMode, MaxAge: defaultSessionMaxAge})
		http.SetCookie(c.Writer, &http.Cookie{Name: cookieName, Value: "", Path: "/studio", HttpOnly: true, Secure: s.secure, SameSite: http.SameSiteStrictMode, MaxAge: -1})
	}
	if path == "logout" || out.Status == 401 {
		if sid != "" {
			s.deleteSession(c.Request.Context(), sid)
		}
		http.SetCookie(c.Writer, &http.Cookie{Name: cookieName, Value: "", Path: "/", HttpOnly: true, Secure: s.secure, SameSite: http.SameSiteStrictMode, MaxAge: -1})
		http.SetCookie(c.Writer, &http.Cookie{Name: cookieName, Value: "", Path: "/studio", HttpOnly: true, Secure: s.secure, SameSite: http.SameSiteStrictMode, MaxAge: -1})
	}
	if out.Status == 204 {
		c.Status(204)
		return
	}
	c.Data(out.Status, "application/json", out.Data)
}
