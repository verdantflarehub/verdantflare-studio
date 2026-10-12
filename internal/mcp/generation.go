package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/verdantflarehub/verdantflare-studio/internal/project"
	"github.com/verdantflarehub/verdantflare-studio/internal/strictjson"
)

func generationControlEnabled() bool { return os.Getenv("STUDIO_GENERATION_CONTROL_ENABLED") == "true" }

func generationMayConsume(name string) bool {
	domain := strings.SplitN(name, ".", 2)[0]
	switch domain {
	case "image", "video", "music", "sound", "audio", "render", "blender":
	default:
		return false
	}
	// New/unknown actions fail closed. Only audited non-generating reads bypass.
	switch name {
	case "image.capabilities", "image.status", "image.result", "image.list", "video.capabilities", "video.status", "video.result", "video.list", "music.capabilities", "music.status", "music.result", "music.list":
		return false
	}
	return true
}

// CheckGenerationDirect is shared by MCP and the legacy Image dashboard proxy.
// It checks the actual project in the body; dropping the header is not a bypass.
func (g *Gateway) CheckGenerationDirect(ctx context.Context, p project.Principal, args map[string]any, header string) (int, error) {
	if !generationControlEnabled() {
		return 200, nil
	}
	id, ok := args["project_id"].(string)
	if !ok || !project.ValidID(id) || (header != "" && header != id) {
		return 400, errors.New("explicit matching project scope required for generation")
	}
	data, status, e := g.generationControl(ctx, p, "policy", project.GenerationPolicyRequest{ProjectID: id})
	if e != nil {
		return status, e
	}
	if status != 200 {
		return status, errors.New("generation project access denied")
	}
	var policy project.GenerationPolicy
	if strictjson.Decode(bytes.NewReader(data), 4096, &policy) != nil {
		return 503, errors.New("generation policy unavailable")
	}
	if policy.Controlled {
		return 403, errors.New("GENERATION_APPROVAL_REQUIRED: use project.generation_submit")
	}
	return 200, nil
}

func generationResult(v any) any { return map[string]any{"isError": false, "structuredContent": v} }

func boundedImageArguments(a project.GenerationAttempt) (map[string]any, error) {
	var values map[string]string
	if json.Unmarshal(a.Arguments, &values) != nil {
		return nil, errors.New("unsupported bounded generation arguments")
	}
	if values["engine"] != "grok" || !versionPattern.MatchString(a.ServiceVersion) {
		return nil, errors.New("generation adapter not bounded")
	}
	// Explicitly audited implementations only; a registry update must not grant
	// an unreviewed adapter the same consumption guarantee.
	if v := strings.TrimPrefix(a.ServiceVersion, "v"); v != "0.2.3" && v != "0.2.4" {
		return nil, errors.New("generation adapter version not audited")
	}
	switch values["model"] {
	case "grok-imagine-image", "grok-imagine-image-2.0", "grok-imagine-image-quality":
	default:
		return nil, errors.New("explicit bounded model required")
	}
	if strings.TrimSpace(values["prompt"]) == "" || len(values["prompt"]) > 32768 || (values["resolution"] != "1k" && values["resolution"] != "2k") {
		return nil, errors.New("explicit generation parameters required")
	}
	allowed := map[string]bool{"engine": true, "model": true, "prompt": true, "resolution": true}
	switch a.Tool {
	case "image.create":
		allowed["aspect_ratio"] = true
		switch values["aspect_ratio"] {
		case "16:9", "9:16", "1:1", "4:3", "3:4":
		default:
			return nil, errors.New("explicit aspect ratio required")
		}
	case "image.edit":
		allowed["source_artifact_id"] = true
		if strings.TrimSpace(values["source_artifact_id"]) == "" {
			return nil, errors.New("fixed source required")
		}
	default:
		return nil, errors.New("generation adapter not bounded")
	}
	out := map[string]any{}
	for k, v := range values {
		if !allowed[k] {
			return nil, errors.New("unapproved generation parameter")
		}
		out[k] = v
	}
	return out, nil
}

