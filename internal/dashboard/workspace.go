package dashboard

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// InstanceEntry is platform registration data, never a browser-provided URL.
type InstanceEntry struct {
	EntryID        string `json:"entryId"`
	ResolvePath    string `json:"resolvePath"`
	EntryRef       string `json:"entryRef"`
	IndexPath      string `json:"indexPath"`
	AssetPath      string `json:"assetPath"`
	ArtifactDigest string `json:"artifactDigest"`
	Rules          []Rule `json:"rules"`
	PublicAssets   bool   `json:"publicAssets"`
}

type InstanceOpen struct {
	InstanceID string `json:"instanceId"`
	EntryID    string `json:"entryId"`
}

func ValidInstanceOpen(in InstanceOpen) bool {
	return identifier.MatchString(in.InstanceID) && identifier.MatchString(in.EntryID)
}

func instancePath(path, id string) string {
	parts := strings.Split(path, "/")
	for i, part := range parts {
		if part == ":instanceId" {
			parts[i] = id
		}
	}
	return strings.Join(parts, "/")
}

func instanceRules(entry InstanceEntry, id string) []Rule {
	rules := slices.Clone(entry.Rules)
	for i := range rules {
		rules[i].UpstreamPath = instancePath(rules[i].UpstreamPath, id)
	}
	return rules
}

func validInstanceEntries(entries []InstanceEntry) bool {
	if len(entries) > 32 {
		return false
	}
	seen := map[string]bool{}
	for _, entry := range entries {
		if !entry.PublicAssets || !identifier.MatchString(entry.EntryID) || seen[entry.EntryID] || !digest.MatchString(entry.ArtifactDigest) ||
			!validPath(entry.ResolvePath) || strings.Contains(entry.ResolvePath, ":") ||
			!validPath(entry.IndexPath) || strings.Contains(entry.IndexPath, ":") ||
			!entryChars.MatchString(entry.EntryRef) || !validPath("/"+entry.EntryRef) || !strings.HasSuffix(entry.EntryRef, ".html") ||
			!strings.HasSuffix(entry.AssetPath, "/") || !validPath(strings.TrimSuffix(entry.AssetPath, "/")) || strings.Contains(entry.AssetPath, ":") ||
			!validRules(instanceRules(entry, "instance-validation")) {
			return false
		}
		for _, rule := range entry.Rules {
			// The caller must not override the workspace's fixed instance binding.
			if strings.Contains(rule.Path, ":instanceId") {
				return false
			}
		}
		seen[entry.EntryID] = true
	}
	return true
}

type InstanceWorkspaceLaunch struct {
	WorkspaceSessionID string    `json:"workspaceSessionId"`
	InstallationID     string    `json:"installationId"`
	AppID              string    `json:"appId"`
	InstanceID         string    `json:"instanceId"`
	InstanceName       string    `json:"instanceName,omitempty"`
	EntryID            string    `json:"entryId"`
	EntryURL           string    `json:"entryUrl"`
	AssetBaseURL       string    `json:"assetBaseUrl"`
	ExpiresAt          time.Time `json:"expiresAt"`
}

type WorkspaceSource interface {
	Source
	ResolveInstance(context.Context, Actor, Binding, InstanceEntry, InstanceOpen) error
}

type workspaceSession struct {
	actor       Actor
	launch      InstanceWorkspaceLaunch
	bindingHash [32]byte
}

// Workspaces owns frontend authorization only, independently from Dashboards and
// App editor leases. There are deliberately no instance start/stop operations.
type Workspaces struct {
	source   WorkspaceSource
	mu       sync.Mutex
	sessions map[string]workspaceSession
	now      func() time.Time
}

func NewWorkspaces(source WorkspaceSource) *Workspaces {
	return &Workspaces{source: source, sessions: map[string]workspaceSession{}, now: time.Now}
}

func workspaceEntry(actor Actor, binding Binding, installationID, entryID string) (InstanceEntry, error) {
	grants, err := validate(actor, binding, installationID)
	if err != nil {
		return InstanceEntry{}, err
	}
	if !slices.Contains(grants, "instances.open") {
		return InstanceEntry{}, ErrDenied
	}
	if !validInstanceEntries(binding.InstanceEntries) {
		return InstanceEntry{}, ErrInvalid
	}
	for _, entry := range binding.InstanceEntries {
		if entry.EntryID == entryID {
			return entry, nil
		}
	}
	return InstanceEntry{}, ErrUnsupported
}

