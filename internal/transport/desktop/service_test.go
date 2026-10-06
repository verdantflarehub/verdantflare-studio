package desktop

import (
	"encoding/json"
	"github.com/google/uuid"
	"github.com/verdantflarehub/verdantflare-studio/internal/application"
	"github.com/verdantflarehub/verdantflare-studio/internal/mcp"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestDesktopCredentialIsolation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/identity/login" {
			w.WriteHeader(201)
			io.WriteString(w, `{"access_token":"private-session"}`)
			return
		}
		if r.Header.Get("Authorization") != "Bearer private-session" {
			t.Error("credential missing")
		}
		if r.URL.Path == "/identity/logout" {
			w.WriteHeader(204)
			return
		}
		io.WriteString(w, `{"items":[]}`)
	}))
	defer server.Close()
	station, _ := application.NewStation(server.URL)
	service := New(station)
	result := service.Call(application.Request{Method: "POST", Path: "login", Body: json.RawMessage(`{}`)})
	encoded, _ := json.Marshal(result)
	if result.Status != 201 || strings.Contains(string(encoded), "private-session") || result.Token != "" {
		t.Fatal("login leaked credential")
	}
	if service.Call(application.Request{Method: "GET", Path: "apps"}).Status != 200 {
		t.Fatal("session unavailable")
	}
	service.Call(application.Request{Method: "POST", Path: "logout"})
	if service.Call(application.Request{Method: "GET", Path: "apps"}).Status != 401 {
		t.Fatal("session not cleared")
	}
}

func TestDesktopMCPRevalidatesSessionAndReturnsJSONRPC(t *testing.T) {
	userID := uuid.Must(uuid.NewV7()).String()
	orgID := uuid.Must(uuid.NewV7()).String()
	stationServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/identity/login":
			w.WriteHeader(201)
			_, _ = io.WriteString(w, `{"access_token":"private-session"}`)
		case "/identity/me":
			if r.Header.Get("Authorization") != "Bearer private-session" {
				t.Errorf("session token was not revalidated")
			}
			identity := map[string]any{
				"station_id":         uuid.Must(uuid.NewV7()).String(),
				"session_id":         uuid.Must(uuid.NewV7()).String(),
				"user_id":            userID,
				"organization_id":    orgID,
				"organization_name":  "Test Organization",
				"request_id":         uuid.Must(uuid.NewV7()).String(),
				"username":           "desktop-user",
				"roles":              []string{"owner"},
				"scopes":             []string{"identity:read"},
				"issued_at":          time.Now().Add(-time.Minute).UTC(),
				"expires_at":         time.Now().Add(time.Hour).UTC(),
				"revocation_version": 0,
				"policy_version":     1,
			}
			_ = json.NewEncoder(w).Encode(identity)
		default:
			w.WriteHeader(404)
		}
	}))
	defer stationServer.Close()
	station, err := application.NewStation(stationServer.URL)
	if err != nil {
		t.Fatal(err)
	}
	service := NewWithGateway(station, mcp.NewGatewayWithClient(nil))
	login := service.Call(application.Request{Method: "POST", Path: "login", Body: json.RawMessage(`{}`)})
	if login.Status != 201 {
		t.Fatalf("login failed: %d", login.Status)
	}
	out := service.MCPCall(MCPRequest{Name: "project.list", Arguments: map[string]any{"limit": 1}})
	if out.Status != 404 || out.RequestID == "" {
		t.Fatalf("unexpected MCP result: %#v", out)
	}
	var envelope map[string]any
	if err := json.Unmarshal(out.Data, &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope["jsonrpc"] != "2.0" || envelope["error"] == nil || strings.Contains(string(out.Data), "private-session") {
		t.Fatalf("invalid or credential-leaking MCP envelope: %s", out.Data)
	}
}

func TestDesktopWorkspaceHandleBoundary(t *testing.T) {
	service := NewWithGateway(nil, nil)
	if result := service.WorkspaceStatus(WorkspaceRequest{WorkspaceID: "bad"}); result.Status != 400 {
		t.Fatalf("invalid workspace handle returned %d", result.Status)
	}
	if result := service.WorkspaceClose(WorkspaceRequest{WorkspaceID: uuid.Must(uuid.NewV7()).String()}); result.Status != 404 {
		t.Fatalf("unknown workspace handle returned %d", result.Status)
	}
	result := service.WorkspaceOpen(WorkspaceOpenRequest{
		Directory: t.TempDir(),
		ProjectID: uuid.Must(uuid.NewV7()).String(),
	})
	if result.Status != 403 {
		t.Fatalf("unauthenticated workspace open returned %d", result.Status)
	}
}
