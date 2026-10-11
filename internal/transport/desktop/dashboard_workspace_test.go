package desktop

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/verdantflarehub/verdantflare-studio/internal/application"
	"github.com/verdantflarehub/verdantflare-studio/internal/dashboard"
	"github.com/verdantflarehub/verdantflare-studio/internal/project"
	"github.com/verdantflarehub/verdantflare-studio/internal/workspace"
)

const localDashboardID = "fd853794-1600-4a93-8332-fbf95faf9a5e"
const localProjectID = "01900000-0000-7000-8000-000000000001"

type workspaceFakeSession struct {
	binding application.DashboardOpened
	err     error
	calls   int
	remote  workspace.Remote
}

func (s *workspaceFakeSession) Inspect(context.Context, string) (application.DashboardOpened, error) {
	s.calls++
	return s.binding, s.err
}
func (s *workspaceFakeSession) Remote() workspace.Remote {
	if s.remote != nil {
		return s.remote
	}
	panic("filesystem or business remote must not be reached")
}

type localWorkspaceRemote struct{ opened project.OpenResult }

func (r localWorkspaceRemote) Open(context.Context, string, string) (project.OpenResult, error) {
	return r.opened, nil
}
func (r localWorkspaceRemote) Metadata(_ context.Context, ref project.ContentRef, _ project.Access) (project.ContentVersion, error) {
	return project.ContentVersion{ContentRef: ref, Size: 0, SHA256: "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", MIME: "text/markdown"}, nil
}
func (r localWorkspaceRemote) Download(context.Context, project.ContentRef, project.Access, io.Writer, int64) error {
	return project.ErrDependency
}
func (r localWorkspaceRemote) Commit(context.Context, project.CommitRequest) (project.Result, error) {
	return project.Result{}, project.ErrDependency
}