func (w *Workspaces) Open(ctx context.Context, actor Actor, installationID string, in InstanceOpen) (InstanceWorkspaceLaunch, error) {
	if !validActor(actor) || !identifier.MatchString(installationID) || !ValidInstanceOpen(in) {
		return InstanceWorkspaceLaunch{}, ErrDenied
	}
	if err := ctx.Err(); err != nil {
		return InstanceWorkspaceLaunch{}, err
	}
	if w.source == nil {
		return InstanceWorkspaceLaunch{}, ErrUnavailable
	}
	binding, err := w.source.Lookup(ctx, actor, installationID)
	if err != nil {
		return InstanceWorkspaceLaunch{}, err
	}
	entry, err := workspaceEntry(actor, binding, installationID, in.EntryID)
	if err != nil {
		return InstanceWorkspaceLaunch{}, err
	}
	var instanceName string
	if named, ok := w.source.(interface {
		ResolveInstanceName(context.Context, Actor, Binding, InstanceEntry, InstanceOpen) (string, error)
	}); ok {
		instanceName, err = named.ResolveInstanceName(ctx, actor, binding, entry, in)
	} else {
		err = w.source.ResolveInstance(ctx, actor, binding, entry, in)
	}
	if err != nil {
		return InstanceWorkspaceLaunch{}, err
	}
	// Resolve can block on a running service. Revalidate release and permissions
	// before publishing a workspace for it, including unchanged revision numbers.
	current, err := w.source.Lookup(ctx, actor, installationID)
	if err != nil {
		return InstanceWorkspaceLaunch{}, err
	}
	if _, err = workspaceEntry(actor, current, installationID, in.EntryID); err != nil {
		return InstanceWorkspaceLaunch{}, err
	}
	if bindingHash(current) != bindingHash(binding) {
		return InstanceWorkspaceLaunch{}, ErrExpired
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return InstanceWorkspaceLaunch{}, err
	}
	for id, session := range w.sessions {
		if !session.launch.ExpiresAt.After(w.now()) {
			delete(w.sessions, id)
		}
	}
	if len(w.sessions) >= 128 {
		return InstanceWorkspaceLaunch{}, ErrUnavailable
	}
	id := uuid.NewString()
	launch := InstanceWorkspaceLaunch{WorkspaceSessionID: id, InstallationID: installationID, AppID: binding.AppID,
		InstanceID: in.InstanceID, InstanceName: instanceName, EntryID: in.EntryID,
		EntryURL:     "/studio/instance-workspaces/" + id + "/assets/" + entry.EntryRef,
		AssetBaseURL: "/studio/instance-assets/" + strings.TrimPrefix(entry.ArtifactDigest, "sha256:") + "/",
		ExpiresAt:    w.now().UTC().Add(15 * time.Minute)}
	w.sessions[id] = workspaceSession{actor, launch, bindingHash(binding)}
	return launch, nil
}

func (w *Workspaces) Check(ctx context.Context, actor Actor, id string) (InstanceWorkspaceLaunch, Binding, InstanceEntry, error) {
	fail := func(err error) (InstanceWorkspaceLaunch, Binding, InstanceEntry, error) {
		return InstanceWorkspaceLaunch{}, Binding{}, InstanceEntry{}, err
	}
	if err := ctx.Err(); err != nil {
		return fail(err)
	}
	w.mu.Lock()
	session, exists := w.sessions[id]
	w.mu.Unlock()
	if !exists || !session.launch.ExpiresAt.After(w.now()) {
		return fail(ErrExpired)
	}
	if !validActor(actor) || session.actor != actor {
		return fail(ErrDenied)
	}
	binding, err := w.source.Lookup(ctx, actor, session.launch.InstallationID)
	if err != nil {
		if errors.Is(err, ErrDenied) || errors.Is(err, ErrInvalid) || errors.Is(err, ErrUnsupported) {
			_ = w.Close(actor, id)
		}
		return fail(err)
	}
	entry, err := workspaceEntry(actor, binding, session.launch.InstallationID, session.launch.EntryID)
	if err == nil && bindingHash(binding) != session.bindingHash {
		err = ErrExpired
	}
	if err != nil {
		_ = w.Close(actor, id)
		return fail(err)
	}
	if err := ctx.Err(); err != nil {
		return fail(err)
	}
	w.mu.Lock()
	_, exists = w.sessions[id]
	w.mu.Unlock()
	if !exists || !session.launch.ExpiresAt.After(w.now()) {
		return fail(ErrExpired)
	}
	return session.launch, binding, entry, nil
}

func (w *Workspaces) Close(actor Actor, id string) error {
	if !validActor(actor) {
		return ErrDenied
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	session, exists := w.sessions[id]
	if !exists {
		return nil
	}
	if session.actor != actor {
		return ErrDenied
	}
	delete(w.sessions, id)
	return nil
}
