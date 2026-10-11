package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/verdantflarehub/verdantflare-studio/internal/dashboard"
	"github.com/verdantflarehub/verdantflare-studio/internal/project"
)

type contentSource struct{ binding dashboard.Binding }

func (s *contentSource) Lookup(context.Context, dashboard.Actor, string) (dashboard.Binding, error) {
	return s.binding, nil
}

type contentGatewayStub struct {
	t                *testing.T
	input            DashboardContentInput
	version          project.ContentVersion
	text             string
	mode             string
	calls, transfers int
}

func (g *contentGatewayStub) CallVerified(_ context.Context, name string, args map[string]any, p project.Principal, scope string) (any, int, error) {
	g.calls++
	if name != "artifact.read" || !p.Valid() || p.OrganizationID != g.version.OrganizationID || scope != g.input.Access.ProjectID || len(args) != 3 {
		g.t.Fatal("uncontrolled content request")
	}
	out := map[string]any{"mode": g.mode, "version": g.version}
	if g.mode == "text" {
		out["text"] = g.text
	}
	if g.mode == "download" {
		out["content_path"] = "https://private.invalid/not-forwarded"
	}
	return map[string]any{"structuredContent": out, "isError": false}, 200, nil
}
func (g *contentGatewayStub) Transfer(_ context.Context, p project.Principal, method, path, query string, body io.Reader, length int64) (*http.Response, int, error) {
	g.transfers++
	if !p.Valid() || method != "GET" || path != "/v2/artifacts/"+g.input.Ref.VersionID+"/content" || body != nil || length != 0 {
		g.t.Fatal("uncontrolled content transfer")
	}
	q, _ := url.ParseQuery(query)
	if q.Get("project_id") != g.input.Access.ProjectID || q.Get("project_revision_id") != g.input.Access.RevisionID || q.Get("store_id") != g.input.Ref.StoreID {
		g.t.Fatal("scope changed")
	}
	return &http.Response{StatusCode: 200, ContentLength: int64(len(g.text)), Header: http.Header{"Content-Type": {g.version.MIME}}, Body: io.NopCloser(strings.NewReader(g.text))}, 200, nil
}

func contentFixture(t *testing.T) (*dashboardServiceFixture, string, *contentSource, *contentGatewayStub) {
	t.Helper()
	f := newDashboardServiceFixture(t)
	ctx, actor, result := f.service.begin(context.Background(), "test-session")
	if result.Status != 200 {
		t.Fatal(result.Status)
	}
	digest := "sha256:" + strings.Repeat("a", 64)
	source := &contentSource{binding: dashboard.Binding{InstallationID: f.installation.InstallationID, AppID: "sample", AppVersion: "1.0.0", StationID: actor.StationID, OrganizationID: actor.OrganizationID,
		ArtifactDigest: digest, Revision: "one", Trusted: true, Grants: []string{"artifact.read"}, Rules: []dashboard.Rule{}, Declaration: dashboard.Declaration{SchemaVersion: 1, AppID: "sample", AppVersion: "1.0.0", Dashboard: &dashboard.Dashboard{
			EntryRef: "dashboard/index.html", ArtifactDigest: digest, Adapter: "wujie", SDKMajor: 1, MinimumSDKVersion: "1.1.0", RequiredCapabilities: []string{"artifact.read"}, OptionalCapabilities: []string{}, AppAPIPolicyRef: "sample-v1", KeepAlive: true,
		}}}}
	f.service.resolver = dashboard.New(source)
	launch, err := f.service.resolver.Open(ctx, actor, f.installation.InstallationID)
	if err != nil {
		t.Fatal(err)
	}
	id := func() string { return uuid.Must(uuid.NewV7()).String() }
	in := DashboardContentInput{Mode: "download", Ref: project.ContentRef{StoreID: id(), ArtifactID: id(), VersionID: id()}, Access: project.Access{ProjectID: id(), RevisionID: id()}}
	hash := sha256.Sum256([]byte("hello"))
	gateway := &contentGatewayStub{t: t, input: in, mode: "download", text: "hello", version: project.ContentVersion{SchemaVersion: 2, ContentRef: in.Ref, OrganizationID: actor.OrganizationID, Size: 5, MIME: "text/plain", SHA256: hex.EncodeToString(hash[:])}}
	return f, launch.DashboardSessionID, source, gateway
}

