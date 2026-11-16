package application

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime"
	"strings"

	"github.com/verdantflarehub/verdantflare-studio/internal/dashboard"
	"github.com/verdantflarehub/verdantflare-studio/internal/project"
	"github.com/verdantflarehub/verdantflare-studio/internal/strictjson"
)

const DashboardUploadLimit int64 = 64 << 20

// Closed import-only branch. It does not accept an endpoint, actor, or an
// existing artifact to overwrite. Artifact owns write idempotency and ACLs.
type DashboardUploadRequest struct {
	Mode    string         `json:"mode"`
	WriteID string         `json:"write_id"`
	Source  project.Source `json:"source"`
	MIME    string         `json:"mime"`
	Size    int64          `json:"size"`
	SHA256  string         `json:"sha256"`
}

type dashboardUpload struct {
	UploadID    string `json:"upload_id"`
	VersionID   string `json:"version_id"`
	ArtifactID  string `json:"artifact_id"`
	State       string `json:"state"`
	ContentPath string `json:"content_path"`
}

func DecodeDashboardUpload(body io.Reader) (DashboardUploadRequest, error) {
	var in DashboardUploadRequest
	if strictjson.Decode(body, 4096, &in) != nil || in.Mode != "prepare" || !project.ValidID(in.WriteID) ||
		in.Source.Kind != "user_import" || !project.ValidID(in.Source.WorldDraftID) ||
		in.Source != (project.Source{Kind: "user_import", WorldDraftID: in.Source.WorldDraftID}) ||
		in.Size < 0 || in.Size > DashboardUploadLimit || !contentHash.MatchString(in.SHA256) {
		return in, dashboard.ErrDenied
	}
	mt, _, err := mime.ParseMediaType(in.MIME)
	if err != nil || len(in.MIME) > 256 || ((mt == "application/json" || strings.HasSuffix(mt, "+json")) && in.Size > 4<<20) {
		return in, dashboard.ErrDenied
	}
	return in, nil
}

func uploadCall(ctx context.Context, gateway DashboardContentGateway, p project.Principal, args map[string]any) (json.RawMessage, int) {
	if gateway == nil {
		return nil, 503
	}
	value, status, err := gateway.CallVerified(ctx, "artifact.write", args, p, "")
	if err != nil || status != 200 {
		if status == 200 {
			status = 503
		}
		return nil, status
	}
	raw, err := json.Marshal(value)
	if err != nil || len(raw) > 64<<10 {
		return nil, 503
	}
	var wrapper struct {
		IsError bool            `json:"isError"`
		Content json.RawMessage `json:"structuredContent"`
	}
	if json.Unmarshal(raw, &wrapper) != nil || len(wrapper.Content) == 0 {
		return nil, 503
	}
	if wrapper.IsError {
		var failure struct {
			Code string `json:"code"`
		}
		_ = json.Unmarshal(wrapper.Content, &failure)
		return nil, map[string]int{"INVALID_ARGUMENT": 400, "PERMISSION_DENIED": 403, "NOT_FOUND": 404, "CONTENT_NOT_READY": 409, "IDEMPOTENCY_CONFLICT": 409}[failure.Code]
	}
	return wrapper.Content, 200
}

