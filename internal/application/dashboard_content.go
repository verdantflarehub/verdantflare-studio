package application

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/verdantflarehub/verdantflare-studio/internal/dashboard"
	"github.com/verdantflarehub/verdantflare-studio/internal/project"
	"github.com/verdantflarehub/verdantflare-studio/internal/strictjson"
)

const DashboardContentLimit = int64(1 << 30)

type DashboardContentGateway interface {
	CallVerified(context.Context, string, map[string]any, project.Principal, string) (any, int, error)
	Transfer(context.Context, project.Principal, string, string, string, io.Reader, int64) (*http.Response, int, error)
}

type DashboardContentInput struct {
	Mode   string             `json:"mode"`
	Ref    project.ContentRef `json:"content_ref"`
	Access project.Access     `json:"access"`
}

func DecodeDashboardContent(body io.Reader) (DashboardContentInput, error) {
	var in DashboardContentInput
	if strictjson.Decode(body, 4096, &in) != nil || !in.Ref.Valid() || !slices.Contains([]string{"metadata", "text", "download"}, in.Mode) {
		return in, dashboard.ErrDenied
	}
	a := in.Access
	noDraft := a.WorldDraftID == "" && a.WorldDraftRevisionID == ""
	projectScope := noDraft && project.ValidID(a.ProjectID) && project.ValidID(a.RevisionID) && a.AssetID == "" && a.AssetVersionID == ""
	assetScope := noDraft && project.ValidID(a.AssetID) && project.ValidID(a.AssetVersionID) && a.ProjectID == "" && a.RevisionID == ""
	draftScope := project.ValidID(a.WorldDraftID) && project.ValidID(a.WorldDraftRevisionID) && a.ProjectID == "" && a.RevisionID == "" && a.AssetID == "" && a.AssetVersionID == ""
	if !projectScope && !assetScope && !draftScope {
		return in, dashboard.ErrDenied
	}
	return in, nil
}

func (d *Dashboards) contentBegin(ctx context.Context, token, sessionID string) (context.Context, dashboard.Actor, dashboard.Launch, Result) {
	return d.contentBeginCapability(ctx, token, sessionID, "artifact.read")
}

func (d *Dashboards) contentBeginCapability(ctx context.Context, token, sessionID, capability string) (context.Context, dashboard.Actor, dashboard.Launch, Result) {
	ctx, actor, result := d.begin(ctx, token)
	if result.Status != 200 {
		return ctx, actor, dashboard.Launch{}, result
	}
	launch, err := d.resolver.Inspect(ctx, actor, sessionID)
	if err != nil {
		return ctx, actor, launch, dashboardError(err, result.RequestID)
	}
	if !slices.Contains(launch.GrantedCapabilities, capability) {
		return ctx, actor, launch, dashboardError(dashboard.ErrDenied, result.RequestID)
	}
	return ctx, actor, launch, result
}

func contentFailure(status int, requestID string) Result {
	code := "CONTENT_UNAVAILABLE"
	switch status {
	case 400:
		code = "INVALID_CONTENT_REQUEST"
	case 401, 403:
		status, code = 403, "CONTENT_DENIED"
	case 404:
		code = "CONTENT_NOT_FOUND"
	case 409:
		code = "CONTENT_NOT_READY"
	case 413:
		code = "CONTENT_TOO_LARGE"
	default:
		status = 503
	}
	out := Error(status, code)
	out.RequestID = requestID
	return out
}

var contentHash = regexp.MustCompile(`^[a-f0-9]{64}$`)

