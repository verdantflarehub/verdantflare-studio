package web

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/google/uuid"
	"github.com/verdantflarehub/verdantflare-studio/internal/application"
)

func TestWorkspacesEnrichmentAndComfyUIGuard(t *testing.T) {
	blenderServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/instances" {
			t.Fatalf("unexpected blender path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"instances":[{"instance_id":"01a12a49-4621-769b-b221-dd75aa76bb2e","name":"谢小娥·梦字迷局 白模"}]}`)
	}))
	defer blenderServer.Close()

	comfyuiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/instances" {
			t.Fatalf("unexpected comfyui path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"instances":[{"instance_id":"01a12b1b-f5c3-75a1-bed3-3ef6ea897cf9","name":"ComfyUI Dashboard A"}]}`)
	}))
	defer comfyuiServer.Close()

	t.Setenv("STUDIO_BLENDER_SERVICE_TOKEN", "blender-token-0123456789abcdef0123456789abcdef")
	t.Setenv("STUDIO_COMFYUI_URL", comfyuiServer.URL+"/internal/mcp")
	t.Setenv("STUDIO_COMFYUI_SERVICE_TOKEN", "comfyui-token-0123456789abcdef0123456789abcdef")

	stationID := uuid.Must(uuid.NewV7()).String()
	sessionID := uuid.Must(uuid.NewV7()).String()
	userID := uuid.Must(uuid.NewV7()).String()
	orgID := uuid.Must(uuid.NewV7()).String()

	releasedCalls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/identity/login":
			w.WriteHeader(http.StatusCreated)
			io.WriteString(w, `{"access_token":"tok-1","user_id":"`+userID+`"}`)
		case "/identity/me":
			w.Header().Set("Content-Type", "application/json")
			me, _ := json.Marshal(map[string]any{
				"station_id":         stationID,
				"session_id":         sessionID,
				"user_id":            userID,
				"organization_id":    orgID,
				"organization_name":  "VerdantFlare",
				"request_id":         uuid.Must(uuid.NewV7()).String(),
				"username":           "admin",
				"roles":              []string{"admin"},
				"scopes":             []string{"identity:read"},
				"issued_at":          time.Now().Add(-time.Minute).UTC().Format(time.RFC3339),
				"expires_at":         time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
				"revocation_version": 0,
				"policy_version":     1,
			})
			w.Write(me)
		case "/api/v1/resources/workspaces":
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{
				"workspaces": [
					{
						"namespace": "verdantflare-comfyui",
						"pvc_name": "comfyui-managed-a",
						"state": "bound_stopped",
						"instance_id": "01a12b1b-f5c3-75a1-bed3-3ef6ea897cf9",
						"allowed_actions": []
					},
					{
						"namespace": "verdantflare-comfyui",
						"pvc_name": "comfyui-managed-b",
						"state": "bound_stopped",
						"instance_id": "01a12b1b-f6a3-7be7-8509-08628c2bdc13",
						"bound_at": "2026-10-14T08:00:00Z",
						"allowed_actions": []
					},
					{
						"namespace": "verdantflare-blender",
						"pvc_name": "blender-pool-003",
						"state": "in_use",
						"instance_id": "01a12a49-4621-769b-b221-dd75aa76bb2e",
						"allowed_actions": []
					}
				],
				"request_id": "req-1"
			}`)
		case "/api/v1/resources/workspaces/release":
			releasedCalls++
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"released":true,"workspace":{"namespace":"verdantflare-comfyui","pvc_name":"comfyui-managed-b","state":"available"}}`)
		default:
			t.Fatalf("unexpected upstream path: %s", r.URL.Path)
		}
	}))
	defer upstream.Close()

	station, err := application.NewStation(upstream.URL)
	if err != nil {
		t.Fatal(err)
	}
	handler, srv := NewServer(station, "https://studio.example", fstest.MapFS{"index.html": {Data: []byte("studio")}})
	srv.blenderEndpoint = func() (string, bool) { return blenderServer.URL + "/internal/mcp", true }

	loginReq := httptest.NewRequest(http.MethodPost, "/api/login", strings.NewReader(`{"username":"admin","password":"x"}`))
	loginReq.Header.Set("Origin", "https://studio.example")
	loginReq.Header.Set("Content-Type", "application/json")
	loginRR := httptest.NewRecorder()
	handler.ServeHTTP(loginRR, loginReq)
	cookie := loginRR.Result().Cookies()[0]

	// 1. GET /api/resources/workspaces should enrich names and promote destroyed ComfyUI to retained
	listReq := httptest.NewRequest(http.MethodGet, "/api/resources/workspaces", nil)
	listReq.AddCookie(cookie)
	listRR := httptest.NewRecorder()
	handler.ServeHTTP(listRR, listReq)
	if listRR.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", listRR.Code, listRR.Body.String())
	}
	var listOut struct {
		Workspaces []map[string]any `json:"workspaces"`
	}
	if err := json.Unmarshal(listRR.Body.Bytes(), &listOut); err != nil {
		t.Fatal(err)
	}
	if got := listOut.Workspaces[0]["state"]; got != "bound_stopped" {
		t.Fatalf("expected comfyui-managed-a to stay bound_stopped, got %v", got)
	}
	if got := listOut.Workspaces[0]["instance_name"]; got != "ComfyUI Dashboard A" {
		t.Fatalf("expected comfyui-managed-a instance_name, got %v", got)
	}
	if got := listOut.Workspaces[1]["state"]; got != "retained" {
		t.Fatalf("expected comfyui-managed-b to be promoted to retained, got %v", got)
	}
	if got := listOut.Workspaces[1]["instance_name"]; got != "ComfyUI 实例（已销毁）" {
		t.Fatalf("expected comfyui-managed-b destroyed label, got %v", got)
	}
	if got := listOut.Workspaces[2]["instance_name"]; got != "谢小娥·梦字迷局 白模" {
		t.Fatalf("expected blender-pool-003 instance_name, got %v", got)
	}

	// 2. POST /api/resources/workspaces/release for active ComfyUI instance should return 409 INSTANCE_NOT_DESTROYED
	relActiveReq := httptest.NewRequest(http.MethodPost, "/api/resources/workspaces/release", strings.NewReader(`{"namespace":"verdantflare-comfyui","pvc_name":"comfyui-managed-a"}`))
	relActiveReq.Header.Set("Origin", "https://studio.example")
	relActiveReq.Header.Set("Content-Type", "application/json")
	relActiveReq.AddCookie(cookie)
	relActiveRR := httptest.NewRecorder()
	handler.ServeHTTP(relActiveRR, relActiveReq)
	if relActiveRR.Code != http.StatusConflict || !strings.Contains(relActiveRR.Body.String(), "INSTANCE_NOT_DESTROYED") {
		t.Fatalf("expected 409 INSTANCE_NOT_DESTROYED, got %d: %s", relActiveRR.Code, relActiveRR.Body.String())
	}
	if releasedCalls != 0 {
		t.Fatalf("expected upstream release not called for active ComfyUI instance")
	}

	// 3. POST /api/resources/workspaces/release for destroyed ComfyUI instance should succeed
	relDestroyedReq := httptest.NewRequest(http.MethodPost, "/api/resources/workspaces/release", strings.NewReader(`{"namespace":"verdantflare-comfyui","pvc_name":"comfyui-managed-b"}`))
	relDestroyedReq.Header.Set("Origin", "https://studio.example")
	relDestroyedReq.Header.Set("Content-Type", "application/json")
	relDestroyedReq.AddCookie(cookie)
	relDestroyedRR := httptest.NewRecorder()
	handler.ServeHTTP(relDestroyedRR, relDestroyedReq)
	if relDestroyedRR.Code != http.StatusOK {
		t.Fatalf("expected 200 for destroyed ComfyUI workspace release, got %d: %s", relDestroyedRR.Code, relDestroyedRR.Body.String())
	}
	if releasedCalls != 1 {
		t.Fatalf("expected upstream release called once, got %d", releasedCalls)
	}
}

