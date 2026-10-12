package project

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/verdantflarehub/verdantflare-studio/internal/testdb"
	"github.com/verdantflarehub/verdantflare-studio/migrations"
)

func generationExample() GenerationPlan {
	return GenerationPlan{ProjectID: newID(), PlanID: newID(), RevisionID: newID(), PreparationSHA256: strings.Repeat("a", 64), Stage: "material_sample", CallLimit: 1, ExpiresAt: time.Now().UTC().Add(time.Hour).Truncate(time.Second), Attempts: []GenerationAttempt{
		{OperationID: newID(), ObjectID: "LOC06", StateID: "night", Purpose: "scene sample", Tool: "image.create", ServiceVersion: "0.2.3", Arguments: json.RawMessage(`{"engine":"grok","model":"grok-imagine-image-2.0","prompt":"scene","resolution":"1k","aspect_ratio":"16:9"}`)},
	}}
}
func TestGenerationPlanValidation(t *testing.T) {
	if e := generationExample().Validate(time.Now()); e != nil {
		t.Fatal(e)
	}
	cases := map[string]func(*GenerationPlan){
		"expired":             func(p *GenerationPlan) { p.ExpiresAt = time.Now().Add(-time.Second) },
		"unbounded":           func(p *GenerationPlan) { p.ExpiresAt = time.Now().Add(48 * time.Hour) },
		"no limit":            func(p *GenerationPlan) { p.CallLimit = 0 },
		"inflated limit":      func(p *GenerationPlan) { p.CallLimit = 2 },
		"duplicate operation": func(p *GenerationPlan) { p.Attempts = append(p.Attempts, p.Attempts[0]) },
		"cross project":       func(p *GenerationPlan) { p.Attempts[0].Arguments = json.RawMessage(`{"project_id":"other"}`) },
		"client idempotency":  func(p *GenerationPlan) { p.Attempts[0].Arguments = json.RawMessage(`{"idempotency_key":"other"}`) },
		"forged approval":     func(p *GenerationPlan) { p.Attempts[0].Arguments = json.RawMessage(`{"approved":true}`) },
		"duplicate JSON": func(p *GenerationPlan) {
			p.Attempts[0].Arguments = json.RawMessage(`{"model":"cheap","model":"expensive"}`)
		},
		"non object":        func(p *GenerationPlan) { p.Attempts[0].Arguments = json.RawMessage(`[]`) },
		"uncontrolled tool": func(p *GenerationPlan) { p.Attempts[0].Tool = "image.faceswap" },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			p := generationExample()
			change(&p)
			if p.Validate(time.Now()) == nil {
				t.Fatal("accepted invalid plan")
			}
		})
	}
}
func TestGenerationHashBindsExactInput(t *testing.T) {
	a := json.RawMessage(`{"a":9007199254740993,"b":{"x":1,"y":2}}`)
	b := json.RawMessage(`{ "b": {"y":2,"x":1}, "a":9007199254740993}`)
	h1, e := generationHash(a)
	if e != nil {
		t.Fatal(e)
	}
	h2, _ := generationHash(b)
	if h1 != h2 {
		t.Fatal("object order changed digest")
	}
	h3, _ := generationHash(json.RawMessage(`{"a":9007199254740992,"b":{"x":1,"y":2}}`))
	if h1 == h3 {
		t.Fatal("numeric precision lost")
	}
}

