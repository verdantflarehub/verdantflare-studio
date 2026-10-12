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
	"github.com/verdantflarehub/verdantflare-studio/internal/testdb"
	"github.com/verdantflarehub/verdantflare-studio/migrations"
)

// Runs on 5090 with a disposable PostgreSQL database. The gateway and Project
// handlers are production code; only the billable image provider is a substitute.
func TestGenerationGatewayDatabaseIntegration(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	if e := migrations.Apply(ctx, db); e != nil {
		t.Fatal(e)
	}
	// Generation control must not touch Artifact. An accidental content call
	// through this nil embedded interface fails the test instead of using S3.
	s, e := project.NewService(ctx, db, &struct{ project.Artifact }{}, nil)
	if e != nil {
		t.Fatal(e)
	}
	id := func() string { return uuid.Must(uuid.NewV7()).String() }
	p := project.Principal{OrganizationID: id(), SubjectID: id(), RequestID: id()}
	pid, rid, commit := id(), id(), id()
	seed := []struct {
		q string
		a []any
	}{
		{`INSERT INTO studio.projects(project_id,organization_id,owner_id,name,category) VALUES($1,$2,$3,'WF2 isolated gateway test','ai_short_drama')`, []any{pid, p.OrganizationID, p.SubjectID}},
		{`INSERT INTO studio.project_members VALUES($1,$2,$3,'owner')`, []any{p.OrganizationID, pid, p.SubjectID}},
		{`INSERT INTO studio.project_commits(organization_id,project_id,commit_id,created_by,request_sha256,revision_id,state,plan) VALUES($1,$2,$3,$4,$5,$6,'preparing','{}')`, []any{p.OrganizationID, pid, commit, p.SubjectID, strings.Repeat("0", 64), rid}},
		{`INSERT INTO studio.project_revisions(revision_id,organization_id,project_id,manifest_ref,manifest_json,created_by,commit_id) VALUES($1,$2,$3,'{}','{"category":"ai_short_drama"}',$4,$5)`, []any{rid, p.OrganizationID, pid, p.SubjectID, commit}},
	}
	for _, q := range seed {
		if _, e = db.Exec(ctx, q.q, q.a...); e != nil {
			t.Fatal(e)
		}
	}
	t.Setenv("STUDIO_GENERATION_CONTROL_ENABLED", "true")
	t.Setenv("STUDIO_PROJECT_SERVICE_TOKEN", strings.Repeat("s", 32))
	t.Setenv("STUDIO_GENERATION_APPROVAL_TOKEN", strings.Repeat("h", 32))
	t.Setenv("STUDIO_IMAGE_TOKEN", strings.Repeat("i", 32))
	control, e := s.GenerationControlHandler(strings.Repeat("s", 32), strings.Repeat("h", 32))
	if e != nil {
		t.Fatal(e)
	}
	mcpHandler, e := s.MCPHandler(strings.Repeat("s", 32), strings.Repeat("a", 32))
	if e != nil {
		t.Fatal(e)
	}
	mux := http.NewServeMux()
	mux.Handle("/internal/v1/generation/", control)
	mux.Handle("/mcp", mcpHandler)
	business := httptest.NewServer(mux)
	defer business.Close()
	var posts atomic.Int32
	var lose atomic.Bool
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		posts.Add(1)
		var call struct {
			ID     string `json:"id"`
			Params struct {
				Arguments map[string]string `json:"arguments"`
			} `json:"params"`
		}
		_ = json.NewDecoder(r.Body).Decode(&call)
		if lose.Load() {
			w.WriteHeader(503)
			_, _ = w.Write([]byte(`{"error":"simulated lost response"}`))
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": call.ID, "result": map[string]any{"isError": false, "structuredContent": map[string]string{"task_id": "fixture-" + call.Params.Arguments["idempotency_key"]}}})
	}))
	defer provider.Close()
	g := NewGatewayWithClient(nil)
	definitions := []ToolDefinition{}
	for _, tool := range project.MCPTools() {
		if strings.HasPrefix(tool.Name, "project.") {
			definitions = append(definitions, ToolDefinition{Name: tool.Name, Description: tool.Description, InputSchema: tool.InputSchema})
		}
	}
	g.services["project"] = ServiceRegistration{Domain: "project", Endpoint: business.URL + "/mcp", Version: "0.5.100", Tools: definitions}
	g.services["image"] = ServiceRegistration{Domain: "image", Endpoint: provider.URL, Version: "0.2.3", Tools: []ToolDefinition{{Name: "image.create"}}}
	for _, lost := range []bool{false, true} {
		lose.Store(lost)
		before := posts.Load()
		planID := id()
		attempts := []project.GenerationAttempt{}
		for i := 0; i < 2; i++ {
			attempts = append(attempts, project.GenerationAttempt{OperationID: id(), ObjectID: "test-scene", StateID: "night", Purpose: "fixture", Tool: "image.create", ServiceVersion: "0.2.3", Arguments: json.RawMessage(`{"engine":"grok","model":"grok-imagine-image-2.0","prompt":"fixture","resolution":"1k","aspect_ratio":"16:9"}`)})
		}
		plan := project.GenerationPlan{ProjectID: pid, PlanID: planID, RevisionID: rid, PreparationSHA256: strings.Repeat("a", 64), Stage: "material_sample", CallLimit: 1, ExpiresAt: time.Now().UTC().Add(time.Hour), Attempts: attempts}
		body, _ := json.Marshal(plan)
		var proposal map[string]any
		_ = json.Unmarshal(body, &proposal)
		if _, status, e := g.CallVerified(ctx, "project.generation_propose", proposal, p, ""); e != nil || status != 200 {
			t.Fatal(status, e)
		}
		key := project.GenerationPlanKey{ProjectID: pid, PlanID: planID}
		saved, e := s.GenerationStatus(ctx, p, key)
		if e != nil {
			t.Fatal(e)
		}
		args := func(op string) map[string]any {
			return map[string]any{"project_id": pid, "plan_id": planID, "operation_id": op}
		}
		if _, status, _ := g.CallVerified(ctx, "project.generation_submit", args(attempts[0].OperationID), p, ""); status != 403 {
			t.Fatal("unapproved request accepted", status)
		}
		if _, status, e := g.GenerationApprove(ctx, p, project.GenerationDecision{GenerationPlanKey: key, RequestSHA256: saved.RequestSHA256}); e != nil || status != 200 {
			t.Fatal(status, e)
		}
		var wg sync.WaitGroup
		for i := 0; i < 24; i++ {
			wg.Add(1)
			go func(index int) {
				defer wg.Done()
				_, status, e := g.CallVerified(ctx, "project.generation_submit", args(attempts[index%2].OperationID), p, "")
				if status != 200 && status != 202 && status != 409 {
					t.Errorf("unexpected result: %d %v", status, e)
				}
			}(i)
		}
		wg.Wait()
		if posts.Load()-before != 1 {
			t.Fatal("concurrent overspend", posts.Load()-before)
		}
		saved, e = s.GenerationStatus(ctx, p, key)
		if e != nil || saved.OccupiedCalls != 1 || saved.RemainingCalls != 0 || len(saved.Operations) != 1 {
			t.Fatal(saved, e)
		}
		state := "submitted"
		if lost {
			state = "unknown"
		}
		if saved.Operations[0].State != state {
			t.Fatal("unexpected durable result", saved.Operations[0].State)
		}
		if _, e = s.GenerationRevoke(ctx, p, key); e != nil {
			t.Fatal(e)
		}
		restarted := NewGatewayWithClient(nil)
		restarted.services = g.services
		if _, status, e := restarted.CallVerified(ctx, "project.generation_submit", args(saved.Operations[0].OperationID), p, ""); e != nil || status != 200 {
			t.Fatal("read-only recovery failed", status, e)
		}
		if posts.Load()-before != 1 {
			t.Fatal("recovery repeated provider call")
		}
	}
}
