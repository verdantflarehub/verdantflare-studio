package mcp

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"
)

var videoContentPath = regexp.MustCompile(`^/artifacts/art_[0-9a-f]{32}/content$`)

func VideoContentPath(path string) bool { return videoContentPath.MatchString(path) }

// VideoContent serves only the legacy Video result namespace. Authorization is
// performed by the public transport with the same policy as media MCP calls.
func (g *Gateway) VideoContent(ctx context.Context, path, byteRange string, identity map[string]string) (*http.Response, int, error) {
	if !VideoContentPath(path) {
		return nil, 400, errors.New("invalid video content path")
	}
	return g.legacyMediaContent(ctx, "video", "video.result", path, byteRange, identity)
}

func (g *Gateway) legacyMediaContent(ctx context.Context, domain, requiredTool, path, byteRange string, identity map[string]string) (*http.Response, int, error) {
	g.mu.RLock()
	svc, ok := g.services[domain]
	g.mu.RUnlock()
	found := false
	for _, tool := range svc.Tools {
		if tool.Name == requiredTool {
			found = true
		}
	}
	u, err := url.Parse(svc.Endpoint)
	if !ok || !found || !endpointValid(svc.Endpoint) || err != nil || u.Path != "/mcp" || u.RawPath != "" {
		return nil, 503, errors.New("media content service unavailable")
	}
	// Never substitute external Studio or shared credentials for a missing domain key.
	token := strings.TrimSpace(os.Getenv("STUDIO_" + strings.ToUpper(domain) + "_TOKEN"))
	if token == "" {
		token = strings.TrimSpace(os.Getenv(strings.ToUpper(domain) + "_MCP_BEARER_TOKEN"))
	}
	if token == "" {
		return nil, 503, errors.New("media service credential unavailable")
	}
	u.Path = path
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, 503, errors.New("media content service unavailable")
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept-Encoding", "identity")
	if byteRange != "" {
		req.Header.Set("Range", byteRange)
	}
	for _, key := range []string{"X-User-Id", "X-Organization-Id", "X-Request-Id"} {
		if value := identity[key]; value != "" {
			req.Header.Set(key, value)
		}
	}
	client := &http.Client{Transport: g.httpClient.Transport, Timeout: 30 * time.Minute, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(req)
	if err != nil {
		return nil, 502, errors.New("media content service unavailable")
	}
	if response.StatusCode >= 300 && response.StatusCode < 400 {
		response.Body.Close()
		return nil, 502, errors.New("media content redirect rejected")
	}
	return response, response.StatusCode, nil
}
