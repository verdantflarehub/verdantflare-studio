package desktop

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/verdantflarehub/verdantflare-studio/internal/application"
	"github.com/verdantflarehub/verdantflare-studio/internal/mcp"
	"github.com/verdantflarehub/verdantflare-studio/internal/project"
	"strings"
	"sync"
)

// Service keeps the Station credential in Go memory, never in the WebView.
// Remember-login and OS credential storage are intentionally not implemented yet.
type Service struct {
	mu         sync.Mutex
	station    *application.Station
	token      string
	mcpGateway *mcp.Gateway
}

func New(s *application.Station) *Service { return &Service{station: s} }

// NewWithGateway installs the trusted service gateway before the Wails service
// is registered. The gateway is never exposed as a WebView-callable method.
func NewWithGateway(s *application.Station, gateway *mcp.Gateway) *Service {
	return &Service{station: s, mcpGateway: gateway}
}

func (s *Service) Call(in application.Request) application.Result {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := s.station.Call(context.Background(), s.token, in)
	if in.Path == "login" && out.Status == 201 {
		s.token = out.Token
	}
	if in.Path == "logout" || out.Status == 401 {
		s.token = ""
	}
	out.Token = ""
	return out
}

type MCPRequest struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
	ProjectID string         `json:"project_id,omitempty"`
	RequestID string         `json:"request_id,omitempty"`
}

type MCPResult struct {
	Status    int             `json:"status"`
	Data      json.RawMessage `json:"data"`
	RequestID string          `json:"request_id"`
}

// MCPCall is the desktop equivalent of the web /mcp endpoint. The browser
// receives only a JSON-RPC envelope; the Core access token and internal MCP
// service token remain in the trusted Go process.
func (s *Service) MCPCall(in MCPRequest) MCPResult {
	id := in.RequestID
	if !project.ValidID(id) {
		id = uuid.Must(uuid.NewV7()).String()
	}
	s.mu.Lock()
	token, gateway := s.token, s.mcpGateway
	s.mu.Unlock()
	if token == "" {
		return mcpFailure(401, id, "Authentication required")
	}
	if gateway == nil {
		return mcpFailure(503, id, "Service discovery unavailable")
	}
	principal, status := s.station.Principal(context.Background(), token, id)
	if status != 200 {
		return mcpFailure(status, id, "Session verification failed")
	}
	if in.Name == "" || in.Arguments == nil {
		in.Arguments = map[string]any{}
	}
	result, status, err := gateway.CallVerified(context.Background(), in.Name, in.Arguments, principal, in.ProjectID)
	if err != nil {
		if status < 400 {
			status = 503
		}
		return mcpFailure(status, id, err.Error())
	}
	data, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "result": result})
	if err != nil {
		return mcpFailure(503, id, "Service response unavailable")
	}
	return MCPResult{Status: status, Data: data, RequestID: id}
}

func mcpFailure(status int, id, message string) MCPResult {
	if status < 400 {
		status = 503
	}
	code := -32603
	switch status {
	case 400:
		code = -32602
	case 401, 403:
		code = -32000
	case 404:
		code = -32601
	}
	if strings.TrimSpace(message) == "" {
		message = "MCP request failed"
	}
	data, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "error": map[string]any{"code": code, "message": message}})
	return MCPResult{Status: status, Data: data, RequestID: id}
}
