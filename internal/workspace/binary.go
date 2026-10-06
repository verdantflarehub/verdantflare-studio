package workspace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"mime"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/verdantflarehub/verdantflare-studio/internal/project"
)

const maxUploadBytes int64 = 1 << 50

// SaveFiles explicitly uploads selected local files and then commits their
// immutable content references into the Project. It never scans the directory
// and writes the pending commit only after every Artifact upload is complete.
func (w *Workspace) SaveFiles(ctx context.Context, inputs []FileInput) (project.Result, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.saveFilesLocked(ctx, inputs)
}

func (w *Workspace) saveFilesLocked(ctx context.Context, inputs []FileInput) (project.Result, error) {
	if len(inputs) == 0 || len(inputs) > project.MaxFiles {
		return project.Result{}, ErrInvalid
	}
	uploader, ok := w.remote.(BinaryRemote)
	if !ok {
		return project.Result{}, project.ErrDependency
	}
	if e := w.authorize(ctx); e != nil {
		return project.Result{}, e
	}
	if e := w.checkProjection(w.manifest, w.manifest); e != nil {
		return project.Result{}, e
	}
	if _, e := w.read(".vf/pending.json", project.MaxManifestBytes); e == nil {
		return project.Result{}, ErrPending
	} else if !errors.Is(e, os.ErrNotExist) {
		return project.Result{}, e
	}

	known := make(map[string]project.File, len(w.manifest.Files))
	paths := make(map[string]string, len(w.manifest.Files))
	for _, file := range w.manifest.Files {
		known[file.ID] = file
		paths[project.PathKey(file.Path)] = file.ID
	}
	seenIDs := map[string]bool{}
	seenPaths := map[string]bool{}
	updates := make([]project.FileUpdate, 0, len(inputs))
	for _, input := range inputs {
		if project.PortablePath(input.Path) != nil || !rolePattern.MatchString(input.Role) {
			return project.Result{}, ErrInvalid
		}
		if _, _, e := mime.ParseMediaType(input.MIME); e != nil || strings.TrimSpace(input.MIME) == "" {
			return project.Result{}, ErrInvalid
		}
		if seenPaths[project.PathKey(input.Path)] {
			return project.Result{}, ErrInvalid
		}
		seenPaths[project.PathKey(input.Path)] = true
		fileID := input.FileID
		if fileID != "" {
			if !project.ValidID(fileID) || seenIDs[fileID] {
				return project.Result{}, ErrInvalid
			}
			old, exists := known[fileID]
			if !exists || old.Path != input.Path || old.Role != input.Role {
				return project.Result{}, ErrInvalid
			}
			seenIDs[fileID] = true
		} else if _, exists := paths[project.PathKey(input.Path)]; exists {
			// A path already in the manifest must identify the existing file; an
			// omitted ID cannot accidentally replace it.
			return project.Result{}, ErrConflict
		} else {
			paths[project.PathKey(input.Path)] = "pending"
		}
		file, size, sum, e := w.localUploadFile(input.Path)
		if e != nil {
			return project.Result{}, e
		}
		content, e := uploader.Upload(ctx, UploadRequest{
			ProjectID: w.state.ProjectID,
			Source:    project.Source{Kind: "user_import", ProjectID: w.state.ProjectID},
			WriteID:   uuid.Must(uuid.NewV7()).String(),
			MIME:      input.MIME,
			Size:      size,
			SHA256:    sum,
		}, file)
		closeErr := file.Close()
		if e == nil {
			e = closeErr
		}
		if e != nil {
			return project.Result{}, e
		}
		if !content.ContentRef.Valid() || content.Size != size || content.SHA256 != sum || content.MIME != input.MIME {
			return project.Result{}, ErrCorrupt
		}
		ref := content.ContentRef
		updates = append(updates, project.FileUpdate{ID: fileID, Path: input.Path, Role: input.Role, Content: &ref})
	}
	request := project.CommitRequest{
		ProjectID:          w.state.ProjectID,
		ExpectedRevisionID: w.state.BaseRevisionID,
		CommitID:           uuid.Must(uuid.NewV7()).String(),
		Changes:            &project.Changes{UpsertFiles: &updates},
	}
	if e := ctx.Err(); e != nil {
		return project.Result{}, e
	}
	if e := w.writeJSON(".vf/pending.json", request, false); e != nil {
		return project.Result{}, e
	}
	return w.resume(ctx)
}

