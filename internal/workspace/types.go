// Package workspace manages an explicit, pinned local Project working copy.
// It never synchronizes directories or infers remote deletion from absent files.
package workspace

import (
	"context"
	"errors"
	"io"
	"regexp"

	"github.com/verdantflarehub/verdantflare-studio/internal/project"
)

var (
	ErrConflict  = errors.New("LOCAL_FILE_CONFLICT")
	ErrInvalid   = errors.New("INVALID_WORKSPACE")
	ErrBusy      = errors.New("WORKSPACE_BUSY")
	ErrPending   = errors.New("PENDING_COMMIT")
	ErrCorrupt   = errors.New("WORKSPACE_CONTENT_MISMATCH")
	aliasPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)
	hashPattern  = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

// Remote is supplied by the trusted host with the current authenticated identity.
// Implementations must enforce authorization on each call, including download.
type Remote interface {
	Open(context.Context, string, string) (project.OpenResult, error)
	Metadata(context.Context, project.ContentRef, project.Access) (project.ContentVersion, error)
	Download(context.Context, project.ContentRef, project.Access, io.Writer, int64) error
	Commit(context.Context, project.CommitRequest) (project.Result, error)
}

type State struct {
	SchemaVersion   int      `json:"schema_version"`
	Kind            string   `json:"kind"`
	ConnectionAlias string   `json:"connection_alias"`
	ProjectID       string   `json:"project_id"`
	BaseRevisionID  string   `json:"base_revision_id"`
	Materialized    []string `json:"materialized_file_ids"`
}

type FileStatus struct {
	FileID string `json:"file_id"`
	Path   string `json:"path"`
	State  string `json:"state"`
}

type transition struct {
	From  string `json:"from_revision_id,omitempty"`
	State State  `json:"state"`
}

func validateManifest(m project.Manifest) error {
	if m.Validate() != nil {
		return ErrInvalid
	}
	paths := map[string]bool{}
	for _, f := range m.Files {
		paths[project.PathKey(f.Path)] = true
	}
	for p := range paths {
		for i, c := range p {
			if c == '/' && paths[p[:i]] {
				return ErrInvalid
			}
		}
	}
	return nil
}

func validateState(s State, m project.Manifest) error {
	if s.SchemaVersion != 1 || s.Kind != "local-workspace" || !aliasPattern.MatchString(s.ConnectionAlias) || s.ProjectID != m.ProjectID || !project.ValidID(s.BaseRevisionID) || s.Materialized == nil {
		return ErrInvalid
	}
	files := map[string]bool{}
	for _, f := range m.Files {
		files[f.ID] = true
	}
	seen := map[string]bool{}
	for _, id := range s.Materialized {
		if !files[id] || seen[id] {
			return ErrInvalid
		}
		seen[id] = true
	}
	return nil
}
