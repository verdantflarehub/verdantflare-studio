package desktop

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"

	"github.com/google/uuid"
	"github.com/verdantflarehub/verdantflare-studio/internal/mcp"
	"github.com/verdantflarehub/verdantflare-studio/internal/project"
	"github.com/verdantflarehub/verdantflare-studio/internal/studioremote"
	"github.com/verdantflarehub/verdantflare-studio/internal/workspace"
)

// gatewayRemote adapts the verified Studio Gateway to the local workspace
// contract. It obtains a fresh Core principal for every operation, including
// metadata and binary reads, so a revoked session cannot keep an old copy
// reading remote content.
type gatewayRemote struct {
	owner   *Service
	session *studioremote.Session
}

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
	if r.session != nil {
		return r.remoteCall(ctx, name, args, out)
	}
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
		Code    string `json:"code"`
		Current string `json:"current_revision_id"`
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
		return &project.ConflictError{CurrentRevisionID: value.Current}
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
	if limit == 0 {
		limit = 1 << 50
	}
	response, status, err := r.transfer(ctx, "GET", path, query.Encode(), nil, 0, limit)
	if err != nil {
		return gatewayError(status)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return gatewayError(response.StatusCode)
	}
	n, err := io.Copy(dst, io.LimitReader(response.Body, limit+1))
	if err != nil {
		return err
	}
	if n > limit || (response.ContentLength >= 0 && n != response.ContentLength) {
		return project.ErrDependency
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

func (r *gatewayRemote) CommitStatus(ctx context.Context, projectID, commitID string) (project.CommitStatus, error) {
	var out project.CommitStatus
	err := r.call(ctx, "project.commit_status", map[string]any{"project_id": projectID, "commit_id": commitID}, projectID, &out)
	return out, err
}

// Upload implements the binary extension of the local workspace Remote. The
// Artifact prepare, controlled PUT, and commit calls all use the same project
// scope and authenticated principal; the browser never receives the upload
// path or service credential.
func (r *gatewayRemote) Upload(ctx context.Context, req workspace.UploadRequest, src io.Reader) (project.ContentVersion, error) {
	if src == nil || !project.ValidID(req.ProjectID) || !project.ValidID(req.WriteID) || req.Size < 0 || req.Size > 1<<50 || len(req.SHA256) != 64 {
		return project.ContentVersion{}, project.ErrInvalid
	}
	var prepared struct {
		Upload struct {
			UploadID    string `json:"upload_id"`
			VersionID   string `json:"version_id"`
			ArtifactID  string `json:"artifact_id"`
			State       string `json:"state"`
			ContentPath string `json:"content_path"`
		} `json:"upload"`
	}
	err := r.call(ctx, "artifact.write", map[string]any{
		"mode":     "prepare",
		"write_id": req.WriteID,
		"source":   req.Source,
		"mime":     req.MIME,
		"size":     req.Size,
		"sha256":   req.SHA256,
	}, req.ProjectID, &prepared)
	if err != nil {
		return project.ContentVersion{}, err
	}
	upload := prepared.Upload
	if !project.ValidID(upload.UploadID) || !project.ValidID(upload.VersionID) || !project.ValidID(upload.ArtifactID) || (upload.State != "prepared" && upload.State != "committed") || upload.ContentPath != "/v2/artifacts/uploads/"+upload.UploadID+"/content" {
		return project.ContentVersion{}, project.ErrDependency
	}
	if upload.State == "prepared" {
		response, status, err := r.transfer(ctx, "PUT", upload.ContentPath, "", src, req.Size, 1<<50)
		if err != nil {
			return project.ContentVersion{}, gatewayError(status)
		}
		defer response.Body.Close()
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			return project.ContentVersion{}, gatewayError(response.StatusCode)
		}
		if _, err = io.Copy(io.Discard, io.LimitReader(response.Body, 4096)); err != nil {
			return project.ContentVersion{}, project.ErrDependency
		}
	}
	var committed struct {
		Version project.ContentVersion `json:"version"`
	}
	err = r.call(ctx, "artifact.write", map[string]any{"mode": "commit", "upload_id": upload.UploadID}, req.ProjectID, &committed)
	if err != nil {
		return project.ContentVersion{}, err
	}
	v := committed.Version
	if v.SchemaVersion != 2 || !v.ContentRef.Valid() || v.ArtifactID != upload.ArtifactID || v.VersionID != upload.VersionID || v.Size != req.Size || v.SHA256 != req.SHA256 || v.MIME != req.MIME || v.Source != req.Source {
		return project.ContentVersion{}, project.ErrDependency
	}
	return v, nil
}

func remoteWorkspaceError(err error) error {
	if errors.Is(err, studioremote.ErrChanged) || errors.Is(err, studioremote.ErrDenied) {
		return project.ErrForbidden
	}
	if errors.Is(err, studioremote.ErrInvalid) {
		return project.ErrInvalid
	}
	return project.ErrDependency
}

func (r *gatewayRemote) transfer(ctx context.Context, method, path, query string, body io.Reader, length, limit int64) (*http.Response, int, error) {
	if r.session != nil {
		resp, err := r.session.Transfer(ctx, method, path, query, body, length, limit)
		if err != nil {
			if errors.Is(remoteWorkspaceError(err), project.ErrForbidden) {
				return nil, 403, err
			}
			return nil, 503, err
		}
		return resp, resp.StatusCode, nil
	}
	p, gateway, err := r.principal(ctx)
	if err != nil {
		if errors.Is(err, project.ErrForbidden) {
			return nil, 403, err
		}
		return nil, 503, err
	}
	return gateway.Transfer(ctx, p, method, path, query, body, length)
}

func (r *gatewayRemote) remoteCall(ctx context.Context, name string, args map[string]any, out any) error {
	id := uuid.Must(uuid.NewV7()).String()
	body, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "method": "tools/call", "params": map[string]any{"name": name, "arguments": args}})
	if err != nil {
		return project.ErrInvalid
	}
	response, err := r.session.Send(ctx, "POST", "/mcp", body)
	if err != nil {
		return remoteWorkspaceError(err)
	}
	if response.Status < 200 || response.Status >= 300 {
		return gatewayError(response.Status)
	}
	var envelope struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      string          `json:"id"`
		Error   json.RawMessage `json:"error"`
		Result  struct {
			IsError    bool            `json:"isError"`
			Structured json.RawMessage `json:"structuredContent"`
		} `json:"result"`
	}
	if json.Unmarshal(response.Data, &envelope) != nil || envelope.JSONRPC != "2.0" || envelope.ID != id || len(envelope.Error) != 0 || len(envelope.Result.Structured) == 0 || string(envelope.Result.Structured) == "null" {
		return project.ErrDependency
	}
	if envelope.Result.IsError {
		return contentError(envelope.Result.Structured)
	}
	if out != nil && json.Unmarshal(envelope.Result.Structured, out) != nil {
		return project.ErrDependency
	}
	return nil
}
