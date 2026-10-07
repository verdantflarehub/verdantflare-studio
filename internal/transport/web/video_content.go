package web

import (
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/verdantflarehub/verdantflare-studio/internal/mcp"
)

func (s *Server) videoContent(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	r := c.Request
	if !mcp.VideoContentPath(r.URL.Path) || r.URL.RawPath != "" || r.URL.RawQuery != "" || r.URL.ForceQuery || r.ContentLength != 0 || len(r.TransferEncoding) != 0 || r.Header.Get("Content-Encoding") != "" || len(r.Header.Values("Range")) > 1 {
		c.Status(400)
		return
	}
	if site := r.Header.Get("Sec-Fetch-Site"); site == "cross-site" || site == "same-site" {
		c.Status(403)
		return
	}
	p, legacy, ok := s.authenticateMCP(c, true)
	if !ok {
		return
	}
	s.mu.Lock()
	gateway := s.mcpGateway
	s.mu.Unlock()
	if gateway == nil {
		c.Status(503)
		return
	}
	controller := http.NewResponseController(c.Writer)
	if controller.SetWriteDeadline(time.Now().Add(30*time.Minute)) != nil {
		c.Status(503)
		return
	}
	identity := map[string]string{"X-Request-Id": c.Writer.Header().Get("X-Request-Id")}
	if !legacy {
		identity["X-User-Id"] = p.SubjectID
		identity["X-Organization-Id"] = p.OrganizationID
	}
	response, status, err := gateway.VideoContent(r.Context(), r.URL.Path, r.Header.Get("Range"), identity)
	if err != nil {
		c.Status(status)
		return
	}
	defer response.Body.Close()
	if status != 200 && status != 206 {
		if status == 416 {
			c.Header("Content-Range", response.Header.Get("Content-Range"))
		} else if status != 404 {
			status = 502
		}
		c.Status(status)
		return
	}
	if response.ContentLength < 0 || response.Header.Get("Content-Encoding") != "" {
		c.Status(502)
		return
	}
	for _, key := range []string{"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges", "ETag"} {
		if value := response.Header.Get(key); value != "" {
			c.Header(key, value)
		}
	}
	c.Header("Content-Disposition", "attachment")
	c.Status(status)
	_, _ = io.CopyN(c.Writer, response.Body, response.ContentLength)
}
