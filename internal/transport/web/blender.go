package web

import (
	"bytes"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/verdantflarehub/verdantflare-studio/internal/project"
)

var blenderAlias = regexp.MustCompile(`^blender[A-Za-z0-9_-]{1,57}$`)

func (s *Server) blenderOrigin() (*url.URL, bool) {
	var endpoint string
	var ok bool
	if s.blenderEndpoint != nil {
		endpoint, ok = s.blenderEndpoint()
	} else if s.mcpGateway != nil {
		endpoint, ok = s.mcpGateway.ServiceEndpoint("blender")
	}
	u, err := url.Parse(endpoint)
	if !ok || err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "/internal/mcp" {
		return nil, false
	}
	return u, true
}

func (s *Server) blenderMCP(c *gin.Context) {
	if !blenderAlias.MatchString(c.Param("instance_alias")) || c.Request.URL.RawQuery != "" || strings.Contains(c.Request.URL.EscapedPath(), "%") {
		c.Status(404)
		return
	}
	p, ok := s.authenticateMCP(c)
	if !ok {
		return
	}
	if c.Request.Method != "POST" && c.Request.Method != "GET" && c.Request.Method != "DELETE" {
		c.Status(405)
		return
	}
	for _, h := range []string{"MCP-Protocol-Version", "Mcp-Session-Id", "Content-Type"} {
		if len(c.Request.Header.Values(h)) > 1 {
			c.Status(400)
			return
		}
	}
	if c.Request.Method == "POST" && strings.Split(c.GetHeader("Content-Type"), ";")[0] != "application/json" {
		c.Status(415)
		return
	}
	s.blenderForward(c, p, "/internal/mcp/"+c.Param("instance_alias"))
}

func (s *Server) blenderInstances(c *gin.Context) {
	p, ok := s.authenticateMCP(c)
	if !ok {
		return
	}
	s.blenderForward(c, p, "/internal/instances")
}

func (s *Server) blenderManagement(c *gin.Context) {
	if c.Request.URL.RawQuery != "" || strings.Contains(c.Request.URL.EscapedPath(), "%") || c.GetHeader("Content-Encoding") != "" {
		c.Status(400)
		return
	}
	path := "/internal/instance-options"
	if c.Request.Method == "POST" {
		if len(c.Request.Header.Values("Content-Type")) != 1 || strings.Split(c.GetHeader("Content-Type"), ";")[0] != "application/json" {
			c.Status(415)
			return
		}
		path = "/internal/instances"
		if alias := c.Param("instance_alias"); alias != "" {
			if !blenderAlias.MatchString(alias) {
				c.Status(404)
				return
			}
			path = "/internal/instances/" + alias + "/start"
		}
		switch c.Request.URL.Path {
		case "/studio/apps/blender/instance-projects":
			path = "/internal/instance-projects"
		case "/studio/apps/blender/instance-source":
			path = "/internal/instance-source"
		case "/studio/apps/blender/instance-project-create":
			path = "/internal/instance-project-create"
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16<<10)
	} else if id := c.Param("operation_id"); id != "" {
		if !project.ValidID(id) {
			c.Status(404)
			return
		}
		path = "/internal/instance-operations/" + id
	}
	p, ok := s.authenticateMCP(c)
	if !ok {
		return
	}
	s.blenderForward(c, p, path)
}

func (s *Server) blenderForward(c *gin.Context, p project.Principal, path string) {
	c.Header("Cache-Control", "no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	u, ok := s.blenderOrigin()
	token := os.Getenv("STUDIO_BLENDER_SERVICE_TOKEN")
	if !ok || token == "" {
		c.Status(503)
		return
	}
	u.Path = path
	body, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, 1<<20))
	if err != nil {
		c.Status(413)
		return
	}
	req, err := http.NewRequestWithContext(c.Request.Context(), c.Request.Method, u.String(), bytes.NewReader(body))
	if err != nil {
		c.Status(400)
		return
	}
	// Reconstruct headers, so cookies, external tokens, project/instance overrides
	// and forwarding headers cannot cross the internal trust boundary.
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-User-Id", p.SubjectID)
	req.Header.Set("X-Organization-Id", p.OrganizationID)
	req.Header.Set("X-Request-Id", p.RequestID)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	for _, h := range []string{"MCP-Protocol-Version", "Mcp-Session-Id"} {
		if v := c.GetHeader(h); v != "" {
			req.Header.Set(h, v)
		}
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 35 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
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
	data, err := io.ReadAll(io.LimitReader(resp.Body, (4<<20)+1))
	if err != nil || len(data) > 4<<20 {
		c.Status(502)
		return
	}
	if len(data) > 0 && strings.Split(resp.Header.Get("Content-Type"), ";")[0] != "application/json" {
		c.Status(502)
		return
	}
	// The first release intentionally uses stateless JSON MCP. Do not fabricate
	// transport sessions or treat a worker's HTML/SSE response as successful JSON.
	if path == "/internal/instances" && resp.StatusCode == http.StatusOK {
		data, err = s.blenderResourceView(c, data)
		if err != nil {
			c.Status(http.StatusBadGateway)
			return
		}
	}
	c.Data(resp.StatusCode, "application/json", data)
}
