package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/verdantflarehub/verdantflare-studio/internal/project"
)

func TestGenerationGatewayPinsRequestAndNeverResubmitsUnknown(t *testing.T) {
	for _, lost := range []bool{false, true} {
		t.Run(map[bool]string{false: "submitted", true: "lost-response"}[lost], func(t *testing.T) {
			t.Setenv("STUDIO_GENERATION_CONTROL_ENABLED", "true")
			t.Setenv("STUDIO_PROJECT_SERVICE_TOKEN", strings.Repeat("s", 32))
			t.Setenv("STUDIO_IMAGE_TOKEN", strings.Repeat("i", 32))
			id := func() string { return uuid.Must(uuid.NewV7()).String() }
			p := project.Principal{OrganizationID: id(), SubjectID: id(), RequestID: id()}
			pid, planID, operationID := id(), id(), id()
			attempt := project.GenerationAttempt{OperationID: operationID, ObjectID: "LOC06", StateID: "night", Purpose: "sample", Tool: "image.create", ServiceVersion: "0.2.3", Arguments: json.RawMessage(`{"engine":"grok","model":"grok-imagine-image-2.0","prompt":"scene","resolution":"1k","aspect_ratio":"16:9"}`)}
			plan := project.GenerationPlanStatus{Plan: project.GenerationPlan{ProjectID: pid, PlanID: planID, RevisionID: id(), Stage: "sample", CallLimit: 1, ExpiresAt: time.Now().Add(time.Hour), Attempts: []project.GenerationAttempt{attempt}}, State: "approved", RemainingCalls: 1, Operations: []project.GenerationOperation{}}
			var mu sync.Mutex
			business := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				defer mu.Unlock()
				w.Header().Set("X-Request-Id", p.RequestID)
				if r.Header.Get("Authorization") != "Bearer "+strings.Repeat("s", 32) {
					t.Error("incorrect internal authority")
				}
				switch r.URL.Path {
				case "/internal/v1/generation/policy":
					_ = json.NewEncoder(w).Encode(project.GenerationPolicy{Controlled: true})
				case "/internal/v1/generation/status":
					_ = json.NewEncoder(w).Encode(plan)
				case "/internal/v1/generation/reserve":
					fresh := len(plan.Operations) == 0
					if fresh {
						plan.Operations = []project.GenerationOperation{{OperationID: operationID, State: "reserved", RequestSHA256: strings.Repeat("a", 64)}}
						plan.OccupiedCalls = 1
						plan.RemainingCalls = 0
					}
					_ = json.NewEncoder(w).Encode(project.GenerationReservation{Operation: plan.Operations[0], Attempt: attempt, Acquired: fresh})
				case "/internal/v1/generation/transition":
					var in project.GenerationTransition
					_ = json.NewDecoder(r.Body).Decode(&in)
					plan.Operations[0].State = in.State
					plan.Operations[0].Response = in.Response
					_ = json.NewEncoder(w).Encode(plan.Operations[0])
				default:
					t.Error("unexpected business operation", r.URL.Path)
					w.WriteHeader(404)
				}
			}))
			defer business.Close()
			var supplierCalls atomic.Int32
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				supplierCalls.Add(1)
				mu.Lock()
				if len(plan.Operations) != 1 || plan.Operations[0].State != "unknown" {
					t.Error("side effect preceded durable unknown state")
				}
				mu.Unlock()
				var call struct {
					ID     string `json:"id"`
					Params struct {
						Name      string            `json:"name"`
						Arguments map[string]string `json:"arguments"`
					} `json:"params"`
				}
				_ = json.NewDecoder(r.Body).Decode(&call)
				if call.Params.Name != "image.create" || call.Params.Arguments["project_id"] != pid || call.Params.Arguments["idempotency_key"] != operationID || call.Params.Arguments["resolution"] != "1k" || call.Params.Arguments["prompt"] != "scene" {
					t.Error("approved request changed")
				}
				if lost {
					w.WriteHeader(503)
					_, _ = w.Write([]byte(`{"error":"unknown"}`))
					return
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": call.ID, "result": map[string]any{"isError": false, "structuredContent": map[string]string{"task_id": "fake-task"}}})
			}))
			defer provider.Close()
			g := NewGatewayWithClient(nil)
			g.services["project"] = ServiceRegistration{Domain: "project", Endpoint: business.URL + "/mcp"}
			g.services["image"] = ServiceRegistration{Domain: "image", Endpoint: provider.URL, Version: "0.2.3", Tools: []ToolDefinition{{Name: "image.create"}}}
			args := map[string]any{"project_id": pid, "plan_id": planID, "operation_id": operationID}
			if _, status, _ := g.CallVerified(context.Background(), "image.create", map[string]any{"project_id": pid}, p, ""); status != 403 {
				t.Fatal("direct generation bypass", status)
			}
			if _, status, _ := g.CallVerified(context.Background(), "image.create", map[string]any{}, p, ""); status != 400 {
				t.Fatal("missing project bypass", status)
			}
			if _, status, _ := g.CallVerified(context.Background(), "project.generation_submit", args, p, id()); status != 400 {
				t.Fatal("cross project bypass", status)
			}
			bad := map[string]any{"project_id": pid, "plan_id": planID, "operation_id": operationID, "prompt": "unapproved"}
			if _, status, _ := g.CallVerified(context.Background(), "project.generation_submit", bad, p, ""); status != 400 {
				t.Fatal("request expansion accepted", status)
			}
			if supplierCalls.Load() != 0 {
				t.Fatal("rejected request reached supplier")
			}
			result, status, e := g.CallVerified(context.Background(), "project.generation_submit", args, p, "")
			if e != nil || (status != 200 && status != 202) {
				t.Fatal(status, e)
			}
			encoded, _ := json.Marshal(result)
			var envelope struct {
				Content []struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"content"`
				Structured json.RawMessage `json:"structuredContent"`
			}
			if json.Unmarshal(encoded, &envelope) != nil || len(envelope.Content) != 1 || envelope.Content[0].Type != "text" {
				t.Fatal("missing MCP content fallback")
			}
			var left, right project.GenerationOperation
			if json.Unmarshal(envelope.Structured, &left) != nil || json.Unmarshal([]byte(envelope.Content[0].Text), &right) != nil || left.OperationID != right.OperationID || left.State != right.State {
				t.Fatal("MCP structured/text result mismatch")
			}
			// Simulate gateway restart using only persisted business state.
			g2 := NewGatewayWithClient(nil)
			g2.services = g.services
			if _, status, e = g2.CallVerified(context.Background(), "project.generation_submit", args, p, ""); e != nil || status != 200 {
				t.Fatal(status, e)
			}
			if supplierCalls.Load() != 1 {
				t.Fatal("duplicate supplier request", supplierCalls.Load())
			}
			if plan.OccupiedCalls != 1 || plan.RemainingCalls != 0 {
				t.Fatal("unknown/submitted call was refunded")
			}
		})
	}
}

func TestGenerationBoundedAdapterRefusesHiddenDefaultsAndUncontrolledCosts(t *testing.T) {
	for _, raw := range []string{
		`{"engine":"codex","model":"gpt-image-2.5-sunburst","prompt":"x","resolution":"1k","aspect_ratio":"16:9"}`,
		`{"engine":"grok","prompt":"x","resolution":"1k","aspect_ratio":"16:9"}`,
		`{"engine":"grok","model":"grok-imagine-image-2.0","prompt":"x","aspect_ratio":"16:9"}`,
		`{"engine":"grok","model":"grok-imagine-image-2.0","prompt":"x","resolution":"1k","aspect_ratio":"16:9","quality":"max"}`,
		`{"engine":"grok","model":"grok-imagine-image-2.0","prompt":"x","resolution":"1k","aspect_ratio":"16:9","n":4}`,
	} {
		if _, e := boundedImageArguments(project.GenerationAttempt{Tool: "image.create", ServiceVersion: "0.2.3", Arguments: json.RawMessage(raw)}); e == nil {
			t.Fatal("unbounded/defaulted request accepted", raw)
		}
	}
}
