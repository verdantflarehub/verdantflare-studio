// Package workspacehttp connects local working copies to the public Studio
// Gateway. It uses only the caller's Core session, never service credentials.
package workspacehttp

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/verdantflarehub/verdantflare-studio/internal/project"
	"github.com/verdantflarehub/verdantflare-studio/internal/workspace"
)

const responseLimit = 16 << 20

type Remote struct {
	base   string
	token  string
	client *http.Client
}

func New(endpoint, token string) (*Remote, error) {
	u, err := url.Parse(endpoint)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.RawPath != "" || (u.Path != "" && u.Path != "/" && u.Path != "/mcp") || strings.TrimSpace(token) == "" || strings.ContainsAny(token, "\r\n") {
		return nil, project.ErrInvalid
	}
	ip := net.ParseIP(u.Hostname())
	loopback := u.Hostname() == "localhost" || (ip != nil && ip.IsLoopback())
	if u.Scheme != "https" && !(u.Scheme == "http" && loopback) {
		return nil, project.ErrInvalid
	}
	u.Path = ""
	return &Remote{base: u.String(), token: token, client: &http.Client{
		Timeout:       30 * time.Minute,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}, nil
}

func statusError(status int) error {
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

func (r *Remote) request(ctx context.Context, method, path, contentType string, body io.Reader, size int64) (*http.Response, error) {
	// net/http owns and closes Request.Body. The workspace owns its source
	// file, including its final close/error check and retry journal.
	var requestBody io.Reader
	if body != nil {
		requestBody = io.NopCloser(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, r.base+path, requestBody)
	if err != nil {
		return nil, project.ErrInvalid
	}
	req.Header.Set("Authorization", "Bearer "+r.token)
	req.Header.Set("Accept", "application/json")
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if body != nil {
		req.ContentLength = size
	}
	resp, err := r.client.Do(req)
	if err != nil {
		return nil, project.ErrDependency
	}
	return resp, nil
}

// Call accepts only the management domains. IDs in a supplied request are
// preserved; transport failures never cause a new operation ID or an auto-retry.
func (r *Remote) Call(ctx context.Context, name string, args any, out any) error {
	switch name {
	case "project.create", "project.list", "project.open", "project.commit", "project.commit_status", "project.use_asset",
		"world.register", "world.list", "world.get", "world.grant", "world.revoke", "world.commit_status",
		"artifact.read", "artifact.write":
	default:
		return project.ErrInvalid
	}
	id := uuid.Must(uuid.NewV7()).String()
	body, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "method": "tools/call", "params": map[string]any{"name": name, "arguments": args}})
	if err != nil {
		return project.ErrInvalid
	}
	callCtx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()
	resp, err := r.request(callCtx, "POST", "/mcp", "application/json", bytes.NewReader(body), int64(len(body)))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, responseLimit+1))
	if err != nil || len(data) > responseLimit {
		return project.ErrDependency
	}
	var envelope struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      string          `json:"id"`
		Error   json.RawMessage `json:"error"`
		Result  struct {
			IsError bool            `json:"isError"`
			Content json.RawMessage `json:"structuredContent"`
		} `json:"result"`
	}
	if json.Unmarshal(data, &envelope) != nil || envelope.JSONRPC != "2.0" || envelope.ID != id || len(envelope.Error) > 0 || len(envelope.Result.Content) == 0 || bytes.Equal(envelope.Result.Content, []byte("null")) {
		return statusError(resp.StatusCode)
	}
	if envelope.Result.IsError {
		var failure struct {
			Code    string `json:"code"`
			Current string `json:"current_revision_id"`
		}
		if json.Unmarshal(envelope.Result.Content, &failure) != nil {
			return project.ErrDependency
		}
		switch failure.Code {
		case "INVALID_ARGUMENT":
			return project.ErrInvalid
		case "PERMISSION_DENIED":
			return project.ErrForbidden
		case "NOT_FOUND":
			return project.ErrNotFound
		case "REVISION_CONFLICT":
			return &project.ConflictError{CurrentRevisionID: failure.Current}
		case "IDEMPOTENCY_CONFLICT":
			return project.ErrIdempotency
		case "CONTENT_NOT_READY":
			return project.ErrNotReady
		default:
			return project.ErrDependency
		}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return statusError(resp.StatusCode)
	}
	if out != nil && json.Unmarshal(envelope.Result.Content, out) != nil {
		return project.ErrDependency
	}
	return nil
}

