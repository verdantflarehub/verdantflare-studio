package application

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/verdantflarehub/verdantflare-studio/internal/project"
)

type uploadGatewayStub struct {
	t                            *testing.T
	in                           DashboardUploadRequest
	u                            dashboardUpload
	version                      project.ContentVersion
	transfers, commits, prepares int
	stored                       bool
	afterTransfer                func()
	mutate                       func(*project.ContentVersion)
	badPath                      bool
}

func (g *uploadGatewayStub) CallVerified(_ context.Context, name string, args map[string]any, p project.Principal, scope string) (any, int, error) {
	if name != "artifact.write" || !p.Valid() || scope != "" {
		g.t.Fatal("incorrect upload principal or capability")
	}
	wrap := func(v any) (any, int, error) { return map[string]any{"structuredContent": v}, 200, nil }
	if args["mode"] == "prepare" {
		g.prepares++
		raw, _ := json.Marshal(args)
		in, err := DecodeDashboardUpload(bytes.NewReader(raw))
		if err != nil || in != g.in {
			return map[string]any{"isError": true, "structuredContent": map[string]string{"code": "IDEMPOTENCY_CONFLICT"}}, 200, nil
		}
		u := g.u
		if g.badPath {
			u.ContentPath = "https://untrusted.invalid/put"
		}
		return wrap(map[string]any{"upload": u})
	}
	if args["mode"] != "commit" || args["upload_id"] != g.u.UploadID || len(args) != 2 || !g.stored {
		return nil, 409, nil
	}
	g.commits++
	g.u.State = "committed"
	v := g.version
	v.OrganizationID = p.OrganizationID
	if g.mutate != nil {
		g.mutate(&v)
	}
	return wrap(map[string]any{"version": v})
}
func (g *uploadGatewayStub) Transfer(_ context.Context, p project.Principal, method, path, query string, body io.Reader, length int64) (*http.Response, int, error) {
	if !p.Valid() || method != "PUT" || path != g.u.ContentPath || query != "" || length != g.in.Size {
		g.t.Fatal("uncontrolled upload transfer")
	}
	g.transfers++
	data, err := io.ReadAll(body)
	hash := sha256.Sum256(data)
	if err != nil || int64(len(data)) != g.in.Size || hex.EncodeToString(hash[:]) != g.in.SHA256 {
		return nil, 400, nil
	}
	g.stored = true
	if g.afterTransfer != nil {
		g.afterTransfer()
	}
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(""))}, 200, nil
}

func uploadFixture(t *testing.T) (*dashboardServiceFixture, string, *contentSource, *uploadGatewayStub, []byte) {
	t.Helper()
	f, _, source, _ := contentFixture(t)
	source.binding.AppID = "world"
	source.binding.Declaration.AppID = "world"
	source.binding.Grants = append(source.binding.Grants, "artifact.write")
	source.binding.Declaration.Dashboard.OptionalCapabilities = []string{"artifact.write"}
	ctx, actor, _ := f.service.begin(context.Background(), "test-session")
	launch, err := f.service.resolver.Open(ctx, actor, f.installation.InstallationID)
	if err != nil {
		t.Fatal(err)
	}
	id := func() string { return uuid.Must(uuid.NewV7()).String() }
	data := bytes.Repeat([]byte("a"), 1901755)
	h := sha256.Sum256(data)
	in := DashboardUploadRequest{Mode: "prepare", WriteID: id(), Source: project.Source{Kind: "user_import", WorldDraftID: id()}, MIME: "application/json", Size: int64(len(data)), SHA256: hex.EncodeToString(h[:])}
	u := dashboardUpload{UploadID: id(), VersionID: id(), ArtifactID: id(), State: "prepared"}
	u.ContentPath = "/v2/artifacts/uploads/" + u.UploadID + "/content"
	v := project.ContentVersion{SchemaVersion: 2, ContentRef: project.ContentRef{StoreID: id(), VersionID: u.VersionID, ArtifactID: u.ArtifactID}, Source: in.Source, Size: in.Size, SHA256: in.SHA256, MIME: in.MIME}
	return f, launch.DashboardSessionID, source, &uploadGatewayStub{t: t, in: in, u: u, version: v}, data
}

