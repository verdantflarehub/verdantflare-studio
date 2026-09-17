package web

import (
	"github.com/gin-gonic/gin"
	"github.com/verdantflarehub/verdantflare-studio/internal/application"
	"io"
	"io/fs"
	"net/http"
	"strings"
	"sync"
	"time"
)

const cookieName = "vf_studio_session"

type session struct {
	token   string
	expires time.Time
}
type Server struct {
	station  *application.Station
	origin   string
	secure   bool
	mu       sync.Mutex
	sessions map[string]session
}

func New(station *application.Station, origin string, assets fs.FS) *gin.Engine {
	s := &Server{station: station, origin: origin, secure: strings.HasPrefix(origin, "https://"), sessions: map[string]session{}}
	r := gin.New()
	r.Use(gin.Recovery())
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok", "service": "verdantflare-studio", "entrypoint": "gin", "version": "0.3.1"})
	})
	r.Any("/studio/api/*path", s.api)
	files := http.StripPrefix("/studio/", http.FileServer(http.FS(assets)))
	r.GET("/studio", func(c *gin.Context) { c.Redirect(302, "/studio/") })
	r.GET("/studio/", gin.WrapH(files))
	r.GET("/studio/assets/*path", gin.WrapH(files))
	return r
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
		s.sessions[sid] = session{out.Token, time.Now().Add(8 * time.Hour)}
		s.mu.Unlock()
		http.SetCookie(c.Writer, &http.Cookie{Name: cookieName, Value: sid, Path: "/studio", HttpOnly: true, Secure: s.secure, SameSite: http.SameSiteStrictMode, MaxAge: 8 * 3600})
	}
	if path == "logout" || out.Status == 401 {
		s.mu.Lock()
		delete(s.sessions, sid)
		s.mu.Unlock()
		http.SetCookie(c.Writer, &http.Cookie{Name: cookieName, Value: "", Path: "/studio", HttpOnly: true, Secure: s.secure, SameSite: http.SameSiteStrictMode, MaxAge: -1})
	}
	if out.Status == 204 {
		c.Status(204)
		return
	}
	c.Data(out.Status, "application/json", out.Data)
}
