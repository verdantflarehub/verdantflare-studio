package web

import (
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/verdantflarehub/verdantflare-studio/internal/application"
)

// The upstream and credential are server configuration, never caller-controlled.
type ImageConfig struct{ URL, Token string }

var imageRead = regexp.MustCompile(`^/(api/(tasks|tasks/[a-zA-Z0-9_-]+|tasks/stats|mcp/status)|artifacts/[a-zA-Z0-9_-]+/content)$`)
var imageWrite = regexp.MustCompile(`^/api/(tasks|tasks/[a-zA-Z0-9_-]+/(title|delete|content-rating)|artifacts/upload)$`)
var imageContent = regexp.MustCompile(`^/artifacts/art-[0-9a-f]{16}/content$`)

func imageStatic(path string) bool {
	if path == "/dashboard" || path == "/dashboard/" || path == "/image/dashboard" || path == "/image/dashboard/" {
		return true
	}
	switch path {
	case "/dashboard/static/dashboard.css", "/dashboard/static/dashboard.js":
		return true
	}
	return false
}

func (s *Server) EnableImage(r *gin.Engine, cfg ImageConfig) {
	if cfg.URL == "" {
		return
	}
	s.mu.Lock()
	s.imageConfig = cfg
	s.mu.Unlock()
	r.Any("/apps/image/*path", func(c *gin.Context) { s.image(c, cfg) })
	r.Any("/studio/apps/image/*path", func(c *gin.Context) { s.image(c, cfg) })
}

func (s *Server) image(c *gin.Context, cfg ImageConfig) {
	c.Header("Cache-Control", "no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	path := c.Param("path")
	static := c.Request.Method == "GET" && imageStatic(path)
	if !static && !(c.Request.Method == "GET" && imageRead.MatchString(path)) && !(c.Request.Method == "POST" && imageWrite.MatchString(path)) {
		c.Status(404)
		return
	}
	if strings.Contains(c.Request.URL.EscapedPath(), "%") {
		c.Status(404)
		return
	}
	if !static {
		if (c.GetHeader("Origin") != "" && c.GetHeader("Origin") != s.origin) || c.GetHeader("Sec-Fetch-Site") == "cross-site" || (c.Request.Method == "POST" && c.GetHeader("Origin") != s.origin) {
			c.Status(403)
			return
		}
		var token string
		if c.GetHeader("Authorization") != "" {
			// Existing admin content access may use the caller's MCP identity.
			// Do not extend Bearer access to dashboard APIs or writes.
			r := c.Request
			if r.Method != http.MethodGet || !imageContent.MatchString(path) || r.URL.RawQuery != "" || r.URL.ForceQuery || r.ContentLength != 0 || len(r.TransferEncoding) != 0 || r.Header.Get("Content-Encoding") != "" || len(r.Header.Values("Range")) > 1 {
				c.Status(400)
				return
			}
			if c.GetHeader("Sec-Fetch-Site") == "same-site" {
				c.Status(403)
				return
			}
			if _, valid := s.authenticateMCP(c); !valid {
				return
			}
			token = c.GetString("verifiedStationToken")
		} else {
			sid, _ := c.Cookie(cookieName)
			var valid bool
			token, valid = s.getSession(c.Request.Context(), sid)
			if !valid {
				c.Status(401)
				return
			}
		}
		auth := s.station.Call(c.Request.Context(), token, application.Request{Path: "me", Method: "GET"})
		if auth.Status != 200 {
			c.Status(auth.Status)
			return
		}
		var identity struct {
			Roles []string `json:"roles"`
		}
		_ = json.Unmarshal(auth.Data, &identity)
		admin := false
		for _, role := range identity.Roles {
			if role == "admin" {
				admin = true
			}
		}
		if !admin {
			c.Status(403)
			return
		}
	}
	upstream, err := url.Parse(cfg.URL)
	if err != nil || upstream.Host == "" || (upstream.Scheme != "http" && upstream.Scheme != "https") || upstream.User != nil || cfg.Token == "" {
		c.Status(503)
		return
	}
	upstream.Path = strings.TrimRight(upstream.Path, "/") + path
	upstream.RawQuery = c.Request.URL.RawQuery
	req, err := http.NewRequestWithContext(c.Request.Context(), c.Request.Method, upstream.String(), http.MaxBytesReader(c.Writer, c.Request.Body, 1<<20))
	if err != nil {
		c.Status(400)
		return
	}
	if !static {
		req.Header.Set("Authorization", "Bearer "+cfg.Token)
		if c.GetHeader("Authorization") == "" && c.GetHeader("X-VF-Include-NSFW") == "1" {
			req.Header.Set("X-VF-Include-NSFW", "1")
		}
	}
	if c.Request.Method == "POST" {
		contentType := c.GetHeader("Content-Type")
		mediaType, params, parseErr := mime.ParseMediaType(contentType)
		valid := parseErr == nil && mediaType == "application/json"
		if path == "/api/artifacts/upload" {
			valid = parseErr == nil && mediaType == "multipart/form-data" && params["boundary"] != ""
		}
		if !valid {
			c.Status(415)
			return
		}
		req.Header.Set("Content-Type", contentType)
	}
	if value := c.GetHeader("Range"); value != "" {
		req.Header.Set("Range", value)
	}
	client := &http.Client{Timeout: 90 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		c.Status(502)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		c.Status(502)
		return
	}
	if resp.StatusCode >= 500 {
		c.Status(502)
		return
	}
	for _, key := range []string{"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges", "Content-Disposition", "ETag"} {
		if v := resp.Header.Get(key); v != "" {
			c.Header(key, v)
		}
	}
	if static {
		c.Header("Content-Security-Policy", "default-src 'none'; script-src "+s.origin+" 'unsafe-inline'; style-src "+s.origin+" 'unsafe-inline'; img-src blob: data: https://images.unsplash.com; media-src blob:; connect-src 'none'; frame-ancestors 'self'; base-uri 'none'; form-action 'none'")
	}
	c.Status(resp.StatusCode)
	_, _ = io.Copy(c.Writer, resp.Body)
}
