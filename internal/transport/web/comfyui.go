package web

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/verdantflarehub/verdantflare-studio/internal/project"
)

const comfyuiHeadlessAlias = "comfyui-api"
const comfyuiMCPMaxRequestBytes = (1 << 20) + 4096
const comfyuiMCPMaxResponseBytes = 4 << 20

func isComfyUIMCPAlias(alias string) bool {
	return comfyuiAlias.MatchString(alias) || alias == comfyuiHeadlessAlias
}

func (s *Server) instanceMCP(c *gin.Context) {
	alias := c.Param("instance_alias")
	if blenderAlias.MatchString(alias) {
		s.blenderMCP(c)
		return
	}
	if isComfyUIMCPAlias(alias) {
		s.comfyuiMCP(c)
		return
	}
	c.Status(404)
}

func (s *Server) comfyuiMCP(c *gin.Context) {
	alias := c.Param("instance_alias")
	if !isComfyUIMCPAlias(alias) || c.Request.URL.RawQuery != "" || strings.Contains(c.Request.URL.EscapedPath(), "%") {
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
	if c.GetHeader("Content-Encoding") != "" || (c.Request.Method == "POST" && strings.Split(c.GetHeader("Content-Type"), ";")[0] != "application/json") {
		c.Status(415)
		return
	}
	s.comfyuiMCPForward(c, p, c.GetBool("verifiedOrgAdmin"), "/internal/mcp/"+alias)
}

func (s *Server) comfyuiMCPForward(c *gin.Context, p project.Principal, orgAdmin bool, path string) {
	c.Header("Cache-Control", "no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	u, ok := s.comfyuiOrigin()
	token := os.Getenv("STUDIO_COMFYUI_SERVICE_TOKEN")
	if !ok || len(token) < 32 || strings.ContainsAny(token, " \t\r\n") {
		c.Status(503)
		return
	}
	u.Path = path
	body, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, comfyuiMCPMaxRequestBytes))
	if err != nil {
		c.Status(413)
		return
	}
	req, err := http.NewRequestWithContext(c.Request.Context(), c.Request.Method, u.String(), bytes.NewReader(body))
	if err != nil {
		c.Status(400)
		return
	}
	// Reconstruct internal headers from verified Studio identity only; cookies,
	// external bearer tokens, spoofed project/instance headers and forwarding
	// headers never cross the internal trust boundary.
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-User-Id", p.SubjectID)
	req.Header.Set("X-Organization-Id", p.OrganizationID)
	req.Header.Set("X-Organization-Admin", strconv.FormatBool(orgAdmin))
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
	client := &http.Client{
		Transport:     transport,
		Timeout:       130 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
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
	data, err := io.ReadAll(io.LimitReader(resp.Body, comfyuiMCPMaxResponseBytes+1))
	if err != nil || len(data) > comfyuiMCPMaxResponseBytes {
		c.Status(502)
		return
	}
	if len(data) > 0 && (strings.Split(resp.Header.Get("Content-Type"), ";")[0] != "application/json" || !json.Valid(data)) {
		c.Status(502)
		return
	}
	c.Data(resp.StatusCode, "application/json", data)
}

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
