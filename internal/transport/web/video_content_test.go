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
	"go.etcd.io/etcd/api/v3/mvccpb"
	clientv3 "go.etcd.io/etcd/client/v3"
	"google.golang.org/grpc"
)

type videoRegistry struct {
	pb.UnimplementedKVServer
	pb.UnimplementedWatchServer
	registration []byte
}

func (v *videoRegistry) Watch(stream pb.Watch_WatchServer) error {
	<-stream.Context().Done()
	return stream.Context().Err()
}

func (v *videoRegistry) Range(context.Context, *pb.RangeRequest) (*pb.RangeResponse, error) {
	return &pb.RangeResponse{Header: &pb.ResponseHeader{Revision: 1}, Kvs: []*mvccpb.KeyValue{{Key: []byte(mcp.ServicesPrefix + "video"), Value: v.registration}}}, nil
}

func TestVideoResultDownloadThroughStudio(t *testing.T) {
	t.Setenv("STUDIO_BEARER_TOKEN", "studio-secret")
	t.Setenv("STUDIO_VIDEO_TOKEN", "video-secret")
	path := "/artifacts/art_" + strings.Repeat("a", 32) + "/content"
	upstreamCalls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls++
		if r.Header.Get("Authorization") != "Bearer video-secret" || r.Header.Get("Cookie") != "" {
			t.Error("external credentials forwarded")
		}
		if r.URL.Path == "/mcp" {
			var req map[string]any
			_ = json.NewDecoder(r.Body).Decode(&req)
			_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": req["id"], "result": map[string]any{"structuredContent": map[string]string{"download_path": path}}})
			return
		}
		if r.URL.Path != path {
			t.Error("unexpected downstream path", r.URL.Path)
		}
		w.Header().Set("Content-Type", "video/mp4")
		http.ServeContent(w, r, "clip.mp4", time.Time{}, strings.NewReader("video-bytes"))
	}))
	defer upstream.Close()
	registration, _ := json.Marshal(mcp.ServiceRegistration{Domain: "video", Endpoint: upstream.URL + "/mcp", HealthEndpoint: upstream.URL + "/health", Version: "1.0.0", UpdatedAt: time.Now().UTC().Format(time.RFC3339), Tools: []mcp.ToolDefinition{{Name: "video.result", Description: "Result", InputSchema: map[string]any{"type": "object"}}}})
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	registry := grpc.NewServer()
	registryData := &videoRegistry{registration: registration}
	pb.RegisterKVServer(registry, registryData)
	pb.RegisterWatchServer(registry, registryData)
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
	station := bearerStation(t, "studio-secret")
	router, server := NewServer(station, "https://studio.example", fstest.MapFS{})
	server.SetMCPGateway(gateway)
	host := httptest.NewServer(router)
	defer host.Close()
	call := func(method, target, auth, body, origin, site, byteRange string) (*http.Response, []byte) {
		t.Helper()
		req, _ := http.NewRequest(method, host.URL+target, strings.NewReader(body))
		if auth != "" {
			req.Header.Set("Authorization", auth)
		}
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		if site != "" {
			req.Header.Set("Sec-Fetch-Site", site)
		}
		if byteRange != "" {
			req.Header.Set("Range", byteRange)
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := host.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		b, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		return resp, b
	}
	resp, body := call("POST", "/mcp", "Bearer studio-secret", `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"video.result","arguments":{"task_id":"existing-task"}}}`, "", "", "")
	var result struct {
		Result struct {
			StructuredContent struct {
				DownloadPath string `json:"download_path"`
			} `json:"structuredContent"`
		} `json:"result"`
	}
	if resp.StatusCode != 200 || json.Unmarshal(body, &result) != nil || result.Result.StructuredContent.DownloadPath != path {
		t.Fatal("result lookup failed", string(body))
	}
	for _, tc := range []struct {
		rangeHeader, want string
		code              int
	}{{"", "video-bytes", 200}, {"bytes=0-4", "video", 206}, {"bytes=500-", "", 416}} {
		resp, body = call("GET", result.Result.StructuredContent.DownloadPath, "Bearer studio-secret", "", "", "", tc.rangeHeader)
		if resp.StatusCode != tc.code || string(body) != tc.want {
			t.Fatalf("download: %d %q", resp.StatusCode, body)
		}
		if tc.code != 416 && (resp.Header.Get("Content-Type") != "video/mp4" || resp.Header.Get("Content-Disposition") != "attachment" || resp.Header.Get("Cache-Control") != "no-store") {
			t.Fatal("download headers lost")
		}
	}
	before := upstreamCalls
	for _, tc := range []struct {
		path, auth, body, origin, site string
		code                           int
	}{
		{path, "", "", "https://studio.example", "", 401},
		{path, "Bearer wrong", "", "", "", 401},
		{path, "Bearer studio-secret", "", "https://other.example", "", 403},
		{path, "Bearer studio-secret", "", "", "cross-site", 403},
		{path, "Bearer studio-secret", "", "", "same-site", 403},
		{path + "?url=http://other.example", "Bearer studio-secret", "", "", "", 400},
		{strings.Replace(path, "art_", "art_%61", 1), "Bearer studio-secret", "", "", "", 400},
		{path, "Bearer studio-secret", "{}", "", "", 400},
		{"/artifacts/0199c0a0-0000-7000-8000-000000000021/content", "Bearer studio-secret", "", "", "", 400},
		{"/v2/artifacts/0199c0a0-0000-7000-8000-000000000021/content", "Bearer studio-secret", "", "", "", 400},
		{"/runtime-artifacts/art_" + strings.Repeat("a", 32) + "/content", "Bearer studio-secret", "", "", "", 404},
	} {
		resp, _ = call("GET", tc.path, tc.auth, tc.body, tc.origin, tc.site, "")
		if resp.StatusCode != tc.code {
			t.Errorf("%s: got %d want %d", tc.path, resp.StatusCode, tc.code)
		}
	}
	if upstreamCalls != before {
		t.Fatal("denied request reached Video")
	}
}