func (d *Dashboards) prepareUpload(ctx context.Context, token, sessionID string, in DashboardUploadRequest, g DashboardContentGateway) (dashboardUpload, project.Principal, Result) {
	var upload dashboardUpload
	var principal project.Principal
	raw, err := json.Marshal(in)
	if err != nil {
		return upload, principal, Error(400, "INVALID_CONTENT_REQUEST")
	}
	if _, err = DecodeDashboardUpload(bytes.NewReader(raw)); err != nil {
		return upload, principal, Error(400, "INVALID_CONTENT_REQUEST")
	}
	ctx, actor, launch, out := d.contentBeginCapability(ctx, token, sessionID, "artifact.write")
	if out.Status != 200 {
		return upload, principal, out
	}
	if launch.AppID != "world" {
		return upload, principal, contentFailure(403, out.RequestID)
	}
	principal = project.Principal{OrganizationID: actor.OrganizationID, SubjectID: actor.UserID, RequestID: out.RequestID}
	var args map[string]any
	_ = json.Unmarshal(raw, &args)
	data, status := uploadCall(ctx, g, principal, args)
	if status != 200 {
		return upload, principal, contentFailure(status, out.RequestID)
	}
	var reply struct {
		Upload dashboardUpload `json:"upload"`
	}
	if strictjson.Decode(bytes.NewReader(data), 64<<10, &reply) != nil {
		return upload, principal, contentFailure(503, out.RequestID)
	}
	u := reply.Upload
	if !project.ValidID(u.UploadID) || !project.ValidID(u.VersionID) || !project.ValidID(u.ArtifactID) ||
		(u.State != "prepared" && u.State != "committed") || u.ContentPath != "/v2/artifacts/uploads/"+u.UploadID+"/content" {
		return upload, principal, contentFailure(503, out.RequestID)
	}
	if _, err = d.resolver.Inspect(ctx, actor, sessionID); err != nil {
		return upload, principal, dashboardError(err, out.RequestID)
	}
	return u, principal, out
}

func (d *Dashboards) PrepareContentUpload(ctx context.Context, token, sessionID string, in DashboardUploadRequest, g DashboardContentGateway) Result {
	u, _, out := d.prepareUpload(ctx, token, sessionID, in, g)
	if out.Status != 200 {
		return out
	}
	u.ContentPath = "/studio/dashboards/" + sessionID + "/uploads/" + in.WriteID
	out.Data, _ = json.Marshal(struct {
		Upload dashboardUpload `json:"upload"`
	}{u})
	return out
}

func (d *Dashboards) UploadContent(ctx context.Context, token, sessionID string, in DashboardUploadRequest, body io.Reader, length int64, g DashboardContentGateway) Result {
	if length != in.Size || length < 0 || length > DashboardUploadLimit {
		return Error(400, "INVALID_CONTENT_REQUEST")
	}
	u, p, out := d.prepareUpload(ctx, token, sessionID, in, g)
	if out.Status != 200 {
		return out
	}
	if u.State != "committed" {
		// The original upload id and expected digest are immutable in Artifact.
		response, status, err := g.Transfer(ctx, p, "PUT", u.ContentPath, "", io.LimitReader(body, length), length)
		if response != nil && response.Body != nil {
			response.Body.Close()
		}
		if err != nil || status != 200 || response == nil || response.StatusCode != 200 {
			if status == 200 {
				status = 503
			}
			return contentFailure(status, out.RequestID)
		}
	}
	ctx, actor, _, check := d.contentBeginCapability(ctx, token, sessionID, "artifact.write")
	if check.Status != 200 {
		return check
	}
	if actor.OrganizationID != p.OrganizationID || actor.UserID != p.SubjectID {
		return contentFailure(403, out.RequestID)
	}
	data, status := uploadCall(ctx, g, p, map[string]any{"mode": "commit", "upload_id": u.UploadID})
	if status != 200 {
		return contentFailure(status, out.RequestID)
	}
	var reply struct {
		Version project.ContentVersion `json:"version"`
	}
	if strictjson.Decode(bytes.NewReader(data), 64<<10, &reply) != nil {
		return contentFailure(503, out.RequestID)
	}
	v := reply.Version
	if !v.ContentRef.Valid() || v.SchemaVersion != 2 || v.OrganizationID != p.OrganizationID || v.Source != in.Source ||
		v.Size != in.Size || v.SHA256 != in.SHA256 || v.MIME != in.MIME || v.ArtifactID != u.ArtifactID || v.VersionID != u.VersionID {
		return contentFailure(503, out.RequestID)
	}
	if _, err := d.resolver.Inspect(ctx, actor, sessionID); err != nil {
		return dashboardError(err, out.RequestID)
	}
	out.Data = data
	return out
}
