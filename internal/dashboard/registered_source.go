package dashboard

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/verdantflarehub/verdantflare-studio/internal/strictjson"
)

// Backend is host configuration only. It is never serialized into Launch.
// AssetPrefix maps package-relative references to the registered resource root.
type Backend struct {
	Origin          string `json:"origin"`
	TokenEnv        string `json:"tokenEnv"`
	DeclarationPath string `json:"declarationPath"`
	IndexPath       string `json:"indexPath"`
	AssetPath       string `json:"assetPath"`
	AssetPrefix     string `json:"assetPrefix"`
}

type RegisteredRelease struct {
	AppID             string          `json:"appId"`
	AppVersion        string          `json:"appVersion"`
	DeclarationDigest string          `json:"declarationDigest"`
	ArtifactDigest    string          `json:"artifactDigest"`
	AppAPIPolicyRef   string          `json:"appApiPolicyRef"`
	Backend           Backend         `json:"backend"`
	Grants            []string        `json:"grants"`
	Rules             []Rule          `json:"rules"`
	InstanceEntries   []InstanceEntry `json:"instanceEntries,omitempty"`
}

type Registry struct {
	SchemaVersion int                 `json:"schemaVersion"`
	Releases      []RegisteredRelease `json:"releases"`
}

// InstalledRelease is derived from the verified Core response and identity.
type InstalledRelease struct {
	InstallationID, AppID, AppVersion, StationID, OrganizationID, State string
	Revision                                                            int64
	CanManage                                                           bool
}

type InstallationReader func(context.Context, Actor, string) (InstalledRelease, error)

type RegisteredSource struct {
	path       string
	read       InstallationReader
	client     *http.Client
	credential func(string) string
}

