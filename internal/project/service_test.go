package project_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/verdantflarehub/verdantflare-studio/internal/artifactclient"
	"github.com/verdantflarehub/verdantflare-studio/internal/project"
	"github.com/verdantflarehub/verdantflare-studio/internal/testdb"
	"github.com/verdantflarehub/verdantflare-studio/migrations"
)

func id() string { return uuid.Must(uuid.NewV7()).String() }

type interruptContent struct {
	project.Artifact
	loseWrite, loseRetain atomic.Bool
	afterRetain           func()
}

func (c *interruptContent) Write(ctx context.Context, p project.Principal, id string, w project.TextWrite) (project.ContentVersion, error) {
	v, e := c.Artifact.Write(ctx, p, id, w)
	if e == nil && c.loseWrite.Swap(false) {
		return project.ContentVersion{}, project.ErrDependency
	}
	return v, e
}
func (c *interruptContent) WriteAssetManifest(ctx context.Context, p project.Principal, asset, version string, w project.TextWrite) (project.ContentVersion, error) {
	v, e := c.Artifact.WriteAssetManifest(ctx, p, asset, version, w)
	if e == nil && c.loseWrite.Swap(false) {
		return project.ContentVersion{}, project.ErrDependency
	}
	return v, e
}
func (c *interruptContent) Retain(ctx context.Context, p project.Principal, o project.RetentionOwner, refs []project.ContentRef) error {
	e := c.Artifact.Retain(ctx, p, o, refs)
	if e == nil && c.afterRetain != nil {
		callback := c.afterRetain
		c.afterRetain = nil
		callback()
	}
	if e == nil && c.loseRetain.Swap(false) {
		return project.ErrDependency
	}
	return e
}

type fixture struct {
	s         *project.Service
	db, artDB *pgxpool.Pool
	p         project.Principal
	content   *interruptContent
	active    *atomic.Pointer[project.Service]
	restart   func()
}

