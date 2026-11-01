package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/verdantflarehub/verdantflare-studio/internal/dashboard"
	"github.com/verdantflarehub/verdantflare-studio/internal/project"
)

type imageResultGateway struct {
	*contentGatewayStub
	in     DashboardImageResultInput
	source *contentGatewayStub
	files  []project.File
}

func (g *imageResultGateway) CallVerified(ctx context.Context, name string, args map[string]any, p project.Principal, scope string) (any, int, error) {
	if name == "project.open" {
		if args["project_id"] != g.in.ProjectID || args["revision_id"] != g.in.RevisionID || len(args) != 2 || !p.Valid() {
			g.t.Fatal("wrong fixed revision request")
		}
		return map[string]any{"structuredContent": map[string]any{"project_id": g.in.ProjectID, "revision_id": g.in.RevisionID, "manifest": map[string]any{"files": g.files}}}, 200, nil
	}
	if args["mode"] == "text" {
		return g.source.CallVerified(ctx, name, args, p, scope)
	}
	return g.contentGatewayStub.CallVerified(ctx, name, args, p, scope)
}

func imageResultFixture(t *testing.T) (*dashboardServiceFixture, string, *imageResultGateway, ImageResultReader) {
	t.Helper()
	f, _, binding, g := contentFixture(t)
	binding.binding.Grants = append(binding.binding.Grants, "image.result.read")
	binding.binding.Declaration.Dashboard.RequiredCapabilities = append(binding.binding.Declaration.Dashboard.RequiredCapabilities, "image.result.read")
	ctx, actor, _ := f.service.begin(context.Background(), "test-session")
	launch, err := f.service.resolver.Open(ctx, actor, f.installation.InstallationID)
	if err != nil {
		t.Fatal(err)
	}
	id := func() string { return uuid.Must(uuid.NewV7()).String() }
	in := DashboardImageResultInput{ProjectID: g.input.Access.ProjectID, RevisionID: g.input.Access.RevisionID, ResultFileID: id(), ProvenanceFileID: id()}
	g.mode, g.version.MIME = "metadata", "image/jpeg"
	source := *g
	source.input.Ref = project.ContentRef{StoreID: id(), ArtifactID: id(), VersionID: id()}
	source.version.ContentRef = source.input.Ref
	source.mode, source.version.MIME = "text", "application/json"
	gw := &imageResultGateway{contentGatewayStub: g, in: in, source: &source, files: []project.File{
		{ID: in.ResultFileID, Role: "image", Content: g.input.Ref},
		{ID: in.ProvenanceFileID, Role: "provenance", Content: source.input.Ref},
	}}
	gw.setSource(map[string]any{"kind": "tryon-result-source", "sha256": g.version.SHA256, "image_artifact_id": "art-0123456789abcdef", "tryon": map[string]any{"image_source": map[string]any{"final_artifact_id": "art-0123456789abcdef", "generation_artifact_id": "art-1111111111111111"}}})
	reader := func(ctx context.Context, p project.Principal, projectID, artifactID string) (ImageResultStatus, int, error) {
		if !p.Valid() || p.SubjectID != actor.UserID || p.OrganizationID != actor.OrganizationID || projectID != in.ProjectID || artifactID != "art-0123456789abcdef" {
			t.Fatal("wrong final image identity")
		}
		return ImageResultStatus{ProjectID: projectID, ArtifactID: artifactID, SHA256: g.version.SHA256, Rating: ImageResultRating{Rating: "unrated"}}, 200, nil
	}
	return f, launch.DashboardSessionID, gw, reader
}

func (g *imageResultGateway) setSource(value any) {
	data, _ := json.Marshal(value)
	g.source.text = string(data)
	hash := sha256.Sum256(data)
	g.source.version.SHA256, g.source.version.Size = hex.EncodeToString(hash[:]), int64(len(data))
}

