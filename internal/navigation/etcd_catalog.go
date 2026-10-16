package navigation

import (
	"bytes"
	"context"
	"encoding/json"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/verdantflarehub/verdantflare-studio/internal/strictjson"
	clientv3 "go.etcd.io/etcd/client/v3"
)

const CatalogEtcdKey = "/verdantflare/studio/navigation/catalog"

type etcdCatalogRelease struct {
	AppID             string          `json:"appId"`
	AppVersion        string          `json:"appVersion"`
	ArtifactDigest    string          `json:"artifactDigest"`
	DeclarationDigest string          `json:"declarationDigest"`
	Declaration       json.RawMessage `json:"declaration"`
}

type etcdCatalogRecord struct {
	SchemaVersion int                     `json:"schemaVersion"`
	Mode          string                  `json:"mode"`
	Releases      []etcdCatalogRelease    `json:"releases"`
	Rules         map[string]registryRule `json:"rules"`
	Defaults      []registryDefault       `json:"defaults"`
}

// EtcdCatalogSource synchronizes the platform-verified navigation catalog into
// etcd (/verdantflare/studio/navigation/catalog) and serves verified snapshots
// from etcd, falling back to the local verified source when etcd is not attached.
type EtcdCatalogSource struct {
	Local     Source
	Artifacts func() (map[string]string, error)
	Client    func() clientv3.KV

	mu         sync.Mutex
	lastDigest string
	cachedRev  int64
	cached     Catalog
}

