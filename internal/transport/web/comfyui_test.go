package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/google/uuid"
	"github.com/verdantflarehub/verdantflare-studio/internal/application"
)

func TestComfyUIResourceHTTPIdentityAndDegradation(t *testing.T) {
	id := func() string { return uuid.Must(uuid.NewV7()).String() }
	identity := map[string]any{"station_id": id(), "session_id": id(), "user_id": id(), "organization_id": id(), "organization_name": "fixture", "request_id": "fixture", "username": "fixture", "roles": []string{"admin"}, "scopes": []string{"identity:read"}, "issued_at": time.Now().Add(-time.Minute).UTC(), "expires_at": time.Now().Add(time.Hour).UTC(), "revocation_version": 0, "policy_version": 1}
	mode := "ready"
	coreCalls := 0
	appCalls := 0
	redirectCalls := 0
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirectCalls++
		w.WriteHeader(200)
	}))
	defer redirect.Close()
	stamp := func() string { return time.Now().UTC().Format(time.RFC3339Nano) }
	core := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer verified-user" {
			t.Error("wrong credential forwarded to Core")
			w.WriteHeader(401)
			return
		}
		if r.URL.Path == "/identity/me" {
			_ = json.NewEncoder(w).Encode(identity)
			return
		}
		if r.URL.Path != "/api/v1/resources/workloads" {
			t.Error("unexpected Core path")
			w.WriteHeader(404)
			return
		}
		coreCalls++
		if mode == "offline" {
			w.WriteHeader(503)
			_, _ = w.Write([]byte(`{"code":"unavailable"}`))
			return
		}
		sampleAt := stamp()
		if mode == "stale" {
			sampleAt = time.Now().Add(-time.Minute).UTC().Format(time.RFC3339Nano)
		}
		cpu := map[string]any{"value": 0, "request": 2, "limit": 8, "quality": "fresh", "sampled_at": sampleAt, "reason": "private-node-secret"}
		if mode == "missing" {
			cpu["value"] = nil
			cpu["quality"] = "unavailable"
		}
		container := map[string]any{"name": "comfyui", "cpu": cpu, "memory": map[string]any{"value": 1073741824, "request": 4294967296, "limit": 17179869184, "sampled_at": sampleAt, "quality": "fresh"}}
		pod := map[string]any{"pod_uid": "private-uid", "namespace": "private-namespace", "pod_name": "private-pod", "instance_alias": "comfyuiA", "containers": []any{container}, "node_name": "private-node-secret"}
		for key, scenario := range map[string]string{"pod_uid": "wrong_uid", "namespace": "wrong_namespace", "pod_name": "wrong_pod", "instance_alias": "wrong_alias"} {
			if mode == scenario {
				pod[key] = "other"
			}
		}
		if mode == "wrong_container" {
			container["name"] = "sidecar"
		}
		pods := []any{pod, map[string]any{"instance_alias": "comfyuiOtherTenant", "node_name": "private-other-tenant"}}
		if mode == "duplicate" {
			pods = append(pods, pod)
		}
		version := 2
		if mode == "old_core" {
			version = 1
		}
		observed := stamp()
		if mode == "stale_config" {
			observed = time.Now().Add(-time.Minute).UTC().Format(time.RFC3339Nano)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"schema_version": version, "updated_at": observed, "workloads": pods})
	}))
	defer core.Close()
	worker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		appCalls++
		if r.Header.Get("Authorization") != "Bearer comfyui-test-internal-service-token-0123456789" {
			t.Error("user token leaked to app")
		}
		if r.URL.Path != "/internal/instances" || r.URL.RawQuery != "" || r.Method != "GET" || r.Header.Get("X-User-Id") != identity["user_id"] || r.Header.Get("X-Organization-Id") != identity["organization_id"] || r.Header.Get("X-Request-Id") == "" || r.Header.Get("Cookie") != "" || r.Header.Get("X-Project-Id") != "" {
			t.Error("incorrect app request identity or route")
		}
		w.Header().Set("Content-Type", "application/json")
		switch mode {
		case "redirect":
			http.Redirect(w, r, redirect.URL, http.StatusFound)
			return
		case "non_json":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte("private-error"))
			return
		case "oversized":
			_, _ = w.Write([]byte(strings.Repeat(" ", (1<<20)+1)))
			return
		}
		if mode == "denied" {
			w.WriteHeader(403)
			_, _ = w.Write([]byte(`{"code":"FORBIDDEN"}`))
			return
		}
		empty := map[string]any{"value": nil, "request": nil, "limit": nil, "sampled_at": nil, "quality": "unavailable"}
		binding := any(map[string]any{"pod_uid": "private-uid", "namespace": "private-namespace", "pod_name": "private-pod", "container": "comfyui"})
		if mode == "no_binding" {
			binding = nil
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"schema_version": 1, "capabilities": map[string]bool{"create": false}, "instances": []any{map[string]any{
			"alias": "comfyuiA", "status": "running", "observed_at": stamp(), "allocated_gpu_count": nil, "_resource_binding": binding,
			"resources": map[string]any{"cpu": empty, "memory": empty, "gpu": empty, "storage": empty},
		}}})
	}))
	defer worker.Close()
	station, _ := application.NewStation(core.URL)
	router, s := NewServer(station, "https://studio.example", fstest.MapFS{})
	s.comfyuiEndpoint = func() (string, bool) { return worker.URL + "/internal/mcp", true }
	t.Setenv("STUDIO_COMFYUI_SERVICE_TOKEN", "comfyui-test-internal-service-token-0123456789")
	for _, scenario := range []string{"ready", "missing", "stale", "offline", "wrong_uid", "wrong_namespace", "wrong_pod", "wrong_alias", "wrong_container", "duplicate", "old_core", "stale_config", "no_binding", "denied", "redirect", "non_json", "oversized"} {
		t.Run(scenario, func(t *testing.T) {
			mode = scenario
			before := coreCalls
			r := httptest.NewRequest("GET", "/studio/apps/comfyui/instances", nil)
			r.Header.Set("Authorization", "Bearer verified-user")
			r.Header.Set("Cookie", "user-browser-cookie=private-cookie")
			r.Header.Set("X-Project-Id", id())
			w := httptest.NewRecorder()
			router.ServeHTTP(w, r)
			body := w.Body.String()
			for _, private := range []string{"private-", "_resource_binding", "comfyuiOtherTenant", "verified-user", "comfyui-test-internal-service-token-0123456789"} {
				if strings.Contains(body, private) {
					t.Fatal("internal data escaped", body)
				}
			}
			if mode == "denied" {
				if w.Code != 403 || coreCalls != before {
					t.Fatal("denied directory enriched")
				}
				return
			}
			if mode == "redirect" || mode == "non_json" || mode == "oversized" {
				if w.Code != 502 || coreCalls != before || redirectCalls != 0 {
					t.Fatal("invalid upstream response was accepted", w.Code)
				}
				return
			}
			if w.Code != 200 {
				t.Fatal(w.Code, body)
			}
			var result struct {
				Instances []struct {
					Resources map[string]blenderResourceMetric `json:"resources"`
				} `json:"instances"`
			}
			if json.Unmarshal(w.Body.Bytes(), &result) != nil || len(result.Instances) != 1 {
				t.Fatal(body)
			}
			cpu := result.Instances[0].Resources["cpu"]
			switch mode {
			case "ready":
				if cpu.Value == nil || *cpu.Value != 0 || cpu.Quality != "fresh" || *cpu.Request != 2 {
					t.Fatal(body)
				}
			case "stale":
				if cpu.Value != nil || cpu.Quality != "stale" || cpu.Request == nil {
					t.Fatal(body)
				}
			case "missing":
				if cpu.Value != nil || cpu.Request == nil {
					t.Fatal(body)
				}
			default:
				if cpu.Value != nil || cpu.Request != nil {
					t.Fatal("mismatched source consumed", body)
				}
			}
			if mode == "no_binding" && coreCalls != before {
				t.Fatal("unnecessary station inventory request")
			}
		})
	}
	for _, scenario := range []struct {
		name, path string
		headers    map[string]string
		status     int
	}{
		{"missing_auth", "/studio/apps/comfyui/instances", map[string]string{"Origin": "https://studio.example"}, 401},
		{"foreign_origin", "/studio/apps/comfyui/instances", map[string]string{"Authorization": "Bearer verified-user", "Origin": "https://foreign.example"}, 403},
		{"spoofed_user", "/studio/apps/comfyui/instances", map[string]string{"Authorization": "Bearer verified-user", "X-User-Id": id()}, 403},
		{"spoofed_org", "/studio/apps/comfyui/instances", map[string]string{"Authorization": "Bearer verified-user", "X-Organization-Id": id()}, 403},
		{"query", "/studio/apps/comfyui/instances?target=other", map[string]string{"Authorization": "Bearer verified-user"}, 400},
		{"encoded_path", "/studio/apps/comfyui/%69nstances", map[string]string{"Authorization": "Bearer verified-user"}, 400},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			before := appCalls
			r := httptest.NewRequest("GET", scenario.path, nil)
			for key, value := range scenario.headers {
				r.Header.Set(key, value)
			}
			w := httptest.NewRecorder()
			router.ServeHTTP(w, r)
			if w.Code != scenario.status || appCalls != before {
				t.Fatal("invalid request reached app", w.Code)
			}
		})
	}
}
