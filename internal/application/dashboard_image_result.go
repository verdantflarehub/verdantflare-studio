package application

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"mime"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/verdantflarehub/verdantflare-studio/internal/dashboard"
	"github.com/verdantflarehub/verdantflare-studio/internal/project"
	"github.com/verdantflarehub/verdantflare-studio/internal/strictjson"
)

type DashboardImageResultInput struct {
	ProjectID        string `json:"project_id"`
	RevisionID       string `json:"project_revision_id"`
	ResultFileID     string `json:"result_file_id"`
	ProvenanceFileID string `json:"provenance_file_id"`
}

type ImageResultRating struct {
	Rating       string  `json:"rating"`
	Source       *string `json:"source"`
	HumanCleared bool    `json:"human_cleared"`
}

type ImageResultStatus struct {
	ProjectID  string            `json:"project_id"`
	ArtifactID string            `json:"artifact_id"`
	SHA256     string            `json:"sha256"`
	Rating     ImageResultRating `json:"content_rating"`
}

type ImageResultReader func(context.Context, project.Principal, string, string) (ImageResultStatus, int, error)

var nativeImageID = regexp.MustCompile(`^art-[0-9a-f]{16}$`)

func DecodeDashboardImageResult(r io.Reader) (DashboardImageResultInput, error) {
	var in DashboardImageResultInput
	if strictjson.Decode(r, 4096, &in) != nil || !project.ValidID(in.ProjectID) || !project.ValidID(in.RevisionID) || !project.ValidID(in.ResultFileID) || !project.ValidID(in.ProvenanceFileID) || in.ResultFileID == in.ProvenanceFileID {
		return in, dashboard.ErrDenied
	}
	return in, nil
}

func imageResultFailure(status int, requestID string) Result {
	code := "RESULT_STATUS_UNAVAILABLE"
	switch status {
	case 401, 410:
		status, code = 401, "SESSION_EXPIRED"
	case 403:
		code = "ACCESS_DENIED"
	case 400, 404, 422:
		status, code = 422, "RESULT_SOURCE_INVALID"
	default:
		status = 503
	}
	out := Error(status, code)
	out.RequestID = requestID
	return out
}

func (d *Dashboards) ReadImageResult(ctx context.Context, token, sessionID string, in DashboardImageResultInput, gateway DashboardContentGateway, read ImageResultReader) Result {
	encoded, _ := json.Marshal(in)
	if _, err := DecodeDashboardImageResult(bytes.NewReader(encoded)); err != nil {
		return Error(400, "INVALID_RESULT_REQUEST")
	}
	ctx, actor, _, out := d.contentBeginCapability(ctx, token, sessionID, "image.result.read")
	if out.Status != 200 {
		return imageResultFailure(out.Status, out.RequestID)
	}
	p := project.Principal{OrganizationID: actor.OrganizationID, SubjectID: actor.UserID, RequestID: out.RequestID}
	// artifact.read remains a separate required capability on Dashboard sessions.
	_, _, _, contentGrant := d.contentBegin(ctx, token, sessionID)
	if contentGrant.Status != 200 {
		return imageResultFailure(contentGrant.Status, out.RequestID)
	}
	out = resolveSavedImageResult(ctx, p, in, gateway, read, dashboard.IncludeNSFW(ctx), func(input DashboardContentInput) Result {
		return d.ReadContent(ctx, token, sessionID, input, gateway)
	})
	if _, err := d.resolver.Inspect(ctx, actor, sessionID); err != nil {
		return imageResultFailure(dashboardError(err, out.RequestID).Status, out.RequestID)
	}
	return out
}