func (r *Remote) Open(ctx context.Context, projectID, revisionID string) (project.OpenResult, error) {
	var out project.OpenResult
	args := map[string]string{"project_id": projectID}
	if revisionID != "" {
		args["revision_id"] = revisionID
	}
	err := r.Call(ctx, "project.open", args, &out)
	return out, err
}

func (r *Remote) Commit(ctx context.Context, req project.CommitRequest) (project.Result, error) {
	var out project.Result
	err := r.Call(ctx, "project.commit", req, &out)
	return out, err
}

func (r *Remote) Metadata(ctx context.Context, ref project.ContentRef, access project.Access) (project.ContentVersion, error) {
	var out struct {
		Mode    string                 `json:"mode"`
		Version project.ContentVersion `json:"version"`
	}
	err := r.Call(ctx, "artifact.read", map[string]any{"mode": "metadata", "content_ref": ref, "access": access}, &out)
	if err == nil && (out.Mode != "metadata" || out.Version.ContentRef != ref) {
		err = project.ErrDependency
	}
	return out.Version, err
}

func (r *Remote) Download(ctx context.Context, ref project.ContentRef, access project.Access, dst io.Writer, limit int64) error {
	if dst == nil || !ref.Valid() || limit < 0 || limit > 1<<50 {
		return project.ErrInvalid
	}
	q := url.Values{"store_id": {ref.StoreID}, "artifact_id": {ref.ArtifactID}}
	if access.ProjectID != "" {
		q.Set("project_id", access.ProjectID)
		q.Set("project_revision_id", access.RevisionID)
	}
	if access.AssetID != "" {
		q.Set("asset_id", access.AssetID)
		q.Set("asset_version_id", access.AssetVersionID)
	}
	resp, err := r.request(ctx, "GET", "/v2/artifacts/"+ref.VersionID+"/content?"+q.Encode(), "", nil, 0)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return statusError(resp.StatusCode)
	}
	if limit == 0 {
		limit = 1 << 50
	}
	n, err := io.Copy(dst, io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return err
	}
	if n > limit {
		return workspace.ErrCorrupt
	}
	return nil
}

func (r *Remote) Upload(ctx context.Context, req workspace.UploadRequest, src io.Reader) (project.ContentVersion, error) {
	if src == nil || !project.ValidID(req.ProjectID) || !project.ValidID(req.WriteID) || req.Source.ProjectID != req.ProjectID || (req.Source.Kind != "user_import" && req.Source.Kind != "user_edit") || req.Size < 0 || req.Size > 1<<50 {
		return project.ContentVersion{}, project.ErrInvalid
	}
	var prepared struct {
		Upload struct {
			ID         string `json:"upload_id"`
			ArtifactID string `json:"artifact_id"`
			VersionID  string `json:"version_id"`
			State      string `json:"state"`
			Path       string `json:"content_path"`
		} `json:"upload"`
	}
	err := r.Call(ctx, "artifact.write", map[string]any{"mode": "prepare", "write_id": req.WriteID, "source": req.Source, "mime": req.MIME, "size": req.Size, "sha256": req.SHA256}, &prepared)
	if err != nil {
		return project.ContentVersion{}, err
	}
	u := prepared.Upload
	if !project.ValidID(u.ID) || !project.ValidID(u.ArtifactID) || !project.ValidID(u.VersionID) || (u.State != "prepared" && u.State != "committed") || u.Path != "/v2/artifacts/uploads/"+u.ID+"/content" {
		return project.ContentVersion{}, project.ErrDependency
	}
	if u.State == "prepared" {
		resp, err := r.request(ctx, "PUT", u.Path, "application/octet-stream", src, req.Size)
		if err != nil {
			return project.ContentVersion{}, err
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			resp.Body.Close()
			return project.ContentVersion{}, statusError(resp.StatusCode)
		}
		_, err = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		resp.Body.Close()
		if err != nil {
			return project.ContentVersion{}, project.ErrDependency
		}
	}
	var out struct {
		Version project.ContentVersion `json:"version"`
	}
	err = r.Call(ctx, "artifact.write", map[string]string{"mode": "commit", "upload_id": u.ID}, &out)
	if err != nil {
		return project.ContentVersion{}, err
	}
	v := out.Version
	if v.SchemaVersion != 2 || !v.ContentRef.Valid() || v.ArtifactID != u.ArtifactID || v.VersionID != u.VersionID || v.Source != req.Source || v.MIME != req.MIME || v.Size != req.Size || v.SHA256 != req.SHA256 {
		return project.ContentVersion{}, project.ErrDependency
	}
	return v, nil
}
