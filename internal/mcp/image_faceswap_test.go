package mcp

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestImageFaceSwapKeepsSynchronousResponseAndCancellation(t *testing.T) {
	t.Setenv("STUDIO_IMAGE_TOKEN", "test-internal-image")
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
			return
		case <-time.After(80 * time.Millisecond):
		}
		io.WriteString(w, `{"jsonrpc":"2.0","id":"test","result":{"content":[]}}`)
	}))
	defer upstream.Close()
	g := NewGatewayWithClient(nil)
	g.httpClient.Timeout = 20 * time.Millisecond
	g.services["image"] = ServiceRegistration{Endpoint: upstream.URL, Tools: []ToolDefinition{{Name: "image.faceswap"}, {Name: "image.status"}}}
	if _, status, err := g.call(t.Context(), "image.faceswap", nil, nil); err != nil || status != 200 {
		t.Fatalf("synchronous fusion: %d %v", status, err)
	}
	if _, status, err := g.call(t.Context(), "image.status", nil, nil); err == nil || status != 502 {
		t.Fatal("unrelated operation lost its timeout")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, _, err := g.call(ctx, "image.faceswap", nil, nil); err == nil {
		t.Fatal("request cancellation ignored")
	}
	if g.httpClient.Timeout != 20*time.Millisecond {
		t.Fatal("shared client timeout mutated")
	}
}
