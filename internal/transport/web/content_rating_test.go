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

type domainRegistry struct {
	videoRegistry
	domain string
}

func (d *domainRegistry) Range(context.Context, *pb.RangeRequest) (*pb.RangeResponse, error) {
	return &pb.RangeResponse{
		Header: &pb.ResponseHeader{Revision: 1},
		Kvs: []*mvccpb.KeyValue{{
			Key:   []byte(mcp.ServicesPrefix + d.domain),
			Value: d.registration,
		}},
	}, nil
}

func testStationWithRoles(t *testing.T, token string, roles []string) (*application.Station, string, string) {
	t.Helper()
	id := func() string { return uuid.Must(uuid.NewV7()).String() }
	userID := id()
	orgID := id()
	identity := map[string]any{
		"station_id":         id(),
		"session_id":         id(),
		"user_id":            userID,
		"organization_id":    orgID,
		"organization_name":  "fixture",
		"request_id":         "fixture",
		"username":           "fixture",
		"roles":              roles,
		"scopes":             []string{"identity:read"},
		"issued_at":          time.Now().Add(-time.Minute).UTC(),
		"expires_at":         time.Now().Add(time.Hour).UTC(),
		"revocation_version": 0,
		"policy_version":     1,
	}
	core := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	return station, userID, orgID
}

