package application

import (
	"context"
	"encoding/json"
	"math"
	"time"

	"github.com/verdantflarehub/verdantflare-studio/internal/project"
)

func TryonResultTool(name string) bool {
	return name == "tryon.get" || name == "tryon.list" || name == "tryon.create" || name == "tryon.retry"
}

func tryonEnvelope(payload any, failed bool) any {
	text, _ := json.Marshal(payload)
	return map[string]any{"isError": failed, "structuredContent": payload,
		"content": []any{map[string]any{"type": "text", "text": string(text)}}}
}

// FilterTryonMCPResults is the public MCP projection. Its caller must use a
// default-only Image reader, never the UI's local include preference.
func FilterTryonMCPResults(ctx context.Context, p project.Principal, name string, args map[string]any, upstream any, gateway DashboardContentGateway, read ImageResultReader) any {
	if !TryonResultTool(name) {
		return upstream
	}
	if !p.Valid() {
		return tryonEnvelope(map[string]any{"code": "ACCESS_DENIED"}, true)
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	data, err := json.Marshal(upstream)
	var envelope struct {
		IsError bool           `json:"isError"`
		Payload map[string]any `json:"structuredContent"`
	}
	if err != nil || len(data) > 8<<20 || json.Unmarshal(data, &envelope) != nil || envelope.Payload == nil {
		return tryonEnvelope(map[string]any{"code": "RESULT_STATUS_UNAVAILABLE"}, true)
	}
	mutation := name == "tryon.create" || name == "tryon.retry"
	failure := func(code string) any {
		payload := map[string]any{"code": code}
		if id, ok := envelope.Payload["task_id"].(string); mutation && ok && project.ValidID(id) {
			payload["task_id"], payload["recovery"] = id, "tryon.get"
		}
		return tryonEnvelope(payload, true)
	}
	if envelope.IsError {
		if code, ok := envelope.Payload["code"].(string); ok && len(code) > 0 && len(code) <= 128 {
			return failure(code)
		}
		return failure("RESULT_STATUS_UNAVAILABLE")
	}
	filter := func(raw any) (map[string]any, bool, string) {
		task, ok := raw.(map[string]any)
		if !ok {
			return nil, false, "RESULT_STATUS_UNAVAILABLE"
		}
		id, _ := task["task_id"].(string)
		projectID, _ := task["project_id"].(string)
		images, ok := task["images"].([]any)
		if !project.ValidID(id) || !project.ValidID(projectID) || !ok || len(images) == 0 || len(images) > 4 {
			return nil, false, "RESULT_STATUS_UNAVAILABLE"
		}
		if expected, ok := args["task_id"].(string); ok && id != expected {
			return nil, false, "RESULT_SOURCE_INVALID"
		}
		if expected, ok := args["project_id"].(string); ok && projectID != expected {
			return nil, false, "RESULT_SOURCE_INVALID"
		}
		out := map[string]any{}
		for _, key := range []string{"task_id", "project_id", "created_at", "upload_deleted", "worker_error", "input", "upload_access", "upload_cleanup"} {
			if value, ok := task[key]; ok {
				out[key] = value
			}
		}
		visible := []any{}
		states := []string{}
		attention := false
		for _, rawImage := range images {
			image, ok := rawImage.(map[string]any)
			if !ok {
				return nil, false, "RESULT_STATUS_UNAVAILABLE"
			}
			index, indexOK := image["index"].(float64)
			attempt, attemptOK := image["attempt"].(float64)
			if !indexOK || !attemptOK || index < 0 || index >= 4 || math.Trunc(index) != index || attempt < 0 || attempt > 9007199254740991 || math.Trunc(attempt) != attempt {
				return nil, false, "RESULT_STATUS_UNAVAILABLE"
			}
			state, _ := image["state"].(string)
			item := map[string]any{"index": image["index"], "attempt": image["attempt"], "state": state, "error": image["error"], "result": nil}
			if state == "succeeded" {
				result, ok := image["result"].(map[string]any)
				if !ok {
					return nil, false, "RESULT_SOURCE_INVALID"
				}
				in := DashboardImageResultInput{ProjectID: projectID}
				in.RevisionID, _ = result["revision_id"].(string)
				in.ResultFileID, _ = result["file_id"].(string)
				in.ProvenanceFileID, _ = result["provenance_file_id"].(string)
				checked := resolveSavedImageResult(ctx, p, in, gateway, read, false, func(input DashboardContentInput) Result {
					return readResultContent(ctx, p, input, gateway)
				})
				if checked.Status != 200 {
					code := "RESULT_STATUS_UNAVAILABLE"
					if checked.Status == 422 {
						code = "RESULT_SOURCE_INVALID"
					}
					if checked.Status == 401 || checked.Status == 403 {
						code = "ACCESS_DENIED"
					}
					return nil, false, code
				}
				var current struct {
					Status  string             `json:"status"`
					Content project.ContentRef `json:"content_ref"`
				}
				if json.Unmarshal(checked.Data, &current) != nil {
					return nil, false, "RESULT_STATUS_UNAVAILABLE"
				}
				if current.Status == "unavailable" {
					continue
				}
				refBytes, _ := json.Marshal(result["content_ref"])
				var saved project.ContentRef
				if current.Status != "visible" || json.Unmarshal(refBytes, &saved) != nil || saved != current.Content {
					return nil, false, "RESULT_SOURCE_INVALID"
				}
				item["result"] = map[string]any{"project_id": projectID, "revision_id": in.RevisionID,
					"file_id": in.ResultFileID, "provenance_file_id": in.ProvenanceFileID, "content_ref": current.Content}
			} else {
				switch state {
				case "queued", "submitting", "generating", "generated", "faceswapping", "saving", "failed", "attention":
				default:
					return nil, false, "RESULT_STATUS_UNAVAILABLE"
				}
			}
			visible = append(visible, item)
			states = append(states, state)
			attention = attention || state == "attention"
		}
		if len(visible) == 0 {
			return out, false, ""
		}
		allSucceeded, allFinished, allQueued, someSucceeded := true, true, true, false
		for _, state := range states {
			allSucceeded = allSucceeded && state == "succeeded"
			allFinished = allFinished && (state == "succeeded" || state == "failed")
			allQueued = allQueued && state == "queued"
			someSucceeded = someSucceeded || state == "succeeded"
		}
		state := "running"
		if allQueued {
			state = "queued"
		}
		if allFinished {
			state = "failed"
			if someSucceeded {
				state = "partial"
			}
		}
		if allSucceeded {
			state = "succeeded"
		}
		out["images"], out["state"], out["needs_attention"] = visible, state, attention
		return out, true, ""
	}
	if name == "tryon.list" {
		rows, ok := envelope.Payload["items"].([]any)
		if !ok || len(rows) > 100 {
			return failure("RESULT_STATUS_UNAVAILABLE")
		}
		items := []any{}
		for _, raw := range rows {
			row, visible, code := filter(raw)
			if code != "" {
				return failure(code)
			}
			if visible {
				items = append(items, row)
			}
		}
		limit := 50
		if value, ok := args["limit"]; ok {
			b, _ := json.Marshal(value)
			_ = json.Unmarshal(b, &limit)
		}
		return tryonEnvelope(map[string]any{"items": items, "has_more": len(rows) == limit}, false)
	}
	row, visible, code := filter(envelope.Payload)
	if code != "" {
		return failure(code)
	}
	if !visible {
		if mutation {
			return tryonEnvelope(map[string]any{"task_id": row["task_id"], "project_id": row["project_id"], "accepted": true}, false)
		}
		return failure("NOT_FOUND")
	}
	return tryonEnvelope(row, false)
}
