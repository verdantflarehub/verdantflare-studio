// Package navigation projects trusted installation metadata without executing Apps.
package navigation

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"slices"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/verdantflarehub/verdantflare-studio/internal/strictjson"
)

var ErrInvalid = errors.New("INVALID_NAVIGATION_BINDING")
var ident = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._:-]{0,127}$`)
var version = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)
var digest = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)
var pathPart = regexp.MustCompile(`^[a-zA-Z0-9_-][a-zA-Z0-9._-]*$`)

type Declaration struct {
	SchemaVersion  int              `json:"schemaVersion"`
	AppID          string           `json:"appId"`
	AppVersion     string           `json:"appVersion"`
	ArtifactDigest string           `json:"artifactDigest"`
	Entry          DeclarationEntry `json:"entry"`
}
type DeclarationEntry struct {
	EntryID       string `json:"entryId"`
	Title         string `json:"title"`
	IconRef       string `json:"iconRef"`
	Route         string `json:"route"`
	AccessRuleRef string `json:"accessRuleRef"`
}
type Scope struct {
	UserID             string `json:"userId"`
	OrganizationID     string `json:"organizationId"`
	StationID          string `json:"stationId"`
	AuthorizationEpoch string `json:"authorizationEpoch"`
}
type AccessRule struct{ Roles, Scopes []string }

// Binding is created only by a host-owned provider after authenticating Core.
type Binding struct {
	InstallationID, AppID, AppVersion, OrganizationID, StationID, State string
	ArtifactDigest, DeclarationDigest, Availability                     string
	Trusted                                                             bool
	Declaration                                                         []byte
}
type Default struct {
	AppID, EntryID, Group string
	Order                 int
	Pinned                bool
}
type Preference struct {
	AppID, EntryID string
	Pinned         bool
	Order          int
}
type Preferences struct {
	UserID, OrganizationID, StationID string
	Entries                           []Preference
}
type Input struct {
	Scope         Scope
	Roles, Scopes []string
	Bindings      []Binding
	Rules         map[string]AccessRule
	Defaults      []Default
	Preferences   Preferences
}
type Entry struct {
	InstallationID string `json:"installationId"`
	AppID          string `json:"appId"`
	AppVersion     string `json:"appVersion"`
	EntryID        string `json:"entryId"`
	Title          string `json:"title"`
	IconRef        string `json:"iconRef"`
	Route          string `json:"route"`
	Group          string `json:"group"`
	Order          int    `json:"order"`
	Pinned         bool   `json:"pinned"`
	PinSource      string `json:"pinSource"`
	Availability   string `json:"availability"`
}
type Projection struct {
	Scope    Scope   `json:"scope"`
	Revision string  `json:"revision"`
	Entries  []Entry `json:"entries"`
}

func sum(raw []byte) string { v := sha256.Sum256(raw); return "sha256:" + hex.EncodeToString(v[:]) }
func subject(value string) bool {
	id, e := uuid.Parse(value)
	return e == nil && id != uuid.Nil && id.String() == value
}
func key(app, entry string) string { return app + "/" + entry }
func orderValid(n int) bool        { return n >= 0 && n <= 1000000 }
func all(need, have []string) bool {
	for _, v := range need {
		if !ident.MatchString(v) || !slices.Contains(have, v) {
			return false
		}
	}
	return true
}
func validRoute(path string) bool {
	if path == "/" {
		return true
	}
	if len(path) > 2048 || !strings.HasPrefix(path, "/") {
		return false
	}
	for _, p := range strings.Split(path[1:], "/") {
		if !pathPart.MatchString(p) || p == "." || p == ".." {
			return false
		}
	}
	return true
}
func decode(b Binding) (Declaration, error) {
	var d Declaration
	if !b.Trusted || !digest.MatchString(b.DeclarationDigest) || sum(b.Declaration) != b.DeclarationDigest ||
		strictjson.Decode(bytes.NewReader(b.Declaration), 64<<10, &d) != nil {
		return d, ErrInvalid
	}
	e := d.Entry
	if d.SchemaVersion != 1 || d.AppID != b.AppID || d.AppVersion != b.AppVersion || d.ArtifactDigest != b.ArtifactDigest ||
		!ident.MatchString(d.AppID) || !version.MatchString(d.AppVersion) || !digest.MatchString(d.ArtifactDigest) ||
		!ident.MatchString(e.EntryID) || !ident.MatchString(e.IconRef) || !ident.MatchString(e.AccessRuleRef) || !validRoute(e.Route) ||
		strings.TrimSpace(e.Title) != e.Title || utf8.RuneCountInString(e.Title) == 0 || utf8.RuneCountInString(e.Title) > 48 ||
		strings.ContainsAny(e.Title, "<>") || strings.IndexFunc(e.Title, unicode.IsControl) >= 0 {
		return d, ErrInvalid
	}
	return d, nil
}

func Resolve(in Input) (Projection, error) {
	out := Projection{Scope: in.Scope, Entries: []Entry{}}
	s := in.Scope
	if !subject(s.UserID) || !subject(s.OrganizationID) || !subject(s.StationID) || !ident.MatchString(s.AuthorizationEpoch) ||
		in.Preferences.UserID != s.UserID || in.Preferences.OrganizationID != s.OrganizationID || in.Preferences.StationID != s.StationID ||
		len(in.Bindings) > 128 || len(in.Defaults) > 128 || len(in.Preferences.Entries) > 128 {
		return out, ErrInvalid
	}
	defaults := map[string]Default{}
	for _, d := range in.Defaults {
		k := key(d.AppID, d.EntryID)
		if _, exists := defaults[k]; exists || !ident.MatchString(d.AppID) || !ident.MatchString(d.EntryID) || !ident.MatchString(d.Group) || !orderValid(d.Order) {
			return out, ErrInvalid
		}
		defaults[k] = d
	}
	prefs := map[string]Preference{}
	for _, p := range in.Preferences.Entries {
		k := key(p.AppID, p.EntryID)
		if _, exists := prefs[k]; exists || !ident.MatchString(p.AppID) || !ident.MatchString(p.EntryID) || !orderValid(p.Order) {
			return out, ErrInvalid
		}
		prefs[k] = p
	}
	seenApps, seenInstallations := map[string]bool{}, map[string]bool{}
	for _, b := range in.Bindings {
		if b.OrganizationID != s.OrganizationID || b.StationID != s.StationID {
			return out, ErrInvalid
		}
		if b.State != "installed" {
			continue
		}
		id, err := uuid.Parse(b.InstallationID)
		if err != nil || id.Version() != 7 || id.String() != b.InstallationID || seenApps[b.AppID] || seenInstallations[b.InstallationID] {
			return out, ErrInvalid
		}
		seenApps[b.AppID], seenInstallations[b.InstallationID] = true, true
		d, err := decode(b)
		if err != nil {
			return out, err
		}
		rule, exists := in.Rules[d.Entry.AccessRuleRef]
		if !exists || (len(rule.Roles) == 0 && len(rule.Scopes) == 0) {
			return out, ErrInvalid
		}
		if !all(rule.Roles, in.Roles) || !all(rule.Scopes, in.Scopes) {
			continue
		}
		if !slices.Contains([]string{"ready", "offline", "unknown"}, b.Availability) {
			return out, ErrInvalid
		}
		e := Entry{InstallationID: b.InstallationID, AppID: b.AppID, AppVersion: b.AppVersion, EntryID: d.Entry.EntryID,
			Title: d.Entry.Title, IconRef: d.Entry.IconRef, Route: d.Entry.Route, Group: "applications", Order: 1000000, PinSource: "none", Availability: b.Availability}
		k := key(e.AppID, e.EntryID)
		if value, exists := defaults[k]; exists {
			e.Pinned, e.Group, e.Order, e.PinSource = value.Pinned, value.Group, value.Order, "default"
		}
		if value, exists := prefs[k]; exists {
			e.Pinned, e.Order, e.PinSource = value.Pinned, value.Order, "user"
		}
		out.Entries = append(out.Entries, e)
	}
	sort.Slice(out.Entries, func(i, j int) bool {
		a, b := out.Entries[i], out.Entries[j]
		if a.Order != b.Order {
			return a.Order < b.Order
		}
		return key(a.AppID, a.EntryID) < key(b.AppID, b.EntryID)
	})
	data, err := json.Marshal(out)
	if err != nil {
		return out, ErrInvalid
	}
	out.Revision = sum(data)
	return out, nil
}
