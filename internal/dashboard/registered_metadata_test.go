package dashboard

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAppCenterOfflineEntries(t *testing.T) {
	for _, scenario := range []string{"valid", "unconfigured", "missing", "tampered", "unsupported", "denied", "identity", "policy", "artifact", "duplicate", "canceled"} {
		t.Run(scenario, func(t *testing.T) {
			f := newRegisteredFixture(t)
			dir := t.TempDir()
			release := &f.registry.Releases[0]
			raw := f.responses[release.Backend.DeclarationPath]
			declaration, err := DecodeDeclaration(strings.NewReader(string(raw)))
			if err != nil {
				t.Fatal(err)
			}
			switch scenario {
			case "unsupported":
				declaration.Dashboard.MinimumSDKVersion = "99.0.0"
			case "denied":
				release.Grants = []string{}
			case "identity":
				declaration.AppID = "different"
			case "policy":
				declaration.Dashboard.AppAPIPolicyRef = "different-policy"
			case "artifact":
				declaration.Dashboard.ArtifactDigest = "sha256:" + strings.Repeat("f", 64)
			}
			raw = marshalFixture(t, declaration)
			if scenario == "duplicate" {
				raw = []byte(strings.Replace(string(raw), `"schemaVersion":1`, `"schemaVersion":1,"schemaVersion":1`, 1))
			}
			release.DeclarationDigest = sum(raw)
			if scenario == "tampered" {
				raw = append(raw, '\n')
			}
			if scenario != "missing" {
				if err := os.WriteFile(filepath.Join(dir, strings.TrimPrefix(release.DeclarationDigest, "sha256:")+".json"), raw, 0600); err != nil {
					t.Fatal(err)
				}
			}
			f.write(t)
			if scenario == "unconfigured" {
				dir = ""
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if scenario == "canceled" {
				cancel()
			}
			entries, err := f.source.RegisteredEntries(ctx, dir)
			if f.calls != 0 {
				t.Fatal("listing contacted App")
			}
			switch scenario {
			case "identity", "policy", "artifact", "tampered", "duplicate":
				if !errors.Is(err, ErrInvalid) {
					t.Fatal("unverified material accepted", err)
				}
				return
			case "canceled":
				if !errors.Is(err, context.Canceled) {
					t.Fatal(err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			entry := entries[release.AppID+"@"+release.AppVersion]
			want := "available"
			if scenario == "missing" || scenario == "unconfigured" {
				want = "unavailable"
			}
			if scenario == "unsupported" {
				want = "incompatible"
			}
			if scenario == "denied" {
				want = "denied"
			}
			if entry.Availability != want || entry.Revision == "" {
				t.Fatalf("entry=%+v", entry)
			}
		})
	}
}

func TestAppCenterCheckedOpenUsesLiveBindingBeforeSession(t *testing.T) {
	f := newRegisteredFixture(t)
	r := New(f.source)
	changed := errors.New("binding changed")
	calls := 0
	_, err := r.OpenChecked(context.Background(), f.actor, f.installed.InstallationID, func(binding Binding) error {
		calls++
		if binding.InstallationRevision != f.installed.Revision || binding.ReleaseRevision != releaseRevision(f.registry.Releases[0]) {
			t.Fatal("missing live binding")
		}
		return changed
	})
	if !errors.Is(err, changed) || calls != 1 || f.calls != 2 || len(r.sessions) != 0 {
		t.Fatal("binding check skipped or session allocated", err, f.calls)
	}
	f.responses[f.registry.Releases[0].Backend.DeclarationPath] = []byte("{}")
	_, err = r.OpenChecked(context.Background(), f.actor, f.installed.InstallationID, func(Binding) error { t.Fatal("tampered release reached check"); return nil })
	if !errors.Is(err, ErrInvalid) || len(r.sessions) != 0 {
		t.Fatal("original byte checks bypassed", err)
	}
}

func TestAppCenterEntryRevisionIgnoresUnrelatedRelease(t *testing.T) {
	f := newRegisteredFixture(t)
	before, err := f.source.RegisteredEntries(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	other := f.registry.Releases[0]
	other.AppID = "unrelated"
	f.registry.Releases = append(f.registry.Releases, other)
	f.write(t)
	after, err := f.source.RegisteredEntries(context.Background(), "")
	key := f.installed.AppID + "@" + f.installed.AppVersion
	if err != nil || before[key].Revision != after[key].Revision {
		t.Fatal("unrelated release invalidated binding", err)
	}
	f.registry.Releases[0].Grants = []string{}
	f.write(t)
	after, err = f.source.RegisteredEntries(context.Background(), "")
	if err != nil || before[key].Revision == after[key].Revision {
		t.Fatal("policy change retained binding", err)
	}
}
