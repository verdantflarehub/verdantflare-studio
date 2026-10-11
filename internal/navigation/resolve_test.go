package navigation

import (
	"encoding/json"
	"strings"
	"testing"
)

func fixture(t *testing.T) Input {
	t.Helper()
	s := Scope{UserID: "01900000-0000-7000-8000-000000000001", OrganizationID: "01900000-0000-7000-8000-000000000002", StationID: "01900000-0000-7000-8000-000000000003", AuthorizationEpoch: "r1"}
	d := Declaration{SchemaVersion: 1, AppID: "sample", AppVersion: "0.1.0", ArtifactDigest: "sha256:" + strings.Repeat("a", 64),
		Entry: DeclarationEntry{EntryID: "main", Title: "应用", IconRef: "apps", Route: "/", AccessRuleRef: "app-reader"}}
	raw, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	return Input{Scope: s, Roles: []string{"admin"}, Scopes: []string{"app:read"},
		Bindings: []Binding{{InstallationID: "01900000-0000-7000-8000-000000000004", AppID: d.AppID, AppVersion: d.AppVersion, OrganizationID: s.OrganizationID, StationID: s.StationID,
			State: "installed", Trusted: true, Availability: "ready", Declaration: raw, DeclarationDigest: sum(raw), ArtifactDigest: d.ArtifactDigest}},
		Rules:       map[string]AccessRule{"app-reader": {Roles: []string{"admin"}, Scopes: []string{"app:read"}}},
		Defaults:    []Default{{AppID: "sample", EntryID: "main", Group: "applications", Order: 10, Pinned: true}},
		Preferences: Preferences{UserID: s.UserID, OrganizationID: s.OrganizationID, StationID: s.StationID, Entries: []Preference{}}}
}
func editDeclaration(t *testing.T, in *Input, change func(*Declaration)) {
	t.Helper()
	var d Declaration
	if err := json.Unmarshal(in.Bindings[0].Declaration, &d); err != nil {
		t.Fatal(err)
	}
	change(&d)
	raw, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	in.Bindings[0].Declaration = raw
	in.Bindings[0].DeclarationDigest = sum(raw)
}
func TestProjectionKeepsExplicitPreferenceAcrossUpgradeAndReinstall(t *testing.T) {
	in := fixture(t)
	first, err := Resolve(in)
	if err != nil || len(first.Entries) != 1 || !first.Entries[0].Pinned {
		t.Fatal("default missing", err)
	}
	in.Preferences.Entries = []Preference{{AppID: "sample", EntryID: "main", Pinned: false, Order: 3}}
	in.Bindings[0].AppVersion = "0.2.0"
	in.Bindings[0].InstallationID = "01900000-0000-7000-8000-000000000005"
	editDeclaration(t, &in, func(d *Declaration) { d.AppVersion = "0.2.0" })
	next, err := Resolve(in)
	if err != nil || next.Entries[0].Pinned || next.Entries[0].PinSource != "user" || next.Entries[0].InstallationID == first.Entries[0].InstallationID || next.Revision == first.Revision {
		t.Fatal("preference lost", err)
	}
	in.Bindings[0].Availability = "offline"
	offline, err := Resolve(in)
	if err != nil || len(offline.Entries) != 1 || offline.Entries[0].Availability != "offline" {
		t.Fatal("offline installation vanished")
	}
	in.Bindings[0].State = "removed"
	removed, err := Resolve(in)
	if err != nil || len(removed.Entries) != 0 {
		t.Fatal("removed installation visible")
	}
}
func TestProjectionRejectsSpoofedOrInconsistentMetadata(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*Input)
	}{
		{"untrusted", func(in *Input) { in.Bindings[0].Trusted = false }},
		{"digest", func(in *Input) { in.Bindings[0].Declaration = append(in.Bindings[0].Declaration, ' ') }},
		{"foreign station", func(in *Input) { in.Bindings[0].StationID = in.Scope.UserID }},
		{"foreign preferences", func(in *Input) { in.Preferences.UserID = in.Scope.StationID }},
		{"duplicate install", func(in *Input) { in.Bindings = append(in.Bindings, in.Bindings[0]) }},
		{"unknown rule", func(in *Input) { in.Rules = map[string]AccessRule{} }},
		{"wrong version", func(in *Input) { editDeclaration(t, in, func(d *Declaration) { d.AppVersion = "9.0.0" }) }},
		{"wrong artifact", func(in *Input) {
			editDeclaration(t, in, func(d *Declaration) { d.ArtifactDigest = "sha256:" + strings.Repeat("b", 64) })
		}},
		{"encoded escape", func(in *Input) { editDeclaration(t, in, func(d *Declaration) { d.Entry.Route = "/%2e%2e/private" }) }},
		{"external icon", func(in *Input) {
			editDeclaration(t, in, func(d *Declaration) { d.Entry.IconRef = "https://outside/icon" })
		}},
		{"system claim", func(in *Input) {
			raw := strings.Replace(string(in.Bindings[0].Declaration), `"schemaVersion":1`, `"schemaVersion":1,"system":true`, 1)
			in.Bindings[0].Declaration = []byte(raw)
			in.Bindings[0].DeclarationDigest = sum([]byte(raw))
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := fixture(t)
			tc.change(&in)
			if _, err := Resolve(in); err == nil {
				t.Fatal("invalid binding accepted")
			}
		})
	}
}
func TestProjectionUsesCurrentAuthorizationAndStableRevision(t *testing.T) {
	in := fixture(t)
	a, err := Resolve(in)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Resolve(in)
	if err != nil || a.Revision != b.Revision {
		t.Fatal("unstable projection")
	}
	in.Scopes = []string{}
	b, err = Resolve(in)
	if err != nil || len(b.Entries) != 0 || b.Revision == a.Revision {
		t.Fatal("revoked scope leaked")
	}
}