func TestMCPStripsNSFWHeaderAndForwardsContentRatingActions(t *testing.T) {
	const userToken = "user-session-token"
	const mcpBearer = "mcp-agent-bearer-token"
	t.Setenv("STUDIO_IMAGE_TOKEN", "image-internal-secret")

	station, userID, orgID := testStationWithRoles(t, userToken, []string{"admin"})

	var mcpSawNSFWHeader bool
	var clearSawAuth, clearSawUser, clearSawOrg, clearSawPath string
	var markSawPath string
	var uiSawNSFWHeader string

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/mcp":
			if r.Header.Get("X-VF-Include-NSFW") != "" {
				mcpSawNSFWHeader = true
			}
			var req struct {
				ID string `json:"id"`
			}
			_ = json.NewDecoder(r.Body).Decode(&req)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      req.ID,
				"result":  map[string]any{"isError": false, "structuredContent": map[string]any{"ok": true}},
			})
		case strings.HasPrefix(r.URL.Path, "/internal/content-ratings/"):
			if strings.HasSuffix(r.URL.Path, "/clear") {
				clearSawPath = r.URL.Path
				clearSawAuth = r.Header.Get("Authorization")
				clearSawUser = r.Header.Get("X-User-Id")
				clearSawOrg = r.Header.Get("X-Organization-Id")
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{
					"version_id":    "ver_01960000-0000-7000-8000-000000000001",
					"rating":        "general",
					"source":        "human",
					"human_cleared": true,
				})
				return
			}
			if strings.HasSuffix(r.URL.Path, "/mark") {
				markSawPath = r.URL.Path
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{
					"version_id":    "ver_01960000-0000-7000-8000-000000000001",
					"rating":        "nsfw",
					"source":        "human",
					"human_cleared": false,
				})
				return
			}
			w.WriteHeader(404)
		case r.URL.Path == "/api/tasks":
			uiSawNSFWHeader = r.Header.Get("X-VF-Include-NSFW")
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"tasks":[]}`))
		default:
			w.WriteHeader(404)
		}
	}))
	defer upstream.Close()

	definitions := []mcp.ToolDefinition{
		{Name: "image.list", Description: "list", InputSchema: map[string]any{"type": "object"}},
		{Name: "image.rate_content", Description: "rate", InputSchema: map[string]any{"type": "object"}},
	}
	registration, _ := json.Marshal(mcp.ServiceRegistration{
		Domain:         "image",
		Endpoint:       upstream.URL + "/mcp",
		HealthEndpoint: upstream.URL + "/health",
		Version:        "1.0.0",
		UpdatedAt:      time.Now().UTC().Format(time.RFC3339),
		Tools:          definitions,
	})
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	registry := grpc.NewServer()
	data := &domainRegistry{videoRegistry: videoRegistry{registration: registration}, domain: "image"}
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

	router, server := NewServer(station, "https://studio.example", fstest.MapFS{})
	server.SetMCPGateway(gateway)
	server.EnableImage(router, ImageConfig{URL: upstream.URL, Token: "image-internal-secret"})
	server.saveSession(context.Background(), "sid-admin", userToken)

	// 1. MCP route must strip X-VF-Include-NSFW even when sent by client
	mcpReq := httptest.NewRequest("POST", "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":"req-1","method":"tools/call","params":{"name":"image.list","arguments":{}}}`))
	mcpReq.Header.Set("Content-Type", "application/json")
	mcpReq.Header.Set("Authorization", "Bearer "+userToken)
	mcpReq.Header.Set("X-VF-Include-NSFW", "1")
	mcpRec := httptest.NewRecorder()
	router.ServeHTTP(mcpRec, mcpReq)
	if mcpRec.Code != 200 {
		t.Fatalf("expected 200 on /mcp, got %d: %s", mcpRec.Code, mcpRec.Body.String())
	}
	if mcpSawNSFWHeader {
		t.Fatal("MCP route forwarded X-VF-Include-NSFW to downstream service")
	}

	// 2. UI proxy route forwards X-VF-Include-NSFW: 1 when authenticated via session cookie
	uiReq := httptest.NewRequest("GET", "/studio/apps/image/api/tasks", nil)
	uiReq.Header.Set("Origin", "https://studio.example")
	uiReq.Header.Set("X-VF-Include-NSFW", "1")
	uiReq.AddCookie(&http.Cookie{Name: cookieName, Value: "sid-admin"})
	uiRec := httptest.NewRecorder()
	router.ServeHTTP(uiRec, uiReq)
	if uiRec.Code != 200 || uiSawNSFWHeader != "1" {
		t.Fatalf("expected UI proxy to forward X-VF-Include-NSFW=1, status=%d header=%q", uiRec.Code, uiSawNSFWHeader)
	}

	// 3. Human clear route succeeds for authenticated writer with session cookie
	clearReq := httptest.NewRequest("POST", "/api/v1/content-ratings/image/ver_01960000-0000-7000-8000-000000000001/clear", strings.NewReader(`{"reason":"false positive"}`))
	clearReq.Header.Set("Origin", "https://studio.example")
	clearReq.Header.Set("Content-Type", "application/json")
	clearReq.AddCookie(&http.Cookie{Name: cookieName, Value: "sid-admin"})
	clearRec := httptest.NewRecorder()
	router.ServeHTTP(clearRec, clearReq)
	if clearRec.Code != 200 {
		t.Fatalf("expected 200 on human clear, got %d: %s", clearRec.Code, clearRec.Body.String())
	}
	if clearSawPath != "/internal/content-ratings/ver_01960000-0000-7000-8000-000000000001/clear" ||
		clearSawAuth != "Bearer image-internal-secret" ||
		clearSawUser != userID ||
		clearSawOrg != orgID {
		t.Fatalf("unexpected clear forwarding: path=%q auth=%q user=%q org=%q", clearSawPath, clearSawAuth, clearSawUser, clearSawOrg)
	}

	// 4. Human mark route succeeds via /studio/api/v1/content-ratings/...
	markReq := httptest.NewRequest("POST", "/studio/api/v1/content-ratings/image/ver_01960000-0000-7000-8000-000000000001/mark", strings.NewReader(`{"reason":"manual 18+ mark"}`))
	markReq.Header.Set("Origin", "https://studio.example")
	markReq.Header.Set("Content-Type", "application/json")
	markReq.AddCookie(&http.Cookie{Name: cookieName, Value: "sid-admin"})
	markRec := httptest.NewRecorder()
	router.ServeHTTP(markRec, markReq)
	if markRec.Code != 200 || markSawPath != "/internal/content-ratings/ver_01960000-0000-7000-8000-000000000001/mark" {
		t.Fatalf("expected 200 on human mark, got %d path=%q", markRec.Code, markSawPath)
	}

	// 5. MCP Bearer token must be rejected on human clear/mark routes
	bearerClearReq := httptest.NewRequest("POST", "/api/v1/content-ratings/image/ver_01960000-0000-7000-8000-000000000001/clear", strings.NewReader(`{}`))
	bearerClearReq.Header.Set("Origin", "https://studio.example")
	bearerClearReq.Header.Set("Content-Type", "application/json")
	bearerClearReq.Header.Set("Authorization", "Bearer "+mcpBearer)
	bearerClearRec := httptest.NewRecorder()
	router.ServeHTTP(bearerClearRec, bearerClearReq)
	if bearerClearRec.Code != 403 {
		t.Fatalf("expected 403 when using Bearer token on human clear route, got %d", bearerClearRec.Code)
	}

	// 6. Read-only viewer role must be rejected with 403 on human clear/mark routes
	viewerStation, _, _ := testStationWithRoles(t, "viewer-token", []string{"viewer"})
	viewerRouter, viewerServer := NewServer(viewerStation, "https://studio.example", fstest.MapFS{})
	viewerServer.SetMCPGateway(gateway)
	viewerServer.saveSession(context.Background(), "sid-viewer", "viewer-token")
	viewerReq := httptest.NewRequest("POST", "/api/v1/content-ratings/image/ver_01960000-0000-7000-8000-000000000001/clear", strings.NewReader(`{}`))
	viewerReq.Header.Set("Origin", "https://studio.example")
	viewerReq.Header.Set("Content-Type", "application/json")
	viewerReq.AddCookie(&http.Cookie{Name: cookieName, Value: "sid-viewer"})
	viewerRec := httptest.NewRecorder()
	viewerRouter.ServeHTTP(viewerRec, viewerReq)
	if viewerRec.Code != 403 {
		t.Fatalf("expected 403 for read-only viewer on human clear route, got %d", viewerRec.Code)
	}
}
