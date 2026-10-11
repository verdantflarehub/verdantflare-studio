package dashboard

import (
	"bytes"
	"context"
	"encoding/json"
	"path"
	"strings"

	"github.com/verdantflarehub/verdantflare-studio/internal/strictjson"
)

// ResolveInstance asks the installed App about its instance. URLs and credentials
// are exclusively platform registration data and cannot come back from this API.
func (s *RegisteredSource) ResolveInstance(ctx context.Context, actor Actor, binding Binding, entry InstanceEntry, in InstanceOpen) error {
	if binding.Backend == nil || !ValidInstanceOpen(in) || entry.EntryID != in.EntryID {
		return ErrDenied
	}
	body, _ := json.Marshal(in)
	data, status, err := s.fetch(ctx, actor, *binding.Backend, "POST", entry.ResolvePath, body, 4096, "application/json")
	if err != nil {
		return err
	}
	var result struct {
		InstanceID string `json:"instanceId"`
		EntryID    string `json:"entryId"`
		State      string `json:"state"`
	}
	if status != 200 || strictjson.Decode(bytes.NewReader(data), 4096, &result) != nil ||
		result.InstanceID != in.InstanceID || result.EntryID != in.EntryID {
		return ErrInvalid
	}
	if result.State != "running" {
		return ErrUnavailable
	}
	_, err = s.workspaceIndex(ctx, actor, binding, entry)
	return err
}

func (s *RegisteredSource) workspaceIndex(ctx context.Context, actor Actor, binding Binding, entry InstanceEntry) (assetIndex, error) {
	if binding.Backend == nil {
		return assetIndex{}, ErrDenied
	}
	data, status, err := s.fetch(ctx, actor, *binding.Backend, "GET", entry.IndexPath, nil, 1<<20, "application/json")
	if err != nil {
		return assetIndex{}, err
	}
	var index assetIndex
	if status != 200 || sum(data) != entry.ArtifactDigest || strictjson.Decode(bytes.NewReader(data), 1<<20, &index) != nil ||
		index.SchemaVersion != 1 || len(index.Files) == 0 || len(index.Files) > 4096 {
		return assetIndex{}, ErrInvalid
	}
	for name, hash := range index.Files {
		if !entryChars.MatchString(name) || !validPath("/"+name) || !rawDigest.MatchString(hash) || workspaceMIME(name) == "" {
			return assetIndex{}, ErrInvalid
		}
	}
	if _, ok := index.Files[entry.EntryRef]; !ok {
		return assetIndex{}, ErrInvalid
	}
	return index, nil
}

func workspaceMIME(name string) string {
	return map[string]string{".html": "text/html", ".js": "text/javascript", ".mjs": "text/javascript", ".css": "text/css",
		".json": "application/json", ".png": "image/png", ".svg": "image/svg+xml", ".jpg": "image/jpeg", ".webp": "image/webp",
		".ico": "image/vnd.microsoft.icon", ".woff": "font/woff", ".woff2": "font/woff2", ".ttf": "font/ttf", ".wasm": "application/wasm"}[path.Ext(name)]
}

// PublicWorkspaceAsset is code distribution only, requiring an explicit public
// registration. It never reads installation or user state and never resolves an
// arbitrary App-supplied URL. Anonymous consumers receive verified release bytes.
func (s *RegisteredSource) PublicWorkspaceAsset(ctx context.Context, artifact, name string) ([]byte, string, error) {
	if !rawDigest.MatchString(artifact) {
		return nil, "", ErrDenied
	}
	registry, _, err := s.registry()
	if err != nil {
		return nil, "", err
	}
	for _, release := range registry.Releases {
		for _, entry := range release.InstanceEntries {
			if entry.PublicAssets && entry.ArtifactDigest == "sha256:"+artifact {
				return s.WorkspaceAsset(ctx, Actor{}, Binding{Backend: &release.Backend}, entry, name)
			}
		}
	}
	return nil, "", ErrDenied
}

// WorkspaceAsset consumes only a binding and entry returned by Workspaces.Check.
func (s *RegisteredSource) WorkspaceAsset(ctx context.Context, actor Actor, binding Binding, entry InstanceEntry, name string) ([]byte, string, error) {
	if !entryChars.MatchString(name) || !validPath("/"+name) {
		return nil, "", ErrDenied
	}
	index, err := s.workspaceIndex(ctx, actor, binding, entry)
	if err != nil {
		return nil, "", err
	}
	hash, exists := index.Files[name]
	if !exists {
		return nil, "", ErrDenied
	}
	mime := workspaceMIME(name)
	data, status, err := s.fetch(ctx, actor, *binding.Backend, "GET", entry.AssetPath+name, nil, 16<<20, mime)
	if err != nil {
		return nil, "", err
	}
	if status != 200 || strings.TrimPrefix(sum(data), "sha256:") != hash {
		return nil, "", ErrInvalid
	}
	return data, mime, nil
}

// WorkspaceRequest fixes instance path parameters before matching caller paths.
func (s *RegisteredSource) WorkspaceRequest(ctx context.Context, actor Actor, binding Binding, entry InstanceEntry, instanceID string, in AppRequest, requestID string) ([]byte, int, error) {
	if !ValidRequest(in) || binding.Backend == nil || !identifier.MatchString(requestID) || !identifier.MatchString(instanceID) {
		return nil, 0, ErrDenied
	}
	if _, err := workspaceEntry(actor, binding, binding.InstallationID, entry.EntryID); err != nil {
		return nil, 0, err
	}
	binding.Rules = instanceRules(entry, instanceID)
	if !validRules(binding.Rules) {
		return nil, 0, ErrInvalid
	}
	return s.requestBound(ctx, actor, binding, in, requestID)
}
