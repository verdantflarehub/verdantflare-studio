package application

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"mime"
	"strings"
	"unicode/utf8"

	"github.com/verdantflarehub/verdantflare-studio/internal/dashboard"
	"github.com/verdantflarehub/verdantflare-studio/internal/project"
	"github.com/verdantflarehub/verdantflare-studio/internal/strictjson"
)

// A bounded text branch of the existing Artifact contract, not an arbitrary
// capability/MCP proxy. Binary uploads use a separately controlled transport.
type DashboardTextWrite struct {
	Mode       string         `json:"mode"`
	WriteID    string         `json:"write_id"`
	ArtifactID string         `json:"artifact_id,omitempty"`
	Source     project.Source `json:"source"`
	MIME       string         `json:"mime"`
	Text       string         `json:"text"`
}

func DecodeDashboardTextWrite(body io.Reader) (DashboardTextWrite, error) {
	var in DashboardTextWrite
	if strictjson.Decode(body, 4<<20, &in) != nil || in.Mode != "text" || !project.ValidID(in.WriteID) ||
		(in.ArtifactID != "" && !project.ValidID(in.ArtifactID)) || len(in.Text) > 1<<20 || !utf8.ValidString(in.Text) {
		return in, dashboard.ErrDenied
	}
	s := in.Source
	projectScope := project.ValidID(s.ProjectID) && s.WorldDraftID == ""
	draftScope := project.ValidID(s.WorldDraftID) && s.ProjectID == ""
	if (s.Kind != "user_edit" && s.Kind != "user_import") || (!projectScope && !draftScope) ||
		s != (project.Source{Kind: s.Kind, ProjectID: s.ProjectID, WorldDraftID: s.WorldDraftID}) {
		return in, dashboard.ErrDenied
	}
	mt, _, err := mime.ParseMediaType(in.MIME)
	if err != nil || len(in.MIME) > 256 || !(strings.HasPrefix(mt, "text/") || mt == "application/json" || strings.HasSuffix(mt, "+json")) {
		return in, dashboard.ErrDenied
	}
	return in, nil
}

func (d *Dashboards) WriteContent(ctx context.Context, token, sessionID string, in DashboardTextWrite, gateway DashboardContentGateway) Result {
	encoded, err := json.Marshal(in)
	if err != nil {
		return Error(400, "INVALID_CONTENT_REQUEST")
	}
	if _, err = DecodeDashboardTextWrite(bytes.NewReader(encoded)); err != nil {
		return Error(400, "INVALID_CONTENT_REQUEST")
	}
	ctx, actor, _, result := d.contentBeginCapability(ctx, token, sessionID, "artifact.write")
	if result.Status != 200 {
		return result
	}
	if gateway == nil {
		return contentFailure(503, result.RequestID)
	}
	var args map[string]any
	_ = json.Unmarshal(encoded, &args)
	p := project.Principal{OrganizationID: actor.OrganizationID, SubjectID: actor.UserID, RequestID: result.RequestID}
	value, status, err := gateway.CallVerified(ctx, "artifact.write", args, p, in.Source.ProjectID)
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
		if failure.Code == "IDEMPOTENCY_CONFLICT" {
			out := Error(409, failure.Code)
			out.RequestID = result.RequestID
			return out
		}
		return contentFailure(map[string]int{"INVALID_ARGUMENT": 400, "PERMISSION_DENIED": 403, "NOT_FOUND": 404, "CONTENT_NOT_READY": 409}[failure.Code], result.RequestID)
	}
	var content struct {
		Version project.ContentVersion `json:"version"`
	}
	if strictjson.Decode(bytes.NewReader(wrapper.Content), 4<<20, &content) != nil {
		return contentFailure(503, result.RequestID)
	}
	v := content.Version
	hash := sha256.Sum256([]byte(in.Text))
	if !v.ContentRef.Valid() || v.SchemaVersion != 2 || v.OrganizationID != actor.OrganizationID || v.Source != in.Source ||
		v.Size != int64(len(in.Text)) || v.SHA256 != hex.EncodeToString(hash[:]) || v.MIME != in.MIME ||
		(in.ArtifactID != "" && v.ArtifactID != in.ArtifactID) {
		return contentFailure(503, result.RequestID)
	}
	if _, err = d.resolver.Inspect(ctx, actor, sessionID); err != nil {
		return dashboardError(err, result.RequestID)
	}
	result.Data, _ = json.Marshal(content)
	return result
}
