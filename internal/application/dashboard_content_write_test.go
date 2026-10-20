package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/verdantflarehub/verdantflare-studio/internal/project"
	"io"
	"net/http"
	"strings"
	"testing"
)

type textContentGateway struct {
	input   DashboardTextWrite
	calls   int
	modify  func(*project.ContentVersion)
	after   func()
	failure string
}

func (g *textContentGateway) CallVerified(_ context.Context, name string, args map[string]any, p project.Principal, scope string) (any, int, error) {
	g.calls++
	if name != "artifact.write" || !p.Valid() || scope != g.input.Source.ProjectID {
		return nil, 403, nil
	}
	data, _ := json.Marshal(args)
	in, err := DecodeDashboardTextWrite(strings.NewReader(string(data)))
	if err != nil || in != g.input {
		return nil, 400, nil
	}
	if g.failure != "" {
		return map[string]any{"isError": true, "structuredContent": map[string]string{"code": g.failure}}, 200, nil
	}
	hash := sha256.Sum256([]byte(in.Text))
	id := func() string { return uuid.Must(uuid.NewV7()).String() }
	version := project.ContentVersion{SchemaVersion: 2, ContentRef: project.ContentRef{StoreID: id(), ArtifactID: id(), VersionID: id()}, OrganizationID: p.OrganizationID, Source: in.Source, SHA256: hex.EncodeToString(hash[:]), Size: int64(len(in.Text)), MIME: in.MIME}
	if g.modify != nil {
		g.modify(&version)
	}
	if g.after != nil {
		g.after()
	}
	return map[string]any{"structuredContent": map[string]any{"version": version}}, 200, nil
}
func (*textContentGateway) Transfer(context.Context, project.Principal, string, string, string, io.Reader, int64) (*http.Response, int, error) {
	return nil, 403, nil
}
func TestDashboardContentWriteClosedShape(t *testing.T) {
	in := DashboardTextWrite{Mode: "text", WriteID: uuid.Must(uuid.NewV7()).String(), Source: project.Source{Kind: "user_edit", WorldDraftID: uuid.Must(uuid.NewV7()).String()}, MIME: "application/json", Text: `{"value":"saved"}`}
	raw, _ := json.Marshal(in)
	if _, err := DecodeDashboardTextWrite(strings.NewReader(string(raw))); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(map[string]any){
		func(v map[string]any) { v["mode"] = "prepare" }, func(v map[string]any) { v["token"] = "not-permitted" }, func(v map[string]any) { v["mime"] = "video/mp4" },
		func(v map[string]any) { v["text"] = strings.Repeat("字", 400000) }, func(v map[string]any) { v["source"].(map[string]any)["kind"] = "task_output" },
		func(v map[string]any) { v["source"].(map[string]any)["project_id"] = uuid.Must(uuid.NewV7()).String() },
		func(v map[string]any) {
			v["source"].(map[string]any)["world_draft_revision_id"] = uuid.Must(uuid.NewV7()).String()
		}, func(v map[string]any) { v["source"] = map[string]any{"kind": "user_edit"} },
	} {
		var v map[string]any
		_ = json.Unmarshal(raw, &v)
		mutate(v)
		b, _ := json.Marshal(v)
		if _, err := DecodeDashboardTextWrite(strings.NewReader(string(b))); err == nil {
			t.Fatal("unsafe write accepted")
		}
	}
}
func TestDashboardContentWriteRequiresOwnGrantAndChecksReceipt(t *testing.T) {
	f, session, source, _ := contentFixture(t)
	in := DashboardTextWrite{Mode: "text", WriteID: uuid.Must(uuid.NewV7()).String(), Source: project.Source{Kind: "user_edit", WorldDraftID: uuid.Must(uuid.NewV7()).String()}, MIME: "text/plain", Text: "实际收录正文"}
	g := &textContentGateway{input: in}
	if result := f.service.WriteContent(context.Background(), "test-session", session, in, g); result.Status == 200 || g.calls != 0 {
		t.Fatal("read grant allowed write")
	}
	source.binding.Grants = append(source.binding.Grants, "artifact.write")
	source.binding.Declaration.Dashboard.OptionalCapabilities = []string{"artifact.write"}
	ctx, actor, _ := f.service.begin(context.Background(), "test-session")
	launch, err := f.service.resolver.Open(ctx, actor, f.installation.InstallationID)
	if err != nil {
		t.Fatal(err)
	}
	session = launch.DashboardSessionID
	if result := f.service.WriteContent(ctx, "test-session", session, in, g); result.Status != 200 {
		t.Fatal(result.Status, string(result.Data))
	}
	for _, modify := range []func(*project.ContentVersion){func(v *project.ContentVersion) { v.SHA256 = strings.Repeat("0", 64) }, func(v *project.ContentVersion) { v.OrganizationID = uuid.Must(uuid.NewV7()).String() }, func(v *project.ContentVersion) { v.Source.WorldDraftID = uuid.Must(uuid.NewV7()).String() }, func(v *project.ContentVersion) { v.Size++ }, func(v *project.ContentVersion) { v.MIME = "text/html" }} {
		g.modify = modify
		if result := f.service.WriteContent(ctx, "test-session", session, in, g); result.Status == 200 {
			t.Fatal("invalid write receipt accepted")
		}
	}
	g.modify = nil
	g.failure = "IDEMPOTENCY_CONFLICT"
	if result := f.service.WriteContent(ctx, "test-session", session, in, g); result.Status != 409 || !strings.Contains(string(result.Data), "IDEMPOTENCY_CONFLICT") {
		t.Fatal("lost idempotency failure", result.Status, string(result.Data))
	}
	g.failure = ""
	g.after = func() { source.binding.Grants = []string{} }
	if result := f.service.WriteContent(ctx, "test-session", session, in, g); result.Status == 200 {
		t.Fatal("revoked page received write response")
	}
}
