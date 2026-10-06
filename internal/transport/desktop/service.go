package desktop

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/verdantflarehub/verdantflare-studio/internal/application"
	"github.com/verdantflarehub/verdantflare-studio/internal/mcp"
	"github.com/verdantflarehub/verdantflare-studio/internal/project"
	"github.com/verdantflarehub/verdantflare-studio/internal/workspace"
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
	workspaces map[string]*workspace.Workspace
}

func New(s *application.Station) *Service { return NewWithGateway(s, nil) }

// NewWithGateway installs the trusted service gateway before the Wails service
// is registered. The gateway is never exposed as a WebView-callable method.
func NewWithGateway(s *application.Station, gateway *mcp.Gateway) *Service {
	return &Service{station: s, mcpGateway: gateway, workspaces: map[string]*workspace.Workspace{}}
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

type WorkspaceOpenRequest struct {
	Directory string `json:"directory"`
	Alias     string `json:"connection_alias,omitempty"`
	ProjectID string `json:"project_id"`
}

type WorkspaceRequest struct {
	WorkspaceID string                  `json:"workspace_id"`
	FileID      string                  `json:"file_id,omitempty"`
	MaxBytes    int64                   `json:"max_bytes,omitempty"`
	FileIDs     []string                `json:"file_ids,omitempty"`
	Files       []workspace.FileInput   `json:"files,omitempty"`
	Imports     []workspace.ImportInput `json:"imports,omitempty"`
}

type WorkspaceResult struct {
	Status    int             `json:"status"`
	Data      json.RawMessage `json:"data"`
	RequestID string          `json:"request_id"`
}

// WorkspaceOpen creates a pinned local copy under a user-selected directory.
// The directory is never synchronized in the background; every operation is
// explicit and uses the authenticated Gateway Remote below.
func (s *Service) WorkspaceOpen(in WorkspaceOpenRequest) WorkspaceResult {
	if in.Alias == "" {
		in.Alias = "station"
	}
	if strings.TrimSpace(in.Directory) == "" || !project.ValidID(in.ProjectID) {
		return workspaceFailure(project.ErrInvalid)
	}
	copy, err := workspace.Open(context.Background(), in.Directory, in.Alias, in.ProjectID, &gatewayRemote{owner: s})
	if err != nil {
		return workspaceFailure(err)
	}
	id := uuid.Must(uuid.NewV7()).String()
	s.mu.Lock()
	if len(s.workspaces) >= 128 {
		s.mu.Unlock()
		_ = copy.Close()
		return workspaceFailure(workspace.ErrBusy)
	}
	s.workspaces[id] = copy
	s.mu.Unlock()
	state, manifest, err := copy.Snapshot(context.Background())
	if err != nil {
		s.WorkspaceClose(WorkspaceRequest{WorkspaceID: id})
		return workspaceFailure(err)
	}
	return workspaceSuccess(map[string]any{"workspace_id": id, "state": state, "manifest": manifest})
}

func (s *Service) WorkspaceFetch(in WorkspaceRequest) WorkspaceResult {
	copy, err := s.workspace(in.WorkspaceID)
	if err != nil {
		return workspaceFailure(err)
	}
	max := in.MaxBytes
	if max == 0 {
		max = 1 << 50
	}
	if err = copy.Fetch(context.Background(), in.FileID, max); err != nil {
		return workspaceFailure(err)
	}
	return workspaceSuccess(map[string]any{"workspace_id": in.WorkspaceID, "file_id": in.FileID})
}

func (s *Service) WorkspaceStatus(in WorkspaceRequest) WorkspaceResult {
	copy, err := s.workspace(in.WorkspaceID)
	if err != nil {
		return workspaceFailure(err)
	}
	status, err := copy.Status(context.Background())
	if err != nil {
		return workspaceFailure(err)
	}
	return workspaceSuccess(map[string]any{"workspace_id": in.WorkspaceID, "files": status})
}

func (s *Service) WorkspaceSaveTexts(in WorkspaceRequest) WorkspaceResult {
	copy, err := s.workspace(in.WorkspaceID)
	if err != nil {
		return workspaceFailure(err)
	}
	result, err := copy.SaveTexts(context.Background(), in.FileIDs)
	if err != nil {
		return workspaceFailure(err)
	}
	return workspaceSuccess(map[string]any{"workspace_id": in.WorkspaceID, "result": result})
}

// WorkspaceSaveFiles explicitly uploads selected local files and commits their
// immutable content references into the pinned Project revision.
func (s *Service) WorkspaceSaveFiles(in WorkspaceRequest) WorkspaceResult {
	copy, err := s.workspace(in.WorkspaceID)
	if err != nil {
		return workspaceFailure(err)
	}
	result, err := copy.SaveFiles(context.Background(), in.Files)
	if err != nil {
		return workspaceFailure(err)
	}
	return workspaceSuccess(map[string]any{"workspace_id": in.WorkspaceID, "result": result})
}

// WorkspaceImportFiles copies explicitly selected external files into the
// workspace and commits them as new Project files.
func (s *Service) WorkspaceImportFiles(in WorkspaceRequest) WorkspaceResult {
	copy, err := s.workspace(in.WorkspaceID)
	if err != nil {
		return workspaceFailure(err)
	}
	result, err := copy.ImportFiles(context.Background(), in.Imports)
	if err != nil {
		return workspaceFailure(err)
	}
	return workspaceSuccess(map[string]any{"workspace_id": in.WorkspaceID, "result": result})
}

func (s *Service) WorkspaceResume(in WorkspaceRequest) WorkspaceResult {
	copy, err := s.workspace(in.WorkspaceID)
	if err != nil {
		return workspaceFailure(err)
	}
	result, err := copy.Resume(context.Background())
	if err != nil {
		return workspaceFailure(err)
	}
	return workspaceSuccess(map[string]any{"workspace_id": in.WorkspaceID, "result": result})
}

// WorkspaceSwitchToHead is an explicit conflict recovery action. The
// workspace refuses local modifications and unfinished commits, then repins
// the local metadata to the current Project head without replacing user files.
func (s *Service) WorkspaceSwitchToHead(in WorkspaceRequest) WorkspaceResult {
	copy, err := s.workspace(in.WorkspaceID)
	if err != nil {
		return workspaceFailure(err)
	}
	state, manifest, err := copy.SwitchToHead(context.Background())
	if err != nil {
		return workspaceFailure(err)
	}
	return workspaceSuccess(map[string]any{"workspace_id": in.WorkspaceID, "state": state, "manifest": manifest})
}

func (s *Service) WorkspaceClose(in WorkspaceRequest) WorkspaceResult {
	s.mu.Lock()
	copy, ok := s.workspaces[in.WorkspaceID]
	if ok {
		delete(s.workspaces, in.WorkspaceID)
	}
	s.mu.Unlock()
	if !ok {
		return workspaceFailure(project.ErrNotFound)
	}
	if err := copy.Close(); err != nil {
		return workspaceFailure(err)
	}
	return workspaceSuccess(map[string]any{"workspace_id": in.WorkspaceID})
}

func (s *Service) workspace(id string) (*workspace.Workspace, error) {
	if !project.ValidID(id) {
		return nil, project.ErrInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	copy, ok := s.workspaces[id]
	if !ok {
		return nil, project.ErrNotFound
	}
	return copy, nil
}

func workspaceSuccess(value any) WorkspaceResult {
	data, err := json.Marshal(value)
	if err != nil {
		return workspaceFailure(project.ErrDependency)
	}
	return WorkspaceResult{Status: 200, Data: data, RequestID: uuid.Must(uuid.NewV7()).String()}
}

func workspaceFailure(err error) WorkspaceResult {
	status := 500
	switch {
	case errors.Is(err, project.ErrInvalid), errors.Is(err, workspace.ErrInvalid):
		status = 400
	case errors.Is(err, project.ErrForbidden):
		status = 403
	case errors.Is(err, project.ErrNotFound):
		status = 404
	case errors.Is(err, project.ErrConflict), errors.Is(err, workspace.ErrConflict), errors.Is(err, workspace.ErrBusy), errors.Is(err, workspace.ErrPending):
		status = 409
	case errors.Is(err, project.ErrDependency), errors.Is(err, workspace.ErrCorrupt):
		status = 503
	}
	code := "WORKSPACE_ERROR"
	if err != nil && err.Error() != "" {
		code = err.Error()
	}
	data, _ := json.Marshal(map[string]any{"code": code})
	return WorkspaceResult{Status: status, Data: data, RequestID: uuid.Must(uuid.NewV7()).String()}
}
