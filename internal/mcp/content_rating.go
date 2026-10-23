package mcp

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/verdantflarehub/verdantflare-studio/internal/project"
)

var contentRatingVersionIDPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._:-]{0,127}$`)

func validContentRatingDomain(domain string) bool {
	switch domain {
	case "image", "speech", "music", "video":
		return true
	default:
		return false
	}
}

func validContentRatingAction(action string) bool {
	return action == "clear" || action == "mark"
}

func validContentRatingVersionID(versionID string) bool {
	return contentRatingVersionIDPattern.MatchString(versionID) && !strings.Contains(versionID, "..")
}

// ForwardContentRatingAction forwards a human clear/mark operation verified by
// the Studio web gateway to the domain service's internal content-ratings API.
// This endpoint is never exposed as an MCP tool.
func (g *Gateway) ForwardContentRatingAction(ctx context.Context, domain, versionID, action string, body []byte, p project.Principal, projectID string) ([]byte, int, error) {
	if !validContentRatingDomain(domain) {
		return nil, 400, errors.New("invalid content rating domain")
	}
	if !validContentRatingAction(action) {
		return nil, 400, errors.New("invalid content rating action")
	}
	if !validContentRatingVersionID(versionID) {
		return nil, 400, errors.New("invalid version_id")
	}
	if !p.Valid() {
		return nil, 403, errors.New("verified identity required")
	}
	g.mu.RLock()
	svc, ok := g.services[domain]
	g.mu.RUnlock()
	if !ok || !endpointValid(svc.Endpoint) {
		return nil, 503, errors.New("domain service unavailable")
	}
	u, err := url.Parse(svc.Endpoint)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil {
		return nil, 503, errors.New("domain service unavailable")
	}
	token := g.resolveToken(domain)
	if token == "" {
		return nil, 503, errors.New("internal service credential unavailable")
	}
	u.Path = "/internal/content-ratings/" + versionID + "/" + action
	u.RawPath = ""
	u.RawQuery = ""
	u.Fragment = ""
	if len(body) == 0 {
		body = []byte("{}")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(body))
	if err != nil {
		return nil, 503, errors.New("domain service unavailable")
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-User-Id", p.SubjectID)
	req.Header.Set("X-Organization-Id", p.OrganizationID)
	req.Header.Set("X-Request-Id", p.RequestID)
	if projectID != "" {
		req.Header.Set("X-Project-Id", projectID)
	}
	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, 502, errors.New("downstream unavailable")
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		return nil, 502, errors.New("downstream redirect rejected")
	}
	if resp.StatusCode >= 500 {
		return nil, 502, errors.New("downstream unavailable")
	}
	respBytes, err := io.ReadAll(io.LimitReader(resp.Body, (64<<10)+1))
	if err != nil || len(respBytes) > 64<<10 {
		return nil, 502, errors.New("invalid downstream response")
	}
	return respBytes, resp.StatusCode, nil
}
