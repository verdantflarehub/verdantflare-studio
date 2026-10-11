// Package dashboard resolves installed App declarations without app-name branches.
// The authoritative contract lives in the design repository's app-dashboard-v1.md.
package dashboard

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/verdantflarehub/verdantflare-studio/internal/strictjson"
)

var (
	ErrUnavailable = errors.New("DASHBOARD_UNAVAILABLE")
	ErrDenied      = errors.New("DASHBOARD_DENIED")
	ErrInvalid     = errors.New("INVALID_DASHBOARD_BINDING")
	ErrExpired     = errors.New("DASHBOARD_SESSION_EXPIRED")
	ErrUnsupported = errors.New("DASHBOARD_UNSUPPORTED")
	identifier     = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._:-]{0,127}$`)
	version        = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)
	digest         = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)
	pathChars      = regexp.MustCompile(`^/[a-zA-Z0-9._:/-]+$`)
	entryChars     = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._/-]{0,255}$`)
	parameterName  = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_]{0,63}$`)
)

// Actor must be derived from the verified Station identity, never request JSON.
type Actor struct {
	UserID, OrganizationID, StationID, AuthorizationEpoch string
}

type Declaration struct {
	SchemaVersion int        `json:"schemaVersion"`
	AppID         string     `json:"appId"`
	AppVersion    string     `json:"appVersion"`
	Dashboard     *Dashboard `json:"dashboard,omitempty"`
}

type Dashboard struct {
	EntryRef             string   `json:"entryRef"`
	ArtifactDigest       string   `json:"artifactDigest"`
	Adapter              string   `json:"adapter"`
	SDKMajor             int      `json:"sdkMajor"`
	MinimumSDKVersion    string   `json:"minimumSdkVersion"`
	RequiredCapabilities []string `json:"requiredCapabilities"`
	OptionalCapabilities []string `json:"optionalCapabilities"`
	AppAPIPolicyRef      string   `json:"appApiPolicyRef"`
	KeepAlive            bool     `json:"keepAlive"`
}

type Rule struct {
	Method         string `json:"method"`
	Path           string `json:"path"`
	UpstreamPath   string `json:"upstreamPath"`
	MaxBody        int    `json:"maxBody"`
	TimeoutSeconds int    `json:"timeoutSeconds,omitempty"`
}

// Binding is platform-owned installed release data. Source must authenticate the
// declaration and pin it to the installed artifact before returning it here.
type Binding struct {
	InstallationID, AppID, AppVersion, StationID, OrganizationID string
	ArtifactDigest, Revision                                     string
	Declaration                                                  Declaration
	Trusted                                                      bool
	Grants                                                       []string
	Rules                                                        []Rule
	Backend                                                      *Backend
	InstanceEntries                                              []InstanceEntry
	InstallationRevision                                         int64
	ReleaseRevision                                              string
}

type Source interface {
	// Return a caller-owned snapshot, with no concurrently mutable shared slices.
	Lookup(context.Context, Actor, string) (Binding, error)
}

// DecodeDeclaration enforces the closed wire shape before semantic validation.
// A declaration alone never establishes publisher trust or installation ownership.
func DecodeDeclaration(reader io.Reader) (Declaration, error) {
	var declaration Declaration
	if err := strictjson.Decode(reader, 64<<10, &declaration); err != nil {
		return Declaration{}, ErrInvalid
	}
	return declaration, nil
}

type Launch struct {
	DashboardSessionID  string    `json:"dashboardSessionId"`
	InstallationID      string    `json:"installationId"`
	AppID               string    `json:"appId"`
	AppVersion          string    `json:"appVersion"`
	ArtifactDigest      string    `json:"artifactDigest"`
	Adapter             string    `json:"adapter"`
	EntryURL            string    `json:"entryUrl"`
	SDKVersion          string    `json:"sdkVersion"`
	GrantedCapabilities []string  `json:"grantedCapabilities"`
	ContextRevision     string    `json:"contextRevision"`
	ExpiresAt           time.Time `json:"expiresAt"`
}

type session struct {
	actor       Actor
	launch      Launch
	bindingHash [32]byte
}
type Resolver struct {
	source   Source
	mu       sync.Mutex
	sessions map[string]session
	now      func() time.Time
}

func New(source Source) *Resolver {
	return &Resolver{source: source, sessions: map[string]session{}, now: time.Now}
}

func validActor(actor Actor) bool {
	return identifier.MatchString(actor.UserID) && identifier.MatchString(actor.OrganizationID) &&
		identifier.MatchString(actor.StationID) && identifier.MatchString(actor.AuthorizationEpoch)
}

func validPath(path string) bool {
	if len(path) > 2048 || !pathChars.MatchString(path) {
		return false
	}
	for _, part := range strings.Split(path[1:], "/") {
		if part == "" || part == "." || part == ".." {
			return false
		}
	}
	return true
}

func bindingHash(binding Binding) [32]byte {
	// Binding contains only JSON-encodable values. Include the complete policy so
	// a provider cannot accidentally reuse a revision while changing permissions.
	encoded, _ := json.Marshal(binding)
	return sha256.Sum256(encoded)
}

func validate(actor Actor, binding Binding, installationID string) ([]string, error) {
	if !validActor(actor) || !identifier.MatchString(installationID) || binding.InstallationID != installationID ||
		binding.StationID != actor.StationID || binding.OrganizationID != actor.OrganizationID || !binding.Trusted {
		return nil, ErrDenied
	}
	return validateDefinition(binding)
}

// validateDefinition is shared by offline metadata and the authenticated loader.
// It validates release compatibility; it does not grant installation access.
func validateDefinition(binding Binding) ([]string, error) {
	d := binding.Declaration
	if d.SchemaVersion != 1 || !identifier.MatchString(binding.AppID) || !identifier.MatchString(binding.Revision) ||
		d.AppID != binding.AppID || d.AppVersion != binding.AppVersion || !version.MatchString(d.AppVersion) {
		return nil, ErrInvalid
	}
	if d.Dashboard == nil {
		return nil, ErrUnsupported
	}
	page := d.Dashboard
	if page.Adapter != "wujie" || page.SDKMajor != 1 || (page.MinimumSDKVersion != "1.0.0" && page.MinimumSDKVersion != "1.1.0" && page.MinimumSDKVersion != "1.2.0") {
		return nil, ErrUnsupported
	}
	if !digest.MatchString(binding.ArtifactDigest) || page.ArtifactDigest != binding.ArtifactDigest ||
		!entryChars.MatchString(page.EntryRef) || !validPath("/"+page.EntryRef) || !identifier.MatchString(page.AppAPIPolicyRef) {
		return nil, ErrInvalid
	}
	if page.RequiredCapabilities == nil || page.OptionalCapabilities == nil || len(page.RequiredCapabilities) > 128 ||
		len(page.OptionalCapabilities) > 128 || len(binding.Grants) > 128 {
		return nil, ErrInvalid
	}
	allowed := map[string]bool{}
	for _, capability := range binding.Grants {
		if !identifier.MatchString(capability) || allowed[capability] {
			return nil, ErrInvalid
		}
		allowed[capability] = true
	}
	requested := append(slices.Clone(page.RequiredCapabilities), page.OptionalCapabilities...)
	seen := map[string]bool{}
	grants := []string{}
	for _, capability := range requested {
		if !identifier.MatchString(capability) || seen[capability] {
			return nil, ErrInvalid
		}
		seen[capability] = true
		if allowed[capability] {
			grants = append(grants, capability)
		}
	}
	for _, capability := range page.RequiredCapabilities {
		if !slices.Contains(grants, capability) {
			return nil, ErrDenied
		}
	}
	if !validRules(binding.Rules) {
		return nil, ErrInvalid
	}
	return grants, nil
}

// Overlapping templates are rejected so declaration order cannot select a
// different upstream or bypass a stricter body limit.
func validRules(rules []Rule) bool {
	if rules == nil || len(rules) > 128 {
		return false
	}
	for i, rule := range rules {
		if !validPath(rule.Path) || !validPath(rule.UpstreamPath) || !sameParameters(rule.Path, rule.UpstreamPath) ||
			rule.MaxBody < 0 || rule.MaxBody > 1<<20 || rule.TimeoutSeconds < 0 || rule.TimeoutSeconds > 60 || !slices.Contains([]string{"GET", "POST", "PUT", "PATCH", "DELETE"}, rule.Method) ||
			(rule.Method == "GET" && rule.MaxBody != 0) {
			return false
		}
		parts := strings.Split(rule.Path, "/")
		for _, prior := range rules[:i] {
			other := strings.Split(prior.Path, "/")
			if prior.Method != rule.Method || len(parts) != len(other) {
				continue
			}
			overlap := true
			for j, part := range parts {
				if part != other[j] && !strings.HasPrefix(part, ":") && !strings.HasPrefix(other[j], ":") {
					overlap = false
					break
				}
			}
			if overlap {
				return false
			}
		}
	}
	return true
}

func (r *Resolver) Open(ctx context.Context, actor Actor, installationID string) (Launch, error) {
	return r.OpenChecked(ctx, actor, installationID, nil)
}

// OpenChecked runs the normal authenticated lookup and release checks before
// checking the caller's pinned binding, and before allocating any session.
func (r *Resolver) OpenChecked(ctx context.Context, actor Actor, installationID string, check func(Binding) error) (Launch, error) {
	if err := ctx.Err(); err != nil {
		return Launch{}, err
	}
	if r.source == nil {
		return Launch{}, ErrUnavailable
	}
	if !validActor(actor) || !identifier.MatchString(installationID) {
		return Launch{}, ErrDenied
	}
	binding, err := r.source.Lookup(ctx, actor, installationID)
	if err != nil {
		return Launch{}, err
	}
	grants, err := validate(actor, binding, installationID)
	if err != nil {
		return Launch{}, err
	}
	if check != nil {
		if err := check(binding); err != nil {
			return Launch{}, err
		}
	}
	id := uuid.NewString()
	launch := Launch{
		DashboardSessionID: id, InstallationID: installationID, AppID: binding.AppID,
		AppVersion: binding.AppVersion, ArtifactDigest: binding.ArtifactDigest, Adapter: "wujie",
		EntryURL:   "/studio/dashboards/" + id + "/assets/" + binding.Declaration.Dashboard.EntryRef,
		SDKVersion: "1.2.0", GrantedCapabilities: grants, ContextRevision: actor.AuthorizationEpoch,
		ExpiresAt: r.now().UTC().Add(15 * time.Minute),
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return Launch{}, err
	}
	for key, entry := range r.sessions {
		if !entry.launch.ExpiresAt.After(r.now()) {
			delete(r.sessions, key)
		}
	}
	if len(r.sessions) >= 1024 {
		return Launch{}, ErrUnavailable
	}
	r.sessions[id] = session{actor, launch, bindingHash(binding)}
	launch.GrantedCapabilities = slices.Clone(grants)
	return launch, nil
}

// Check revalidates the current installation on every read/write. A cache is not
// an authorization grant; changed releases and revocation require a new session.
func (r *Resolver) Check(ctx context.Context, actor Actor, sessionID string) (Binding, error) {
	if err := ctx.Err(); err != nil {
		return Binding{}, err
	}
	r.mu.Lock()
	entry, ok := r.sessions[sessionID]
	r.mu.Unlock()
	if !ok || !entry.launch.ExpiresAt.After(r.now()) {
		return Binding{}, ErrExpired
	}
	if !validActor(actor) || entry.actor != actor {
		return Binding{}, ErrDenied
	}
	binding, err := r.source.Lookup(ctx, actor, entry.launch.InstallationID)
	if err != nil {
		if errors.Is(err, ErrDenied) || errors.Is(err, ErrInvalid) || errors.Is(err, ErrUnsupported) {
			_ = r.Close(actor, sessionID)
		}
		return Binding{}, err
	}
	_, err = validate(actor, binding, entry.launch.InstallationID)
	if err != nil {
		_ = r.Close(actor, sessionID)
		return Binding{}, err
	}
	if bindingHash(binding) != entry.bindingHash {
		_ = r.Close(actor, sessionID)
		return Binding{}, ErrExpired
	}
	if err := ctx.Err(); err != nil {
		return Binding{}, err
	}
	// A close or expiry while Lookup was in flight must also revoke this request.
	r.mu.Lock()
	_, stillOpen := r.sessions[sessionID]
	r.mu.Unlock()
	if !stillOpen || !entry.launch.ExpiresAt.After(r.now()) {
		return Binding{}, ErrExpired
	}
	return binding, nil
}

func (r *Resolver) AuthorizeRequest(ctx context.Context, actor Actor, sessionID, method, path string, bodySize int) (Binding, error) {
	if !validPath(path) || bodySize < 0 || bodySize > 1<<20 || (method == "GET" && bodySize != 0) {
		return Binding{}, ErrDenied
	}
	binding, err := r.Check(ctx, actor, sessionID)
	if err != nil {
		return Binding{}, err
	}
	grants, err := validate(actor, binding, binding.InstallationID)
	if err != nil || !slices.Contains(grants, "app.request") {
		return Binding{}, ErrDenied
	}
	for _, rule := range binding.Rules {
		if _, matched := ruleTarget(rule, method, path, bodySize); matched {
			return binding, nil
		}
	}
	return Binding{}, ErrDenied
}

// Inspect revalidates an existing page without renewing its lifetime or allocating
// a second session. This is the authorization gate for keep-alive activation.
func (r *Resolver) Inspect(ctx context.Context, actor Actor, sessionID string) (Launch, error) {
	if _, err := r.Check(ctx, actor, sessionID); err != nil {
		return Launch{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	entry, ok := r.sessions[sessionID]
	if !ok || !entry.launch.ExpiresAt.After(r.now()) {
		return Launch{}, ErrExpired
	}
	launch := entry.launch
	launch.GrantedCapabilities = slices.Clone(launch.GrantedCapabilities)
	return launch, nil
}

func (r *Resolver) Close(actor Actor, sessionID string) error {
	if !validActor(actor) {
		return ErrDenied
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	entry, ok := r.sessions[sessionID]
	if !ok {
		return nil
	}
	if entry.actor != actor {
		return ErrDenied
	}
	delete(r.sessions, sessionID)
	return nil
}
