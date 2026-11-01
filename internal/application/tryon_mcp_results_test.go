package application

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/verdantflarehub/verdantflare-studio/internal/dashboard"
	"github.com/verdantflarehub/verdantflare-studio/internal/project"
)

func tryonFilterFixture(t *testing.T) (project.Principal, *imageResultGateway, ImageResultReader, map[string]any) {
	f, _, g, reader := imageResultFixture(t)
	_, actor, result := f.service.begin(context.Background(), "test-session")
	p := project.Principal{SubjectID: actor.UserID, OrganizationID: actor.OrganizationID, RequestID: result.RequestID}
	task := map[string]any{"task_id": uuid.Must(uuid.NewV7()).String(), "project_id": g.in.ProjectID,
		"state": "succeeded", "input": map[string]any{"count": 1}, "images": []any{map[string]any{
			"index": 0, "attempt": 1, "state": "succeeded", "result": map[string]any{
				"content_ref": g.input.Ref, "file_id": g.in.ResultFileID, "revision_id": g.in.RevisionID,
				"provenance_file_id": g.in.ProvenanceFileID, "image_artifact_id": "art-0123456789abcdef",
				"url": "https://must-not-forward.invalid"}, "content_rating": "must-not-forward"}}}
	return p, g, reader, task
}

func upstreamTryon(payload any) any {
	return map[string]any{"isError": false, "structuredContent": payload,
		"content": []any{map[string]any{"type": "text", "text": "stale hidden references"},
			map[string]any{"type": "resource_link", "uri": "https://must-not-forward.invalid"}}}
}

