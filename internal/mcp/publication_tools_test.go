package mcp

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/verdantflarehub/verdantflare-studio/internal/project"
)

func TestAppPublishingUsesSignedVerifiedIdentity(t *testing.T) {
	key := strings.Repeat("k", 40)
	id := func() string { return uuid.Must(uuid.NewV7()).String() }
	p := project.Principal{SubjectID: id(), OrganizationID: id(), RequestID: id()}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(body)
		message := fmt.Sprintf("%s\n%s\n%s\n%s\n%s\n%s\n%x", r.Method, r.URL.Path, r.Header.Get("X-App-Publish-Timestamp"), r.Header.Get("X-App-Publish-Nonce"), p.SubjectID, p.OrganizationID, digest)
		mac := hmac.New(sha256.New, []byte(key))
		_, _ = mac.Write([]byte(message))
		if r.Header.Get("X-App-Publish-User") != p.SubjectID || r.Header.Get("X-App-Publish-Organization") != p.OrganizationID || r.Header.Get("X-App-Publish-Signature") != hex.EncodeToString(mac.Sum(nil)) {
			t.Error("publishing request lacks signed verified identity")
		}
		if _, err := strconv.ParseInt(r.Header.Get("X-App-Publish-Timestamp"), 10, 64); err != nil {
			t.Error(err)
		}
		var request struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(body, &request); err != nil {
			t.Error(err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": map[string]any{"structuredContent": []any{}}})
	}))
	defer server.Close()
	t.Setenv("STUDIO_APP_PUBLISH_ENDPOINT", server.URL+"/api/control/internal/studio-publish/mcp")
	t.Setenv("STUDIO_APP_PUBLISH_HMAC_KEY", key)
	gateway := NewGatewayWithClient(nil)
	gateway.httpClient = server.Client()
	if len(gateway.ListTools()) != 5 {
		t.Fatal("publishing tools were not discovered")
	}
	if _, status, err := gateway.CallTool(context.Background(), "app_publish.organizations", map[string]any{}, nil); status != http.StatusForbidden || err == nil {
		t.Fatalf("unverified call was accepted: %d %v", status, err)
	}
	if _, status, err := gateway.CallVerified(context.Background(), "app_publish.organizations", map[string]any{}, p, ""); status != http.StatusOK || err != nil {
		t.Fatalf("verified publishing call failed: %d %v", status, err)
	}
}
