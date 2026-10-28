package web

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/verdantflarehub/verdantflare-studio/internal/application"
	"github.com/verdantflarehub/verdantflare-studio/internal/project"
	"github.com/verdantflarehub/verdantflare-studio/internal/videotitle"
)

// The upstream and credential are server configuration, never caller-controlled.
type VideoConfig struct{ URL, Token string }

var videoRead = regexp.MustCompile(`^/(api/(dashboard|capabilities|model-catalog|mcp/status|models|models/[a-z0-9-]+/instances(/[a-zA-Z0-9._-]+(/gpus/[a-zA-Z0-9._:-]+)?)?|tasks/[a-zA-Z0-9_-]+(/(thumbnail|content))?)|artifacts/[a-zA-Z0-9_-]+/content)$`)
var videoWrite = regexp.MustCompile(`^/api/(tasks|tasks/[a-zA-Z0-9_-]+/(result|title|delete|content-rating)|artifacts/import)$`)
var videoTask = regexp.MustCompile(`^/dashboard/tasks/[a-zA-Z0-9_-]+$`)
var videoMemberRead = regexp.MustCompile(`^/api/higgsfield/(projects|capabilities|assets|assets/art_[a-zA-Z0-9_-]+/content|tasks|tasks/[a-zA-Z0-9_-]+(/(thumbnail|content))?)$`)
var videoMemberWrite = regexp.MustCompile(`^/api/higgsfield/(preflight|tasks|attempt|imports/(prepare|chunk|status|commit)|tasks/[a-zA-Z0-9_-]+/result)$`)

func videoStatic(path string) bool {
	if path == "/dashboard" || videoTask.MatchString(path) {
		return true
	}
	switch path {
	case "/dashboard/frontend/higgsfield-member.js", "/dashboard/frontend/higgsfield-member.css":
		return true
	case "/dashboard/static/model-catalog.js", "/dashboard/static/model-catalog.css", "/dashboard/frontend/model-catalog.js", "/dashboard/frontend/model-catalog.css":
		return true
	case "/dashboard/static/dashboard.css", "/dashboard/static/dashboard.js", "/dashboard/static/resources.js", "/dashboard/static/task-detail.js", "/dashboard/static/studio-embed.js", "/dashboard/static/studio-theme.css":
		return true
	case "/dashboard/frontend/dashboard.css", "/dashboard/frontend/dashboard.js", "/dashboard/frontend/resources.js", "/dashboard/frontend/task-detail.js", "/dashboard/frontend/studio-embed.js", "/dashboard/frontend/studio-theme.css":
		return true
	}
	return false
}
func (s *Server) video(c *gin.Context, cfg VideoConfig) {
	c.Header("Cache-Control", "no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	path := c.Param("path")
	static := c.Request.Method == "GET" && videoStatic(path)
	member := c.Request.Method == "GET" && videoMemberRead.MatchString(path) || c.Request.Method == "POST" && videoMemberWrite.MatchString(path)
	if !static && !member && !(c.Request.Method == "GET" && videoRead.MatchString(path)) && !(c.Request.Method == "POST" && videoWrite.MatchString(path)) {
		c.Status(404)
		return
	}
	if strings.Contains(c.Request.URL.EscapedPath(), "%") {
		c.Status(404)
		return
	}
	var principal project.Principal
	if !static {
		if (c.GetHeader("Origin") != "" && c.GetHeader("Origin") != s.origin) || c.GetHeader("Sec-Fetch-Site") == "cross-site" || (c.Request.Method == "POST" && c.GetHeader("Origin") != s.origin) {
			c.Status(403)
			return
		}
		sid, _ := c.Cookie(cookieName)
		token, valid := s.getSession(c.Request.Context(), sid)
		if !valid {
			c.Status(401)
			return
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
		if !admin && !member {
			c.Status(403)
			return
		}
		var status int
		principal, status = s.station.Principal(c.Request.Context(), token, uuid.Must(uuid.NewV7()).String())
		if status != 200 {
			c.Status(status)
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
		req.Header.Set("X-User-Id", principal.SubjectID)
		req.Header.Set("X-Organization-Id", principal.OrganizationID)
		req.Header.Set("X-Request-Id", principal.RequestID)
		if c.GetHeader("X-VF-Include-NSFW") == "1" {
			req.Header.Set("X-VF-Include-NSFW", "1")
		}
	}
	if c.Request.Method == "POST" {
		if strings.HasSuffix(path, "/title") {
			body, readErr := io.ReadAll(req.Body)
			if readErr != nil || c.Request.URL.RawQuery != "" || !videotitle.ValidBody(body) {
				c.Status(400)
				return
			}
			req.Body = io.NopCloser(bytes.NewReader(body))
			req.ContentLength = int64(len(body))
		}
		if strings.Split(c.GetHeader("Content-Type"), ";")[0] != "application/json" {
			c.Status(415)
			return
		}
		req.Header.Set("Content-Type", "application/json")
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
		// Video owns channel-error sanitization. Preserve its bounded public
		// envelope, never an arbitrary provider response or proxy error page.
		if !static && writeVideoChannelError(c, resp) {
			return
		}
		c.Status(502)
		return
	}
	for _, key := range []string{"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges", "Content-Disposition", "ETag"} {
		if v := resp.Header.Get(key); v != "" {
			c.Header(key, v)
		}
	}
	if static {
		c.Header("Content-Security-Policy", "default-src 'none'; script-src "+s.origin+"; style-src "+s.origin+" 'unsafe-inline'; img-src blob: data:; media-src blob:; connect-src 'none'; frame-ancestors 'self'; base-uri 'none'; form-action 'none'")
	}
	c.Status(resp.StatusCode)
	_, _ = io.Copy(c.Writer, resp.Body)
}
