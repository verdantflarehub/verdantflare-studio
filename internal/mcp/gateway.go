package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/verdantflarehub/verdantflare-studio/internal/project"
	clientv3 "go.etcd.io/etcd/client/v3"
)

const ServicesPrefix = "/verdantflare/mcp/services/"
const DefaultTimeout = 30 * time.Second

type ToolDefinition struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}
type ServiceRegistration struct {
	Domain         string           `json:"domain"`
	Endpoint       string           `json:"endpoint"`
	HealthEndpoint string           `json:"health_endpoint"`
	Version        string           `json:"version"`
	UpdatedAt      string           `json:"updated_at"`
	Tools          []ToolDefinition `json:"tools"`
}
type Gateway struct {
	client     *clientv3.Client
	httpClient *http.Client
	mu         sync.RWMutex
	services   map[string]ServiceRegistration
}

func NewGateway(endpoints []string) (*Gateway, error) {
	cli, e := clientv3.New(clientv3.Config{Endpoints: endpoints, DialTimeout: 5 * time.Second})
	if e != nil {
		return nil, errors.New("invalid registry configuration")
	}
	return NewGatewayWithClient(cli), nil
}
func NewGatewayWithClient(cli *clientv3.Client) *Gateway {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	return &Gateway{client: cli, httpClient: &http.Client{Transport: transport, Timeout: 120 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, services: map[string]ServiceRegistration{}}
}

var domainPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]*$`)
var toolPattern = regexp.MustCompile(`^[a-z][a-z0-9_]*\.[a-z][a-z0-9_]*$`)
var versionPattern = regexp.MustCompile(`^v?[0-9]+\.[0-9]+\.[0-9]+.*$`)

func endpointValid(raw string) bool {
	u, e := url.Parse(raw)
	return e == nil && u.Host != "" && (u.Scheme == "http" || u.Scheme == "https") && u.User == nil && u.RawQuery == "" && u.Fragment == ""
}
func validRegistration(key string, r ServiceRegistration) bool {
	if key != ServicesPrefix+r.Domain || !domainPattern.MatchString(r.Domain) || !endpointValid(r.Endpoint) || !endpointValid(r.HealthEndpoint) || !versionPattern.MatchString(r.Version) || r.Tools == nil {
		return false
	}
	if _, e := time.Parse(time.RFC3339, r.UpdatedAt); e != nil {
		return false
	}
	seen := map[string]bool{}
	for _, t := range r.Tools {
		if !toolPattern.MatchString(t.Name) || !strings.HasPrefix(t.Name, r.Domain+".") || t.Description == "" || t.InputSchema == nil || seen[t.Name] {
			return false
		}
		seen[t.Name] = true
	}
	return true
}
func (g *Gateway) snapshot(ctx context.Context) (int64, error) {
	resp, e := g.client.Get(ctx, ServicesPrefix, clientv3.WithPrefix())
	if e != nil {
		return 0, e
	}
	next := map[string]ServiceRegistration{}
	for _, kv := range resp.Kvs {
		var r ServiceRegistration
		if json.Unmarshal(kv.Value, &r) == nil && validRegistration(string(kv.Key), r) {
			next[r.Domain] = r
		}
	}
	g.mu.Lock()
	g.services = next
	g.mu.Unlock()
	return resp.Header.Revision, nil
}
func (g *Gateway) StartDiscovery(ctx context.Context) error {
	if g.client == nil {
		return errors.New("registry unavailable")
	}
	initial, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	rev, e := g.snapshot(initial)
	if e != nil {
		return errors.New("registry unavailable")
	}
	go g.watch(ctx, rev+1)
	return nil
}
func (g *Gateway) watch(ctx context.Context, revision int64) {
	for ctx.Err() == nil {
		watchCtx, cancel := context.WithCancel(ctx)
		stream := g.client.Watch(watchCtx, ServicesPrefix, clientv3.WithPrefix(), clientv3.WithRev(revision))
		for response := range stream {
			if response.Err() != nil || response.Canceled {
				break
			}
			for _, event := range response.Events {
				key := string(event.Kv.Key)
				domain := strings.TrimPrefix(key, ServicesPrefix)
				g.mu.Lock()
				if event.Type == clientv3.EventTypeDelete {
					delete(g.services, domain)
				} else {
					var r ServiceRegistration
					if json.Unmarshal(event.Kv.Value, &r) == nil && validRegistration(key, r) {
						g.services[domain] = r
					} else {
						delete(g.services, domain)
					}
				}
				g.mu.Unlock()
			}
			revision = response.Header.Revision + 1
		}
		cancel()
		g.mu.Lock()
		g.services = map[string]ServiceRegistration{}
		g.mu.Unlock()
		for ctx.Err() == nil {
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Second):
			}
			snapshotCtx, stop := context.WithTimeout(ctx, 5*time.Second)
			rev, e := g.snapshot(snapshotCtx)
			stop()
			if e == nil {
				revision = rev + 1
				break
			}
		}
	}
}
func Managed(name string) bool {
	domain := strings.SplitN(name, ".", 2)[0]
	return domain == "project" || domain == "world" || domain == "artifact"
}
func (g *Gateway) ListTools() []ToolDefinition {
	g.mu.RLock()
	defer g.mu.RUnlock()
	out := []ToolDefinition{}
	for _, r := range g.services {
		out = append(out, r.Tools...)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// ServiceEndpoint returns only endpoints from the validated discovery snapshot.
// Instance aliases are resolved by the application, never by caller-supplied URLs.
func (g *Gateway) ServiceEndpoint(domain string) (string, bool) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	r, ok := g.services[domain]
	return r.Endpoint, ok
}
func (g *Gateway) CallTool(ctx context.Context, name string, args map[string]any, headers map[string]string) (any, int, error) {
	if Managed(name) {
		return nil, 403, errors.New("verified identity required")
	}
	return g.call(ctx, name, args, headers)
}
func (g *Gateway) CallVerified(ctx context.Context, name string, args map[string]any, p project.Principal, projectHeader string) (any, int, error) {
	if !p.Valid() {
		return nil, 403, errors.New("verified identity required")
	}
	projectID := projectHeader
	if Managed(name) {
		if strings.HasPrefix(name, "artifact.") {
			projectID = ""
			if name == "artifact.write" {
				if source, ok := args["source"].(map[string]any); ok {
					projectID, _ = source["project_id"].(string)
				}
			}
			if name == "artifact.read" {
				if access, ok := args["access"].(map[string]any); ok {
					projectID, _ = access["project_id"].(string)
				}
			}
			if projectHeader != "" && (projectID == "" || projectID != projectHeader) {
				return nil, 400, errors.New("project scope mismatch")
			}
		}
		key := ""
		if strings.HasPrefix(name, "project.") && name != "project.create" && name != "project.list" {
			key = "project_id"
		}
		if name == "world.register" {
			key = "source_project_id"
		}
		if key != "" {
			value, ok := args[key].(string)
			if !ok || !project.ValidID(value) || (projectHeader != "" && projectHeader != value) {
				return nil, 400, errors.New("project scope mismatch")
			}
			projectID = value
		} else if name != "artifact.read" && name != "artifact.write" && projectHeader != "" {
			return nil, 400, errors.New("unexpected project scope")
		}
	} else if projectID == "" {
		projectID = "prj_studio_default"
	}
	return g.call(ctx, name, args, map[string]string{"X-User-Id": p.SubjectID, "X-Organization-Id": p.OrganizationID, "X-Request-Id": p.RequestID, "X-Project-Id": projectID})
}
func (g *Gateway) call(ctx context.Context, name string, args map[string]any, headers map[string]string) (any, int, error) {
	if !toolPattern.MatchString(name) {
		return nil, 400, errors.New("invalid tool name")
	}
	domain := strings.SplitN(name, ".", 2)[0]
	g.mu.RLock()
	svc, ok := g.services[domain]
	g.mu.RUnlock()
	if !ok {
		return nil, 404, errors.New("tool service unavailable")
	}
	found := false
	for _, tool := range svc.Tools {
		if tool.Name == name {
			found = true
			break
		}
	}
	if !found {
		return nil, 404, errors.New("tool not registered")
	}
	if !endpointValid(svc.Endpoint) {
		return nil, 503, errors.New("invalid registered endpoint")
	}
	token := g.resolveToken(domain)
	if Managed(name) && token == "" {
		return nil, 503, errors.New("internal service credential unavailable")
	}
	payload := map[string]any{"jsonrpc": "2.0", "id": headers["X-Request-Id"], "method": "tools/call", "params": map[string]any{"name": name, "arguments": args}}
	b, e := json.Marshal(payload)
	if e != nil || len(b) > 5<<20 {
		return nil, 400, errors.New("invalid tool arguments")
	}
	req, e := http.NewRequestWithContext(ctx, "POST", svc.Endpoint, bytes.NewReader(b))
	if e != nil {
		return nil, 503, errors.New("service unavailable")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	for _, key := range []string{"X-User-Id", "X-Organization-Id", "X-Project-Id", "X-Request-Id"} {
		if headers[key] != "" {
			req.Header.Set(key, headers[key])
		}
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, e := g.httpClient.Do(req)
	if e != nil {
		return nil, 502, errors.New("downstream unavailable")
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		return nil, 502, errors.New("downstream redirect rejected")
	}
	responseLimit := 12 << 20
	if domain == "artifact" {
		// A 1 MiB UTF-8 text can require six bytes per escaped character,
		// and MCP carries both structuredContent and its text fallback.
		responseLimit = 16 << 20
	}
	b, e = io.ReadAll(io.LimitReader(resp.Body, int64(responseLimit)+1))
	if e != nil || len(b) > responseLimit {
		return nil, 502, errors.New("invalid downstream response")
	}
	var envelope map[string]json.RawMessage
	if json.Unmarshal(b, &envelope) != nil {
		if Managed(name) {
			return nil, 502, errors.New("invalid downstream response")
		}
		return map[string]any{"content": []map[string]string{{"type": "text", "text": string(b)}}}, resp.StatusCode, nil
	}
	if failure, ok := envelope["error"]; ok && string(failure) != "null" {
		return nil, 502, errors.New("downstream protocol error")
	}
	if result, ok := envelope["result"]; ok {
		if Managed(name) {
			var version, id string
			if json.Unmarshal(envelope["jsonrpc"], &version) != nil || version != "2.0" || json.Unmarshal(envelope["id"], &id) != nil || id != headers["X-Request-Id"] {
				return nil, 502, errors.New("downstream response identity mismatch")
			}
			// Preserve opaque extension numbers and nulls exactly through the
			// relay instead of rounding them through float64.
			return result, resp.StatusCode, nil
		}
		var out any
		_ = json.Unmarshal(result, &out)
		return out, resp.StatusCode, nil
	}
	if Managed(name) {
		return nil, 502, errors.New("invalid downstream response")
	}
	var out any
	_ = json.Unmarshal(b, &out)
	return out, resp.StatusCode, nil
}
func (g *Gateway) resolveToken(domain string) string {
	if domain == "project" || domain == "world" {
		return os.Getenv("STUDIO_PROJECT_SERVICE_TOKEN")
	}
	if domain == "artifact" {
		return os.Getenv("STUDIO_ARTIFACT_SERVICE_TOKEN")
	}
	for _, key := range []string{"STUDIO_" + strings.ToUpper(domain) + "_TOKEN", strings.ToUpper(domain) + "_MCP_BEARER_TOKEN", "STUDIO_BEARER_TOKEN", "INTERNAL_SERVICE_TOKEN"} {
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			return v
		}
	}
	return ""
}
func (g *Gateway) Client() *clientv3.Client { return g.client }
func (g *Gateway) Close() error {
	if g.client != nil {
		return g.client.Close()
	}
	return nil
}
