package web

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/google/uuid"
	"github.com/verdantflarehub/verdantflare-studio/internal/application"
	"github.com/verdantflarehub/verdantflare-studio/internal/mcp"
	pb "go.etcd.io/etcd/api/v3/etcdserverpb"
	"go.etcd.io/etcd/api/v3/mvccpb"
	clientv3 "go.etcd.io/etcd/client/v3"
	"google.golang.org/grpc"
)

func bearerStation(t *testing.T, token string) *application.Station {
	t.Helper()
	id := func() string { return uuid.Must(uuid.NewV7()).String() }
	identity := map[string]any{"station_id": id(), "session_id": id(), "user_id": id(), "organization_id": id(), "organization_name": "fixture", "request_id": "fixture", "username": "fixture", "roles": []string{"admin"}, "scopes": []string{"identity:read"}, "issued_at": time.Now().Add(-time.Minute).UTC(), "expires_at": time.Now().Add(time.Hour).UTC(), "revocation_version": 0, "policy_version": 1}
	core := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/identity/me" || r.Method != "GET" {
			t.Error("MCP client tried an extra login")
		}
		if r.Header.Get("Authorization") != "Bearer "+token {
			w.WriteHeader(401)
			_, _ = w.Write([]byte(`{"code":"UNAUTHENTICATED"}`))
			return
		}
		_ = json.NewEncoder(w).Encode(identity)
	}))
	t.Cleanup(core.Close)
	station, err := application.NewStation(core.URL)
	if err != nil {
		t.Fatal(err)
	}
	return station
}

type projectRegistry struct{ videoRegistry }

func (p *projectRegistry) Range(context.Context, *pb.RangeRequest) (*pb.RangeResponse, error) {
	return &pb.RangeResponse{Header: &pb.ResponseHeader{Revision: 1}, Kvs: []*mvccpb.KeyValue{{Key: []byte(mcp.ServicesPrefix + "project"), Value: p.registration}}}, nil
}

func TestProjectDiscoveryAndCallsNeedOnlyUserBearer(t *testing.T) {
	const token = "already-configured-user-bearer"
	t.Setenv("STUDIO_BEARER_TOKEN", token) // Must not hide management tools.
	t.Setenv("STUDIO_PROJECT_SERVICE_TOKEN", strings.Repeat("s", 32))
	pid := uuid.Must(uuid.NewV7()).String()
	calls := []string{}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+strings.Repeat("s", 32) || r.Header.Get("X-User-Id") == "" || r.Header.Get("X-Organization-Id") == "" || r.Header.Get("Cookie") != "" {
			t.Error("invalid trusted identity or external credential forwarded")
		}
		var req struct {
			ID     string `json:"id"`
			Params struct {
				Name string `json:"name"`
			} `json:"params"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		calls = append(calls, req.Params.Name)
		if req.Params.Name == "project.open" {
			if r.Header.Get("X-Project-Id") != pid {
				t.Error("formal project ID missing")
			}
		} else if r.Header.Get("X-Project-Id") != "" {
			t.Error("create/list used a fabricated project")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": map[string]any{"isError": false, "structuredContent": map[string]string{"project_id": pid}}})
	}))
	defer upstream.Close()
	toolNames := []string{"project.create", "project.list", "project.open"}
	definitions := []mcp.ToolDefinition{}
	for _, name := range toolNames {
		definitions = append(definitions, mcp.ToolDefinition{Name: name, Description: "fixture", InputSchema: map[string]any{"type": "object"}})
	}
	registration, _ := json.Marshal(mcp.ServiceRegistration{Domain: "project", Endpoint: upstream.URL + "/mcp", HealthEndpoint: upstream.URL + "/health", Version: "1.0.0", UpdatedAt: time.Now().UTC().Format(time.RFC3339), Tools: definitions})
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	registry := grpc.NewServer()
	data := &projectRegistry{videoRegistry{registration: registration}}
	pb.RegisterKVServer(registry, data)
	pb.RegisterWatchServer(registry, data)
	go registry.Serve(listener)
	defer registry.Stop()
	cli, err := clientv3.New(clientv3.Config{Endpoints: []string{listener.Addr().String()}, DialTimeout: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer cli.Close()
	gateway := mcp.NewGatewayWithClient(cli)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	if err := gateway.StartDiscovery(ctx); err != nil {
		t.Fatal(err)
	}
	router, server := NewServer(bearerStation(t, token), "https://studio.example", fstest.MapFS{})
	server.SetMCPGateway(gateway)
	call := func(body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "/mcp", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		return w
	}
	w := call(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	if w.Code != 200 {
		t.Fatal("tool discovery failed", w.Code)
	}
	catalog := w.Body.String()
	for _, name := range toolNames {
		if !strings.Contains(catalog, name) {
			t.Fatal("project tool hidden", name)
		}
		args := `{}`
		if name == "project.open" {
			args = `{"project_id":"` + pid + `"}`
		}
		w = call(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"` + name + `","arguments":` + args + `}}`)
		if w.Code != 200 || !strings.Contains(w.Body.String(), pid) {
			t.Fatal("bearer project call failed", name, w.Code)
		}
	}
	if len(calls) != 3 {
		t.Fatal("business calls missing or repeated")
	}
}