func TestDashboardWorkspaceReauthorizationReleasesOldLockAndPreservesPending(t *testing.T) {
	id := func(n string) string { return "01900000-0000-7000-8000-00000000000" + n }
	ref := project.ContentRef{StoreID: id("3"), ArtifactID: id("4"), VersionID: id("5")}
	manifest := project.Manifest{SchemaVersion: 1, Kind: "project", ProjectID: localProjectID, Name: "Workspace", Category: "image", Status: "draft", EntryDocumentID: id("2"), Files: []project.File{{ID: id("2"), Path: "review.md", Role: "review", Content: ref}}, Selections: []project.Selection{}, AssetRefs: []project.AssetRef{}, RunRefs: []project.RunRef{}, DomainDocuments: []project.DomainDocument{}}
	remote := localWorkspaceRemote{project.OpenResult{Result: project.Result{ProjectID: localProjectID, RevisionID: id("6"), ManifestRef: ref, Manifest: manifest, Invalidated: []string{}}, HeadRevisionID: id("6")}}
	directory := t.TempDir()
	old := &workspaceFakeSession{binding: workspaceBinding(), remote: remote}
	current := old
	bridge := NewDashboardWorkspaces(nil, WorkspaceDialogFuncs{ChooseDirectory: func(context.Context) (string, error) { return directory, nil }})
	defer bridge.Close()
	bridge.bind = func() (workspaceGrantSession, error) { return current, nil }
	first, err := bridge.execute(context.Background(), localDashboardID, dashboardWorkspaceRequest{Operation: "openDirectory", ProjectID: localProjectID})
	if err != nil {
		t.Fatal(err)
	}
	firstHandle := first.(map[string]any)["workspace_handle"].(string)
	marker := []byte(`{"retained":"original unresolved operation"}`)
	pendingPath := filepath.Join(directory, ".vf", "pending.json")
	if err = os.WriteFile(pendingPath, marker, 0600); err != nil {
		t.Fatal(err)
	}
	old.err = project.ErrForbidden
	current = &workspaceFakeSession{binding: workspaceBinding(), remote: remote}
	current.binding.Launch.DashboardSessionID = "c710693c-d83d-4723-8757-20d5842c31b6"
	second, err := bridge.execute(context.Background(), current.binding.Launch.DashboardSessionID, dashboardWorkspaceRequest{Operation: "openDirectory", ProjectID: localProjectID})
	if err != nil {
		t.Fatalf("reauthorized directory still locked: %v", err)
	}
	if second.(map[string]any)["workspace_handle"] == firstHandle || bridge.handles[firstHandle] != nil {
		t.Fatal("old handle survived new authorization")
	}
	got, err := os.ReadFile(pendingPath)
	if err != nil || string(got) != string(marker) {
		t.Fatal("recovery record changed", err)
	}
}
func workspaceBinding() application.DashboardOpened {
	return application.DashboardOpened{Launch: dashboard.Launch{DashboardSessionID: localDashboardID, InstallationID: "install", AppID: "project", AppVersion: "0.1.0", ArtifactDigest: "sha256:" + strings.Repeat("a", 64), ContextRevision: "revision-a", ExpiresAt: time.Now().Add(time.Hour), GrantedCapabilities: []string{"workspace.local"}}, Context: application.DashboardContext{UserID: "user", OrganizationID: "organization", StationID: "station", AppID: "project", InstallationID: "install", ContextRevision: "revision-a"}}
}
func TestDashboardWorkspaceClosedRequestShape(t *testing.T) {
	for _, raw := range []string{
		`{"operation":"openDirectory","project_id":"` + localProjectID + `","directory":"C:/private"}`,
		`{"operation":"openDirectory","project_id":"` + localProjectID + `","workspace_handle":"fd853794-1600-4a93-8332-fbf95faf9a5e"}`,
		`{"operation":"importFiles","workspace_handle":"fd853794-1600-4a93-8332-fbf95faf9a5e","role":"reference","source_path":"C:/private"}`,
		`{"operation":"saveFiles","workspace_handle":"fd853794-1600-4a93-8332-fbf95faf9a5e","files":[{"path":"../private"}]}`,
		`{"operation":"execute","command":"anything"}`,
		`{"operation":"status","operation":"close","workspace_handle":"fd853794-1600-4a93-8332-fbf95faf9a5e"}`,
	} {
		if _, err := decodeDashboardWorkspace([]byte(raw)); !errors.Is(err, project.ErrInvalid) {
			t.Fatalf("unsafe request accepted: %v", err)
		}
	}
	if _, err := decodeDashboardWorkspace([]byte(`{"operation":"openDirectory","project_id":"` + localProjectID + `"}`)); err != nil {
		t.Fatal(err)
	}
}
func TestDashboardWorkspaceRechecksGrantAfterNativeSelection(t *testing.T) {
	session := &workspaceFakeSession{binding: workspaceBinding()}
	directory := t.TempDir()
	selected := 0
	bridge := NewDashboardWorkspaces(nil, WorkspaceDialogFuncs{ChooseDirectory: func(context.Context) (string, error) {
		selected++
		session.binding.Launch.GrantedCapabilities = nil
		return directory, nil
	}})
	bridge.bind = func() (workspaceGrantSession, error) { return session, nil }
	_, err := bridge.execute(context.Background(), localDashboardID, dashboardWorkspaceRequest{Operation: "openDirectory", ProjectID: localProjectID})
	if !errors.Is(err, project.ErrForbidden) || selected != 1 {
		t.Fatal("grant was not rechecked after dialog", err)
	}
	if _, err = os.Stat(filepath.Join(directory, ".vf")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("revoked dialog initialized local files")
	}
	session.binding = workspaceBinding()
	session.binding.Context.OrganizationID = "different"
	entry := &dashboardWorkspaceEntry{session: session, binding: workspaceBinding()}
	if err = entry.authorize(context.Background()); !errors.Is(err, project.ErrForbidden) {
		t.Fatal("handle followed another authority scope")
	}
}
func TestDashboardWorkspaceHTTPRejectsExternalOriginAndCallerCredentials(t *testing.T) {
	bridge := NewDashboardWorkspaces(nil, nil)
	bridge.bind = func() (workspaceGrantSession, error) {
		t.Fatal("untrusted request reached session")
		return nil, project.ErrForbidden
	}
	h := bridge.Wrap(http.NotFoundHandler())
	for key, value := range map[string]string{"Origin": "https://outside.invalid", "Cookie": "session=caller", "Authorization": "Bearer caller", "Sec-Fetch-Site": "cross-site"} {
		req := httptest.NewRequest("POST", "http://studio.local/studio/dashboards/"+localDashboardID+"/capabilities/workspace.local", strings.NewReader(`{"operation":"openDirectory","project_id":"`+localProjectID+`"}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set(key, value)
		out := httptest.NewRecorder()
		h.ServeHTTP(out, req)
		if out.Code != 400 && out.Code != 403 {
			t.Fatal("origin or credential accepted", out.Code)
		}
	}
}
func TestDashboardWorkspaceSanitizesPrivateFilesystemFailures(t *testing.T) {
	result := workspaceCapabilityFailure(&os.PathError{Op: "open", Path: "C:/private/user/file", Err: os.ErrPermission})
	var body map[string]string
	if json.Unmarshal(result.Data, &body) != nil || body["code"] != "DEPENDENCY_UNAVAILABLE" || strings.Contains(string(result.Data), "private") {
		t.Fatal("filesystem path exposed")
	}
}
