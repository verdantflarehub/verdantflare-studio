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
	g.mu.RLock()
	svc, ok := g.services["video"]
	g.mu.RUnlock()
	found := false
	for _, tool := range svc.Tools {
		if tool.Name == "video.result" {
			found = true
		}
	}
	u, err := url.Parse(svc.Endpoint)
	if !ok || !found || !endpointValid(svc.Endpoint) || err != nil || u.Path != "/mcp" || u.RawPath != "" {
		return nil, 503, errors.New("video content service unavailable")
	}
	// Never substitute the external Studio credential for a missing Video key.
	token := strings.TrimSpace(os.Getenv("STUDIO_VIDEO_TOKEN"))
	if token == "" {
		token = strings.TrimSpace(os.Getenv("VIDEO_MCP_BEARER_TOKEN"))
	}
	if token == "" {
		return nil, 503, errors.New("video service credential unavailable")
	}
	u.Path = path
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, 503, errors.New("video content service unavailable")
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
		return nil, 502, errors.New("video content service unavailable")
	}
	if response.StatusCode >= 300 && response.StatusCode < 400 {
		response.Body.Close()
		return nil, 502, errors.New("video content redirect rejected")
	}
	return response, response.StatusCode, nil
}