// ImportFiles copies explicitly selected external files into new relative
// workspace paths and then uses the same immutable Artifact/Project save path.
// Existing targets are never overwritten; an import that cannot be committed
// leaves the copied local file available for a later explicit retry.
func (w *Workspace) ImportFiles(ctx context.Context, inputs []ImportInput) (project.Result, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(inputs) == 0 || len(inputs) > project.MaxFiles {
		return project.Result{}, ErrInvalid
	}
	seen := map[string]bool{}
	for _, input := range inputs {
		if strings.TrimSpace(input.SourcePath) == "" || project.PortablePath(input.Path) != nil || !rolePattern.MatchString(input.Role) {
			return project.Result{}, ErrInvalid
		}
		if _, _, e := mime.ParseMediaType(input.MIME); e != nil || strings.TrimSpace(input.MIME) == "" || seen[project.PathKey(input.Path)] {
			return project.Result{}, ErrInvalid
		}
		seen[project.PathKey(input.Path)] = true
		info, e := os.Lstat(input.SourcePath)
		if e != nil {
			return project.Result{}, e
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Size() < 0 || info.Size() > maxUploadBytes {
			return project.Result{}, ErrInvalid
		}
		if e = w.safe(input.Path, true); e != nil {
			return project.Result{}, e
		}
		if _, e = w.root.Lstat(input.Path); e == nil {
			return project.Result{}, ErrConflict
		} else if !errors.Is(e, os.ErrNotExist) {
			return project.Result{}, e
		}
		source, e := os.Open(filepath.Clean(input.SourcePath))
		if e != nil {
			return project.Result{}, e
		}
		tmp, target, e := w.temporary()
		if e != nil {
			source.Close()
			return project.Result{}, e
		}
		n, copyErr := io.Copy(target, source)
		closeSource := source.Close()
		if copyErr == nil {
			copyErr = closeSource
		}
		if copyErr == nil && n != info.Size() {
			copyErr = ErrCorrupt
		}
		if copyErr == nil {
			copyErr = target.Sync()
		}
		closeTarget := target.Close()
		if copyErr == nil {
			copyErr = closeTarget
		}
		if copyErr != nil {
			_ = w.root.Remove(tmp)
			return project.Result{}, copyErr
		}
		if e = w.safe(input.Path, true); e == nil {
			e = w.root.Link(tmp, input.Path)
		}
		_ = w.root.Remove(tmp)
		if e != nil {
			return project.Result{}, e
		}
		if e = syncDirectory(w.root, filepath.ToSlash(filepath.Dir(input.Path))); e != nil {
			return project.Result{}, e
		}
	}
	fileInputs := make([]FileInput, 0, len(inputs))
	for _, input := range inputs {
		fileInputs = append(fileInputs, FileInput{Path: input.Path, Role: input.Role, MIME: input.MIME})
	}
	return w.saveFilesLocked(ctx, fileInputs)
}

func (w *Workspace) localUploadFile(name string) (*os.File, int64, string, error) {
	if e := w.safe(name, false); e != nil {
		return nil, 0, "", e
	}
	file, e := w.root.Open(name)
	if e != nil {
		return nil, 0, "", e
	}
	info, e := file.Stat()
	if e != nil || !info.Mode().IsRegular() || info.Size() < 0 || info.Size() > maxUploadBytes {
		file.Close()
		if e != nil {
			return nil, 0, "", e
		}
		return nil, 0, "", ErrInvalid
	}
	hash := sha256.New()
	if _, e = io.Copy(hash, file); e != nil {
		file.Close()
		return nil, 0, "", e
	}
	if _, e = file.Seek(0, io.SeekStart); e != nil {
		file.Close()
		return nil, 0, "", e
	}
	return file, info.Size(), hex.EncodeToString(hash.Sum(nil)), nil
}
