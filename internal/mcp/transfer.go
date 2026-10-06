package mcp

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/verdantflarehub/verdantflare-studio/internal/project"
)

// TransferScope recognizes only the public binary routes. It never accepts a
// destination URL, internal retention operation, or ambiguous access scope.
func TransferScope(method, path, query string) (string, error) {
	parts := strings.Split(path, "/")
	if method == "PUT" && len(parts) == 6 && parts[0] == "" && parts[1] == "v2" && parts[2] == "artifacts" && parts[3] == "uploads" && project.ValidID(parts[4]) && parts[5] == "content" && query == "" {
		return "", nil
	}
	if method != "GET" || len(parts) != 5 || parts[0] != "" || parts[1] != "v2" || parts[2] != "artifacts" || !project.ValidID(parts[3]) || parts[4] != "content" {
		return "", errors.New("invalid content route")
	}
	q, err := url.ParseQuery(query)
	allowed := map[string]bool{"store_id": true, "artifact_id": true, "project_id": true, "project_revision_id": true, "asset_id": true, "asset_version_id": true}
	if err != nil {
		return "", errors.New("invalid content query")
	}
	for k, v := range q {
		if !allowed[k] || len(v) != 1 || !project.ValidID(v[0]) {
			return "", errors.New("invalid content query")
		}
	}
	if !project.ValidID(q.Get("store_id")) || !project.ValidID(q.Get("artifact_id")) {
		return "", errors.New("missing content identity")
	}
	hasProject, hasAsset := q.Get("project_id") != "", q.Get("asset_id") != ""
	if hasProject != (q.Get("project_revision_id") != "") || hasAsset != (q.Get("asset_version_id") != "") || (hasProject && hasAsset) {
		return "", errors.New("invalid access scope")
	}
	return q.Get("project_id"), nil
}

func (g *Gateway) Transfer(ctx context.Context, p project.Principal, method, path, query string, body io.Reader, length int64) (*http.Response, int, error) {
	if !p.Valid() {
		return nil, 403, errors.New("verified identity required")
	}
	if _, err := TransferScope(method, path, query); err != nil {
		return nil, 400, err
	}
	g.mu.RLock()
	svc, ok := g.services["artifact"]
	g.mu.RUnlock()
	name := "artifact.read"
	if method == "PUT" {
		name = "artifact.write"
	}
	found := false
	for _, t := range svc.Tools {
		if t.Name == name {
			found = true
		}
	}
	if !ok || !found || !endpointValid(svc.Endpoint) {
		return nil, 503, errors.New("content service unavailable")
	}
	u, err := url.Parse(svc.Endpoint)
	if err != nil || u.Path != "/mcp" {
		return nil, 503, errors.New("invalid content service endpoint")
	}
	token := g.resolveToken("artifact")
	if token == "" {
		return nil, 503, errors.New("internal service credential unavailable")
	}
	u.Path = path
	u.RawPath = ""
	u.RawQuery = query
	// The caller owns its source (a desktop file or inbound HTTP body).
	// Closing the outbound request must not close that source a second time.
	var requestBody io.Reader
	if body != nil {
		requestBody = io.NopCloser(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, u.String(), requestBody)
	if err != nil {
		return nil, 400, errors.New("invalid content request")
	}
	req.ContentLength = length
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-User-Id", p.SubjectID)
	req.Header.Set("X-Organization-Id", p.OrganizationID)
	req.Header.Set("X-Request-Id", p.RequestID)
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Accept-Encoding", "identity")
	client := &http.Client{Transport: g.httpClient.Transport, Timeout: 30 * time.Minute, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(req)
	if err != nil {
		return nil, 502, errors.New("content service unavailable")
	}
	if response.StatusCode >= 300 && response.StatusCode < 400 {
		response.Body.Close()
		return nil, 502, errors.New("content redirect rejected")
	}
	return response, response.StatusCode, nil
}
