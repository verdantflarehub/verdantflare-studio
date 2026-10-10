package web

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// Instance directory only. Upstream editor bytes and APIs are not a wildcard proxy.
func (s *Server) comfyuiOrigin() (*url.URL, bool) {
	var endpoint string
	var ok bool
	if s.comfyuiEndpoint != nil {
		endpoint, ok = s.comfyuiEndpoint()
	} else if configured := os.Getenv("STUDIO_COMFYUI_URL"); configured != "" {
		endpoint, ok = configured, true
	} else if s.mcpGateway != nil {
		endpoint, ok = s.mcpGateway.ServiceEndpoint("comfyui")
	} else {
		ok = false
	}
	u, err := url.Parse(endpoint)
	if !ok || err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "/internal/mcp" {
		return nil, false
	}
	return u, true
}

func (s *Server) comfyuiInstances(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	if c.Request.URL.RawQuery != "" || strings.Contains(c.Request.URL.EscapedPath(), "%") {
		c.Status(400)
		return
	}
	p, ok := s.authenticateMCP(c)
	if !ok {
		return
	}
	u, ok := s.comfyuiOrigin()
	token := os.Getenv("STUDIO_COMFYUI_SERVICE_TOKEN")
	if !ok || len(token) < 32 || strings.ContainsAny(token, " \t\r\n") {
		c.Status(503)
		return
	}
	u.Path = "/internal/instances"
	req, err := http.NewRequestWithContext(c.Request.Context(), "GET", u.String(), nil)
	if err != nil {
		c.Status(502)
		return
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-User-Id", p.SubjectID)
	req.Header.Set("X-Organization-Id", p.OrganizationID)
	req.Header.Set("X-Request-Id", p.RequestID)
	req.Header.Set("Accept", "application/json")
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		c.Status(502)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == 401 || resp.StatusCode == 403 || resp.StatusCode == 404 {
			c.Status(resp.StatusCode)
		} else {
			c.Status(502)
		}
		return
	}
	if strings.Split(resp.Header.Get("Content-Type"), ";")[0] != "application/json" {
		c.Status(502)
		return
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, (1<<20)+1))
	if err != nil || len(data) > 1<<20 || !json.Valid(data) {
		c.Status(502)
		return
	}
	// Same authoritative Pod/container join and stale handling as Blender.
	data, err = s.blenderResourceView(c, data)
	if err != nil {
		c.Status(502)
		return
	}
	c.Data(200, "application/json", data)
}
