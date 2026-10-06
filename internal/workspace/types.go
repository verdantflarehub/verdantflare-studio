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
	rolePattern  = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)
)

// Remote is supplied by the trusted host with the current authenticated identity.
// Implementations must enforce authorization on each call, including download.
type Remote interface {
	Open(context.Context, string, string) (project.OpenResult, error)
	Metadata(context.Context, project.ContentRef, project.Access) (project.ContentVersion, error)
	Download(context.Context, project.ContentRef, project.Access, io.Writer, int64) error
	Commit(context.Context, project.CommitRequest) (project.Result, error)
}

// BinaryRemote is an optional extension used by explicit local media saves.
// Implementations prepare and commit an immutable Artifact version and must
// stream the bytes through a separately authorized transport.
type BinaryRemote interface {
	Upload(context.Context, UploadRequest, io.Reader) (project.ContentVersion, error)
}

// ConflictRemote confirms that the exact pending request reached a terminal
// conflict before a human may replace it with a new, reviewed operation.
type ConflictRemote interface {
	CommitStatus(context.Context, string, string) (project.CommitStatus, error)
}

type UploadRequest struct {
	ProjectID string
	Source    project.Source
	WriteID   string
	MIME      string
	Size      int64
	SHA256    string
}

// FileInput describes an explicitly selected local file. An empty FileID
// creates a new Project file; an existing FileID may only keep its pinned path
// and role while replacing its content.
type FileInput struct {
	FileID string `json:"file_id,omitempty"`
	Path   string `json:"path"`
	Role   string `json:"role"`
	MIME   string `json:"mime"`
}

// ImportInput explicitly copies one user-selected external file into the
// workspace before saving it as a new Project file. It never represents a
// directory scan or a remote deletion.
type ImportInput struct {
	SourcePath string `json:"source_path"`
	Path       string `json:"path"`
	Role       string `json:"role"`
	MIME       string `json:"mime"`
}

// uploadJournal is durable local recovery state for binary Artifact writes.
// It records stable write IDs and completed content versions, but never a
// service credential or an upload URL. A journal is only promoted to
// pending.json after every declared content reference is complete.
type uploadJournal struct {
	SchemaVersion      int                 `json:"schema_version"`
	ProjectID          string              `json:"project_id"`
	ExpectedRevisionID string              `json:"expected_revision_id"`
	CommitID           string              `json:"commit_id"`
	Files              []uploadJournalFile `json:"files"`
}

type uploadJournalFile struct {
	FileID  string                  `json:"file_id,omitempty"`
	Path    string                  `json:"path"`
	Role    string                  `json:"role"`
	MIME    string                  `json:"mime"`
	WriteID string                  `json:"write_id"`
	Size    int64                   `json:"size"`
	SHA256  string                  `json:"sha256"`
	Content *project.ContentVersion `json:"content_version,omitempty"`
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