func encodeCatalogRecord(cat Catalog) ([]byte, string, error) {
	rec := etcdCatalogRecord{
		SchemaVersion: 1,
		Mode:          cat.Mode,
		Releases:      make([]etcdCatalogRelease, 0, len(cat.Releases)),
		Rules:         make(map[string]registryRule, len(cat.Rules)),
		Defaults:      make([]registryDefault, 0, len(cat.Defaults)),
	}
	keys := make([]string, 0, len(cat.Releases))
	for k := range cat.Releases {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for _, k := range keys {
		b := cat.Releases[k]
		rec.Releases = append(rec.Releases, etcdCatalogRelease{
			AppID:             b.AppID,
			AppVersion:        b.AppVersion,
			ArtifactDigest:    b.ArtifactDigest,
			DeclarationDigest: b.DeclarationDigest,
			Declaration:       json.RawMessage(b.Declaration),
		})
	}
	for name, rule := range cat.Rules {
		rec.Rules[name] = registryRule{Roles: slices.Clone(rule.Roles), Scopes: slices.Clone(rule.Scopes)}
	}
	for _, def := range cat.Defaults {
		rec.Defaults = append(rec.Defaults, registryDefault{
			AppID:   def.AppID,
			EntryID: def.EntryID,
			Group:   def.Group,
			Order:   def.Order,
			Pinned:  def.Pinned,
		})
	}
	raw, err := json.Marshal(rec)
	if err != nil || len(raw) > 1<<20 {
		return nil, "", ErrInvalid
	}
	return raw, sum(raw), nil
}

func decodeCatalogRecord(raw []byte, artifacts map[string]string) (Catalog, error) {
	out := Catalog{Releases: map[string]Binding{}, Rules: map[string]AccessRule{}, Defaults: []Default{}}
	var rec etcdCatalogRecord
	if strictjson.Decode(bytes.NewReader(raw), 1<<20, &rec) != nil || rec.SchemaVersion != 1 || (rec.Mode != "legacy" && rec.Mode != "preinstalled") || rec.Releases == nil || rec.Defaults == nil || rec.Rules == nil || len(rec.Releases) > 128 || len(rec.Defaults) > 128 || len(rec.Rules) > 128 {
		return out, ErrInvalid
	}
	out.Mode = rec.Mode
	if rec.Mode == "legacy" {
		if len(rec.Releases)+len(rec.Defaults)+len(rec.Rules) != 0 {
			return out, ErrInvalid
		}
		return out, nil
	}
	for name, rule := range rec.Rules {
		if !ident.MatchString(name) || rule.Roles == nil || rule.Scopes == nil || len(rule.Roles)+len(rule.Scopes) == 0 || len(rule.Roles)+len(rule.Scopes) > 128 {
			return out, ErrInvalid
		}
		for _, values := range [][]string{rule.Roles, rule.Scopes} {
			seen := map[string]bool{}
			for _, value := range values {
				if !ident.MatchString(value) || seen[value] {
					return out, ErrInvalid
				}
				seen[value] = true
			}
		}
		out.Rules[name] = AccessRule{Roles: rule.Roles, Scopes: rule.Scopes}
	}
	for _, release := range rec.Releases {
		k := release.AppID + "@" + release.AppVersion
		if _, exists := out.Releases[k]; exists || (artifacts != nil && artifacts[k] != release.ArtifactDigest) || len(release.Declaration) == 0 || len(release.Declaration) > 64<<10 {
			return out, ErrInvalid
		}
		binding := Binding{
			AppID:             release.AppID,
			AppVersion:        release.AppVersion,
			ArtifactDigest:    release.ArtifactDigest,
			DeclarationDigest: release.DeclarationDigest,
			Declaration:       []byte(release.Declaration),
			Trusted:           true,
			Availability:      "unknown",
		}
		declaration, err := decode(binding)
		if err != nil {
			return out, err
		}
		if _, exists := out.Rules[declaration.Entry.AccessRuleRef]; !exists {
			return out, ErrInvalid
		}
		out.Releases[k] = binding
	}
	seen := map[string]bool{}
	for _, def := range rec.Defaults {
		k := key(def.AppID, def.EntryID)
		if seen[k] || !ident.MatchString(def.AppID) || !ident.MatchString(def.EntryID) || !ident.MatchString(def.Group) || !orderValid(def.Order) {
			return out, ErrInvalid
		}
		seen[k] = true
		out.Defaults = append(out.Defaults, Default{AppID: def.AppID, EntryID: def.EntryID, Group: def.Group, Order: def.Order, Pinned: def.Pinned})
	}
	return out, nil
}

func cloneCatalog(in Catalog) Catalog {
	out := Catalog{
		Mode:     in.Mode,
		Releases: make(map[string]Binding, len(in.Releases)),
		Rules:    make(map[string]AccessRule, len(in.Rules)),
		Defaults: slices.Clone(in.Defaults),
	}
	for k, b := range in.Releases {
		b.Declaration = slices.Clone(b.Declaration)
		out.Releases[k] = b
	}
	for k, r := range in.Rules {
		out.Rules[k] = AccessRule{Roles: slices.Clone(r.Roles), Scopes: slices.Clone(r.Scopes)}
	}
	return out
}

func (s *EtcdCatalogSource) Load(ctx context.Context) (Catalog, error) {
	if s.Local == nil {
		return Catalog{}, ErrUnavailable
	}
	localCat, err := s.Local.Load(ctx)
	if err != nil {
		return Catalog{}, err
	}
	if s.Client == nil || localCat.Mode == "legacy" {
		return localCat, nil
	}
	client := s.Client()
	if client == nil {
		return localCat, nil
	}
	raw, digest, err := encodeCatalogRecord(localCat)
	if err != nil {
		return Catalog{}, err
	}
	var artifacts map[string]string
	if s.Artifacts != nil {
		artifacts, err = s.Artifacts()
		if err != nil {
			return Catalog{}, ErrUnavailable
		}
	}
	etcdCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	resp, getErr := client.Get(etcdCtx, CatalogEtcdKey)
	if getErr != nil || resp == nil {
		return localCat, nil
	}
	s.mu.Lock()
	needSync := s.lastDigest != digest || len(resp.Kvs) == 0
	s.mu.Unlock()
	if needSync {
		existingMatch := len(resp.Kvs) == 1 && sum(resp.Kvs[0].Value) == digest
		if !existingMatch {
			putResp, putErr := client.Put(etcdCtx, CatalogEtcdKey, strings.TrimSpace(string(raw)))
			if putErr == nil && putResp != nil && putResp.Header != nil {
				s.mu.Lock()
				s.lastDigest = digest
				s.cachedRev = putResp.Header.Revision
				s.cached = cloneCatalog(localCat)
				s.mu.Unlock()
				return localCat, nil
			}
		} else {
			s.mu.Lock()
			s.lastDigest = digest
			s.mu.Unlock()
		}
	}
	if len(resp.Kvs) == 1 {
		modRev := resp.Kvs[0].ModRevision
		s.mu.Lock()
		if modRev > 0 && s.cachedRev == modRev && s.cached.Mode != "" {
			cached := cloneCatalog(s.cached)
			s.mu.Unlock()
			return cached, nil
		}
		s.mu.Unlock()
		if etcdCat, decErr := decodeCatalogRecord(resp.Kvs[0].Value, artifacts); decErr == nil {
			s.mu.Lock()
			s.cachedRev = modRev
			s.cached = cloneCatalog(etcdCat)
			s.mu.Unlock()
			return etcdCat, nil
		}
	}
	return localCat, nil
}

