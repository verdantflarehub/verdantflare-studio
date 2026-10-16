package navigation

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.etcd.io/etcd/api/v3/mvccpb"
	clientv3 "go.etcd.io/etcd/client/v3"
)

func sourceFixture(t *testing.T) (*RegisteredSource, registry, []byte) {
	t.Helper()
	dir := t.TempDir()
	artifact := "sha256:" + strings.Repeat("a", 64)
	declaration := Declaration{SchemaVersion: 1, AppID: "sample", AppVersion: "1.0.0", ArtifactDigest: artifact, Entry: DeclarationEntry{EntryID: "main", Title: "Sample", IconRef: "network", Route: "/", AccessRuleRef: "sample-read"}}
	data, err := json.Marshal(declaration)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(dir, "navigation.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	reg := registry{SchemaVersion: 1, Mode: "preinstalled", Releases: []registryRelease{{AppID: "sample", AppVersion: "1.0.0", ArtifactDigest: artifact, DeclarationDigest: sum(data), DeclarationRef: "navigation.json"}}, Rules: map[string]registryRule{"sample-read": {Roles: []string{"admin"}, Scopes: []string{"app:read"}}}, Defaults: []registryDefault{{AppID: "sample", EntryID: "main", Group: "applications", Order: 1, Pinned: true}}}
	source := &RegisteredSource{Path: filepath.Join(dir, "registry.json"), Artifacts: func() (map[string]string, error) { return map[string]string{"sample@1.0.0": artifact}, nil }}
	writeSourceRegistry(t, source, reg)
	return source, reg, data
}
func writeSourceRegistry(t *testing.T, s *RegisteredSource, reg registry) {
	t.Helper()
	data, err := json.Marshal(reg)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(s.Path, data, 0600); err != nil {
		t.Fatal(err)
	}
}
func TestRegisteredNavigationLoadsVerifiedOfflineMetadata(t *testing.T) {
	s, _, _ := sourceFixture(t)
	catalog, err := s.Load(context.Background())
	if err != nil || len(catalog.Releases) != 1 || !catalog.Releases["sample@1.0.0"].Trusted || catalog.Releases["sample@1.0.0"].Availability != "unknown" {
		t.Fatal("local metadata rejected", err)
	}
}

func TestNavigationMigrationModeRequiresExplicitValidConfiguration(t *testing.T) {
	ctx := context.Background()
	missing := &RegisteredSource{}
	if catalog, err := missing.Load(ctx); err != nil || catalog.Mode != "legacy" {
		t.Fatal("unconfigured host lost legacy mode", err)
	}
	s, reg, _ := sourceFixture(t)
	reg.Mode = "legacy"
	writeSourceRegistry(t, s, reg)
	if _, err := s.Load(ctx); err == nil {
		t.Fatal("legacy config accepted dynamic policy")
	}
	reg.Releases = []registryRelease{}
	reg.Rules = map[string]registryRule{}
	reg.Defaults = []registryDefault{}
	writeSourceRegistry(t, s, reg)
	if catalog, err := s.Load(ctx); err != nil || catalog.Mode != "legacy" {
		t.Fatal("explicit legacy rejected", err)
	}
	reg.Mode = "unknown"
	writeSourceRegistry(t, s, reg)
	if _, err := s.Load(ctx); err == nil {
		t.Fatal("invalid mode fell back")
	}
	s.Path = filepath.Join(t.TempDir(), "missing.json")
	if _, err := s.Load(ctx); err == nil {
		t.Fatal("missing configured file fell back")
	}
}
func TestRegisteredNavigationRejectsUnboundAndUnsafeMetadata(t *testing.T) {
	for _, kind := range []string{"artifact", "digest", "version", "traversal", "absolute", "windows", "duplicate", "rule", "unknown", "tampered"} {
		t.Run(kind, func(t *testing.T) {
			s, reg, data := sourceFixture(t)
			switch kind {
			case "artifact":
				reg.Releases[0].ArtifactDigest = "sha256:" + strings.Repeat("b", 64)
			case "digest":
				reg.Releases[0].DeclarationDigest = "sha256:" + strings.Repeat("c", 64)
			case "version":
				reg.Releases[0].AppVersion = "2.0.0"
			case "traversal":
				reg.Releases[0].DeclarationRef = "../navigation.json"
			case "absolute":
				reg.Releases[0].DeclarationRef = "/navigation.json"
			case "windows":
				reg.Releases[0].DeclarationRef = "C:\\navigation.json"
			case "duplicate":
				reg.Releases = append(reg.Releases, reg.Releases[0])
			case "rule":
				reg.Rules = map[string]registryRule{}
			case "unknown":
				data = []byte(strings.Replace(string(data), `"schemaVersion":1`, `"schemaVersion":1,"system":true`, 1))
				reg.Releases[0].DeclarationDigest = sum(data)
			case "tampered":
				data = append(data, ' ')
			}
			if kind == "unknown" || kind == "tampered" {
				if err := os.WriteFile(filepath.Join(filepath.Dir(s.Path), "navigation.json"), data, 0600); err != nil {
					t.Fatal(err)
				}
			}
			writeSourceRegistry(t, s, reg)
			if _, err := s.Load(context.Background()); err == nil {
				t.Fatal("unsafe metadata accepted")
			}
		})
	}
}

func TestEtcdCatalogSourceSyncsAndReadsVerifiedSnapshot(t *testing.T) {
	s, _, _ := sourceFixture(t)
	kv := &preferenceKV{data: map[string]*mvccpb.KeyValue{}}
	etcdSource := &EtcdCatalogSource{
		Local:     s,
		Artifacts: s.Artifacts,
		Client:    func() clientv3.KV { return kv },
	}
	cat, err := etcdSource.Load(context.Background())
	if err != nil || len(cat.Releases) != 1 || kv.data[CatalogEtcdKey] == nil {
		t.Fatal("failed to sync catalog to etcd", err)
	}
	second := &EtcdCatalogSource{
		Local:     s,
		Artifacts: s.Artifacts,
		Client:    func() clientv3.KV { return kv },
	}
	cat2, err := second.Load(context.Background())
	if err != nil || len(cat2.Releases) != 1 || !cat2.Releases["sample@1.0.0"].Trusted {
		t.Fatal("failed to read catalog from etcd", err)
	}
}
