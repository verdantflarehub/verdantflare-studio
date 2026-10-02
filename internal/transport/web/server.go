package web

import (
	"io"
	"io/fs"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/verdantflarehub/verdantflare-studio/internal/application"
	"github.com/verdantflarehub/verdantflare-studio/internal/mcp"
)

const cookieName = "vf_studio_session"
const defaultSessionTTL = 72 * time.Hour
const defaultSessionMaxAge = int(defaultSessionTTL / time.Second) // 259200 (3 days)

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
	mcpGateway *mcp.Gateway
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
	r.GET("/", gin.WrapH(rootFiles))
	r.GET("/assets/*path", gin.WrapH(rootFiles))
	r.GET("/studio", func(c *gin.Context) { c.Redirect(301, "/") })
	r.GET("/studio/", func(c *gin.Context) { c.Redirect(301, "/") })
	r.GET("/studio/assets/*path", gin.WrapH(studioFiles))
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
	s.mu.Lock()
	now := time.Now()
	for id, ss := range s.sessions {
		if !ss.expires.After(now) {
			delete(s.sessions, id)
		}
	}
	ss := s.sessions[sid]
	s.mu.Unlock()
	out := s.station.Call(c.Request.Context(), ss.token, application.Request{Path: path, Method: c.Request.Method, Body: body})
	c.Header("X-Request-ID", out.RequestID)
	if path == "login" && out.Status == 201 {
		s.mu.Lock()
		delete(s.sessions, sid)
		if len(s.sessions) >= 10000 {
			s.mu.Unlock()
			c.JSON(503, gin.H{"code": "SERVICE_UNAVAILABLE"})
			return
		}
		sid = application.ID()
		s.sessions[sid] = session{out.Token, time.Now().Add(defaultSessionTTL)}
		s.mu.Unlock()
		http.SetCookie(c.Writer, &http.Cookie{Name: cookieName, Value: sid, Path: "/", HttpOnly: true, Secure: s.secure, SameSite: http.SameSiteStrictMode, MaxAge: defaultSessionMaxAge})
		http.SetCookie(c.Writer, &http.Cookie{Name: cookieName, Value: "", Path: "/studio", HttpOnly: true, Secure: s.secure, SameSite: http.SameSiteStrictMode, MaxAge: -1})
	}
	if path == "logout" || out.Status == 401 {
		s.mu.Lock()
		delete(s.sessions, sid)
		s.mu.Unlock()
		http.SetCookie(c.Writer, &http.Cookie{Name: cookieName, Value: "", Path: "/", HttpOnly: true, Secure: s.secure, SameSite: http.SameSiteStrictMode, MaxAge: -1})
		http.SetCookie(c.Writer, &http.Cookie{Name: cookieName, Value: "", Path: "/studio", HttpOnly: true, Secure: s.secure, SameSite: http.SameSiteStrictMode, MaxAge: -1})
	}
	if out.Status == 204 {
		c.Status(204)
		return
	}
	c.Data(out.Status, "application/json", out.Data)
}