func TestDashboardContentClosedScopeAndReadModes(t *testing.T) {
	_, _, _, g := contentFixture(t)
	data, _ := json.Marshal(g.input)
	if _, err := DecodeDashboardContent(strings.NewReader(string(data))); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(map[string]any){
		func(v map[string]any) { v["mode"] = "delete" },
		func(v map[string]any) { v["token"] = "forbidden" },
		func(v map[string]any) { v["access"] = map[string]any{} },
		func(v map[string]any) { v["access"].(map[string]any)["asset_id"] = uuid.Must(uuid.NewV7()).String() },
	} {
		var value map[string]any
		_ = json.Unmarshal(data, &value)
		mutate(value)
		encoded, _ := json.Marshal(value)
		if _, err := DecodeDashboardContent(strings.NewReader(string(encoded))); err == nil {
			t.Fatal("invalid capability accepted")
		}
	}
}

func TestDashboardContentHandleRetainsScopeAndRevokesWithPage(t *testing.T) {
	f, session, _, g := contentFixture(t)
	out := f.service.ReadContent(context.Background(), "test-session", session, g.input, g)
	var body struct {
		Path string `json:"content_path"`
	}
	if out.Status != 200 || json.Unmarshal(out.Data, &body) != nil || !strings.HasPrefix(body.Path, "/studio/dashboards/"+session+"/content/") || strings.Contains(string(out.Data), "private.invalid") {
		t.Fatal("unsafe content result", out.Status)
	}
	id := body.Path[strings.LastIndex(body.Path, "/")+1:]
	response, result := f.service.DownloadContent(context.Background(), "test-session", session, id, g)
	if result.Status != 200 {
		t.Fatal(result.Status)
	}
	response.Body.Close()
	if g.transfers != 1 {
		t.Fatal("content not reauthorized")
	}
	f.service.Close(context.Background(), "test-session", session)
	if _, result := f.service.DownloadContent(context.Background(), "test-session", session, id, g); result.Status == 200 || g.transfers != 1 {
		t.Fatal("closed page could use old handle")
	}
}

func TestDashboardContentRejectsChangedGrantAndCorruptText(t *testing.T) {
	f, session, source, g := contentFixture(t)
	g.input.Mode, g.mode = "text", "text"
	if out := f.service.ReadContent(context.Background(), "test-session", session, g.input, g); out.Status != 200 {
		t.Fatal(out.Status)
	}
	g.text = "wrong"
	if out := f.service.ReadContent(context.Background(), "test-session", session, g.input, g); out.Status == 200 {
		t.Fatal("corrupt text accepted")
	}
	source.binding.Grants = []string{}
	if out := f.service.ReadContent(context.Background(), "test-session", session, g.input, g); out.Status == 200 || g.calls != 2 {
		t.Fatal("revoked grant reached content")
	}
}

func TestDashboardContentRejectsCrossSubjectHandleAndWrongLength(t *testing.T) {
	f, session, _, g := contentFixture(t)
	out := f.service.ReadContent(context.Background(), "test-session", session, g.input, g)
	var value struct {
		Path string `json:"content_path"`
	}
	_ = json.Unmarshal(out.Data, &value)
	id := value.Path[strings.LastIndex(value.Path, "/")+1:]
	g.text = "too long"
	if _, result := f.service.DownloadContent(context.Background(), "test-session", session, id, g); result.Status == 200 {
		t.Fatal("length mismatch accepted")
	}
	f.identity.UserID = uuid.Must(uuid.NewV7()).String()
	if _, result := f.service.DownloadContent(context.Background(), "test-session", session, id, g); result.Status == 200 || g.transfers != 1 {
		t.Fatal("another subject used handle")
	}
}
