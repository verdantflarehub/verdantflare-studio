package dashboard

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
)

// RegisteredEntry describes a locally verified declaration, never a live App.
// Revision is scoped to this release, so unrelated registry edits do not revoke
// an AppCenter card's binding. Runtime loading still verifies original bytes.
type RegisteredEntry struct {
	Revision       string
	ArtifactDigest string
	Quality        string
	Availability   string
	ReasonCode     string
}

func releaseRevision(release RegisteredRelease) string {
	raw, _ := json.Marshal(release)
	return sum(raw)
}

func (s *RegisteredSource) RegisteredEntries(ctx context.Context, directory string) (map[string]RegisteredEntry, error) {
	registry, _, err := s.registry()
	if err != nil {
		return nil, err
	}
	var root *os.Root
	if directory != "" {
		root, err = os.OpenRoot(directory)
		if err != nil {
			return nil, ErrUnavailable
		}
		defer root.Close()
	}
	entries := make(map[string]RegisteredEntry, len(registry.Releases))
	for _, release := range registry.Releases {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		entry := RegisteredEntry{Revision: releaseRevision(release), ArtifactDigest: release.ArtifactDigest, Quality: "unknown", Availability: "unavailable", ReasonCode: "ENTRY_UNVERIFIED"}
		key := release.AppID + "@" + release.AppVersion
		entries[key] = entry
		if root == nil {
			continue
		}
		file, err := root.Open(strings.TrimPrefix(release.DeclarationDigest, "sha256:") + ".json")
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, ErrUnavailable
		}
		info, statErr := file.Stat()
		if statErr != nil || !info.Mode().IsRegular() || info.Size() > 64<<10 {
			file.Close()
			return nil, ErrInvalid
		}
		raw, readErr := io.ReadAll(io.LimitReader(file, (64<<10)+1))
		file.Close()
		if readErr != nil {
			return nil, ErrUnavailable
		}
		if len(raw) > 64<<10 || sum(raw) != release.DeclarationDigest {
			return nil, ErrInvalid
		}
		declaration, err := DecodeDeclaration(bytes.NewReader(raw))
		if err != nil {
			return nil, ErrInvalid
		}
		page := declaration.Dashboard
		if page == nil || page.AppAPIPolicyRef != release.AppAPIPolicyRef || !strings.HasPrefix(page.EntryRef, release.Backend.AssetPrefix) {
			return nil, ErrInvalid
		}
		_, err = validateDefinition(Binding{AppID: release.AppID, AppVersion: release.AppVersion,
			Revision: entry.Revision, ArtifactDigest: release.ArtifactDigest, Declaration: declaration,
			Grants: release.Grants, Rules: release.Rules})
		entry.Quality = "known"
		switch {
		case err == nil:
			entry.Availability, entry.ReasonCode = "available", ""
		case errors.Is(err, ErrUnsupported):
			entry.Availability, entry.ReasonCode = "incompatible", "HOST_UNSUPPORTED"
		case errors.Is(err, ErrDenied):
			entry.Availability, entry.ReasonCode = "denied", "PERMISSION_DENIED"
		default:
			return nil, ErrInvalid
		}
		entries[key] = entry
	}
	return entries, nil
}

// RegisteredArtifacts returns only locally registered immutable artifact
// identities. Reading these does not contact an App or assert that it is online.
func (s *RegisteredSource) RegisteredArtifacts() (map[string]string, error) {
	registry, _, err := s.registry()
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(registry.Releases))
	for _, release := range registry.Releases {
		out[release.AppID+"@"+release.AppVersion] = release.ArtifactDigest
	}
	return out, nil
}