var credentialName = regexp.MustCompile(`^STUDIO_APP_[A-Z0-9_]+_TOKEN$`)
var serviceHost = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?\.[a-z0-9](?:[a-z0-9-]*[a-z0-9])?\.svc\.cluster\.local$`)
var rawDigest = regexp.MustCompile(`^[a-f0-9]{64}$`)

// maxRegistryReleases bounds historical App/version rows. maxRegistryBytes is the
// file cap. 512 leaves room for accumulated releases so one new version cannot
// reject the entire AppCenter view.
const maxRegistryReleases = 512
const maxRegistryBytes = 5 << 20

func NewRegisteredSource(path string, reader InstallationReader) *RegisteredSource {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	return &RegisteredSource{path: path, read: reader, credential: os.Getenv, client: &http.Client{
		Transport: transport, Timeout: 15 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}
}

func (s *RegisteredSource) Close() { s.client.CloseIdleConnections() }

// Versions exposes only the platform registration membership, not origins,
// credentials or a claim that App assets have already passed load-time checks.
func (s *RegisteredSource) Versions() (map[string]bool, error) {
	registry, _, err := s.registry()
	if err != nil {
		return nil, err
	}
	versions := map[string]bool{}
	for _, release := range registry.Releases {
		versions[release.AppID+"@"+release.AppVersion] = true
		if strings.Contains(release.Backend.Origin, ".verdantflare-app.svc.cluster.local") {
			versions[release.AppID] = true
		}
	}
	return versions, nil
}

func sum(data []byte) string {
	hash := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(hash[:])
}

func (s *RegisteredSource) registry() (Registry, string, error) {
	if s.path == "" {
		return Registry{}, "", ErrUnavailable
	}
	f, err := os.Open(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return Registry{}, "", ErrDenied
		}
		return Registry{}, "", ErrUnavailable
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxRegistryBytes+1))
	if err != nil || len(data) > maxRegistryBytes {
		return Registry{}, "", ErrInvalid
	}
	var registry Registry
	if strictjson.Decode(bytes.NewReader(data), maxRegistryBytes, &registry) != nil || registry.SchemaVersion != 1 || registry.Releases == nil || len(registry.Releases) > maxRegistryReleases {
		return Registry{}, "", ErrInvalid
	}
	seen := map[string]bool{}
	for _, release := range registry.Releases {
		key := release.AppID + "@" + release.AppVersion
		if seen[key] || !identifier.MatchString(release.AppID) || !version.MatchString(release.AppVersion) ||
			!digest.MatchString(release.DeclarationDigest) || !digest.MatchString(release.ArtifactDigest) ||
			!identifier.MatchString(release.AppAPIPolicyRef) || release.Grants == nil || release.Rules == nil || len(release.Grants) > 128 || len(release.Rules) > 128 || !validBackend(release.Backend) {
			return Registry{}, "", ErrInvalid
		}
		seen[key] = true
		if !validInstanceEntries(release.InstanceEntries) {
			return Registry{}, "", ErrInvalid
		}
		grants := map[string]bool{}
		for _, grant := range release.Grants {
			if !identifier.MatchString(grant) || grants[grant] {
				return Registry{}, "", ErrInvalid
			}
			grants[grant] = true
		}
		if !validRules(release.Rules) {
			return Registry{}, "", ErrInvalid
		}
	}
	return registry, strings.TrimPrefix(sum(data), "sha256:"), nil
}

func validBackend(b Backend) bool {
	u, err := url.Parse(b.Origin)
	if err != nil {
		return false
	}
	if strings.Contains(u.Host, ":") {
		port, err := strconv.Atoi(u.Port())
		if err != nil || port < 1 || port > 65535 || strconv.Itoa(port) != u.Port() {
			return false
		}
	}
	labels := strings.Split(u.Hostname(), ".")
	if len(labels) != 5 || len(labels[0]) > 63 || len(labels[1]) > 63 {
		return false
	}
	return (u.Scheme == "http" || u.Scheme == "https") && serviceHost.MatchString(u.Hostname()) &&
		u.User == nil && u.Path == "" && u.RawQuery == "" && !u.ForceQuery && u.Fragment == "" && !strings.ContainsAny(b.Origin, "\\ \t\r\n#?") &&
		credentialName.MatchString(b.TokenEnv) && validPath(b.DeclarationPath) && validPath(b.IndexPath) &&
		!strings.Contains(b.DeclarationPath+b.IndexPath+b.AssetPath, ":") &&
		strings.HasSuffix(b.AssetPath, "/") && validPath(strings.TrimSuffix(b.AssetPath, "/")) &&
		strings.HasSuffix(b.AssetPrefix, "/") && entryChars.MatchString(b.AssetPrefix) && validPath("/"+strings.TrimSuffix(b.AssetPrefix, "/"))
}

func sameParameters(path, upstream string) bool {
	parameters := func(path string) (map[string]bool, bool) {
		out := map[string]bool{}
		for _, part := range strings.Split(path, "/") {
			if strings.HasPrefix(part, ":") {
				name := strings.TrimPrefix(part, ":")
				if !parameterName.MatchString(name) || out[name] {
					return nil, false
				}
				out[name] = true
			} else if strings.Contains(part, ":") {
				return nil, false
			}
		}
		return out, true
	}
	one, ok := parameters(path)
	if !ok {
		return false
	}
	two, ok := parameters(upstream)
	if !ok || len(one) != len(two) {
		return false
	}
	for key := range one {
		if !two[key] {
			return false
		}
	}
	return true
}

func (s *RegisteredSource) Lookup(ctx context.Context, actor Actor, id string) (Binding, error) {
	if s.read == nil || !validActor(actor) || !identifier.MatchString(id) {
		return Binding{}, ErrDenied
	}
	installed, err := s.read(ctx, actor, id)
	if err != nil {
		return Binding{}, err
	}
	if installed.InstallationID != id || installed.OrganizationID != actor.OrganizationID || installed.StationID != actor.StationID || installed.Revision < 1 {
		return Binding{}, ErrDenied
	}
	if installed.State != "installed" {
		return Binding{}, ErrDenied
	}
	registry, revision, err := s.registry()
	if err != nil {
		return Binding{}, err
	}
	var matched *RegisteredRelease
	for i := range registry.Releases {
		release := &registry.Releases[i]
		if release.AppID == installed.AppID && release.AppVersion == installed.AppVersion {
			matched = release
			break
		}
	}
	if matched == nil {
		for i := len(registry.Releases) - 1; i >= 0; i-- {
			release := &registry.Releases[i]
			if release.AppID == installed.AppID && strings.Contains(release.Backend.Origin, ".verdantflare-app.svc.cluster.local") {
				matched = release
				break
			}
		}
	}
	if matched != nil {
		release := *matched
		data, _, err := s.fetch(ctx, actor, release.Backend, "GET", release.Backend.DeclarationPath, nil, 64<<10, "application/json")
		if err != nil {
			return Binding{}, err
		}
		if sum(data) != release.DeclarationDigest {
			for i := len(registry.Releases) - 1; i >= 0; i-- {
				candidate := &registry.Releases[i]
				if candidate.AppID == installed.AppID && candidate.DeclarationDigest == sum(data) {
					release = *candidate
					break
				}
			}
		}
		if sum(data) != release.DeclarationDigest {
			return Binding{}, ErrInvalid
		}
		declaration, err := DecodeDeclaration(bytes.NewReader(data))
		if err != nil {
			return Binding{}, err
		}
		if release.AppVersion != installed.AppVersion && strings.Contains(release.Backend.Origin, ".verdantflare-app.svc.cluster.local") {
			declaration.AppVersion = installed.AppVersion
		}
		if declaration.Dashboard == nil || declaration.Dashboard.AppAPIPolicyRef != release.AppAPIPolicyRef || !strings.HasPrefix(declaration.Dashboard.EntryRef, release.Backend.AssetPrefix) {
			return Binding{}, ErrInvalid
		}
		rules := []Rule{}
		for _, rule := range release.Rules {
			if installed.CanManage || (rule.Method == "GET" && !rule.RequireManage) || contentBusinessRule(installed.AppID, rule) {
				rules = append(rules, rule)
			}
		}
		binding := Binding{InstallationID: id, AppID: installed.AppID, AppVersion: installed.AppVersion, StationID: actor.StationID,
			OrganizationID: actor.OrganizationID, ArtifactDigest: release.ArtifactDigest, Revision: revision,
			Declaration: declaration, Trusted: true, Grants: slices.Clone(release.Grants), Rules: rules, Backend: &release.Backend}
		binding.InstallationRevision = installed.Revision
		binding.ReleaseRevision = releaseRevision(release)
		// Editing workspaces require the current manage permission. Removing it
		// changes the binding and retires any previously opened workspace.
		if installed.CanManage {
			binding.InstanceEntries = release.InstanceEntries
		}
		// The full installation revision also invalidates sessions after an accepted
		// management operation, even if the release declaration is unchanged.
		binding.Revision = sum([]byte(revision + ":" + fmtRevision(installed.Revision)))
		if _, err = validate(actor, binding, id); err != nil {
			return Binding{}, err
		}
		if _, err = s.readIndex(ctx, actor, binding); err != nil {
			return Binding{}, err
		}
		return binding, nil
	}
	return Binding{}, ErrDenied
}

func fmtRevision(revision int64) string {
	// Keep revision formatting independent of locale and public identifiers.
	return strconv.FormatInt(revision, 10)
}

func (s *RegisteredSource) internalRequest(ctx context.Context, actor Actor, backend Backend, method, path string, body []byte, mime string) (*http.Request, error) {
	if !validBackend(backend) || !validPath(path) {
		return nil, ErrInvalid
	}
	token := s.credential(backend.TokenEnv)
	if len(token) < 32 || strings.ContainsAny(token, " \t\r\n") {
		return nil, ErrUnavailable
	}
	req, err := http.NewRequestWithContext(ctx, method, backend.Origin+path, bytes.NewReader(body))
	if err != nil {
		return nil, ErrInvalid
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-User-Id", actor.UserID)
	req.Header.Set("X-Organization-Id", actor.OrganizationID)
	req.Header.Set("X-Station-Id", actor.StationID)
	req.Header.Set("X-Request-Id", uuid.Must(uuid.NewV7()).String())
	req.Header.Set("Accept", mime)
	if len(body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	return req, nil
}

func (s *RegisteredSource) fetch(ctx context.Context, actor Actor, backend Backend, method, path string, body []byte, maximum int64, mime string) ([]byte, int, error) {
	req, err := s.internalRequest(ctx, actor, backend, method, path, body, mime)
	if err != nil {
		return nil, 0, err
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, 0, ErrUnavailable
	}
	defer resp.Body.Close()
	if resp.StatusCode == 401 || resp.StatusCode == 403 || resp.StatusCode == 404 {
		return nil, resp.StatusCode, ErrDenied
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, resp.StatusCode, ErrUnavailable
	}
	if strings.TrimSpace(strings.Split(resp.Header.Get("Content-Type"), ";")[0]) != mime {
		return nil, 0, ErrInvalid
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maximum+1))
	if err != nil || int64(len(data)) > maximum {
		return nil, 0, ErrInvalid
	}
	return data, resp.StatusCode, nil
}

type assetIndex struct {
	SchemaVersion int               `json:"schemaVersion"`
	Files         map[string]string `json:"files"`
}

func (s *RegisteredSource) readIndex(ctx context.Context, actor Actor, binding Binding) (assetIndex, error) {
	if binding.Backend == nil || binding.Declaration.Dashboard == nil {
		return assetIndex{}, ErrUnavailable
	}
	data, _, err := s.fetch(ctx, actor, *binding.Backend, "GET", binding.Backend.IndexPath, nil, 64<<10, "application/json")
	if err != nil {
		return assetIndex{}, err
	}
	if sum(data) != binding.ArtifactDigest {
		return assetIndex{}, ErrInvalid
	}
	// File names are data, not Go fields; decode their bounded dictionary after
	// strict validation of duplicate JSON keys and the two-field outer shape.
	var index assetIndex
	if strictjson.Decode(bytes.NewReader(data), 64<<10, &index) != nil || index.SchemaVersion != 1 || len(index.Files) == 0 || len(index.Files) > 256 {
		return assetIndex{}, ErrInvalid
	}
	for path, hash := range index.Files {
		if !entryChars.MatchString(path) || !validPath("/"+path) || !rawDigest.MatchString(hash) {
			return assetIndex{}, ErrInvalid
		}
	}
	entry := strings.TrimPrefix(binding.Declaration.Dashboard.EntryRef, binding.Backend.AssetPrefix)
	if _, ok := index.Files[entry]; !ok {
		return assetIndex{}, ErrInvalid
	}
	return index, nil
}

// Asset consumes a Binding already checked by Resolver for this request.
func (s *RegisteredSource) Asset(ctx context.Context, actor Actor, binding Binding, path string) ([]byte, string, error) {
	if binding.Backend == nil || !entryChars.MatchString(path) || !validPath("/"+path) || !strings.HasPrefix(path, binding.Backend.AssetPrefix) {
		return nil, "", ErrDenied
	}
	index, err := s.readIndex(ctx, actor, binding)
	if err != nil {
		return nil, "", err
	}
	relative := strings.TrimPrefix(path, binding.Backend.AssetPrefix)
	expected, ok := index.Files[relative]
	if !ok {
		return nil, "", ErrDenied
	}
	mime := ""
	switch {
	case strings.HasSuffix(relative, ".html"):
		mime = "text/html"
	case strings.HasSuffix(relative, ".mjs"), strings.HasSuffix(relative, ".js"):
		mime = "text/javascript"
	case strings.HasSuffix(relative, ".css"):
		mime = "text/css"
	default:
		return nil, "", ErrUnsupported
	}
	data, _, err := s.fetch(ctx, actor, *binding.Backend, "GET", binding.Backend.AssetPath+relative, nil, 4<<20, mime)
	if err != nil {
		return nil, "", err
	}
	if sum(data) != "sha256:"+expected {
		return nil, "", ErrInvalid
	}
	return data, mime + "; charset=utf-8", nil
}

var _ Source = (*RegisteredSource)(nil)
