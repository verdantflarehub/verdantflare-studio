package dashboard

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func workspaceEntryFixture() InstanceEntry {
	return InstanceEntry{EntryID: "editor", PublicAssets: true, ResolvePath: "/internal/workspace/resolve", EntryRef: "vf/workspace.html",
		IndexPath: "/internal/workspace-index", AssetPath: "/internal/workspace/assets/", ArtifactDigest: "sha256:" + strings.Repeat("b", 64),
		Rules: []Rule{{Method: "POST", Path: "/draft", UpstreamPath: "/internal/workspace/instances/:instanceId/draft", MaxBody: 1 << 20}}}
}

type workspaceFixture struct {
	binding  Binding
	resolve  func()
	lookup   func()
	resolved []InstanceOpen
}

func (f *workspaceFixture) Lookup(context.Context, Actor, string) (Binding, error) {
	if f.lookup != nil {
		f.lookup()
	}
	return copyBinding(f.binding), nil
}
func (f *workspaceFixture) ResolveInstance(_ context.Context, _ Actor, _ Binding, _ InstanceEntry, in InstanceOpen) error {
	f.resolved = append(f.resolved, in)
	if f.resolve != nil {
		f.resolve()
	}
	return nil
}
func workspaceFixtureFor() (Actor, *workspaceFixture, *Workspaces) {
	actor, binding := fixture()
	binding.InstanceEntries = []InstanceEntry{workspaceEntryFixture()}
	source := &workspaceFixture{binding: binding}
	return actor, source, NewWorkspaces(source)
}

func TestWorkspaceSeparatesTwoInstancesAndOutlivesDashboard(t *testing.T) {
	actor, source, workspaces := workspaceFixtureFor()
	dashboards := New(source)
	opener := openFixture(t, dashboards, actor)
	one, err := workspaces.Open(context.Background(), actor, "install-a", InstanceOpen{"instance-one", "editor"})
	if err != nil {
		t.Fatal(err)
	}
	two, err := workspaces.Open(context.Background(), actor, "install-a", InstanceOpen{"instance-two", "editor"})
	if err != nil {
		t.Fatal(err)
	}
	if one.WorkspaceSessionID == two.WorkspaceSessionID || one.InstanceID == two.InstanceID {
		t.Fatal("instance bindings shared")
	}
	_ = dashboards.Close(actor, opener.DashboardSessionID)
	checked, _, _, err := workspaces.Check(context.Background(), actor, one.WorkspaceSessionID)
	if err != nil || checked != one {
		t.Fatal("dashboard close affected independent workspace", err)
	}
	_ = workspaces.Close(actor, one.WorkspaceSessionID)
	if _, _, _, err = workspaces.Check(context.Background(), actor, two.WorkspaceSessionID); err != nil {
		t.Fatal(err)
	}
	if len(source.resolved) != 2 {
		t.Fatal("close or read triggered another instance operation")
	}
}

func TestWorkspaceRejectsForeignActorUndeclaredEntryAndRevokedGrant(t *testing.T) {
	actor, source, workspaces := workspaceFixtureFor()
	for _, in := range []InstanceOpen{{"one", "other"}, {"../one", "editor"}, {"one", "https://other.invalid"}} {
		if _, err := workspaces.Open(context.Background(), actor, "install-a", in); err == nil {
			t.Fatal("invalid entry accepted")
		}
	}
	if len(source.resolved) != 0 {
		t.Fatal("invalid request reached App")
	}
	launch, err := workspaces.Open(context.Background(), actor, "install-a", InstanceOpen{"one", "editor"})
	if err != nil {
		t.Fatal(err)
	}
	foreign := actor
	foreign.UserID = "other"
	if _, _, _, err = workspaces.Check(context.Background(), foreign, launch.WorkspaceSessionID); !errors.Is(err, ErrDenied) {
		t.Fatal(err)
	}
	if err = workspaces.Close(foreign, launch.WorkspaceSessionID); !errors.Is(err, ErrDenied) {
		t.Fatal(err)
	}
	source.binding.InstanceEntries = nil
	if _, _, _, err = workspaces.Check(context.Background(), actor, launch.WorkspaceSessionID); !errors.Is(err, ErrUnsupported) {
		t.Fatal(err)
	}
	source.binding.InstanceEntries = []InstanceEntry{workspaceEntryFixture()}
	if _, _, _, err = workspaces.Check(context.Background(), actor, launch.WorkspaceSessionID); !errors.Is(err, ErrExpired) {
		t.Fatal("revoked workspace revived", err)
	}
}

func TestWorkspaceRechecksBindingAfterResolutionAndCloseDuringRead(t *testing.T) {
	actor, source, workspaces := workspaceFixtureFor()
	source.resolve = func() { source.binding.InstanceEntries[0].ArtifactDigest = "sha256:" + strings.Repeat("c", 64) }
	if _, err := workspaces.Open(context.Background(), actor, "install-a", InstanceOpen{"one", "editor"}); !errors.Is(err, ErrExpired) {
		t.Fatal(err)
	}
	if len(workspaces.sessions) != 0 {
		t.Fatal("stale open leaked")
	}
	source.resolve = nil
	launch, err := workspaces.Open(context.Background(), actor, "install-a", InstanceOpen{"one", "editor"})
	if err != nil {
		t.Fatal(err)
	}
	source.lookup = func() { _ = workspaces.Close(actor, launch.WorkspaceSessionID) }
	if _, _, _, err = workspaces.Check(context.Background(), actor, launch.WorkspaceSessionID); !errors.Is(err, ErrExpired) {
		t.Fatal("closed during lookup", err)
	}
}

func TestWorkspaceExpiryCapacityAndRegistrationConstraints(t *testing.T) {
	actor, _, workspaces := workspaceFixtureFor()
	now := time.Now()
	workspaces.now = func() time.Time { return now }
	for range 128 {
		if _, err := workspaces.Open(context.Background(), actor, "install-a", InstanceOpen{"one", "editor"}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := workspaces.Open(context.Background(), actor, "install-a", InstanceOpen{"one", "editor"}); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	now = now.Add(15 * time.Minute)
	if _, err := workspaces.Open(context.Background(), actor, "install-a", InstanceOpen{"one", "editor"}); err != nil || len(workspaces.sessions) != 1 {
		t.Fatal("expired slots not reclaimed", err)
	}
	for _, mutate := range []func(*InstanceEntry){
		func(e *InstanceEntry) { e.AssetPath = "https://other.invalid/" },
		func(e *InstanceEntry) { e.ResolvePath += "?target=other" },
		func(e *InstanceEntry) { e.EntryRef = "../secrets.html" },
		func(e *InstanceEntry) { e.Rules[0].Path = "/:instanceId/draft" },
		func(e *InstanceEntry) { e.Rules[0].UpstreamPath = "/internal/:unbound/draft" },
	} {
		entry := workspaceEntryFixture()
		mutate(&entry)
		if validInstanceEntries([]InstanceEntry{entry}) {
			t.Fatal("unsafe registration accepted")
		}
	}
}
