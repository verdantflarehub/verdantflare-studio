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

func TestBlenderResourceHTTPIdentityAndDegradation(t *testing.T) {
	id := func() string { return uuid.Must(uuid.NewV7()).String() }
	identity := map[string]any{"station_id": id(), "session_id": id(), "user_id": id(), "organization_id": id(), "organization_name": "fixture", "request_id": "fixture", "username": "fixture", "roles": []string{"admin"}, "scopes": []string{"identity:read"}, "issued_at": time.Now().Add(-time.Minute).UTC(), "expires_at": time.Now().Add(time.Hour).UTC(), "revocation_version": 0, "policy_version": 1}
	mode := "ready"
	coreCalls := 0
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
		container := map[string]any{"name": "blender", "cpu": cpu, "memory": map[string]any{"value": 1073741824, "request": 4294967296, "limit": 17179869184, "sampled_at": sampleAt, "quality": "fresh"}}
		container["gpu_request"], container["gpu_limit"] = 1, 1
		pod := map[string]any{"pod_uid": "private-uid", "namespace": "private-namespace", "pod_name": "private-pod", "instance_alias": "blenderA", "containers": []any{container}, "node_name": "private-node-secret"}
		pod["created_at"] = time.Now().Add(-time.Hour).UTC().Format(time.RFC3339Nano)
		if mode == "dynamic" {
			delete(pod, "instance_alias")
		}
		for key, scenario := range map[string]string{"pod_uid": "wrong_uid", "namespace": "wrong_namespace", "pod_name": "wrong_pod", "instance_alias": "wrong_alias"} {
			if mode == scenario {
				pod[key] = "other"
			}
		}
		if mode == "wrong_container" {
			container["name"] = "sidecar"
		}
		pods := []any{pod, map[string]any{"instance_alias": "blenderOtherTenant", "node_name": "private-other-tenant"}}
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
		gpu := map[string]any{"uuid": "private-gpu", "memory": map[string]any{"value": 0, "quality": "fresh", "sampled_at": stamp()}}
		if mode == "gpu_wrong" {
			gpu["uuid"] = "private-other-gpu"
		}
		if mode == "gpu_shared" {
			container["gpu_request"] = 0.5
		}
		if mode == "gpu_old_process" {
			pod["created_at"] = time.Now().Add(time.Second).UTC().Format(time.RFC3339Nano)
		}
		gpus := []any{gpu}
		if mode == "gpu_duplicate" {
			gpus = append(gpus, gpu)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"schema_version": version, "updated_at": observed, "workloads": pods, "gpu_samples": gpus})
	}))
	defer core.Close()
	worker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer internal-service" {
			t.Error("user token leaked to app")
		}
		w.Header().Set("Content-Type", "application/json")
		if mode == "denied" {
			w.WriteHeader(403)
			_, _ = w.Write([]byte(`{"code":"FORBIDDEN"}`))
			return
		}
		empty := map[string]any{"value": nil, "request": nil, "limit": nil, "sampled_at": nil, "quality": "unavailable"}
		binding := any(map[string]any{"pod_uid": "private-uid", "namespace": "private-namespace", "pod_name": "private-pod", "container": "blender", "gpu_uuid": "private-gpu"})
		if mode == "no_binding" {
			binding = nil
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"schema_version": 1, "capabilities": map[string]bool{"create": false}, "instances": []any{map[string]any{
			"alias": "blenderA", "status": "running", "observed_at": stamp(), "allocated_gpu_count": nil, "_resource_binding": binding,
			"resources": map[string]any{"cpu": empty, "memory": empty, "gpu": empty, "storage": empty},
		}}})
	}))
	defer worker.Close()
	station, _ := application.NewStation(core.URL)
	router, s := NewServer(station, "https://studio.example", fstest.MapFS{})
	s.blenderEndpoint = func() (string, bool) { return worker.URL + "/internal/mcp", true }
	t.Setenv("STUDIO_BLENDER_SERVICE_TOKEN", "internal-service")
	for _, scenario := range []string{"ready", "dynamic", "gpu_wrong", "gpu_shared", "gpu_old_process", "gpu_duplicate", "missing", "stale", "offline", "wrong_uid", "wrong_namespace", "wrong_pod", "wrong_alias", "wrong_container", "duplicate", "old_core", "stale_config", "no_binding", "denied"} {
		t.Run(scenario, func(t *testing.T) {
			mode = scenario
			before := coreCalls
			r := httptest.NewRequest("GET", "/studio/apps/blender/instances", nil)
			r.Header.Set("Authorization", "Bearer verified-user")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, r)
			body := w.Body.String()
			for _, private := range []string{"private-", "_resource_binding", "blenderOtherTenant", "verified-user", "internal-service"} {
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
			gpu := result.Instances[0].Resources["gpu"]
			if mode == "ready" && (gpu.Value == nil || *gpu.Value != 0 || gpu.Scope != "exclusive_gpu") {
				t.Fatal("valid zero GPU sample lost")
			}
			if strings.HasPrefix(mode, "gpu_") && gpu.Value != nil {
				t.Fatal("unattributable GPU sample accepted")
			}
			switch mode {
			case "ready", "dynamic", "gpu_wrong", "gpu_shared", "gpu_old_process", "gpu_duplicate":
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
}