func (g *Gateway) generationSubmit(ctx context.Context, p project.Principal, args map[string]any, header string) (any, int, error) {
	if !generationControlEnabled() {
		return nil, 503, errors.New("generation execution control not enabled")
	}
	data, e := json.Marshal(args)
	if e != nil {
		return nil, 400, errors.New("invalid generation operation")
	}
	var key project.GenerationOperationKey
	if strictjson.Decode(bytes.NewReader(data), 4096, &key) != nil || !project.ValidID(key.ProjectID) || !project.ValidID(key.PlanID) || !project.ValidID(key.OperationID) || (header != "" && header != key.ProjectID) {
		return nil, 400, errors.New("invalid generation scope")
	}
	data, status, e := g.generationControl(ctx, p, "status", key.GenerationPlanKey)
	if e != nil {
		return nil, status, e
	}
	if status != 200 {
		return nil, status, errors.New("generation plan unavailable")
	}
	var plan project.GenerationPlanStatus
	if json.Unmarshal(data, &plan) != nil || plan.Plan.ProjectID != key.ProjectID || plan.Plan.PlanID != key.PlanID {
		return nil, 502, errors.New("generation plan identity mismatch")
	}
	// Read-only replay is available after expiry, revocation or adapter upgrade.
	for _, op := range plan.Operations {
		if op.OperationID == key.OperationID {
			return generationResult(op), 200, nil
		}
	}
	var attempt project.GenerationAttempt
	for _, a := range plan.Plan.Attempts {
		if a.OperationID == key.OperationID {
			attempt = a
			break
		}
	}
	if attempt.OperationID == "" {
		return nil, 403, errors.New("operation outside approved scope")
	}
	actual, e := boundedImageArguments(attempt)
	if e != nil {
		return nil, 409, e
	}
	g.mu.RLock()
	svc, exists := g.services["image"]
	g.mu.RUnlock()
	if !exists || svc.Version != attempt.ServiceVersion {
		return nil, 409, errors.New("approved service version unavailable")
	}
	if len(g.resolveToken("image")) < 32 {
		return nil, 503, errors.New("image execution credential unavailable")
	}
	data, status, e = g.generationControl(ctx, p, "reserve", key)
	if e != nil {
		return nil, status, e
	}
	if status != 200 {
		return nil, status, errors.New("generation approval or allowance unavailable")
	}
	var reservation project.GenerationReservation
	if json.Unmarshal(data, &reservation) != nil || reservation.Operation.OperationID != key.OperationID {
		return nil, 502, errors.New("invalid generation reservation")
	}
	if !reservation.Acquired {
		return generationResult(reservation.Operation), 200, nil
	}
	before, _ := json.Marshal(attempt)
	after, _ := json.Marshal(reservation.Attempt)
	if !bytes.Equal(before, after) {
		return nil, 502, errors.New("reserved request mismatch")
	}
	// Persist uncertainty before any network side effect. A failed write or a
	// failed response recording is never followed by a supplier retry.
	_, status, e = g.generationControl(ctx, p, "transition", project.GenerationTransition{GenerationOperationKey: key, State: "unknown"})
	if e != nil {
		return nil, status, e
	}
	if status != 200 {
		return nil, status, errors.New("generation dispatch not authorized")
	}
	actual["project_id"] = key.ProjectID
	actual["idempotency_key"] = key.OperationID
	result, status, e := g.callVersion(ctx, attempt.Tool, actual, map[string]string{"X-User-Id": p.SubjectID, "X-Organization-Id": p.OrganizationID, "X-Request-Id": p.RequestID, "X-Project-Id": key.ProjectID}, attempt.ServiceVersion)
	if e != nil || status < 200 || status >= 300 {
		return generationResult(project.GenerationOperation{OperationID: key.OperationID, State: "unknown", RequestSHA256: reservation.Operation.RequestSHA256}), 202, nil
	}
	encoded, e := json.Marshal(result)
	if e != nil {
		return nil, 502, errors.New("generation response unavailable; read original operation")
	}
	var response struct {
		IsError           bool `json:"isError"`
		StructuredContent struct {
			TaskID string `json:"task_id"`
		} `json:"structuredContent"`
	}
	if json.Unmarshal(encoded, &response) != nil || response.IsError || response.StructuredContent.TaskID == "" {
		return generationResult(project.GenerationOperation{OperationID: key.OperationID, State: "unknown", RequestSHA256: reservation.Operation.RequestSHA256}), 202, nil
	}
	data, status, e = g.generationControl(ctx, p, "transition", project.GenerationTransition{GenerationOperationKey: key, State: "submitted", Response: encoded})
	if e != nil || status != 200 {
		return nil, 502, errors.New("generation submitted; outcome recording uncertain; read original operation")
	}
	var op project.GenerationOperation
	if json.Unmarshal(data, &op) != nil {
		return nil, 502, errors.New("generation outcome unavailable")
	}
	return generationResult(op), 200, nil
}

// GenerationApprove is called only by the dedicated fresh-authentication UI
// route. It is not exposed via tools/list, tools/call or generic service relay.
func (g *Gateway) GenerationApprove(ctx context.Context, p project.Principal, decision project.GenerationDecision) (json.RawMessage, int, error) {
	return g.generationControl(ctx, p, "approve", decision)
}

func (g *Gateway) generationControl(ctx context.Context, p project.Principal, action string, input any) (json.RawMessage, int, error) {
	if !p.Valid() {
		return nil, 403, errors.New("verified identity required")
	}
	if action != "approve" && action != "reserve" && action != "transition" && action != "policy" && action != "status" {
		return nil, 400, errors.New("invalid generation operation")
	}
	token := g.resolveToken("project")
	if action == "approve" {
		token = os.Getenv("STUDIO_GENERATION_APPROVAL_TOKEN")
		if token == g.resolveToken("project") {
			return nil, 503, errors.New("approval credential must be distinct")
		}
	}
	if len(token) < 32 || strings.ContainsAny(token, " \t\r\n") {
		return nil, 503, errors.New("generation control unavailable")
	}
	base, ok := g.BusinessEndpoint("project")
	if !ok {
		return nil, 503, errors.New("project service unavailable")
	}
	body, e := json.Marshal(input)
	if e != nil || len(body) > 1<<20 {
		return nil, 400, errors.New("invalid generation request")
	}
	req, e := http.NewRequestWithContext(ctx, http.MethodPost, base+"/internal/v1/generation/"+action, bytes.NewReader(body))
	if e != nil {
		return nil, 503, errors.New("generation control unavailable")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Organization-Id", p.OrganizationID)
	req.Header.Set("X-User-Id", p.SubjectID)
	req.Header.Set("X-Request-Id", p.RequestID)
	resp, e := g.httpClient.Do(req)
	if e != nil {
		return nil, 502, errors.New("generation control unavailable")
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		return nil, 502, errors.New("generation redirect rejected")
	}
	data, e := io.ReadAll(io.LimitReader(resp.Body, 2<<20+1))
	if e != nil || len(data) > 2<<20 || !json.Valid(data) {
		return nil, 502, errors.New("invalid generation response")
	}
	if resp.StatusCode >= 500 {
		return nil, 503, errors.New("generation control unavailable")
	}
	if resp.Header.Get("X-Request-Id") != p.RequestID {
		return nil, 502, errors.New("generation response identity mismatch")
	}
	return data, resp.StatusCode, nil
}
