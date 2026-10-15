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

func childFixture(t *testing.T) Input {
	in := fixture(t)
	in.Rules["restricted"] = AccessRule{Scopes: []string{"app:write"}}
	editDeclaration(t, &in, func(d *Declaration) {
		d.SchemaVersion = 2
		d.Entry.Children = []DeclarationChild{
			{EntryID: "storage", Title: "存储管理", Route: "/?view=storage", AccessRuleRef: "app-reader", Order: 20},
			{EntryID: "monitor", Title: "资源监控", Route: "/", AccessRuleRef: "app-reader", Order: 10},
			{EntryID: "private", Title: "管理", Route: "/admin", AccessRuleRef: "restricted", Order: 30},
		}
	})
	return in
}
func TestChildrenProjectTogetherWithParentPermissionsAndPreference(t *testing.T) {
	in := childFixture(t)
	in.Preferences.Entries = []Preference{{AppID: "sample", EntryID: "main", Pinned: false, Order: 3}}
	in.Bindings[0].Availability = "offline"
	p, err := Resolve(in)
	if err != nil || len(p.Entries) != 1 {
		t.Fatal("missing parent", err)
	}
	e := p.Entries[0]
	if e.Pinned || e.Availability != "offline" || len(e.Children) != 2 || e.Children[0].EntryID != "monitor" || e.Children[1].Route != "/?view=storage" {
		t.Fatal("incorrect child projection", e)
	}
	in.Scopes = append(in.Scopes, "app:write")
	next, err := Resolve(in)
	if err != nil || len(next.Entries[0].Children) != 3 || next.Revision == p.Revision {
		t.Fatal("child permission did not change revision", err)
	}
	in.Roles = nil
	denied, err := Resolve(in)
	if err != nil || len(denied.Entries) != 0 {
		t.Fatal("parent authorization bypassed", err)
	}
}
func TestChildrenFilterEmptyContainersButKeepDirectParents(t *testing.T) {
	in := childFixture(t)
	editDeclaration(t, &in, func(d *Declaration) { d.Entry.Children = d.Entry.Children[2:] })
	p, err := Resolve(in)
	if err != nil || len(p.Entries) != 1 || len(p.Entries[0].Children) != 0 {
		t.Fatal("direct parent lost", err)
	}
	editDeclaration(t, &in, func(d *Declaration) { d.Entry.Route = "" })
	p, err = Resolve(in)
	if err != nil || len(p.Entries) != 0 {
		t.Fatal("empty container leaked", err)
	}
}
func TestChildDeclarationBoundaries(t *testing.T) {
	for name, mutate := range map[string]func(*Declaration){
		"old version":      func(d *Declaration) { d.SchemaVersion = 1 },
		"duplicate":        func(d *Declaration) { d.Entry.Children[1].EntryID = "storage" },
		"parent collision": func(d *Declaration) { d.Entry.Children[0].EntryID = "main" },
		"unknown rule":     func(d *Declaration) { d.Entry.Children[0].AccessRuleRef = "unknown" },
		"external":         func(d *Declaration) { d.Entry.Children[0].Route = "//outside.example" },
		"encoded":          func(d *Declaration) { d.Entry.Children[0].Route = "/?view=%2Fadmin" },
		"duplicate query":  func(d *Declaration) { d.Entry.Children[0].Route = "/?view=a&view=b" },
		"fragment":         func(d *Declaration) { d.Entry.Children[0].Route = "/#tasks" },
		"order":            func(d *Declaration) { d.Entry.Children[0].Order = -1 },
		"capacity":         func(d *Declaration) { d.Entry.Children = make([]DeclarationChild, 33) },
	} {
		t.Run(name, func(t *testing.T) {
			in := childFixture(t)
			editDeclaration(t, &in, mutate)
			if _, err := Resolve(in); err == nil {
				t.Fatal("invalid child declaration accepted")
			}
		})
	}
	in := fixture(t)
	raw := strings.Replace(string(in.Bindings[0].Declaration), `"entry":{`, `"entry":{"children":[],`, 1)
	in.Bindings[0].Declaration = []byte(raw)
	in.Bindings[0].DeclarationDigest = sum([]byte(raw))
	if _, err := Resolve(in); err == nil {
		t.Fatal("v1 accepted the v2 field")
	}
}