func resolveSavedImageResult(ctx context.Context, p project.Principal, in DashboardImageResultInput, gateway DashboardContentGateway, read ImageResultReader, include bool, content func(DashboardContentInput) Result) Result {
	out := Result{Status: 200, RequestID: p.RequestID}
	encoded, _ := json.Marshal(in)
	if _, err := DecodeDashboardImageResult(bytes.NewReader(encoded)); err != nil || !p.Valid() {
		return imageResultFailure(422, p.RequestID)
	}
	if gateway == nil || read == nil {
		return imageResultFailure(503, p.RequestID)
	}
	data, err := mediaCall(ctx, gateway, p, "project.open", map[string]any{"project_id": in.ProjectID, "revision_id": in.RevisionID}, in.ProjectID)
	if err != nil {
		failure := mediaFailure(err, out.RequestID)
		return imageResultFailure(failure.Status, out.RequestID)
	}
	var opened struct {
		ProjectID  string `json:"project_id"`
		RevisionID string `json:"revision_id"`
		Manifest   struct {
			Files []project.File `json:"files"`
		} `json:"manifest"`
	}
	if json.Unmarshal(data, &opened) != nil || opened.ProjectID != in.ProjectID || opened.RevisionID != in.RevisionID {
		return imageResultFailure(422, out.RequestID)
	}
	var resultFile, sourceFile project.File
	var results, sources int
	for _, f := range opened.Manifest.Files {
		if f.ID == in.ResultFileID {
			resultFile = f
			results++
		}
		if f.ID == in.ProvenanceFileID {
			sourceFile = f
			sources++
		}
	}
	if results != 1 || sources != 1 || resultFile.Role != "image" || sourceFile.Role != "provenance" || !resultFile.Content.Valid() || !sourceFile.Content.Valid() {
		return imageResultFailure(422, out.RequestID)
	}
	access := project.Access{ProjectID: in.ProjectID, RevisionID: in.RevisionID}
	metadata := content(DashboardContentInput{Mode: "metadata", Ref: resultFile.Content, Access: access})
	if metadata.Status != 200 {
		return imageResultFailure(metadata.Status, out.RequestID)
	}
	source := content(DashboardContentInput{Mode: "text", Ref: sourceFile.Content, Access: access})
	if source.Status != 200 {
		return imageResultFailure(source.Status, out.RequestID)
	}
	var image struct {
		Version project.ContentVersion `json:"version"`
	}
	var text struct {
		Text string `json:"text"`
	}
	if json.Unmarshal(metadata.Data, &image) != nil || !strings.HasPrefix(image.Version.MIME, "image/") || json.Unmarshal(source.Data, &text) != nil {
		return imageResultFailure(422, out.RequestID)
	}
	value, err := strictjson.ReadValue(strings.NewReader(text.Text), 1<<20)
	var provenance struct {
		Kind       string `json:"kind"`
		SHA256     string `json:"sha256"`
		ArtifactID string `json:"image_artifact_id"`
		Tryon      struct {
			ImageSource *struct {
				FinalArtifactID string `json:"final_artifact_id"`
			} `json:"image_source"`
		} `json:"tryon"`
	}
	if err != nil || json.Unmarshal(value, &provenance) != nil || provenance.Kind != "tryon-result-source" || provenance.SHA256 != image.Version.SHA256 {
		return imageResultFailure(422, out.RequestID)
	}
	final := provenance.ArtifactID
	if provenance.Tryon.ImageSource != nil {
		final = provenance.Tryon.ImageSource.FinalArtifactID
		if provenance.ArtifactID != "" && provenance.ArtifactID != final {
			return imageResultFailure(422, out.RequestID)
		}
	}
	if !nativeImageID.MatchString(final) {
		return imageResultFailure(422, out.RequestID)
	}
	current, status, err := read(ctx, p, in.ProjectID, final)
	if err != nil {
		return imageResultFailure(503, out.RequestID)
	}
	if status == 404 {
		out.Data = json.RawMessage(`{"status":"unavailable"}`)
		return out
	}
	if status != 200 {
		return imageResultFailure(status, out.RequestID)
	}
	if current.ProjectID != in.ProjectID || current.ArtifactID != final || current.SHA256 != image.Version.SHA256 {
		return imageResultFailure(422, out.RequestID)
	}
	rating := current.Rating
	if (rating.Rating != "unrated" && rating.Rating != "general" && rating.Rating != "nsfw") || (rating.Source != nil && *rating.Source != "ai" && *rating.Source != "human") {
		return imageResultFailure(503, out.RequestID)
	}
	if rating.Rating == "nsfw" && !include {
		out.Data = json.RawMessage(`{"status":"unavailable"}`)
		return out
	}
	out.Data, _ = json.Marshal(map[string]any{"status": "visible", "content_rating": rating, "content_ref": resultFile.Content, "access": access})
	return out
}

// Public MCP has no Dashboard session. Artifact and Project services validate
// the same real principal and fixed scope; only metadata/text is consumed here.
func readResultContent(ctx context.Context, p project.Principal, in DashboardContentInput, gateway DashboardContentGateway) Result {
	encoded, _ := json.Marshal(in)
	var args map[string]any
	_ = json.Unmarshal(encoded, &args)
	data, err := mediaCall(ctx, gateway, p, "artifact.read", args, in.Access.ProjectID)
	if err != nil {
		return mediaFailure(err, p.RequestID)
	}
	var value struct {
		Mode    string                 `json:"mode"`
		Version project.ContentVersion `json:"version"`
		Text    *string                `json:"text,omitempty"`
	}
	if strictjson.Decode(bytes.NewReader(data), 4<<20, &value) != nil {
		return contentFailure(503, p.RequestID)
	}
	v := value.Version
	mt, _, mimeErr := mime.ParseMediaType(v.MIME)
	if value.Mode != in.Mode || v.ContentRef != in.Ref || v.SchemaVersion != 2 || v.OrganizationID != p.OrganizationID || v.Size < 0 || !contentHash.MatchString(v.SHA256) || mimeErr != nil {
		return contentFailure(503, p.RequestID)
	}
	if in.Mode == "text" {
		if value.Text == nil || int64(len(*value.Text)) != v.Size || v.Size > 1<<20 || !utf8.ValidString(*value.Text) || !(strings.HasPrefix(mt, "text/") || mt == "application/json" || strings.HasSuffix(mt, "+json")) {
			return contentFailure(503, p.RequestID)
		}
		hash := sha256.Sum256([]byte(*value.Text))
		if hex.EncodeToString(hash[:]) != v.SHA256 {
			return contentFailure(503, p.RequestID)
		}
	} else if in.Mode != "metadata" || value.Text != nil {
		return contentFailure(503, p.RequestID)
	}
	return Result{Status: 200, RequestID: p.RequestID, Data: data}
}
