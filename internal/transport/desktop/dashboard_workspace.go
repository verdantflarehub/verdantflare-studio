package desktop

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/verdantflarehub/verdantflare-studio/internal/application"
	"github.com/verdantflarehub/verdantflare-studio/internal/project"
	"github.com/verdantflarehub/verdantflare-studio/internal/strictjson"
	"github.com/verdantflarehub/verdantflare-studio/internal/studioremote"
	"github.com/verdantflarehub/verdantflare-studio/internal/workspace"
)

// WorkspaceDialogs is injected by the native host at startup. Paths never cross
// the Dashboard API boundary in either direction.
type WorkspaceDialogs interface {
	Directory(context.Context) (string, error)
	Files(context.Context) ([]string, error)
}
type WorkspaceDialogFuncs struct {
	ChooseDirectory func(context.Context) (string, error)
	ChooseFiles     func(context.Context) ([]string, error)
}

func (d WorkspaceDialogFuncs) Directory(ctx context.Context) (string, error) {
	if d.ChooseDirectory == nil {
		return "", project.ErrDependency
	}
	return d.ChooseDirectory(ctx)
}
func (d WorkspaceDialogFuncs) Files(ctx context.Context) ([]string, error) {
	if d.ChooseFiles == nil {
		return nil, project.ErrDependency
	}
	return d.ChooseFiles(ctx)
}

type workspaceGrantSession interface {
	Inspect(context.Context, string) (application.DashboardOpened, error)
	Remote() workspace.Remote
}
type remoteWorkspaceGrant struct {
	service *Service
	session *studioremote.Session
}

func (s remoteWorkspaceGrant) Inspect(ctx context.Context, id string) (application.DashboardOpened, error) {
	var out application.DashboardOpened
	response, err := s.session.Send(ctx, "GET", "/studio/dashboards/"+id, nil)
	if err != nil {
		return out, remoteWorkspaceError(err)
	}
	if response.Status != 200 {
		if response.Status == 401 || response.Status == 403 || response.Status == 404 || response.Status == 409 {
			return out, project.ErrForbidden
		}
		return out, gatewayError(response.Status)
	}
	if strictjson.Decode(bytes.NewReader(response.Data), 64<<10, &out) != nil {
		return out, project.ErrDependency
	}
	return out, nil
}
func (s remoteWorkspaceGrant) Remote() workspace.Remote {
	return &gatewayRemote{owner: s.service, session: s.session}
}

type dashboardWorkspaceEntry struct {
	session   workspaceGrantSession
	binding   application.DashboardOpened
	copy      *workspace.Workspace
	projectID string
}
type DashboardWorkspaces struct {
	mu      sync.Mutex
	dialogs WorkspaceDialogs
	bind    func() (workspaceGrantSession, error)
	handles map[string]*dashboardWorkspaceEntry
}

func NewDashboardWorkspaces(service *Service, dialogs WorkspaceDialogs) *DashboardWorkspaces {
	b := &DashboardWorkspaces{dialogs: dialogs, handles: map[string]*dashboardWorkspaceEntry{}}
	b.bind = func() (workspaceGrantSession, error) {
		if service == nil || service.remote == nil {
			return nil, project.ErrDependency
		}
		session, err := service.remote.Session()
		if err != nil {
			return nil, remoteWorkspaceError(err)
		}
		return remoteWorkspaceGrant{service, session}, nil
	}
	return b
}

var dashboardWorkspacePath = regexp.MustCompile(`^/studio/dashboards/([0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12})/capabilities/workspace\.local$`)
var workspaceRole = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)

type dashboardWorkspaceRequest struct {
	Operation  string                        `json:"operation"`
	ProjectID  string                        `json:"project_id,omitempty"`
	Handle     string                        `json:"workspace_handle,omitempty"`
	FileID     string                        `json:"file_id,omitempty"`
	MaxBytes   int64                         `json:"max_bytes,omitempty"`
	FileIDs    []string                      `json:"file_ids,omitempty"`
	Role       string                        `json:"role,omitempty"`
	Resolution *workspace.ConflictResolution `json:"resolution,omitempty"`
}

