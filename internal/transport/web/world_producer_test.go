package web

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/verdantflarehub/verdantflare-studio/internal/project"
)

func TestWorldProducerPrivateReadOnly(t *testing.T) {
	id := func() string { return uuid.Must(uuid.NewV7()).String() }
	p := project.Principal{SubjectID: id(), OrganizationID: id(), RequestID: id()}
	pid := id()
	t.Setenv("STUDIO_WORLD_SERVICE_TOKEN", "world-secret")
	t.Setenv("STUDIO_BLENDER_CONTENT_TOKEN", "blender-secret")
	calls := []string{}
	wrongProject := false
	handler := worldProducerHandler(func(ctx context.Context, name string, args map[string]any, actor project.Principal, header string) (any, int, error) {
		if actor != p {
			t.Fatal("identity changed")
		}
		calls = append(calls, name)
		if name == "project.open" {
			if args["project_id"] != pid || len(args) != 1 {
				t.Fatal("unexpected project call")
			}
			return map[string]any{"structuredContent": map[string]any{"project_id": pid}}, 200, nil
		}
		if name != "image.result" || args["execution_proof_only"] != true || args["task_id"] != "task1" || len(args) != 2 || header != pid {
			t.Fatal("non-read-only call")
		}
		projectID := pid
		if wrongProject {
			projectID = id()
		}
		return map[string]any{"structuredContent": map[string]any{"download_path": "PRIVATE", "execution_proof": map[string]any{
			"schema_version": 1, "service_id": "image", "run_id": "task1", "project_id": projectID, "state": "succeeded", "model_id": "model", "route": "codex", "prompt_sha256": "hash", "parameters": map[string]any{}, "inputs": []any{}, "outputs": []any{},
		}}}, 200, nil
	})
	invoke := func(token, path string, body map[string]any) int {
		raw, _ := json.Marshal(body)
		r := httptest.NewRequest("POST", path, bytes.NewReader(raw))
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set("X-User-Id", p.SubjectID)
		r.Header.Set("X-Organization-Id", p.OrganizationID)
		r.Header.Set("X-Request-Id", p.RequestID)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if bytes.Contains(w.Body.Bytes(), []byte("PRIVATE")) {
			t.Fatal("metadata escaped")
		}
		return w.Code
	}
	body := map[string]any{"project_id": pid, "service_id": "image", "run_id": "task1"}
	if invoke("world-secret", "/world/producer-result", body) != 200 || len(calls) != 2 {
		t.Fatal("proof failed")
	}
	calls = nil
	if invoke("blender-secret", "/world/producer-result", body) != 401 || len(calls) != 0 {
		t.Fatal("cross-service authorization")
	}
	for _, path := range []string{"/world/producer-result?url=x", "/world/%70roducer-result", "/world/other"} {
		if invoke("world-secret", path, body) != 404 {
			t.Fatal("route bypass")
		}
	}
	body["tool"] = "image.generate"
	if invoke("world-secret", "/world/producer-result", body) != 400 {
		t.Fatal("tool injection")
	}
	delete(body, "tool")
	wrongProject = true
	if invoke("world-secret", "/world/producer-result", body) != 503 {
		t.Fatal("cross-project proof")
	}
}
