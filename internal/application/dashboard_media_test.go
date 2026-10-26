package application

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"image"
	"image/png"
	"io"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/verdantflarehub/verdantflare-studio/internal/contentcache"
	"github.com/verdantflarehub/verdantflare-studio/internal/project"
)

func TestWorldEvidenceThumbnailUsesPersistentSourceAndDerivedCache(t *testing.T) {
	f, session, _, original := contentFixture(t)
	t.Setenv("STUDIO_MEDIA_CACHE_DIR", t.TempDir())
	var raw bytes.Buffer
	if err := png.Encode(&raw, image.NewNRGBA(image.Rect(0, 0, 800, 400))); err != nil {
		t.Fatal(err)
	}
	original.text = raw.String()
	original.mode = "metadata"
	original.input.Access = project.Access{AssetID: uuid.Must(uuid.NewV7()).String(), AssetVersionID: uuid.Must(uuid.NewV7()).String()}
	original.version.MIME = "image/png"
	original.version.Size = int64(raw.Len())
	hash := sha256.Sum256(raw.Bytes())
	original.version.SHA256 = hex.EncodeToString(hash[:])
	g := &mediaGatewayStub{contentGatewayStub: original}
	in := g.input
	in.Representation = "thumbnail-320-v1"
	out := f.service.ReadContent(context.Background(), "test-session", session, in, g)
	if out.Status != 200 {
		t.Fatal(out.Status, string(out.Data))
	}
	var value struct {
		Path string `json:"content_path"`
	}
	json.Unmarshal(out.Data, &value)
	body, _, status := readMedia(t, f.service, g, value.Path)
	if status != 200 {
		t.Fatal(status)
	}
	img, _, err := image.Decode(strings.NewReader(body))
	if err != nil || img.Bounds().Dx() != 320 || img.Bounds().Dy() != 160 {
		t.Fatal("invalid World preview", err)
	}
	if _, _, status = readMedia(t, f.service, g, value.Path); status != 200 || g.transfers != 1 || g.generated != 0 {
		t.Fatal("World original reloaded or Project tool called", status, g.transfers, g.generated)
	}
}

type mediaGatewayStub struct {
	*contentGatewayStub
	metadata, authorizations, generated int
	denied                              bool
}