func checkedTryonEnvelope(t *testing.T, out any, failed bool) map[string]any {
	t.Helper()
	b, _ := json.Marshal(out)
	if strings.Contains(string(b), "stale hidden") || strings.Contains(string(b), "must-not-forward") || strings.Contains(string(b), "content_rating") || strings.Contains(string(b), "art-0123456789abcdef") {
		t.Fatal("unfiltered payload leaked", string(b))
	}
	var envelope struct {
		IsError bool           `json:"isError"`
		Payload map[string]any `json:"structuredContent"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if json.Unmarshal(b, &envelope) != nil || envelope.IsError != failed || len(envelope.Content) != 1 || envelope.Content[0].Type != "text" {
		t.Fatal("invalid envelope", string(b))
	}
	var text map[string]any
	if json.Unmarshal([]byte(envelope.Content[0].Text), &text) != nil {
		t.Fatal("invalid fallback text")
	}
	a, _ := json.Marshal(envelope.Payload)
	c, _ := json.Marshal(text)
	if string(a) != string(c) {
		t.Fatal("text and structured content diverged")
	}
	return envelope.Payload
}

func TestTryonMCPResultRebuildsBothRepresentations(t *testing.T) {
	p, g, reader, task := tryonFilterFixture(t)
	out := FilterTryonMCPResults(context.Background(), p, "tryon.get", map[string]any{"task_id": task["task_id"]}, upstreamTryon(task), g, reader)
	payload := checkedTryonEnvelope(t, out, false)
	if payload["state"] != "succeeded" || len(payload["images"].([]any)) != 1 {
		t.Fatal("visible result lost")
	}
	result := payload["images"].([]any)[0].(map[string]any)["result"].(map[string]any)
	if result["file_id"] != g.in.ResultFileID || result["provenance_file_id"] != g.in.ProvenanceFileID {
		t.Fatal("fixed source changed")
	}
}

func TestTryonMCPHiddenGetAndEmptyPagePagination(t *testing.T) {
	p, g, _, task := tryonFilterFixture(t)
	hidden := func(context.Context, project.Principal, string, string) (ImageResultStatus, int, error) {
		return ImageResultStatus{}, 404, nil
	}
	out := FilterTryonMCPResults(context.Background(), p, "tryon.get", map[string]any{}, upstreamTryon(task), g, hidden)
	if checkedTryonEnvelope(t, out, true)["code"] != "NOT_FOUND" {
		t.Fatal("hidden query is distinguishable")
	}
	out = FilterTryonMCPResults(context.Background(), p, "tryon.list", map[string]any{"limit": 1}, upstreamTryon(map[string]any{"items": []any{task}}), g, hidden)
	page := checkedTryonEnvelope(t, out, false)
	if len(page["items"].([]any)) != 0 || page["has_more"] != true {
		t.Fatal("filtered page cannot continue")
	}
	out = FilterTryonMCPResults(context.Background(), p, "tryon.list", map[string]any{"limit": 1}, upstreamTryon(map[string]any{"items": []any{}}), g, hidden)
	if checkedTryonEnvelope(t, out, false)["has_more"] != false {
		t.Fatal("end of source page not detected")
	}
}

func TestTryonMCPAlwaysExcludesNSFWEvenWithUIContext(t *testing.T) {
	p, g, valid, task := tryonFilterFixture(t)
	reader := func(ctx context.Context, p project.Principal, projectID, artifactID string) (ImageResultStatus, int, error) {
		value, status, err := valid(ctx, p, projectID, artifactID)
		value.Rating.Rating = "nsfw"
		return value, status, err
	}
	out := FilterTryonMCPResults(dashboard.WithIncludeNSFW(context.Background(), true), p, "tryon.get", map[string]any{}, upstreamTryon(task), g, reader)
	if checkedTryonEnvelope(t, out, true)["code"] != "NOT_FOUND" {
		t.Fatal("UI preference reached MCP projection")
	}
}

func TestTryonMCPAcceptedMutationKeepsRecoveryWithoutResults(t *testing.T) {
	p, g, _, task := tryonFilterFixture(t)
	for _, name := range []string{"tryon.create", "tryon.retry"} {
		for _, unavailable := range []bool{false, true} {
			reader := func(context.Context, project.Principal, string, string) (ImageResultStatus, int, error) {
				if unavailable {
					return ImageResultStatus{}, 503, errors.New("offline")
				}
				return ImageResultStatus{}, 404, nil
			}
			out := FilterTryonMCPResults(context.Background(), p, name, map[string]any{}, upstreamTryon(task), g, reader)
			payload := checkedTryonEnvelope(t, out, unavailable)
			if payload["task_id"] != task["task_id"] || payload["images"] != nil {
				t.Fatal("mutation recovery changed")
			}
			if unavailable && (payload["code"] != "RESULT_STATUS_UNAVAILABLE" || payload["recovery"] != "tryon.get") {
				t.Fatal("accepted operation may be replayed")
			}
			if !unavailable && payload["accepted"] != true {
				t.Fatal("accepted mutation misreported")
			}
		}
	}
}

func TestTryonMCPFailureIsAtomicAndCorruptSourceIsRejected(t *testing.T) {
	p, g, valid, task := tryonFilterFixture(t)
	calls := 0
	reader := func(ctx context.Context, p project.Principal, projectID, artifactID string) (ImageResultStatus, int, error) {
		calls++
		if calls == 2 {
			return ImageResultStatus{}, 503, errors.New("offline")
		}
		return valid(ctx, p, projectID, artifactID)
	}
	out := FilterTryonMCPResults(context.Background(), p, "tryon.list", map[string]any{}, upstreamTryon(map[string]any{"items": []any{task, task}}), g, reader)
	if checkedTryonEnvelope(t, out, true)["items"] != nil {
		t.Fatal("partial list escaped a failed check")
	}
	g.setSource(map[string]any{"kind": "tryon-result-source", "sha256": strings.Repeat("0", 64), "image_artifact_id": "art-0123456789abcdef"})
	out = FilterTryonMCPResults(context.Background(), p, "tryon.get", map[string]any{}, upstreamTryon(task), g, valid)
	if checkedTryonEnvelope(t, out, true)["code"] != "RESULT_SOURCE_INVALID" {
		t.Fatal("bad source accepted")
	}
}

func TestTryonMCPHidesOnlySavedImagesAndRecomputesState(t *testing.T) {
	p, g, _, task := tryonFilterFixture(t)
	task["images"] = append(task["images"].([]any), map[string]any{"index": 1, "attempt": 2, "state": "failed", "result": nil, "error": map[string]any{"code": "GENERATION_FAILED"}})
	hidden := func(context.Context, project.Principal, string, string) (ImageResultStatus, int, error) {
		return ImageResultStatus{}, 404, nil
	}
	out := FilterTryonMCPResults(context.Background(), p, "tryon.get", map[string]any{}, upstreamTryon(task), g, hidden)
	payload := checkedTryonEnvelope(t, out, false)
	images := payload["images"].([]any)
	if payload["state"] != "failed" || len(images) != 1 || images[0].(map[string]any)["index"] != float64(1) || images[0].(map[string]any)["attempt"] != float64(2) {
		t.Fatal("stable retry identity lost")
	}
}

func TestTryonMCPFiltersEachSavedResultAndReadsAgainAfterClear(t *testing.T) {
	p, g, valid, task := tryonFilterFixture(t)
	first := task["images"].([]any)[0].(map[string]any)
	second := map[string]any{"index": 1, "attempt": 1, "state": "succeeded", "result": first["result"]}
	task["images"] = []any{first, second}
	calls, hideFirst := 0, true
	reader := func(ctx context.Context, p project.Principal, projectID, artifactID string) (ImageResultStatus, int, error) {
		calls++
		if calls == 1 && hideFirst {
			return ImageResultStatus{}, 404, nil
		}
		return valid(ctx, p, projectID, artifactID)
	}
	filtered := FilterTryonMCPResults(context.Background(), p, "tryon.get", map[string]any{}, upstreamTryon(task), g, reader)
	images := checkedTryonEnvelope(t, filtered, false)["images"].([]any)
	if len(images) != 1 || images[0].(map[string]any)["index"] != float64(1) {
		t.Fatal("saved results were not individually filtered")
	}
	hideFirst = false
	filtered = FilterTryonMCPResults(context.Background(), p, "tryon.get", map[string]any{}, upstreamTryon(task), g, reader)
	if len(checkedTryonEnvelope(t, filtered, false)["images"].([]any)) != 2 || calls != 4 {
		t.Fatal("old visibility was cached")
	}
}
