package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
)

const (
	ServicesPrefix = "/verdantflare/mcp/services/"
	DefaultTimeout = 30 * time.Second
)

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

	mu       sync.RWMutex
	services map[string]ServiceRegistration
}

func NewGateway(etcdEndpoints []string) (*Gateway, error) {
	cli, err := clientv3.New(clientv3.Config{
		Endpoints:   etcdEndpoints,
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to etcd: %w", err)
	}

	gw := &Gateway{
		client: cli,
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
		},
		services: make(map[string]ServiceRegistration),
	}

	return gw, nil
}

// NewGatewayWithClient creates a Gateway using an existing etcd client (useful for testing or custom configs)
func NewGatewayWithClient(cli *clientv3.Client) *Gateway {
	return &Gateway{
		client: cli,
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
		},
		services: make(map[string]ServiceRegistration),
	}
}

// StartDiscovery syncs existing registered services and starts background watch loop
func (g *Gateway) StartDiscovery(ctx context.Context) error {
	if g.client == nil {
		return fmt.Errorf("etcd client is not initialized")
	}

	// 1. Initial full fetch
	resp, err := g.client.Get(ctx, ServicesPrefix, clientv3.WithPrefix())
	if err != nil {
		return fmt.Errorf("failed to fetch initial services from etcd: %w", err)
	}

	g.mu.Lock()
	for _, kv := range resp.Kvs {
		var reg ServiceRegistration
		if err := json.Unmarshal(kv.Value, &reg); err == nil && reg.Domain != "" {
			g.services[reg.Domain] = reg
			log.Printf("[MCP Gateway] Discovered service [%s] (%s) with %d tools", reg.Domain, reg.Endpoint, len(reg.Tools))
		}
	}
	g.mu.Unlock()

	// 2. Start background watch
	go g.watchLoop(ctx)

	return nil
}

func (g *Gateway) watchLoop(ctx context.Context) {
	rch := g.client.Watch(ctx, ServicesPrefix, clientv3.WithPrefix())
	for {
		select {
		case <-ctx.Done():
			log.Printf("[MCP Gateway] Discovery watcher stopped")
			return
		case wresp, ok := <-rch:
			if !ok {
				log.Printf("[MCP Gateway] Watch channel closed, restarting watch in 2s...")
				time.Sleep(2 * time.Second)
				rch = g.client.Watch(ctx, ServicesPrefix, clientv3.WithPrefix())
				continue
			}
			for _, ev := range wresp.Events {
				key := string(ev.Kv.Key)
				domain := strings.TrimPrefix(key, ServicesPrefix)
				if domain == "" {
					continue
				}

				switch ev.Type {
				case clientv3.EventTypePut:
					var reg ServiceRegistration
					if err := json.Unmarshal(ev.Kv.Value, &reg); err != nil {
						log.Printf("[MCP Gateway] Failed to decode registration for key %s: %v", key, err)
						continue
					}
					g.mu.Lock()
					g.services[reg.Domain] = reg
					g.mu.Unlock()
					log.Printf("[MCP Gateway] Mounted/Updated service [%s] (%s) with %d tools", reg.Domain, reg.Endpoint, len(reg.Tools))

				case clientv3.EventTypeDelete:
					g.mu.Lock()
					delete(g.services, domain)
					g.mu.Unlock()
					log.Printf("[MCP Gateway] Drained/Removed service [%s]", domain)
				}
			}
		}
	}
}

// ListTools returns aggregated tool definitions from all registered services
func (g *Gateway) ListTools() []ToolDefinition {
	g.mu.RLock()
	defer g.mu.RUnlock()

	var allTools []ToolDefinition
	for _, svc := range g.services {
		allTools = append(allTools, svc.Tools...)
	}
	return allTools
}

// CallTool routes a tool call to the responsible downstream microservice
func (g *Gateway) CallTool(ctx context.Context, toolName string, arguments map[string]any, headers map[string]string) (any, int, error) {
	parts := strings.SplitN(toolName, ".", 2)
	if len(parts) < 2 {
		return nil, http.StatusBadRequest, fmt.Errorf("invalid tool name '%s': must follow <domain>.<action> format", toolName)
	}
	domain := parts[0]

	g.mu.RLock()
	svc, exists := g.services[domain]
	g.mu.RUnlock()

	if !exists {
		return nil, http.StatusNotFound, fmt.Errorf("no microservice registered for domain '%s'", domain)
	}

	payload := map[string]any{
		"jsonrpc": "2.0",
		"id":      headers["X-Request-Id"],
		"method":  "tools/call",
		"params": map[string]any{
			"name":      toolName,
			"arguments": arguments,
		},
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, http.StatusInternalServerError, fmt.Errorf("failed to encode downstream request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, svc.Endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, http.StatusInternalServerError, fmt.Errorf("failed to create downstream request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		if v != "" {
			req.Header.Set(k, v)
		}
	}

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, http.StatusBadGateway, fmt.Errorf("downstream microservice [%s] unreachable: %w", domain, err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, http.StatusBadGateway, fmt.Errorf("failed to read downstream response: %w", err)
	}

	var jsonResp map[string]any
	if err := json.Unmarshal(respBytes, &jsonResp); err != nil {
		// If response is not standard JSON-RPC, return raw body as text content
		return map[string]any{
			"content": []map[string]any{
				{"type": "text", "text": string(respBytes)},
			},
		}, resp.StatusCode, nil
	}

	if errObj, hasErr := jsonResp["error"]; hasErr && errObj != nil {
		return jsonResp, resp.StatusCode, nil
	}

	if result, hasResult := jsonResp["result"]; hasResult {
		return result, resp.StatusCode, nil
	}

	return jsonResp, resp.StatusCode, nil
}

func (g *Gateway) Close() error {
	if g.client != nil {
		return g.client.Close()
	}
	return nil
}
