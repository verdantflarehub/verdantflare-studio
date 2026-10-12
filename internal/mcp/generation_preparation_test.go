package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/verdantflarehub/verdantflare-studio/internal/project"
)

func TestGenerationPreparationPreservesProjectAuthorization(t *testing.T) {
	t.Setenv("STUDIO_GENERATION_CONTROL_ENABLED", "true")
	t.Setenv("STUDIO_PROJECT_SERVICE_TOKEN", strings.Repeat("p", 32))
	t.Setenv("STUDIO_VIDEO_TOKEN", strings.Repeat("v", 32))
	id := func() string { return uuid.Must(uuid.NewV7()).String() }
	p := project.Principal{OrganizationID: id(), SubjectID: id(), RequestID: id()}
	pid := id()
	var policyStatus atomic.Int32
	policyStatus.Store(200)
	var downstreamCalls atomic.Int32
	policy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/v1/generation/policy" || r.Header.Get("Authorization") != "Bearer "+strings.Repeat("p", 32) || r.Header.Get("X-User-Id") != p.SubjectID || r.Header.Get("X-Organization-Id") != p.OrganizationID {
			t.Error("project authorization identity lost")
		}
		var in project.GenerationPolicyRequest
		if json.NewDecoder(r.Body).Decode(&in) != nil || in.ProjectID != pid {
			t.Error("project authorization scope lost")
		}
		w.Header().Set("X-Request-Id", p.RequestID)
		w.WriteHeader(int(policyStatus.Load()))
		_ = json.NewEncoder(w).Encode(project.GenerationPolicy{Controlled: true})
	}))
	defer policy.Close()
	video := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		downstreamCalls.Add(1)
		if r.Header.Get("Authorization") != "Bearer "+strings.Repeat("v", 32) || r.Header.Get("X-Project-Id") != pid || r.Header.Get("X-User-Id") != p.SubjectID || r.Header.Get("X-Organization-Id") != p.OrganizationID {
			t.Error("video identity or project scope lost")
		}
		var in struct {
			Params struct {
				Name string         `json:"name"`
				Args map[string]any `json:"arguments"`
			} `json:"params"`
		}
		if json.NewDecoder(r.Body).Decode(&in) != nil || !generationPreparation(in.Params.Name) || in.Params.Args["project_id"] != pid || in.Params.Args["sentinel"] != "unchanged" {
			t.Error("unexpected downstream operation or rewritten arguments")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": p.RequestID, "result": map[string]any{"isError": false}})
	}))
	defer video.Close()
	g := NewGatewayWithClient(nil)
	g.services["project"] = ServiceRegistration{Domain: "project", Endpoint: policy.URL + "/mcp"}
	names := []string{"video.import", "video.import_prepare", "video.import_chunk", "video.import_status", "video.import_commit", "video.preflight"}
	definitions := []ToolDefinition{}
	for _, name := range append(append([]string{}, names...), "video.create", "video.generate", "video.import_execute") {
		definitions = append(definitions, ToolDefinition{Name: name})
	}
	g.services["video"] = ServiceRegistration{Domain: "video", Endpoint: video.URL + "/mcp", Tools: definitions}
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			args := map[string]any{"project_id": pid, "sentinel": "unchanged"}
			before := downstreamCalls.Load()
			for _, header := range []string{"", pid} {
				if _, status, err := g.CallVerified(context.Background(), name, args, p, header); err != nil || status != 200 {
					t.Fatalf("authorized preparation rejected: %d %v", status, err)
				}
			}
			if downstreamCalls.Load() != before+2 {
				t.Fatal("preparation not forwarded exactly once per request")
			}
			before = downstreamCalls.Load()
			if _, status, _ := g.CallTool(context.Background(), name, args, nil); status != 403 {
				t.Fatal("unverified relay accepted preparation")
			}
			if _, status, _ := g.CallVerified(context.Background(), name, args, project.Principal{}, ""); status != 403 {
				t.Fatal("invalid identity accepted")
			}
			for _, badArgs := range []map[string]any{{}, {"project_id": "prj_studio_default"}} {
				if _, status, _ := g.CallVerified(context.Background(), name, badArgs, p, ""); status != 400 {
					t.Fatal("invalid project scope accepted")
				}
			}
			if _, status, _ := g.CallVerified(context.Background(), name, args, p, id()); status != 400 {
				t.Fatal("conflicting project header accepted")
			}
			for _, denied := range []int32{403, 503} {
				policyStatus.Store(denied)
				if _, status, _ := g.CallVerified(context.Background(), name, args, p, ""); status != int(denied) {
					t.Fatalf("authorization failure not preserved: %d", status)
				}
			}
			policyStatus.Store(200)
			if downstreamCalls.Load() != before {
				t.Fatal("rejected preparation reached downstream")
			}
		})
	}
	before := downstreamCalls.Load()
	for _, name := range []string{"video.create", "video.generate", "video.import_execute"} {
		if _, status, err := g.CallVerified(context.Background(), name, map[string]any{"project_id": pid}, p, ""); status != 403 || err == nil || !strings.Contains(err.Error(), "GENERATION_APPROVAL_REQUIRED") {
			t.Fatalf("generation or unknown operation escaped approval: %s %d %v", name, status, err)
		}
	}
	if downstreamCalls.Load() != before {
		t.Fatal("generation reached downstream without approval")
	}
}