func setup(t *testing.T) *fixture {
	t.Helper()
	binary := os.Getenv("ARTIFACT_TEST_BINARY")
	if binary == "" {
		t.Skip("ARTIFACT_TEST_BINARY required for real Studio/Artifact integration")
	}
	if _, e := os.Stat(binary); e != nil {
		t.Fatal(e)
	}
	db := testdb.New(t)
	artDB := testdb.New(t)
	ctx := context.Background()
	if e := migrations.Apply(ctx, db); e != nil {
		t.Fatal(e)
	}
	if e := migrations.Apply(ctx, db); e != nil {
		t.Fatal(e)
	}
	p := project.Principal{OrganizationID: id(), SubjectID: id(), RequestID: id()}
	token, authToken := strings.Repeat("s", 32), strings.Repeat("a", 32)
	active := new(atomic.Pointer[project.Service])
	authority := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s := active.Load()
		if s == nil {
			w.WriteHeader(503)
			return
		}
		h, e := s.AuthorityHandler(authToken)
		if e != nil {
			t.Error(e)
			w.WriteHeader(500)
			return
		}
		h.ServeHTTP(w, r)
	}))
	t.Cleanup(authority.Close)
	l, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	addr := l.Addr().String()
	l.Close()
	env := []string{}
	for _, v := range os.Environ() {
		if !strings.HasPrefix(strings.ToUpper(v), "ARTIFACT_") {
			env = append(env, v)
		}
	}
	env = append(env, "ARTIFACT_DATABASE_URL="+testdb.ConnString(artDB), "ARTIFACT_STORE_ID="+id(), "ARTIFACT_STORAGE_BACKEND=local", "ARTIFACT_CONTENT_ROOT="+filepath.Join(t.TempDir(), "content"), "ARTIFACT_SERVICE_TOKEN="+token, "ARTIFACT_AUTHORITY_TOKEN="+authToken, "ARTIFACT_AUTHORITY_URL="+authority.URL, "ARTIFACT_LISTEN_ADDR="+addr)
	migrateCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	command := exec.CommandContext(migrateCtx, binary, "migrate")
	command.Env = env
	if out, e := command.CombinedOutput(); e != nil {
		t.Fatalf("Artifact migration: %v: %s", e, out)
	}
	client, e := artifactclient.New("http://"+addr, token)
	if e != nil {
		t.Fatal(e)
	}
	content := &interruptContent{Artifact: client}
	service, e := project.NewService(ctx, db, content, nil)
	if e != nil {
		t.Fatal(e)
	}
	active.Store(service)
	var running *exec.Cmd
	var done chan error
	stop := func() {
		if running != nil {
			_ = running.Process.Kill()
			<-done
			running = nil
		}
	}
	start := func() {
		t.Helper()
		stop()
		running = exec.Command(binary, "serve")
		running.Env = env
		var logs bytes.Buffer
		running.Stdout = &logs
		running.Stderr = &logs
		if e := running.Start(); e != nil {
			t.Fatal(e)
		}
		done = make(chan error, 1)
		go func(cmd *exec.Cmd, signal chan error) { signal <- cmd.Wait() }(running, done)
		deadline := time.NewTimer(5 * time.Second)
		defer deadline.Stop()
		tick := time.NewTicker(20 * time.Millisecond)
		defer tick.Stop()
		client := &http.Client{Timeout: 200 * time.Millisecond}
		for {
			select {
			case e := <-done:
				running = nil
				t.Fatalf("Artifact startup: %v: %s", e, logs.String())
			case <-deadline.C:
				t.Fatal("Artifact did not listen")
			case <-tick.C:
				r, _ := http.NewRequest("POST", "http://"+addr+"/v2/artifacts/uploads", strings.NewReader("{}"))
				r.Header.Set("Authorization", "Bearer "+token)
				r.Header.Set("X-User-Id", p.SubjectID)
				r.Header.Set("X-Organization-Id", p.OrganizationID)
				r.Header.Set("X-Request-Id", p.RequestID)
				r.Header.Set("Content-Type", "application/json")
				response, e := client.Do(r)
				if e != nil {
					continue
				}
				io.Copy(io.Discard, response.Body)
				response.Body.Close()
				if response.StatusCode != 400 {
					t.Fatalf("startup probe status %d", response.StatusCode)
				}
				return
			}
		}
	}
	t.Cleanup(stop)
	start()
	return &fixture{service, db, artDB, p, content, active, start}
}
func createRequest() project.CreateRequest {
	return project.CreateRequest{CommitID: id(), Name: "Portable project", Category: "image", EntryPath: "review.md", EntryText: "# Original review"}
}
func count(t *testing.T, db *pgxpool.Pool, query string) int {
	t.Helper()
	var n int
	if e := db.QueryRow(context.Background(), query).Scan(&n); e != nil {
		t.Fatal(e)
	}
	return n
}
func TestCreateRecoveryAcrossPersistentServices(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	r := createRequest()
	f.content.loseWrite.Store(true)
	if _, e := f.s.Create(ctx, f.p, r); !errors.Is(e, project.ErrDependency) {
		t.Fatalf("lost upload result: %v", e)
	}
	list, e := f.s.List(ctx, f.p, project.ListRequest{})
	if e != nil || len(list.Items) != 0 {
		t.Fatal("unfinished project visible", e)
	}
	f.restart() // real Artifact process restart, same immutable storage and database
	f.content.loseRetain.Store(true)
	if _, e := f.s.Create(ctx, f.p, r); !errors.Is(e, project.ErrDependency) {
		t.Fatalf("lost retain result: %v", e)
	}
	if count(t, f.db, "SELECT count(*) FROM studio.project_revisions") != 0 {
		t.Fatal("published before completed preparation")
	}
	pool, e := pgxpool.NewWithConfig(ctx, f.db.Config())
	if e != nil {
		t.Fatal(e)
	}
	defer pool.Close()
	second, e := project.NewService(ctx, pool, f.content, nil)
	if e != nil {
		t.Fatal(e)
	}
	f.active.Store(second)
	result, e := second.Create(ctx, f.p, r)
	if e != nil {
		t.Fatal(e)
	}
	retry, e := second.Create(ctx, f.p, r)
	if e != nil || retry.RevisionID != result.RevisionID || retry.ManifestRef != result.ManifestRef {
		t.Fatal("response loss duplicated revision", e)
	}
	if count(t, f.db, "SELECT count(*) FROM studio.project_revisions") != 1 || count(t, f.artDB, "SELECT count(*) FROM station.artifact_versions") != 2 {
		t.Fatal("retry duplicated content or revision")
	}
	opened, e := second.Open(ctx, f.p, result.ProjectID, "")
	if e != nil || opened.RevisionID != result.RevisionID {
		t.Fatal("new client could not open fixed revision", e)
	}
	entry := opened.Manifest.Files[0]
	text, e := f.content.Read(ctx, f.p, entry.Content, project.Access{ProjectID: result.ProjectID, RevisionID: result.RevisionID}, 1<<20)
	if e != nil || string(text) != r.EntryText {
		t.Fatal("entry content not recoverable", e)
	}
	r.Name = "changed declaration"
	if _, e := second.Create(ctx, f.p, r); !errors.Is(e, project.ErrIdempotency) {
		t.Fatalf("changed create allowed: %v", e)
	}
	if _, e = f.db.Exec(ctx, "UPDATE studio.project_revisions SET manifest_json='{}'"); e == nil {
		t.Fatal("revision mutation allowed")
	}
}
func TestConcurrentEditsAndSelectionInvalidation(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	base, e := f.s.Create(ctx, f.p, createRequest())
	if e != nil {
		t.Fatal(e)
	}
	selections := []project.Selection{{Purpose: "approved-review", FileIDs: []string{base.Manifest.EntryDocumentID}}}
	selected, e := f.s.Commit(ctx, f.p, project.CommitRequest{ProjectID: base.ProjectID, ExpectedRevisionID: base.RevisionID, CommitID: id(), Changes: &project.Changes{Selections: &selections}})
	if e != nil {
		t.Fatal(e)
	}
	entry := selected.Manifest.Files[0]
	nextText := "# Updated review"
	updates := []project.FileUpdate{{ID: entry.ID, Path: entry.Path, Role: entry.Role, Text: &nextText, MIME: "text/markdown"}}
	edited, e := f.s.Commit(ctx, f.p, project.CommitRequest{ProjectID: base.ProjectID, ExpectedRevisionID: selected.RevisionID, CommitID: id(), Changes: &project.Changes{UpsertFiles: &updates}})
	if e != nil {
		t.Fatal(e)
	}
	if len(edited.Manifest.Selections) != 0 || len(edited.Invalidated) != 1 {
		t.Fatal("changed bytes kept prior approval")
	}
	old, e := f.s.Open(ctx, f.p, base.ProjectID, selected.RevisionID)
	if e != nil || len(old.Manifest.Selections) != 1 || old.Manifest.Files[0].Content != entry.Content {
		t.Fatal("history changed", e)
	}
	type outcome struct {
		result  project.Result
		err     error
		request project.CommitRequest
	}
	out := make(chan outcome, 2)
	var wg sync.WaitGroup
	for _, name := range []string{"client A", "client B"} {
		r := project.CommitRequest{ProjectID: base.ProjectID, ExpectedRevisionID: edited.RevisionID, CommitID: id(), Changes: &project.Changes{Metadata: &project.MetadataChange{Name: &name}}}
		wg.Add(1)
		go func() { defer wg.Done(); v, e := f.s.Commit(ctx, f.p, r); out <- outcome{v, e, r} }()
	}
	wg.Wait()
	close(out)
	success, conflict := 0, 0
	for o := range out {
		if o.err == nil {
			success++
			again, e := f.s.Commit(ctx, f.p, o.request)
			if e != nil || again.RevisionID != o.result.RevisionID {
				t.Fatal("committed retry changed identity")
			}
		} else if errors.Is(o.err, project.ErrConflict) {
			conflict++
			status, e := f.s.Status(ctx, f.p, base.ProjectID, o.request.CommitID)
			if e != nil || status.State != "conflict" {
				t.Fatal("conflict not persistent", e)
			}
		} else {
			t.Fatal(o.err)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatalf("CAS outcomes %d/%d", success, conflict)
	}
	if count(t, f.db, "SELECT count(*) FROM studio.project_revisions") != 4 {
		t.Fatal("CAS produced wrong history")
	}
}
func TestLiveProjectGrantsAndAtomicPublication(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	base, e := f.s.Create(ctx, f.p, createRequest())
	if e != nil {
		t.Fatal(e)
	}
	outsider := f.p
	outsider.SubjectID = id()
	list, e := f.s.List(ctx, outsider, project.ListRequest{})
	if e != nil || len(list.Items) != 0 {
		t.Fatal("organization membership exposed projects")
	}
	if _, e = f.s.Open(ctx, outsider, base.ProjectID, ""); !errors.Is(e, project.ErrForbidden) {
		t.Fatalf("outsider open: %v", e)
	}
	otherOrg := f.p
	otherOrg.OrganizationID = id()
	if _, e = f.s.Open(ctx, otherOrg, base.ProjectID, ""); !errors.Is(e, project.ErrForbidden) {
		t.Fatal("cross organization open")
	}
	_, e = f.db.Exec(ctx, `CREATE FUNCTION studio.test_fail_publish() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='project.commit' THEN RAISE EXCEPTION 'injected audit failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER test_fail_publish BEFORE INSERT ON studio.studio_events FOR EACH ROW EXECUTE FUNCTION studio.test_fail_publish()`)
	if e != nil {
		t.Fatal(e)
	}
	name := "After fault"
	req := project.CommitRequest{ProjectID: base.ProjectID, ExpectedRevisionID: base.RevisionID, CommitID: id(), Changes: &project.Changes{Metadata: &project.MetadataChange{Name: &name}}}
	if _, e = f.s.Commit(ctx, f.p, req); e == nil {
		t.Fatal("audit failure did not abort publication")
	}
	status, e := f.s.Status(ctx, f.p, base.ProjectID, req.CommitID)
	if e != nil || status.State != "preparing" {
		t.Fatal("failed transaction not recoverable", e)
	}
	opened, e := f.s.Open(ctx, f.p, base.ProjectID, "")
	if e != nil || opened.RevisionID != base.RevisionID {
		t.Fatal("failed transaction moved head")
	}
	_, e = f.db.Exec(ctx, "UPDATE studio.project_members SET role='reader' WHERE project_id=$1", base.ProjectID)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.s.Commit(ctx, f.p, req); !errors.Is(e, project.ErrForbidden) {
		t.Fatal("revoked writer resumed publication", e)
	}
	_, e = f.db.Exec(ctx, "UPDATE studio.project_members SET role='owner' WHERE project_id=$1;", base.ProjectID)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.db.Exec(ctx, "DROP TRIGGER test_fail_publish ON studio.studio_events"); e != nil {
		t.Fatal(e)
	}
	recovered, e := f.s.Commit(ctx, f.p, req)
	if e != nil || recovered.Manifest.Name != name {
		t.Fatal("could not recover rolled back publication", e)
	}
	owner := project.RetentionOwner{Kind: "project_revision", ID: recovered.RevisionID, CommitID: req.CommitID}
	if e = f.s.AuthorizeArtifact(ctx, f.p, project.Permission{Action: "release", Owner: &owner}); !errors.Is(e, project.ErrForbidden) {
		t.Fatal("live revision retention releasable")
	}
	_, e = f.db.Exec(ctx, "DELETE FROM studio.project_members WHERE project_id=$1", base.ProjectID)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.content.Metadata(ctx, f.p, recovered.ManifestRef, project.Access{ProjectID: base.ProjectID, RevisionID: recovered.RevisionID}); !errors.Is(e, project.ErrForbidden) {
		t.Fatal("revoked member fetched content", e)
	}
}

func TestHTTPClientsAndCentralContracts(t *testing.T) {
	f := setup(t)
	token, authorityToken := strings.Repeat("p", 32), strings.Repeat("a", 32)
	handler, e := f.s.HTTPHandler(token, authorityToken)
	if e != nil {
		t.Fatal(e)
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	clients := []*http.Client{{Timeout: 30 * time.Second}, {Timeout: 30 * time.Second}}
	seq := 0
	evidence := func(schema string, data []byte) {
		if dir := os.Getenv("STUDIO_PROJECT_CONTRACT_EVIDENCE_DIR"); dir != "" {
			if e := os.MkdirAll(dir, 0700); e != nil {
				t.Fatal(e)
			}
			seq++
			if e := os.WriteFile(filepath.Join(dir, fmt.Sprintf("%03d.%s.json", seq, schema)), data, 0600); e != nil {
				t.Fatal(e)
			}
		}
	}
	body := func(v any) []byte {
		b, e := json.Marshal(v)
		if e != nil {
			t.Fatal(e)
		}
		return b
	}
	call := func(client int, action string, data []byte, status int, mutate func(*http.Request)) []byte {
		t.Helper()
		r, _ := http.NewRequest("POST", server.URL+"/internal/v1/project/"+action, bytes.NewReader(data))
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-User-Id", f.p.SubjectID)
		r.Header.Set("X-Organization-Id", f.p.OrganizationID)
		r.Header.Set("X-Request-Id", f.p.RequestID)
		if mutate != nil {
			mutate(r)
		}
		response, e := clients[client].Do(r)
		if e != nil {
			t.Fatal(e)
		}
		defer response.Body.Close()
		out, e := io.ReadAll(response.Body)
		if e != nil {
			t.Fatal(e)
		}
		if response.StatusCode != status {
			t.Fatalf("%s: got %d want %d: %s", action, response.StatusCode, status, out)
		}
		if status >= 400 {
			evidence("project-error", out)
		} else {
			req := map[string]string{"create": "project-create-request", "commit": "project-commit-request", "open": "project-open-request", "list": "project-list-request", "commit_status": "project-status-request"}[action]
			result := map[string]string{"create": "project-result", "commit": "project-result", "open": "project-open-result", "list": "project-list-result", "commit_status": "project-status"}[action]
			evidence(req, data)
			evidence(result, out)
		}
		return out
	}
	req := createRequest()
	valid := body(req)
	call(0, "create", valid, 403, func(r *http.Request) { r.Header.Del("Authorization") })
	call(0, "create", valid, 403, func(r *http.Request) { r.Header.Add("X-User-Id", id()) })
	call(0, "create", []byte(strings.Replace(string(valid), `"name":`, `"Name":`, 1)), 400, nil)
	call(0, "create", []byte(strings.Replace(string(valid), `"name":`, `"name":"duplicate","name":`, 1)), 400, nil)
	var created project.Result
	if e := json.Unmarshal(call(0, "create", valid, 200, nil), &created); e != nil {
		t.Fatal(e)
	}
	var opened project.OpenResult
	if e := json.Unmarshal(call(1, "open", body(project.OpenRequest{ProjectID: created.ProjectID}), 200, nil), &opened); e != nil {
		t.Fatal(e)
	}
	if opened.ManifestRef != created.ManifestRef {
		t.Fatal("second HTTP client opened different content")
	}
	call(1, "open", body(project.OpenRequest{ProjectID: created.ProjectID, RevisionID: created.RevisionID, RequireWrite: true}), 200, nil)
	var editable project.ListResult
	if e := json.Unmarshal(call(1, "list", body(project.ListRequest{RequireWrite: true}), 200, nil), &editable); e != nil || len(editable.Items) != 1 || editable.Items[0].ProjectID != created.ProjectID {
		t.Fatal("owner project absent from writable list")
	}
	if _, e := f.db.Exec(context.Background(), "UPDATE studio.project_members SET role='reader' WHERE project_id=$1 AND subject_id=$2", created.ProjectID, f.p.SubjectID); e != nil {
		t.Fatal(e)
	}
	call(1, "open", body(project.OpenRequest{ProjectID: created.ProjectID, RevisionID: created.RevisionID}), 200, nil)
	call(1, "open", body(project.OpenRequest{ProjectID: created.ProjectID, RevisionID: created.RevisionID, RequireWrite: true}), 403, nil)
	if e := json.Unmarshal(call(1, "list", body(project.ListRequest{RequireWrite: true}), 200, nil), &editable); e != nil || len(editable.Items) != 0 {
		t.Fatal("reader project in writable list")
	}
	if e := json.Unmarshal(call(1, "list", body(project.ListRequest{}), 200, nil), &editable); e != nil || len(editable.Items) != 1 {
		t.Fatal("default readable list changed")
	}
	if _, e := f.db.Exec(context.Background(), "UPDATE studio.project_members SET role='editor' WHERE project_id=$1 AND subject_id=$2", created.ProjectID, f.p.SubjectID); e != nil {
		t.Fatal(e)
	}
	if e := json.Unmarshal(call(1, "list", body(project.ListRequest{RequireWrite: true}), 200, nil), &editable); e != nil || len(editable.Items) != 1 {
		t.Fatal("editor project absent from writable list")
	}
	if _, e := f.db.Exec(context.Background(), "UPDATE studio.project_members SET role='owner' WHERE project_id=$1 AND subject_id=$2", created.ProjectID, f.p.SubjectID); e != nil {
		t.Fatal(e)
	}
	m := opened.Manifest
	localID := id()
	m.Files = append(m.Files, project.File{ID: localID, Path: "notes.md", Role: "review", Content: m.Files[0].Content})
	m.Extensions = map[string]json.RawMessage{"example.note": json.RawMessage(`null`)}
	m.Selections = []project.Selection{{Purpose: "copy-reference", FileIDs: []string{localID}}}
	commit := project.CommitRequest{ProjectID: created.ProjectID, ExpectedRevisionID: created.RevisionID, CommitID: id(), Manifest: &m}
	var updated project.Result
	if e := json.Unmarshal(call(1, "commit", body(commit), 200, nil), &updated); e != nil {
		t.Fatal(e)
	}
	if updated.Manifest.Files[1].ID == localID || updated.Manifest.Selections[0].FileIDs[0] != updated.Manifest.Files[1].ID {
		t.Fatal("new manifest file IDs not allocated consistently")
	}
	if string(updated.Manifest.Extensions["example.note"]) != "null" {
		t.Fatal("opaque extension changed")
	}
	call(0, "list", body(project.ListRequest{Limit: 1}), 200, nil)
	call(0, "commit_status", body(project.StatusRequest{ProjectID: created.ProjectID, CommitID: commit.CommitID}), 200, nil)
	stale := project.CommitRequest{ProjectID: created.ProjectID, ExpectedRevisionID: created.RevisionID, CommitID: id(), Changes: &project.Changes{}}
	failure := call(0, "commit", body(stale), 409, nil)
	var errBody struct {
		Details map[string]string `json:"details"`
	}
	if json.Unmarshal(failure, &errBody) != nil || errBody.Details["current_revision_id"] != updated.RevisionID {
		t.Fatal("conflict omitted current revision")
	}
	call(0, "commit_status", body(project.StatusRequest{ProjectID: created.ProjectID, CommitID: stale.CommitID}), 200, nil)
	call(1, "open", body(project.OpenRequest{ProjectID: created.ProjectID}), 403, func(r *http.Request) { r.Header.Set("X-User-Id", id()) })
}