func (d *Dashboards) ReadContent(ctx context.Context, token, sessionID string, in DashboardContentInput, gateway DashboardContentGateway) Result {
	encoded, err := json.Marshal(in)
	if err != nil {
		return Error(400, "INVALID_CONTENT_REQUEST")
	}
	if _, err := DecodeDashboardContent(bytes.NewReader(encoded)); err != nil {
		return Error(400, "INVALID_CONTENT_REQUEST")
	}
	ctx, actor, launch, result := d.contentBegin(ctx, token, sessionID)
	if result.Status != 200 {
		return result
	}
	if gateway == nil {
		return contentFailure(503, result.RequestID)
	}
	var args map[string]any
	_ = json.Unmarshal(encoded, &args)
	principal := project.Principal{OrganizationID: actor.OrganizationID, SubjectID: actor.UserID, RequestID: result.RequestID}
	value, status, err := gateway.CallVerified(ctx, "artifact.read", args, principal, in.Access.ProjectID)
	if err != nil || status != 200 {
		return contentFailure(status, result.RequestID)
	}
	data, err := json.Marshal(value)
	if err != nil || len(data) > 4<<20 {
		return contentFailure(503, result.RequestID)
	}
	var wrapper struct {
		IsError bool            `json:"isError"`
		Content json.RawMessage `json:"structuredContent"`
	}
	if json.Unmarshal(data, &wrapper) != nil || len(wrapper.Content) == 0 {
		return contentFailure(503, result.RequestID)
	}
	if wrapper.IsError {
		var failure struct {
			Code string `json:"code"`
		}
		_ = json.Unmarshal(wrapper.Content, &failure)
		statuses := map[string]int{"INVALID_ARGUMENT": 400, "PERMISSION_DENIED": 403, "NOT_FOUND": 404, "CONTENT_NOT_READY": 409}
		return contentFailure(statuses[failure.Code], result.RequestID)
	}
	var content struct {
		Mode    string                 `json:"mode"`
		Version project.ContentVersion `json:"version"`
		Text    *string                `json:"text,omitempty"`
		Path    string                 `json:"content_path,omitempty"`
	}
	if strictjson.Decode(bytes.NewReader(wrapper.Content), 4<<20, &content) != nil {
		return contentFailure(503, result.RequestID)
	}
	v := content.Version
	mt, _, mimeErr := mime.ParseMediaType(v.MIME)
	if content.Mode != in.Mode || v.ContentRef != in.Ref || v.SchemaVersion != 2 || v.OrganizationID != actor.OrganizationID || v.Size < 0 || !contentHash.MatchString(v.SHA256) || mimeErr != nil {
		return contentFailure(503, result.RequestID)
	}
	if in.Mode == "text" {
		if content.Text == nil || int64(len(*content.Text)) != v.Size || v.Size > 1<<20 || !utf8.ValidString(*content.Text) || !(strings.HasPrefix(mt, "text/") || mt == "application/json" || strings.HasSuffix(mt, "+json")) {
			return contentFailure(503, result.RequestID)
		}
		hash := sha256.Sum256([]byte(*content.Text))
		if hex.EncodeToString(hash[:]) != v.SHA256 {
			return contentFailure(503, result.RequestID)
		}
	} else if content.Text != nil {
		return contentFailure(503, result.RequestID)
	}
	if in.Mode == "download" && v.Size > DashboardContentLimit {
		return contentFailure(413, result.RequestID)
	}
	if _, err := d.resolver.Inspect(ctx, actor, sessionID); err != nil {
		return dashboardError(err, result.RequestID)
	}
	if in.Mode == "download" {
		id, err := d.content.Add(dashboard.ContentHandle{Actor: actor, SessionID: sessionID, Version: v, Access: in.Access, ExpiresAt: launch.ExpiresAt})
		if err != nil {
			return dashboardError(err, result.RequestID)
		}
		content.Path = "/studio/dashboards/" + sessionID + "/content/" + id
	} else {
		content.Path = ""
	}
	result.Data, _ = json.Marshal(content)
	return result
}

func (d *Dashboards) DownloadContent(ctx context.Context, token, sessionID, handle string, gateway DashboardContentGateway) (*http.Response, Result) {
	ctx, actor, _, result := d.contentBegin(ctx, token, sessionID)
	if result.Status != 200 {
		return nil, result
	}
	item, err := d.content.Get(actor, sessionID, handle)
	if err != nil {
		return nil, dashboardError(err, result.RequestID)
	}
	if gateway == nil {
		return nil, contentFailure(503, result.RequestID)
	}
	v, a := item.Version, item.Access
	q := url.Values{"store_id": {v.StoreID}, "artifact_id": {v.ArtifactID}}
	if a.ProjectID != "" {
		q.Set("project_id", a.ProjectID)
		q.Set("project_revision_id", a.RevisionID)
	}
	if a.AssetID != "" {
		q.Set("asset_id", a.AssetID)
		q.Set("asset_version_id", a.AssetVersionID)
	}
	if a.WorldDraftID != "" {
		q.Set("world_draft_id", a.WorldDraftID)
		q.Set("world_draft_revision_id", a.WorldDraftRevisionID)
	}
	p := project.Principal{OrganizationID: actor.OrganizationID, SubjectID: actor.UserID, RequestID: result.RequestID}
	resp, status, err := gateway.Transfer(ctx, p, "GET", "/v2/artifacts/"+v.VersionID+"/content", q.Encode(), nil, 0)
	if err != nil {
		return nil, contentFailure(status, result.RequestID)
	}
	if status != 200 {
		resp.Body.Close()
		return nil, contentFailure(status, result.RequestID)
	}
	mt, _, parseErr := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	wantMT, _, _ := mime.ParseMediaType(v.MIME)
	if resp.ContentLength != v.Size || resp.ContentLength > DashboardContentLimit || parseErr != nil || mt != wantMT || resp.Header.Get("Content-Encoding") != "" {
		resp.Body.Close()
		return nil, contentFailure(503, result.RequestID)
	}
	if _, err := d.resolver.Inspect(ctx, actor, sessionID); err != nil {
		resp.Body.Close()
		return nil, dashboardError(err, result.RequestID)
	}
	return resp, result
}
