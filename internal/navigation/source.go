package navigation

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/verdantflarehub/verdantflare-studio/internal/strictjson"
)

var ErrUnavailable = errors.New("NAVIGATION_UNAVAILABLE")

type Catalog struct {
	Mode     string
	Releases map[string]Binding
	Rules    map[string]AccessRule
	Defaults []Default
}

type Source interface {
	Load(context.Context) (Catalog, error)
}

// RegisteredSource reads platform-owned, immutable release metadata; no App is
// started or called in order to build navigation.
type RegisteredSource struct {
	Path      string
	Artifacts func() (map[string]string, error)
}

type registryRelease struct {
	AppID             string `json:"appId"`
	AppVersion        string `json:"appVersion"`
	ArtifactDigest    string `json:"artifactDigest"`
	DeclarationDigest string `json:"declarationDigest"`
	DeclarationRef    string `json:"declarationRef"`
}
type registryDefault struct {
	AppID   string `json:"appId"`
	EntryID string `json:"entryId"`
	Group   string `json:"group"`
	Order   int    `json:"order"`
	Pinned  bool   `json:"pinned"`
}
type registryRule struct {
	Roles  []string `json:"roles"`
	Scopes []string `json:"scopes"`
}
type registry struct {
	SchemaVersion int                     `json:"schemaVersion"`
	Mode          string                  `json:"mode"`
	Releases      []registryRelease       `json:"releases"`
	Rules         map[string]registryRule `json:"rules"`
	Defaults      []registryDefault       `json:"defaults"`
}

func readLocal(root *os.Root, ref string, limit int64) ([]byte, error) {
	if !fs.ValidPath(ref) || strings.ContainsAny(ref, "\\:") || ref == "." {
		return nil, ErrInvalid
	}
	f, err := root.Open(ref)
	if err != nil {
		return nil, ErrUnavailable
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return nil, ErrInvalid
	}
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, ErrUnavailable
	}
	if int64(len(data)) > limit {
		return nil, ErrInvalid
	}
	return data, nil
}

func (s *RegisteredSource) Load(ctx context.Context) (Catalog, error) {
	out := Catalog{Releases: map[string]Binding{}, Rules: map[string]AccessRule{}, Defaults: []Default{}}
	if err := ctx.Err(); err != nil {
		return out, err
	}
	if s.Path == "" {
		out.Mode = "legacy"
		return out, nil
	}
	if s.Artifacts == nil {
		return out, ErrUnavailable
	}
	root, err := os.OpenRoot(filepath.Dir(s.Path))
	if err != nil {
		return out, ErrUnavailable
	}
	defer root.Close()
	data, err := readLocal(root, filepath.Base(s.Path), 1<<20)
	if err != nil {
		return out, err
	}
	var reg registry
	if strictjson.Decode(bytes.NewReader(data), 1<<20, &reg) != nil || reg.SchemaVersion != 1 || (reg.Mode != "legacy" && reg.Mode != "preinstalled") || reg.Releases == nil || reg.Defaults == nil || reg.Rules == nil || len(reg.Releases) > 128 || len(reg.Defaults) > 128 || len(reg.Rules) > 128 {
		return out, ErrInvalid
	}
	out.Mode = reg.Mode
	if reg.Mode == "legacy" {
		if len(reg.Releases)+len(reg.Defaults)+len(reg.Rules) != 0 {
			return out, ErrInvalid
		}
		return out, nil
	}
	artifacts, err := s.Artifacts()
	if err != nil {
		return out, ErrUnavailable
	}
	for name, rule := range reg.Rules {
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
	for _, release := range reg.Releases {
		k := release.AppID + "@" + release.AppVersion
		if _, exists := out.Releases[k]; exists || artifacts[k] != release.ArtifactDigest {
			return out, ErrInvalid
		}
		data, err := readLocal(root, release.DeclarationRef, 64<<10)
		if err != nil {
			return out, err
		}
		binding := Binding{AppID: release.AppID, AppVersion: release.AppVersion, ArtifactDigest: release.ArtifactDigest, DeclarationDigest: release.DeclarationDigest, Declaration: data, Trusted: true, Availability: "unknown"}
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
	for _, def := range reg.Defaults {
		k := key(def.AppID, def.EntryID)
		if seen[k] || !ident.MatchString(def.AppID) || !ident.MatchString(def.EntryID) || !ident.MatchString(def.Group) || !orderValid(def.Order) {
			return out, ErrInvalid
		}
		seen[k] = true
		out.Defaults = append(out.Defaults, Default{AppID: def.AppID, EntryID: def.EntryID, Group: def.Group, Order: def.Order, Pinned: def.Pinned})
	}
	return out, nil
}