func TestDashboardUploadLargeContentAndSameOperationRecovery(t *testing.T) {
	f, session, _, g, data := uploadFixture(t)
	ctx := context.Background()
	prepared := f.service.PrepareContentUpload(ctx, "test-session", session, g.in, g)
	if prepared.Status != 200 || !strings.Contains(string(prepared.Data), "/studio/dashboards/"+session+"/uploads/"+g.in.WriteID) || strings.Contains(string(prepared.Data), "/v2/artifacts/") {
		t.Fatal(prepared.Status, string(prepared.Data))
	}
	for i := 0; i < 2; i++ {
		out := f.service.UploadContent(ctx, "test-session", session, g.in, bytes.NewReader(data), int64(len(data)), g)
		if out.Status != 200 {
			t.Fatal(out.Status, string(out.Data))
		}
	}
	if g.transfers != 1 || g.commits != 2 {
		t.Fatal("committed upload was duplicated", g.transfers, g.commits)
	}
	changed := g.in
	changed.SHA256 = strings.Repeat("0", 64)
	if out := f.service.UploadContent(ctx, "test-session", session, changed, bytes.NewReader(data), int64(len(data)), g); out.Status != 409 || g.transfers != 1 {
		t.Fatal("changed recovery intent accepted")
	}
}

func TestDashboardUploadRejectsInvalidBytesAndRevocation(t *testing.T) {
	f, session, source, g, data := uploadFixture(t)
	ctx := context.Background()
	wrong := bytes.Repeat([]byte("b"), len(data))
	if out := f.service.UploadContent(ctx, "test-session", session, g.in, bytes.NewReader(wrong), int64(len(wrong)), g); out.Status == 200 || g.commits != 0 {
		t.Fatal("invalid digest committed")
	}
	g.afterTransfer = func() { source.binding.Grants = nil }
	if out := f.service.UploadContent(ctx, "test-session", session, g.in, bytes.NewReader(data), int64(len(data)), g); out.Status == 200 || g.commits != 0 {
		t.Fatal("revoked session committed")
	}
}

func TestDashboardUploadRejectsWrongReceiptAndDestination(t *testing.T) {
	for _, mutate := range []func(*project.ContentVersion){func(v *project.ContentVersion) { v.Size++ }, func(v *project.ContentVersion) { v.SHA256 = strings.Repeat("0", 64) }, func(v *project.ContentVersion) { v.Source.WorldDraftID = uuid.Must(uuid.NewV7()).String() }, func(v *project.ContentVersion) { v.OrganizationID = uuid.Must(uuid.NewV7()).String() }, func(v *project.ContentVersion) { v.VersionID = uuid.Must(uuid.NewV7()).String() }} {
		f, s, _, g, b := uploadFixture(t)
		g.mutate = mutate
		if out := f.service.UploadContent(context.Background(), "test-session", s, g.in, bytes.NewReader(b), int64(len(b)), g); out.Status == 200 {
			t.Fatal("incorrect receipt accepted")
		}
	}
	f, s, _, g, b := uploadFixture(t)
	g.badPath = true
	if out := f.service.UploadContent(context.Background(), "test-session", s, g.in, bytes.NewReader(b), int64(len(b)), g); out.Status == 200 || g.transfers != 0 {
		t.Fatal("arbitrary destination used")
	}
}

func TestDashboardUploadClosedShapeAndLimits(t *testing.T) {
	_, _, _, g, _ := uploadFixture(t)
	for _, change := range []func(*DashboardUploadRequest){func(v *DashboardUploadRequest) { v.Size = 4<<20 + 1 }, func(v *DashboardUploadRequest) { v.Source.ProjectID = v.Source.WorldDraftID }, func(v *DashboardUploadRequest) { v.Source.Kind = "task_output" }, func(v *DashboardUploadRequest) { v.SHA256 = "bad" }, func(v *DashboardUploadRequest) { v.Mode = "commit" }} {
		in := g.in
		change(&in)
		raw, _ := json.Marshal(in)
		if _, err := DecodeDashboardUpload(bytes.NewReader(raw)); err == nil {
			t.Fatal("invalid import metadata accepted")
		}
	}
	raw, _ := json.Marshal(g.in)
	bad := append([]byte(`{"user_id":"other",`), raw[1:]...)
	if _, err := DecodeDashboardUpload(bytes.NewReader(bad)); err == nil {
		t.Fatal("foreign identity accepted")
	}
}

func TestDashboardUploadRequiresWorldAndWriteGrant(t *testing.T) {
	for _, app := range []string{"world", "project"} {
		t.Run(app, func(t *testing.T) {
			f, _, source, g, _ := uploadFixture(t)
			source.binding.AppID, source.binding.Declaration.AppID = app, app
			if app == "world" {
				source.binding.Grants = []string{"artifact.read"}
			}
			ctx, actor, _ := f.service.begin(context.Background(), "test-session")
			launch, err := f.service.resolver.Open(ctx, actor, f.installation.InstallationID)
			if err != nil {
				t.Fatal(err)
			}
			out := f.service.PrepareContentUpload(ctx, "test-session", launch.DashboardSessionID, g.in, g)
			if out.Status == 200 || g.prepares != 0 {
				t.Fatal("unauthorized prepare reached Artifact")
			}
		})
	}
}