// Runs only with an explicitly configured remote disposable database. No
// Artifact, supplier, original project, local container or real quota is used.
func TestGenerationLedgerIntegration(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	if e := migrations.Apply(ctx, db); e != nil {
		t.Fatal(e)
	}
	s := &Service{db: db}
	p := Principal{OrganizationID: newID(), SubjectID: newID(), RequestID: newID()}
	plan := generationExample()
	commit := newID()
	seed := []struct {
		q string
		a []any
	}{
		{`INSERT INTO studio.projects(project_id,organization_id,owner_id,name,category) VALUES($1,$2,$3,'isolated generation test','ai_short_drama')`, []any{plan.ProjectID, p.OrganizationID, p.SubjectID}},
		{`INSERT INTO studio.project_members VALUES($1,$2,$3,'owner')`, []any{p.OrganizationID, plan.ProjectID, p.SubjectID}},
		{`INSERT INTO studio.project_commits(organization_id,project_id,commit_id,created_by,request_sha256,revision_id,state,plan) VALUES($1,$2,$3,$4,$5,$6,'preparing','{}')`, []any{p.OrganizationID, plan.ProjectID, commit, p.SubjectID, strings.Repeat("0", 64), plan.RevisionID}},
		{`INSERT INTO studio.project_revisions(revision_id,organization_id,project_id,manifest_ref,manifest_json,created_by,commit_id) VALUES($1,$2,$3,'{}','{"category":"ai_short_drama"}',$4,$5)`, []any{plan.RevisionID, p.OrganizationID, plan.ProjectID, p.SubjectID, commit}},
	}
	for _, q := range seed {
		if _, e := db.Exec(ctx, q.q, q.a...); e != nil {
			t.Fatal(e)
		}
	}
	key := GenerationPlanKey{plan.ProjectID, plan.PlanID}
	op := GenerationOperationKey{key, plan.Attempts[0].OperationID}
	status, e := s.GenerationPropose(ctx, p, plan)
	if e != nil {
		t.Fatal(e)
	}
	if status.State != "pending" || status.OccupiedCalls != 0 {
		t.Fatal(status)
	}
	if _, e = s.GenerationReserve(ctx, p, op); !errors.Is(e, ErrApproval) {
		t.Fatal("pending plan executed", e)
	}
	changed := plan
	changed.Attempts = append([]GenerationAttempt{}, plan.Attempts...)
	changed.Attempts[0].Purpose = "changed"
	if _, e = s.GenerationPropose(ctx, p, changed); !errors.Is(e, ErrIdempotency) {
		t.Fatal("changed proposal accepted", e)
	}
	if _, e = s.GenerationApprove(ctx, p, GenerationDecision{key, strings.Repeat("f", 64)}); !errors.Is(e, ErrIdempotency) {
		t.Fatal("wrong hash approved", e)
	}
	if _, e = s.GenerationApprove(ctx, p, GenerationDecision{key, status.RequestSHA256}); e != nil {
		t.Fatal(e)
	}
	var acquired atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 24; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, e := s.GenerationReserve(ctx, p, op)
			if e != nil {
				t.Error(e)
			}
			if r.Acquired {
				acquired.Add(1)
			}
		}()
	}
	wg.Wait()
	if acquired.Load() != 1 {
		t.Fatal("duplicate dispatch rights", acquired.Load())
	}
	// Reconstruct the service to prove no in-memory lease is needed.
	s = &Service{db: db}
	r, e := s.GenerationReserve(ctx, p, op)
	if e != nil || r.Acquired {
		t.Fatal("restart redispatched", r, e)
	}
	if _, e = s.GenerationTransition(ctx, p, GenerationTransition{GenerationOperationKey: op, State: "unknown"}); e != nil {
		t.Fatal(e)
	}
	if _, e = s.GenerationTransition(ctx, p, GenerationTransition{GenerationOperationKey: op, State: "released"}); !errors.Is(e, ErrGenerationState) {
		t.Fatal("unknown refunded", e)
	}
	if _, e = s.GenerationRevoke(ctx, p, key); e != nil {
		t.Fatal(e)
	}
	if _, e = s.GenerationApprove(ctx, p, GenerationDecision{key, status.RequestSHA256}); !errors.Is(e, ErrApproval) {
		t.Fatal("revocation undone", e)
	}
	response := json.RawMessage(`{"task_id":"isolated-task"}`)
	if _, e = s.GenerationTransition(ctx, p, GenerationTransition{GenerationOperationKey: op, State: "submitted", Response: response}); e != nil {
		t.Fatal("existing response could not recover after revoke", e)
	}
	status, e = s.GenerationStatus(ctx, p, key)
	if e != nil || status.OccupiedCalls != 1 || status.RemainingCalls != 0 || len(status.Operations) != 1 {
		t.Fatal(status, e)
	}
	// Separate operations compete for the final stage slot.
	plan.PlanID = newID()
	plan.Attempts[0].OperationID = newID()
	second := plan.Attempts[0]
	second.OperationID = newID()
	plan.Attempts = append(plan.Attempts, second)
	status, e = s.GenerationPropose(ctx, p, plan)
	if e != nil {
		t.Fatal(e)
	}
	key = GenerationPlanKey{plan.ProjectID, plan.PlanID}
	if _, e = s.GenerationApprove(ctx, p, GenerationDecision{key, status.RequestSHA256}); e != nil {
		t.Fatal(e)
	}
	acquired.Store(0)
	var rejected atomic.Int32
	for _, a := range plan.Attempts {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			r, e := s.GenerationReserve(ctx, p, GenerationOperationKey{key, id})
			if errors.Is(e, ErrBudget) {
				rejected.Add(1)
			} else if e != nil {
				t.Error(e)
			}
			if r.Acquired {
				acquired.Add(1)
			}
		}(a.OperationID)
	}
	wg.Wait()
	if acquired.Load() != 1 || rejected.Load() != 1 {
		t.Fatal("budget race", acquired.Load(), rejected.Load())
	}
	status, e = s.GenerationStatus(ctx, p, key)
	if e != nil {
		t.Fatal(e)
	}
	winner := GenerationOperationKey{key, status.Operations[0].OperationID}
	if _, e = s.GenerationTransition(ctx, p, GenerationTransition{GenerationOperationKey: winner, State: "released"}); e != nil {
		t.Fatal(e)
	}
	if r, e = s.GenerationReserve(ctx, p, winner); e != nil || r.Acquired || r.Operation.State != "released" {
		t.Fatal("released operation resubmitted", r, e)
	}
	outsider := p
	outsider.SubjectID = newID()
	if _, e = s.GenerationStatus(ctx, outsider, key); !errors.Is(e, ErrForbidden) {
		t.Fatal("cross-user disclosure", e)
	}
}
