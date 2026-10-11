package application

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/verdantflarehub/verdantflare-studio/internal/dashboard"
)

type workspaceApplicationSource struct {
	service *Dashboards
	binding dashboard.Binding
	resolve func(dashboard.Actor)
}

func (s *workspaceApplicationSource) Lookup(ctx context.Context, actor dashboard.Actor, id string) (dashboard.Binding, error) {
	// Exercise the real service's private credential and installation checks.
	if _, err := s.service.readInstallation(ctx, actor, id); err != nil {
		return dashboard.Binding{}, err
	}
	return s.binding, nil
}
func (s *workspaceApplicationSource) ResolveInstance(_ context.Context, actor dashboard.Actor, _ dashboard.Binding, _ dashboard.InstanceEntry, _ dashboard.InstanceOpen) error {
	if s.resolve != nil {
		s.resolve(actor)
	}
	return nil
}
func newWorkspaceApplicationFixture(t *testing.T) (*dashboardServiceFixture, *workspaceApplicationSource, string) {
	t.Helper()
	f := newDashboardServiceFixture(t)
	hash := "sha256:" + strings.Repeat("a", 64)
	source := &workspaceApplicationSource{service: f.service, binding: dashboard.Binding{
		InstallationID: f.installation.InstallationID, AppID: "sample", AppVersion: "1.0.0", StationID: f.identity.StationID, OrganizationID: f.identity.OrganizationID,
		ArtifactDigest: hash, Revision: "test-revision", Trusted: true, Grants: []string{"instances.open"}, Rules: []dashboard.Rule{},
		Declaration: dashboard.Declaration{SchemaVersion: 1, AppID: "sample", AppVersion: "1.0.0", Dashboard: &dashboard.Dashboard{
			EntryRef: "dashboard/index.html", ArtifactDigest: hash, Adapter: "wujie", SDKMajor: 1, MinimumSDKVersion: "1.0.0",
			RequiredCapabilities: []string{"instances.open"}, OptionalCapabilities: []string{}, AppAPIPolicyRef: "sample-v1", KeepAlive: true}},
		InstanceEntries: []dashboard.InstanceEntry{{EntryID: "editor", PublicAssets: true, ResolvePath: "/internal/resolve", EntryRef: "vf/workspace.html",
			IndexPath: "/internal/workspace-index", AssetPath: "/internal/workspace/", ArtifactDigest: hash, Rules: []dashboard.Rule{}}},
	}}
	f.service.resolver = dashboard.New(source)
	f.service.workspaces = dashboard.NewWorkspaces(source)
	out := f.service.Open(context.Background(), "test-session", f.installation.InstallationID)
	var opened DashboardOpened
	if out.Status != 200 || json.Unmarshal(out.Data, &opened) != nil {
		t.Fatal("cannot open fixture", out.Status)
	}
	return f, source, opened.Launch.DashboardSessionID
}

func TestDashboardInstanceOpenSurvivesDashboardCloseButRechecksCoreIdentity(t *testing.T) {
	f, _, opener := newWorkspaceApplicationFixture(t)
	ctx := context.Background()
	out := f.service.OpenInstance(ctx, "test-session", opener, dashboard.InstanceOpen{InstanceID: "one", EntryID: "editor"})
	var workspace dashboard.InstanceWorkspaceLaunch
	if out.Status != 200 || json.Unmarshal(out.Data, &workspace) != nil {
		t.Fatal(out.Status)
	}
	if out = f.service.Close(ctx, "test-session", opener); out.Status != 204 {
		t.Fatal(out.Status)
	}
	if out = f.service.InspectWorkspace(ctx, "test-session", workspace.WorkspaceSessionID); out.Status != 200 {
		t.Fatal("dashboard closed workspace", out.Status)
	}
	f.identityStatus = 401
	if out = f.service.InspectWorkspace(ctx, "test-session", workspace.WorkspaceSessionID); out.Status != 401 {
		t.Fatal("workspace retained revoked identity", out.Status)
	}
}

func TestDashboardClosingOpenerDuringResolutionRejectsUnpublishedWorkspace(t *testing.T) {
	f, source, opener := newWorkspaceApplicationFixture(t)
	source.resolve = func(actor dashboard.Actor) { _ = f.service.resolver.Close(actor, opener) }
	out := f.service.OpenInstance(context.Background(), "test-session", opener, dashboard.InstanceOpen{InstanceID: "one", EntryID: "editor"})
	if out.Status != 410 {
		t.Fatal("late workspace open published", out.Status)
	}
}