func (g *mediaGatewayStub) CallVerified(ctx context.Context, name string, args map[string]any, p project.Principal, scope string) (any, int, error) {
	if g.denied {
		return nil, 403, nil
	}
	switch name {
	case "project.open":
		g.authorizations++
		return map[string]any{"structuredContent": map[string]any{"project_id": g.input.Access.ProjectID, "revision_id": g.input.Access.RevisionID, "manifest": map[string]any{"files": []project.File{{ID: uuid.Must(uuid.NewV7()).String(), Content: g.input.Ref}}}}}, 200, nil
	case "project.thumbnail":
		g.generated++
		return map[string]any{"structuredContent": map[string]any{"mime": "image/webp", "data": base64.StdEncoding.EncodeToString([]byte("webp-preview"))}}, 200, nil
	default:
		g.metadata++
		return g.contentGatewayStub.CallVerified(ctx, name, args, p, scope)
	}
}
func readMedia(t *testing.T, d *Dashboards, g DashboardContentGateway, path string) (string, string, int) {
	t.Helper()
	parts := strings.Split(path, "/")
	f, out := d.DownloadMedia(context.Background(), "test-session", parts[3], parts[4], parts[5], g)
	if out.Status != 200 {
		return "", "", out.Status
	}
	defer f.File.Close()
	b, e := io.ReadAll(io.NewSectionReader(f.File, 32, f.Size))
	if e != nil {
		t.Fatal(e)
	}
	return string(b), f.ETag, 200
}
func TestStableMediaWarmReadRestartSessionRotationAndRevocation(t *testing.T) {
	f, session, source, original := contentFixture(t)
	dir := t.TempDir()
	t.Setenv("STUDIO_MEDIA_CACHE_DIR", dir)
	f.service.mediaCache()
	original.mode = "metadata"
	g := &mediaGatewayStub{contentGatewayStub: original}
	in := g.input
	in.Representation = "original-v1"
	describe := func(session string) string {
		t.Helper()
		out := f.service.ReadContent(context.Background(), "test-session", session, in, g)
		if out.Status != 200 {
			t.Fatal("describe", out.Status, string(out.Data))
		}
		var body struct {
			Path string `json:"content_path"`
		}
		_ = json.Unmarshal(out.Data, &body)
		return body.Path
	}
	path := describe(session)
	if body, tag, status := readMedia(t, f.service, g, path); status != 200 || body != "hello" || tag != "\""+g.version.SHA256+"\"" {
		t.Fatal("cold read incorrect", status, body, tag)
	}
	if _, _, status := readMedia(t, f.service, g, path); status != 200 || g.transfers != 1 || g.metadata != 1 {
		t.Fatal("warm read repeated source/metadata", status, g.transfers, g.metadata)
	}
	f.service.media = contentcache.New(dir+"/source", 3<<30)
	f.service.mediaIndex = contentcache.New(dir+"/index", 64<<20)
	f.service.mediaDerived = contentcache.New(dir+"/derived", 1<<30)
	f.service.Close(context.Background(), "test-session", session)
	if body, _, status := readMedia(t, f.service, g, path); status != 200 || body != "hello" || g.transfers != 1 {
		t.Fatal("persistent URL depended on old session", status)
	}
	ctx, actor, out := f.service.begin(context.Background(), "test-session")
	if out.Status != 200 {
		t.Fatal(out.Status)
	}
	launch, err := f.service.resolver.Open(ctx, actor, source.binding.InstallationID)
	if err != nil {
		t.Fatal(err)
	}
	if describe(launch.DashboardSessionID) != path || g.metadata != 1 {
		t.Fatal("session rotation changed URL or metadata")
	}
	g.denied = true
	if _, _, status := readMedia(t, f.service, g, path); status != 403 {
		t.Fatal("cached bytes bypassed membership revocation", status)
	}
	g.denied = false
	source.binding.Grants = []string{}
	if _, _, status := readMedia(t, f.service, g, path); status == 200 {
		t.Fatal("cached bytes bypassed installation revocation")
	}
}
func TestStableMediaRepresentationIdentityAndScopeIsolation(t *testing.T) {
	f, session, _, original := contentFixture(t)
	t.Setenv("STUDIO_MEDIA_CACHE_DIR", t.TempDir())
	f.service.mediaCache()
	original.mode = "metadata"
	g := &mediaGatewayStub{contentGatewayStub: original}
	in := g.input
	in.Representation = "thumbnail-320-v1"
	result := f.service.ReadContent(context.Background(), "test-session", session, in, g)
	var value struct {
		Path string `json:"content_path"`
	}
	_ = json.Unmarshal(result.Data, &value)
	if result.Status != 200 {
		t.Fatal(result.Status, string(result.Data))
	}
	if b, tag, status := readMedia(t, f.service, g, value.Path); status != 200 || b != "webp-preview" || tag == "\""+g.version.SHA256+"\"" {
		t.Fatal("thumbnail reused source ETag", status, tag)
	}
	_, _, _ = readMedia(t, f.service, g, value.Path)
	if g.generated != 1 || g.transfers != 0 {
		t.Fatal("thumbnail regenerated or original transferred", g.generated, g.transfers)
	}
	parts := strings.Split(value.Path, "/")
	parts[3] = strings.Repeat("a", 64)
	if _, _, status := readMedia(t, f.service, g, strings.Join(parts, "/")); status != 403 {
		t.Fatal("foreign cache scope accepted", status)
	}
	in.Representation = "unknown"
	data, _ := json.Marshal(in)
	if _, err := DecodeDashboardContent(strings.NewReader(string(data))); err == nil {
		t.Fatal("unknown representation accepted")
	}
}
func TestMediaScopeStableAcrossLoginRefreshButChangesWithPolicy(t *testing.T) {
	identity := identityContext{UserID: "one", OrganizationID: "org", StationID: "station", SessionID: "session1", Roles: []string{"user", "admin"}, Scopes: []string{"app:read", "artifact:read"}}
	first := mediaIdentityScope(identity)
	identity.SessionID = "session2"
	slices.Reverse(identity.Roles)
	if first != mediaIdentityScope(identity) {
		t.Fatal("ordinary login refresh/order changed media URLs")
	}
	identity.PolicyVersion++
	if first == mediaIdentityScope(identity) {
		t.Fatal("permission policy did not isolate cache")
	}
	identity.PolicyVersion--
	identity.UserID = "two"
	if first == mediaIdentityScope(identity) {
		t.Fatal("another user reused cache scope")
	}
}