func decodeDashboardWorkspace(data []byte) (dashboardWorkspaceRequest, error) {
	var in dashboardWorkspaceRequest
	if strictjson.Decode(bytes.NewReader(data), project.MaxManifestBytes, &in) != nil {
		return in, project.ErrInvalid
	}
	allowed := map[string]bool{"operation": true}
	switch in.Operation {
	case "openDirectory":
		allowed["project_id"] = true
		if !project.ValidID(in.ProjectID) {
			return in, project.ErrInvalid
		}
	case "status", "previewConflict", "resume", "switchToHead", "close":
		allowed["workspace_handle"] = true
	case "fetch":
		allowed["workspace_handle"] = true
		allowed["file_id"] = true
		allowed["max_bytes"] = true
		if !project.ValidID(in.FileID) || in.MaxBytes < 0 || in.MaxBytes > 1<<40 {
			return in, project.ErrInvalid
		}
	case "saveTexts", "saveFiles":
		allowed["workspace_handle"] = true
		allowed["file_ids"] = true
		if len(in.FileIDs) == 0 || len(in.FileIDs) > project.MaxFiles {
			return in, project.ErrInvalid
		}
		seen := map[string]bool{}
		for _, id := range in.FileIDs {
			if !project.ValidID(id) || seen[id] {
				return in, project.ErrInvalid
			}
			seen[id] = true
		}
	case "importFiles":
		allowed["workspace_handle"] = true
		allowed["role"] = true
		if !workspaceRole.MatchString(in.Role) {
			return in, project.ErrInvalid
		}
	case "resolveConflict":
		allowed["workspace_handle"] = true
		allowed["resolution"] = true
		if in.Resolution == nil {
			return in, project.ErrInvalid
		}
	default:
		return in, project.ErrInvalid
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(data, &fields) != nil {
		return in, project.ErrInvalid
	}
	for key := range fields {
		if !allowed[key] {
			return in, project.ErrInvalid
		}
	}
	if in.Operation != "openDirectory" {
		id, err := uuid.Parse(in.Handle)
		if err != nil || id.Version() != 4 || id.String() != in.Handle {
			return in, project.ErrInvalid
		}
	}
	return in, nil
}
func validWorkspaceGrant(binding application.DashboardOpened, id string) bool {
	l, c := binding.Launch, binding.Context
	return l.DashboardSessionID == id && l.InstallationID != "" && l.InstallationID == c.InstallationID && l.AppID == c.AppID && l.ContextRevision == c.ContextRevision && l.ContextRevision != "" && c.UserID != "" && c.OrganizationID != "" && c.StationID != "" && l.ExpiresAt.After(time.Now()) && slices.Contains(l.GrantedCapabilities, "workspace.local")
}
func sameWorkspaceGrant(a, b application.DashboardOpened) bool {
	return a.Launch.DashboardSessionID == b.Launch.DashboardSessionID && a.Launch.InstallationID == b.Launch.InstallationID && a.Launch.AppID == b.Launch.AppID && a.Launch.AppVersion == b.Launch.AppVersion && a.Launch.ArtifactDigest == b.Launch.ArtifactDigest && a.Launch.ContextRevision == b.Launch.ContextRevision && a.Context.UserID == b.Context.UserID && a.Context.OrganizationID == b.Context.OrganizationID && a.Context.StationID == b.Context.StationID
}
func (e *dashboardWorkspaceEntry) authorize(ctx context.Context) error {
	now, err := e.session.Inspect(ctx, e.binding.Launch.DashboardSessionID)
	if err != nil {
		return err
	}
	if !validWorkspaceGrant(now, e.binding.Launch.DashboardSessionID) || !sameWorkspaceGrant(e.binding, now) {
		return project.ErrForbidden
	}
	return ctx.Err()
}

type dashboardWorkspaceRemote struct {
	entry  *dashboardWorkspaceEntry
	remote workspace.Remote
}

func (r dashboardWorkspaceRemote) Open(ctx context.Context, id, revision string) (project.OpenResult, error) {
	if id != r.entry.projectID {
		return project.OpenResult{}, project.ErrForbidden
	}
	if err := r.entry.authorize(ctx); err != nil {
		return project.OpenResult{}, err
	}
	return r.remote.Open(ctx, id, revision)
}
func (r dashboardWorkspaceRemote) Metadata(ctx context.Context, ref project.ContentRef, a project.Access) (project.ContentVersion, error) {
	if err := r.entry.authorize(ctx); err != nil {
		return project.ContentVersion{}, err
	}
	return r.remote.Metadata(ctx, ref, a)
}
func (r dashboardWorkspaceRemote) Download(ctx context.Context, ref project.ContentRef, a project.Access, dst io.Writer, n int64) error {
	if err := r.entry.authorize(ctx); err != nil {
		return err
	}
	return r.remote.Download(ctx, ref, a, dst, n)
}
func (r dashboardWorkspaceRemote) Commit(ctx context.Context, req project.CommitRequest) (project.Result, error) {
	if req.ProjectID != r.entry.projectID {
		return project.Result{}, project.ErrForbidden
	}
	if err := r.entry.authorize(ctx); err != nil {
		return project.Result{}, err
	}
	return r.remote.Commit(ctx, req)
}
func (r dashboardWorkspaceRemote) Upload(ctx context.Context, req workspace.UploadRequest, src io.Reader) (project.ContentVersion, error) {
	if req.ProjectID != r.entry.projectID {
		return project.ContentVersion{}, project.ErrForbidden
	}
	if err := r.entry.authorize(ctx); err != nil {
		return project.ContentVersion{}, err
	}
	remote, ok := r.remote.(workspace.BinaryRemote)
	if !ok {
		return project.ContentVersion{}, project.ErrDependency
	}
	return remote.Upload(ctx, req, src)
}
func (r dashboardWorkspaceRemote) CommitStatus(ctx context.Context, id, commit string) (project.CommitStatus, error) {
	if id != r.entry.projectID {
		return project.CommitStatus{}, project.ErrForbidden
	}
	if err := r.entry.authorize(ctx); err != nil {
		return project.CommitStatus{}, err
	}
	remote, ok := r.remote.(workspace.ConflictRemote)
	if !ok {
		return project.CommitStatus{}, project.ErrDependency
	}
	return remote.CommitStatus(ctx, id, commit)
}

// Close releases in-memory handles only. Persistent local recovery records stay.
func (b *DashboardWorkspaces) Close() {
	b.mu.Lock()
	entries := b.handles
	b.handles = map[string]*dashboardWorkspaceEntry{}
	b.mu.Unlock()
	for _, entry := range entries {
		_ = entry.copy.Close()
	}
}
func (b *DashboardWorkspaces) retire(id string, entry *dashboardWorkspaceEntry) {
	b.mu.Lock()
	if b.handles[id] == entry {
		delete(b.handles, id)
	}
	b.mu.Unlock()
	if entry.copy != nil {
		_ = entry.copy.Close()
	}
}
func (b *DashboardWorkspaces) snapshot(ctx context.Context, id string, e *dashboardWorkspaceEntry) (any, error) {
	state, manifest, err := e.copy.Snapshot(ctx)
	if err != nil {
		return nil, err
	}
	files, err := e.copy.Status(ctx)
	if err != nil {
		return nil, err
	}
	return map[string]any{"workspace_handle": id, "state": state, "manifest": manifest, "files": files}, nil
}
func (b *DashboardWorkspaces) open(ctx context.Context, sessionID string, in dashboardWorkspaceRequest) (any, error) {
	if b.dialogs == nil || b.bind == nil {
		return nil, project.ErrDependency
	}
	session, err := b.bind()
	if err != nil {
		return nil, err
	}
	binding, err := session.Inspect(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if !validWorkspaceGrant(binding, sessionID) {
		return nil, project.ErrForbidden
	}
	b.mu.Lock()
	entries := make(map[string]*dashboardWorkspaceEntry, len(b.handles))
	for id, e := range b.handles {
		entries[id] = e
	}
	b.mu.Unlock()
	for id, e := range entries {
		if err = e.authorize(ctx); errors.Is(err, project.ErrForbidden) {
			b.retire(id, e)
		}
	}
	b.mu.Lock()
	full := len(b.handles) >= 32
	b.mu.Unlock()
	if full {
		return nil, workspace.ErrBusy
	}
	directory, err := b.dialogs.Directory(ctx)
	if err != nil {
		return nil, project.ErrDependency
	}
	if directory == "" {
		return map[string]bool{"cancelled": true}, nil
	}
	entry := &dashboardWorkspaceEntry{session: session, binding: binding, projectID: in.ProjectID}
	if err = entry.authorize(ctx); err != nil {
		return nil, err
	}
	remote := dashboardWorkspaceRemote{entry: entry, remote: session.Remote()}
	copy, err := workspace.Open(ctx, directory, "studio", in.ProjectID, remote)
	if err != nil {
		return nil, err
	}
	entry.copy = copy
	id := uuid.NewString()
	value, err := b.snapshot(ctx, id, entry)
	if err != nil {
		_ = copy.Close()
		return nil, err
	}
	if err = entry.authorize(ctx); err != nil {
		_ = copy.Close()
		return nil, err
	}
	b.mu.Lock()
	if len(b.handles) >= 32 {
		b.mu.Unlock()
		_ = copy.Close()
		return nil, workspace.ErrBusy
	}
	b.handles[id] = entry
	b.mu.Unlock()
	return value, nil
}
func fileMIME(path string) string {
	kind := mime.TypeByExtension(strings.ToLower(filepath.Ext(path)))
	if kind == "" {
		kind = "application/octet-stream"
	}
	return kind
}
func (b *DashboardWorkspaces) execute(ctx context.Context, sessionID string, in dashboardWorkspaceRequest) (any, error) {
	if in.Operation == "openDirectory" {
		return b.open(ctx, sessionID, in)
	}
	b.mu.Lock()
	entry := b.handles[in.Handle]
	b.mu.Unlock()
	if entry == nil || entry.binding.Launch.DashboardSessionID != sessionID {
		return nil, project.ErrForbidden
	}
	if err := entry.authorize(ctx); err != nil {
		if errors.Is(err, project.ErrForbidden) {
			b.retire(in.Handle, entry)
		}
		return nil, err
	}
	var value any
	var err error
	switch in.Operation {
	case "status":
		value, err = b.snapshot(ctx, in.Handle, entry)
	case "close":
		b.retire(in.Handle, entry)
		return map[string]string{"workspace_handle": in.Handle}, nil
	case "fetch":
		n := in.MaxBytes
		if n == 0 {
			n = 1 << 40
		}
		err = entry.copy.Fetch(ctx, in.FileID, n)
		value = map[string]string{"workspace_handle": in.Handle, "file_id": in.FileID}
	case "previewConflict":
		value, err = entry.copy.PreviewConflict(ctx)
	case "resume":
		value, err = entry.copy.Resume(ctx)
	case "resolveConflict":
		value, err = entry.copy.ResolveConflict(ctx, *in.Resolution)
	case "switchToHead":
		_, _, err = entry.copy.SwitchToHead(ctx)
		if err == nil {
			value, err = b.snapshot(ctx, in.Handle, entry)
		}
	case "saveTexts":
		value, err = entry.copy.SaveTexts(ctx, in.FileIDs)
	case "saveFiles":
		_, manifest, e := entry.copy.Snapshot(ctx)
		if e != nil {
			return nil, e
		}
		byID := map[string]project.File{}
		for _, f := range manifest.Files {
			byID[f.ID] = f
		}
		files := []workspace.FileInput{}
		for _, id := range in.FileIDs {
			file, ok := byID[id]
			if !ok {
				return nil, project.ErrInvalid
			}
			files = append(files, workspace.FileInput{FileID: id, Path: file.Path, Role: file.Role, MIME: fileMIME(file.Path)})
		}
		value, err = entry.copy.SaveFiles(ctx, files)
	case "importFiles":
		if b.dialogs == nil {
			return nil, project.ErrDependency
		}
		paths, e := b.dialogs.Files(ctx)
		if e != nil {
			return nil, project.ErrDependency
		}
		if len(paths) == 0 {
			return map[string]bool{"cancelled": true}, nil
		}
		if len(paths) > project.MaxFiles {
			return nil, project.ErrInvalid
		}
		if err = entry.authorize(ctx); err != nil {
			return nil, err
		}
		imports := []workspace.ImportInput{}
		names := map[string]bool{}
		for _, source := range paths {
			name := filepath.Base(source)
			key := project.PathKey(name)
			if project.PortablePath(name) != nil || names[key] {
				return nil, project.ErrInvalid
			}
			names[key] = true
			imports = append(imports, workspace.ImportInput{SourcePath: source, Path: name, Role: in.Role, MIME: fileMIME(name)})
		}
		value, err = entry.copy.ImportFiles(ctx, imports)
	default:
		return nil, project.ErrInvalid
	}
	if err != nil {
		return nil, err
	}
	if err = entry.authorize(ctx); err != nil {
		return nil, err
	}
	return value, nil
}
func workspaceCapabilityFailure(err error) WorkspaceResult {
	// Never serialize an os.PathError or transport error containing private paths.
	for _, safe := range []error{project.ErrInvalid, project.ErrForbidden, project.ErrNotFound, project.ErrConflict, project.ErrIdempotency, workspace.ErrConflict, workspace.ErrInvalid, workspace.ErrBusy, workspace.ErrPending, workspace.ErrCorrupt} {
		if errors.Is(err, safe) {
			return workspaceFailure(safe)
		}
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return WorkspaceResult{Status: 409, Data: json.RawMessage(`{"code":"WORKSPACE_CANCELLED"}`), RequestID: uuid.Must(uuid.NewV7()).String()}
	}
	return workspaceFailure(project.ErrDependency)
}

// Wrap handles only the local capability; every other path stays with the
// normal host asset transport. No local HTTP listener is started.
func (b *DashboardWorkspaces) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		match := dashboardWorkspacePath.FindStringSubmatch(r.URL.Path)
		if match == nil {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		fail := func(err error) {
			out := workspaceCapabilityFailure(err)
			w.Header().Set("X-Request-Id", out.RequestID)
			w.WriteHeader(out.Status)
			_, _ = w.Write(out.Data)
		}
		mt, _, mediaErr := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if r.Method != "POST" || r.URL.RawPath != "" || r.URL.RawQuery != "" || r.URL.ForceQuery || mediaErr != nil || mt != "application/json" || len(r.Header.Values("Authorization")) != 0 || len(r.Header.Values("Cookie")) != 0 || len(r.Header.Values("Content-Encoding")) != 0 {
			fail(project.ErrInvalid)
			return
		}
		if site := r.Header.Get("Sec-Fetch-Site"); site != "" && site != "same-origin" && site != "none" {
			fail(project.ErrForbidden)
			return
		}
		origins := r.Header.Values("Origin")
		if len(origins) > 1 {
			fail(project.ErrForbidden)
			return
		}
		if len(origins) == 1 {
			origin, err := url.Parse(origins[0])
			if err != nil || origin.Host != r.Host || origin.User != nil || origin.Path != "" || (origin.Scheme != "http" && origin.Scheme != "https") {
				fail(project.ErrForbidden)
				return
			}
		}
		data, err := io.ReadAll(io.LimitReader(r.Body, project.MaxManifestBytes+1))
		if err != nil || len(data) > project.MaxManifestBytes {
			fail(project.ErrInvalid)
			return
		}
		in, err := decodeDashboardWorkspace(data)
		if err != nil {
			fail(err)
			return
		}
		value, err := b.execute(r.Context(), match[1], in)
		if err != nil {
			fail(err)
			return
		}
		out := workspaceSuccess(value)
		w.Header().Set("X-Request-Id", out.RequestID)
		w.WriteHeader(out.Status)
		_, _ = w.Write(out.Data)
	})
}
