package web

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/verdantflarehub/verdantflare-studio/internal/project"
)

var comfyuiAlias = regexp.MustCompile(`^comfyui[A-Za-z0-9]{1,57}$`)
var comfyuiAsset = regexp.MustCompile(`^[A-Za-z0-9_./-]+$`)

func (s *Server) comfyuiCall(ctx context.Context, method, path string, body []byte, p *project.Principal) (*http.Response, func(), error) {
	u, ok := s.comfyuiOrigin()
	token := os.Getenv("STUDIO_COMFYUI_SERVICE_TOKEN")
	if !ok || len(token) < 32 || strings.ContainsAny(token, " \t\r\n") {
		return nil, func() {}, io.ErrClosedPipe
	}
	u.Path = path
	req, err := http.NewRequestWithContext(ctx, method, u.String(), bytes.NewReader(body))
	if err != nil {
		return nil, func() {}, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	if p != nil {
		req.Header.Set("X-User-Id", p.SubjectID)
		req.Header.Set("X-Organization-Id", p.OrganizationID)
		req.Header.Set("X-Request-Id", p.RequestID)
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	client := &http.Client{Transport: transport, Timeout: 25 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(req)
	return response, transport.CloseIdleConnections, err
}

func (s *Server) comfyuiEditor(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	action := c.Param("action")
	if !comfyuiAlias.MatchString(c.Param("instance_alias")) || c.Request.URL.RawQuery != "" || strings.Contains(c.Request.URL.EscapedPath(), "%") {
		c.Status(400)
		return
	}
	switch action {
	case "open", "renew", "close", "draft", "request", "events":
	default:
		c.Status(404)
		return
	}
	p, ok := s.authenticateMCP(c)
	if !ok {
		return
	}
	if len(c.Request.Header.Values("Content-Type")) != 1 || strings.Split(c.GetHeader("Content-Type"), ";")[0] != "application/json" || c.GetHeader("Content-Encoding") != "" {
		c.Status(415)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, (1<<20)+4096))
	if err != nil {
		c.Status(413)
		return
	}
	if !json.Valid(body) {
		c.Status(400)
		return
	}
	response, done, err := s.comfyuiCall(c.Request.Context(), "POST", "/internal/editor/"+c.Param("instance_alias")+"/"+action, body, &p)
	defer done()
	if err != nil {
		c.Status(502)
		return
	}
	defer response.Body.Close()
	if action == "events" && response.StatusCode == 200 {
		if response.Header.Get("Content-Type") != "application/x-ndjson" {
			c.Status(502)
			return
		}
		c.Header("Content-Type", "application/x-ndjson")
		c.Header("X-Accel-Buffering", "no")
		c.Status(200)
		scanner := bufio.NewScanner(io.LimitReader(response.Body, 1<<20))
		scanner.Buffer(make([]byte, 4096), 65536)
		for scanner.Scan() {
			if !json.Valid(scanner.Bytes()) {
				return
			}
			if _, err := c.Writer.Write(append(append([]byte(nil), scanner.Bytes()...), '\n')); err != nil {
				return
			}
			c.Writer.Flush()
		}
		return
	}
	if strings.Split(response.Header.Get("Content-Type"), ";")[0] != "application/json" || response.StatusCode < 200 || response.StatusCode >= 500 {
		c.Status(502)
		return
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, (8<<20)+1))
	if err != nil || len(data) > 8<<20 || !json.Valid(data) {
		c.Status(502)
		return
	}
	c.Data(response.StatusCode, "application/json", data)
}

// This route serves only immutable, non-personal upstream + adapter bytes.
// Opaque-origin module imports cannot carry Studio's session cookie.
func (s *Server) comfyuiStatic(c *gin.Context) {
	path := strings.TrimPrefix(c.Param("path"), "/")
	entry := path == ""
	if entry {
		path = "vf-embed.html"
	}
	if !comfyuiAsset.MatchString(path) || strings.Contains(c.Request.URL.EscapedPath(), "%") {
		c.Status(404)
		return
	}
	for _, part := range strings.Split(path, "/") {
		if part == "" || strings.HasPrefix(part, ".") {
			c.Status(404)
			return
		}
	}
	if c.Request.URL.RawQuery != "" && (!entry || len(c.Request.URL.Query()) != 1 || len(c.Request.URL.Query()["parent_origin"]) != 1 || c.Query("parent_origin") != s.origin) {
		c.Status(400)
		return
	}
	response, done, err := s.comfyuiCall(c.Request.Context(), "GET", "/internal/frontend/"+path, nil, nil)
	defer done()
	if err != nil {
		c.Status(503)
		return
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		c.Status(404)
		return
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, (16<<20)+1))
	if err != nil || len(data) > 16<<20 {
		c.Status(502)
		return
	}
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Referrer-Policy", "no-referrer")
	c.Header("Cache-Control", "no-cache")
	if strings.HasSuffix(path, ".html") {
		base := s.origin + "/apps/comfyui/static/"
		c.Header("Content-Security-Policy", "default-src 'none'; script-src "+base+" blob: 'wasm-unsafe-eval'; style-src "+base+" 'unsafe-inline'; img-src "+base+" data: blob:; font-src "+base+" data:; connect-src data:; worker-src blob:; frame-src 'none'; frame-ancestors "+s.origin+"; base-uri 'none'; form-action 'none'; sandbox allow-scripts allow-downloads")
	}
	c.Data(200, response.Header.Get("Content-Type"), data)
}