func TestImageResultFixedSourceAndLegacyIdentity(t *testing.T) {
	f, session, g, reader := imageResultFixture(t)
	for _, legacy := range []bool{false, true} {
		if legacy {
			g.setSource(map[string]any{"kind": "tryon-result-source", "sha256": g.version.SHA256, "image_artifact_id": "art-0123456789abcdef"})
		}
		out := f.service.ReadImageResult(context.Background(), "test-session", session, g.in, g, reader)
		if out.Status != 200 || !strings.Contains(string(out.Data), `"status":"visible"`) || strings.Contains(string(out.Data), "art-0123") {
			t.Fatalf("unexpected result %d %s", out.Status, out.Data)
		}
	}
}

func TestImageResultRejectsBadProvenanceBeforeImageCall(t *testing.T) {
	for _, change := range []string{"hash", "identity", "kind", "missing-file", "corrupt-bytes"} {
		t.Run(change, func(t *testing.T) {
			f, session, g, _ := imageResultFixture(t)
			value := map[string]any{"kind": "tryon-result-source", "sha256": g.version.SHA256, "image_artifact_id": "art-0123456789abcdef"}
			switch change {
			case "hash":
				value["sha256"] = strings.Repeat("0", 64)
			case "identity":
				value["tryon"] = map[string]any{"image_source": map[string]any{"final_artifact_id": "art-1111111111111111"}}
			case "kind":
				value["kind"] = "reference"
			case "missing-file":
				g.files = g.files[:1]
			}
			g.setSource(value)
			if change == "corrupt-bytes" {
				g.source.text += " "
			}
			reader := func(context.Context, project.Principal, string, string) (ImageResultStatus, int, error) {
				t.Fatal("invalid source reached Image")
				return ImageResultStatus{}, 200, nil
			}
			out := f.service.ReadImageResult(context.Background(), "test-session", session, g.in, g, reader)
			if out.Status == 200 {
				t.Fatal("invalid source visible")
			}
		})
	}
}

func TestImageResultUnavailableFailuresAndPreference(t *testing.T) {
	for _, state := range []string{"hidden", "denied", "failure", "hash", "nsfw-off", "nsfw-on", "closed"} {
		t.Run(state, func(t *testing.T) {
			f, session, g, valid := imageResultFixture(t)
			ctx := context.Background()
			if state == "nsfw-on" {
				ctx = dashboard.WithIncludeNSFW(ctx, true)
			}
			reader := func(ctx context.Context, p project.Principal, projectID, artifactID string) (ImageResultStatus, int, error) {
				value, status, err := valid(ctx, p, projectID, artifactID)
				switch state {
				case "hidden":
					status = 404
				case "denied":
					status = 403
				case "failure":
					err = errors.New("offline")
				case "hash":
					value.SHA256 = strings.Repeat("0", 64)
				case "nsfw-off", "nsfw-on":
					value.Rating.Rating = "nsfw"
				case "closed":
					f.service.Close(ctx, "test-session", session)
				}
				return value, status, err
			}
			out := f.service.ReadImageResult(ctx, "test-session", session, g.in, g, reader)
			switch state {
			case "hidden", "nsfw-off":
				if out.Status != 200 || string(out.Data) != `{"status":"unavailable"}` {
					t.Fatal("hidden metadata leaked", string(out.Data))
				}
			case "nsfw-on":
				if out.Status != 200 || !strings.Contains(string(out.Data), `"status":"visible"`) {
					t.Fatal("preference ignored")
				}
			case "denied":
				if out.Status != 403 {
					t.Fatal(out.Status)
				}
			case "failure":
				if out.Status != 503 {
					t.Fatal(out.Status)
				}
			default:
				if out.Status == 200 {
					t.Fatal("invalid result accepted")
				}
			}
		})
	}
}

func TestImageResultInputRejectsNativeIDAndExtraFields(t *testing.T) {
	_, _, g, _ := imageResultFixture(t)
	data, _ := json.Marshal(g.in)
	if _, err := DecodeDashboardImageResult(bytesReader(data)); err != nil {
		t.Fatal(err)
	}
	data = append(data[:len(data)-1], []byte(`,"artifact_id":"art-0123456789abcdef"}`)...)
	if _, err := DecodeDashboardImageResult(bytesReader(data)); err == nil {
		t.Fatal("native ID accepted")
	}
}

func bytesReader(b []byte) *strings.Reader { return strings.NewReader(string(b)) }
