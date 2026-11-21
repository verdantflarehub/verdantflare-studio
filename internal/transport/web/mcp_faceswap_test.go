package web

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/verdantflarehub/verdantflare-studio/internal/mcp"
	pb "go.etcd.io/etcd/api/v3/etcdserverpb"
	clientv3 "go.etcd.io/etcd/client/v3"
	"google.golang.org/grpc"
)

func TestMCPFaceSwapResponseOutlivesOrdinaryWriteDeadline(t *testing.T) {
	t.Setenv("STUDIO_IMAGE_TOKEN", "image-fixture")
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			return
		}
		time.Sleep(80 * time.Millisecond)
		json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request["id"], "result": map[string]any{"structuredContent": map[string]string{"status": "completed"}}})
	}))
	defer upstream.Close()
	registration, _ := json.Marshal(mcp.ServiceRegistration{Domain: "image", Endpoint: upstream.URL, HealthEndpoint: upstream.URL + "/health", Version: "1.0.0", UpdatedAt: time.Now().UTC().Format(time.RFC3339), Tools: []mcp.ToolDefinition{{Name: "image.faceswap", Description: "Fusion", InputSchema: map[string]any{"type": "object"}}}})
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	registry := grpc.NewServer()
	data := &videoRegistry{registration: registration, domain: "image"}
	pb.RegisterKVServer(registry, data)
	pb.RegisterWatchServer(registry, data)
	go registry.Serve(listener)
	defer registry.Stop()
	cli, err := clientv3.New(clientv3.Config{Endpoints: []string{listener.Addr().String()}, DialTimeout: time.Second})
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
	router, server := NewServer(bearerStation(t, "studio-fixture"), "https://studio.example", fstest.MapFS{})
	server.SetMCPGateway(gateway)
	host := httptest.NewUnstartedServer(router)
	host.Config.WriteTimeout = 20 * time.Millisecond
	host.Start()
	defer host.Close()
	request, _ := http.NewRequest("POST", host.URL+"/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"image.faceswap","arguments":{}}}`))
	request.Header.Set("Authorization", "Bearer studio-fixture")
	request.Header.Set("Content-Type", "application/json")
	response, err := host.Client().Do(request)
	if err != nil {
		t.Fatal("fusion response lost at host write deadline", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil || response.StatusCode != 200 || !strings.Contains(string(body), `"completed"`) {
		t.Fatalf("fusion response: %d %s %v", response.StatusCode, body, err)
	}
}
