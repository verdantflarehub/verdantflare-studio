package appcenter

import (
	"context"
	"net/url"
	"os"
	"regexp"
	"strings"

	"github.com/google/uuid"
)

type LegacyBinding struct {
	BindingID       string `json:"bindingId"`
	AppID           string `json:"appId"`
	OrganizationID  string `json:"organizationId"`
	StationID       string `json:"stationId"`
	ObservationKey  string `json:"observationKey"`
	WorkloadUID     string `json:"workloadUID"`
	AdapterID       string `json:"adapterId"`
	BackendRevision string `json:"backendRevision"`
	Route           string `json:"route"`
}
type LegacyRegistry struct {
	SchemaVersion int             `json:"schemaVersion"`
	Revision      string          `json:"revision"`
	Records       []LegacyBinding `json:"records"`
}
type LegacyOpened struct {
	SchemaVersion   int    `json:"schemaVersion"`
	Transport       string `json:"transport"`
	AdapterID       string `json:"adapterId"`
	BindingID       string `json:"bindingId"`
	BindingRevision string `json:"bindingRevision"`
	Route           string `json:"route"`
}

var mediaRoute = regexp.MustCompile(`^/dashboard(?:/tasks/[a-zA-Z0-9_-]+)?(?:#(?:tasks|models|mcp))?$`)
var legacyBlenderAlias = regexp.MustCompile(`^blender[A-Za-z0-9_-]{1,57}$`)

// Only the compatibility adapter owns these historical bookmark keys. Neither
// app names nor arbitrary module/URL values can select an implementation.
func LegacyRoute(route string) (string, string, error) {
	if len(route) > 2048 || strings.ContainsAny(route, "\\#") || strings.IndexFunc(route, func(r rune) bool { return r < 32 || r == 127 }) >= 0 {
		return "", "", ErrInvalid
	}
	u, err := url.Parse(route)
	if err != nil || u.Scheme != "" || u.Host != "" || u.User != nil || u.Fragment != "" || u.Path != "/market" || u.RawPath != "" || u.ForceQuery {
		return "", "", ErrInvalid
	}
	q, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return "", "", ErrInvalid
	}
	for _, values := range q {
		if len(values) != 1 {
			return "", "", ErrInvalid
		}
	}
	for key, adapter := range map[string]string{"image": "image-iframe-v1", "video": "video-iframe-v1"} {
		if value := q.Get(key); value != "" {
			if len(q) != 1 || !mediaRoute.MatchString(value) {
				return "", "", ErrInvalid
			}
			return adapter, "/market?" + q.Encode(), nil
		}
	}
	alias := q.Get("blender")
	if alias != "instances" && alias != "create" && !legacyBlenderAlias.MatchString(alias) {
		return "", "", ErrInvalid
	}
	for key, value := range q {
		switch key {
		case "blender":
		case "panel":
			if (alias == "instances" || alias == "create") || (value[0] != "overview" && value[0] != "resources" && value[0] != "activity") {
				return "", "", ErrInvalid
			}
		case "operation":
			id, e := uuid.Parse(value[0])
			if e != nil || id.Version() != 7 || id.Variant() != uuid.RFC4122 || id.String() != value[0] || q.Get("panel") == "" {
				return "", "", ErrInvalid
			}
		default:
			return "", "", ErrInvalid
		}
	}
	return "blender-host-v1", "/market?" + q.Encode(), nil
}

// BackendRevision binds only host configuration identity, never a credential.
func LegacyBackendRevision(adapter, endpoint string) string {
	if adapter != "image-iframe-v1" && adapter != "video-iframe-v1" && adapter != "blender-host-v1" {
		return ""
	}
	u, err := url.Parse(endpoint)
	if err != nil || u.Host == "" || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.RawPath != "" {
		return ""
	}
	return Digest(struct{ Adapter, Endpoint string }{adapter, u.String()})
}

// LoadLegacy joins local bindings to already authenticated facts. It never
// creates cards or installation identities and never executes an App.
func LoadLegacy(ctx context.Context, path string, facts Facts, backends map[string]string) (map[string]LegacyBinding, string, error) {
	result := map[string]LegacyBinding{}
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}
	if path == "" {
		return result, Digest(result), nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, "", ErrUnavailable
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
		return nil, "", ErrInvalid
	}
	var registry LegacyRegistry
	if Decode(f, "legacy", 1<<20, &registry) != nil {
		return nil, "", ErrInvalid
	}
	ids, objects := map[string]bool{}, map[string]bool{}
	for _, binding := range registry.Records {
		if err := ctx.Err(); err != nil {
			return nil, "", err
		}
		for _, value := range []string{binding.OrganizationID, binding.StationID, binding.WorkloadUID} {
			id, e := uuid.Parse(value)
			if e != nil || id == uuid.Nil || id.String() != value {
				return nil, "", ErrInvalid
			}
		}
		adapter, canonical, e := LegacyRoute(binding.Route)
		if e != nil || adapter != binding.AdapterID || canonical != binding.Route {
			return nil, "", ErrInvalid
		}
		key := binding.OrganizationID + "/" + binding.StationID + "/" + binding.ObservationKey
		if ids[binding.BindingID] || objects[key] {
			return nil, "", ErrInvalid
		}
		ids[binding.BindingID], objects[key] = true, true
		if binding.OrganizationID != facts.Scope.OrganizationID || binding.StationID != facts.Scope.StationID {
			continue
		}
		for _, fact := range facts.Items {
			if fact.Origin == "observation" && fact.Installation == nil && fact.AppID == binding.AppID && fact.CardKey == binding.ObservationKey {
				result[fact.CardKey] = binding
			}
		}
	}
	// Only matched records affect this scope's source identity.
	return result, Digest(struct {
		Bindings map[string]LegacyBinding
		Backends map[string]string
	}{result, backends}), nil
}
