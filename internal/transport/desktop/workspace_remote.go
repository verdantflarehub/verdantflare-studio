package desktop

import (
	"context"
	"encoding/json"
	"io"
	"net/url"

	"github.com/google/uuid"
	"github.com/verdantflarehub/verdantflare-studio/internal/mcp"
	"github.com/verdantflarehub/verdantflare-studio/internal/project"
)

// gatewayRemote adapts the verified Studio Gateway to the local workspace
// contract. It obtains a fresh Core principal for every operation, including
// metadata and binary reads, so a revoked session cannot keep an old copy
// reading remote content.
type gatewayRemote struct{ owner *Service }

func (r *gatewayRemote) principal(ctx context.Context) (project.Principal, *mcp.Gateway, error) {
	r.owner.mu.Lock()
	token, gateway := r.owner.token, r.owner.mcpGateway
	r.owner.mu.Unlock()
	if token == "" {
		return project.Principal{}, nil, project.ErrForbidden
	}
	if gateway == nil {
		return project.Principal{}, nil, project.ErrDependency
	}
	p, status := r.owner.station.Principal(ctx, token, uuid.Must(uuid.NewV7()).String())
	if status != 200 {
		if status == 401 || status == 403 {
			return project.Principal{}, nil, project.ErrForbidden
		}
		return project.Principal{}, nil, project.ErrDependency
	}
	return p, gateway, nil
}

func (r *gatewayRemote) call(ctx context.Context, name string, args map[string]any, scope string, out any) error {
	p, gateway, err := r.principal(ctx)
	if err != nil {
		return err
	}
	value, status, err := gateway.CallVerified(ctx, name, args, p, scope)
	if err != nil {
		return gatewayError(status)
	}
	b, err := json.Marshal(value)
	if err != nil {
		return project.ErrDependency
	}
	var wrapper struct {
		Structured json.RawMessage `json:"structuredContent"`
		IsError    bool            `json:"isError"`
	}
	if json.Unmarshal(b, &wrapper) == nil && len(wrapper.Structured) > 0 {
		if wrapper.IsError {
			return contentError(wrapper.Structured)
		}
		b = wrapper.Structured
	}
	if status < 200 || status >= 300 {
		return gatewayError(status)
	}
	if out == nil {
		return nil
	}
	if json.Unmarshal(b, out) != nil {
		return project.ErrDependency
	}
	return nil
}

func gatewayError(status int) error {
	switch status {
	case 400:
		return project.ErrInvalid
	case 401, 403:
		return project.ErrForbidden
	case 404:
		return project.ErrNotFound
	case 409:
		return project.ErrConflict
	default:
		return project.ErrDependency
	}
}

func contentError(data []byte) error {
	var value struct {
		Code string `json:"code"`
	}
	if json.Unmarshal(data, &value) != nil {
		return project.ErrDependency
	}
	switch value.Code {
	case project.ErrInvalid.Error():
		return project.ErrInvalid
	case project.ErrForbidden.Error():
		return project.ErrForbidden
	case project.ErrNotFound.Error():
		return project.ErrNotFound
	case project.ErrConflict.Error():
		return project.ErrConflict
	case project.ErrNotReady.Error():
		return project.ErrNotReady
	case project.ErrIdempotency.Error():
		return project.ErrIdempotency
	default:
		return project.ErrDependency
	}
}

func (r *gatewayRemote) Open(ctx context.Context, projectID, revisionID string) (project.OpenResult, error) {
	var out project.OpenResult
	err := r.call(ctx, "project.open", map[string]any{"project_id": projectID, "revision_id": revisionID}, projectID, &out)
	return out, err
}

func (r *gatewayRemote) Metadata(ctx context.Context, ref project.ContentRef, access project.Access) (project.ContentVersion, error) {
	var out struct {
		Mode    string                 `json:"mode"`
		Version project.ContentVersion `json:"version"`
	}
	args := map[string]any{"mode": "metadata", "content_ref": ref, "access": access}
	err := r.call(ctx, "artifact.read", args, access.ProjectID, &out)
	if err == nil && (out.Mode != "metadata" || out.Version.ContentRef != ref) {
		err = project.ErrDependency
	}
	return out.Version, err
}

func (r *gatewayRemote) Download(ctx context.Context, ref project.ContentRef, access project.Access, dst io.Writer, limit int64) error {
	if dst == nil || !ref.Valid() || limit < 0 {
		return project.ErrInvalid
	}
	p, gateway, err := r.principal(ctx)
	if err != nil {
		return err
	}
	query := url.Values{"store_id": {ref.StoreID}, "artifact_id": {ref.ArtifactID}}
	if access.ProjectID != "" {
		query.Set("project_id", access.ProjectID)
		query.Set("project_revision_id", access.RevisionID)
	}
	if access.AssetID != "" {
		query.Set("asset_id", access.AssetID)
		query.Set("asset_version_id", access.AssetVersionID)
	}
	path := "/v2/artifacts/" + ref.VersionID + "/content"
	response, status, err := gateway.Transfer(ctx, p, "GET", path, query.Encode(), nil, 0)
	if err != nil {
		return gatewayError(status)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return gatewayError(response.StatusCode)
	}
	if limit == 0 {
		limit = 1 << 50
	}
	_, err = io.Copy(dst, io.LimitReader(response.Body, limit+1))
	if err != nil {
		return err
	}
	return nil
}

func (r *gatewayRemote) Commit(ctx context.Context, req project.CommitRequest) (project.Result, error) {
	var out project.Result
	b, err := json.Marshal(req)
	if err != nil {
		return out, project.ErrInvalid
	}
	var args map[string]any
	if err = json.Unmarshal(b, &args); err != nil {
		return out, project.ErrInvalid
	}
	err = r.call(ctx, "project.commit", args, req.ProjectID, &out)
	return out, err
}
