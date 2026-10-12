package application

import (
	"bytes"
	"context"
	"encoding/json"
	"mime"
	"net/http"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/verdantflarehub/verdantflare-studio/internal/appcenter"
	"github.com/verdantflarehub/verdantflare-studio/internal/strictjson"
)

func (a *AppCenter) Prepare(ctx context.Context, token string, in appcenter.CommandPrepare) Result {
	id := uuid.Must(uuid.NewV7()).String()
	fail := func(status int, code string) Result { return appCenterError(status, code, id) }
	raw, err := json.Marshal(in)
	var checked appcenter.CommandPrepare
	if err != nil || appcenter.Decode(bytes.NewReader(raw), "command-prepare", 16<<10, &checked) != nil {
		return fail(400, "INVALID_APPCENTER_REQUEST")
	}
	view, _, facts, out := a.readSnapshot(ctx, token, true)
	if out.Status != 200 {
		return out
	}
	id = out.RequestID
	var target *appcenter.Card
	for i := range view.Items {
		if view.Items[i].CardKey == in.CardKey {
			target = &view.Items[i]
			break
		}
	}
	if target == nil || target.BindingRevision != in.ExpectedBindingRevision {
		return fail(409, "APPCENTER_BINDING_CHANGED")
	}
	if !slices.ContainsFunc(target.Actions, func(action appcenter.Action) bool {
		return action.ID == in.ActionID && action.Kind == in.ActionID && action.Enabled
	}) {
		return fail(409, "ACTION_UNSUPPORTED")
	}
	for _, fact := range facts.Items {
		if fact.CardKey != target.CardKey {
			continue
		}
		var candidate *appcenter.Candidate
		for i := range fact.Lifecycle.Candidates {
			current := &fact.Lifecycle.Candidates[i]
			if current.Kind != in.ActionID {
				continue
			}
			if candidate != nil {
				return fail(502, "APPCENTER_INVALID_FACTS")
			}
			candidate = current
		}
		if candidate == nil || !candidate.Enabled {
			return fail(409, "ACTION_UNSUPPORTED")
		}
		prepared := appcenter.CommandCore{SchemaVersion: 1,
			Command: appcenter.Command{RequestID: in.RequestID, IdempotencyKey: in.IdempotencyKey,
				OrganizationID: view.Scope.OrganizationID, StationID: view.Scope.StationID,
				AppID: fact.AppID, AppVersion: candidate.AppVersion, Action: candidate.Kind},
			Expected: appcenter.CommandExpected{AppVersion: candidate.AppVersion, ReleaseDigest: candidate.ReleaseDigest,
				WorkloadUID: candidate.ExpectedWorkloadUID, PolicyRevision: fact.Lifecycle.PolicyRevision}}
		if fact.Installation != nil {
			prepared.Expected.InstallationID = &fact.Installation.InstallationID
			prepared.Expected.InstallationRevision = &fact.Installation.Revision
		}
		raw, err = json.Marshal(prepared)
		if err != nil || appcenter.Decode(bytes.NewReader(raw), "command-core", 16<<10, &prepared) != nil {
			return fail(502, "APPCENTER_INVALID_FACTS")
		}
		return Result{Status: 200, Data: raw, RequestID: id}
	}
	return fail(502, "APPCENTER_INVALID_FACTS")
}

// Submit must not resolve a fresh view: accepted operations remain recoverable
// after catalog, presentation or entry changes. Core owns durable replay and
// new-command preconditions; this boundary reauthenticates every call.
func (a *AppCenter) Submit(ctx context.Context, token string, in appcenter.CommandSubmit) Result {
	id := uuid.Must(uuid.NewV7()).String()
	fail := func(status int, code string) Result { return appCenterError(status, code, id) }
	raw, err := json.Marshal(in)
	var checked appcenter.CommandSubmit
	if err != nil || appcenter.Decode(bytes.NewReader(raw), "command-submit", 16<<10, &checked) != nil {
		return fail(400, "INVALID_APPCENTER_REQUEST")
	}
	if a == nil || a.station == nil {
		return fail(503, "APPCENTER_DEPENDENCY_UNAVAILABLE")
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	identity, status := a.station.verifiedIdentity(ctx, token, id)
	if status == 401 {
		return fail(401, "UNAUTHENTICATED")
	}
	if status == 403 {
		return fail(403, "PERMISSION_DENIED")
	}
	if status != 200 {
		return fail(503, "APPCENTER_DEPENDENCY_UNAVAILABLE")
	}
	command := in.Prepared.Command
	if !slices.Contains(identity.Roles, "admin") || !slices.Contains(identity.Scopes, "app:read") || !slices.Contains(identity.Scopes, "app:manage") ||
		identity.OrganizationID != command.OrganizationID || identity.StationID != command.StationID {
		return fail(403, "PERMISSION_DENIED")
	}
	raw, _ = json.Marshal(in.Prepared)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.station.base+"/appcenter/v1/commands", bytes.NewReader(raw))
	if err != nil {
		return fail(503, "APPCENTER_DEPENDENCY_UNAVAILABLE")
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Request-ID", id)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := a.station.client.Do(req)
	if err != nil {
		return fail(503, "APPCENTER_SUBMISSION_UNKNOWN")
	}
	defer resp.Body.Close()
	media, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		return fail(502, "APPCENTER_SUBMISSION_UNKNOWN")
	}
	if resp.StatusCode != 202 {
		var failure struct {
			Code      string `json:"code"`
			RequestID string `json:"requestId"`
		}
		if strictjson.Decode(resp.Body, 16<<10, &failure) != nil || failure.RequestID != id {
			return fail(502, "APPCENTER_SUBMISSION_UNKNOWN")
		}
		allowed := map[int][]string{
			400: {"INVALID_ARGUMENT", "INVALID_APPCENTER_REQUEST"},
			401: {"UNAUTHENTICATED", "SESSION_REVOKED"}, 403: {"PERMISSION_DENIED"},
			409: {"CONFLICT", "APPCENTER_BINDING_CHANGED", "RELEASE_UNVERIFIED", "ACTION_UNSUPPORTED"},
		}
		if slices.Contains(allowed[resp.StatusCode], failure.Code) {
			return fail(resp.StatusCode, failure.Code)
		}
		// Even an upstream 5xx may follow a committed operation.
		return fail(503, "APPCENTER_SUBMISSION_UNKNOWN")
	}
	var operation map[string]any
	if appcenter.Decode(resp.Body, "command-operation", 16<<10, &operation) != nil {
		return fail(502, "APPCENTER_SUBMISSION_UNKNOWN")
	}
	for key, expected := range map[string]string{"organization_id": command.OrganizationID, "station_id": command.StationID,
		"app_id": command.AppID, "app_version": command.AppVersion, "action": command.Action} {
		if operation[key] != expected {
			return fail(502, "APPCENTER_SUBMISSION_UNKNOWN")
		}
	}
	raw, _ = json.Marshal(operation)
	return Result{Status: 202, Data: raw, RequestID: id}
}
