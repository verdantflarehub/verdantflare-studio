package project

import (
	"encoding/json"
	"mime"
	"sort"
	"strings"
	"unicode/utf8"
)

func validInline(text, kind string) bool {
	if len(text) > 1<<20 || !utf8.ValidString(text) {
		return false
	}
	mt, _, err := mime.ParseMediaType(kind)
	return err == nil && (strings.HasPrefix(mt, "text/") || mt == "application/json")
}
func initial(r CreateRequest, projectID string) (preparePlan, error) {
	if !ValidID(r.CommitID) || !validText(r.Name) || !tokenPattern.MatchString(r.Category) || PortablePath(r.EntryPath) != nil || !validInline(r.EntryText, "text/markdown") {
		return preparePlan{}, ErrInvalid
	}
	entryID := newID()
	kind := "text/markdown"
	if strings.HasSuffix(strings.ToLower(r.EntryPath), ".txt") {
		kind = "text/plain"
	}
	m := Manifest{SchemaVersion: 1, Kind: "project", ProjectID: projectID, Name: r.Name, Category: r.Category, Status: "draft", EntryDocumentID: entryID, Files: []File{{ID: entryID, Path: r.EntryPath, Role: "review"}}, Selections: []Selection{}, AssetRefs: []AssetRef{}, RunRefs: []RunRef{}, DomainDocuments: []DomainDocument{}}
	if m.validate(map[string]bool{entryID: true}) != nil {
		return preparePlan{}, ErrInvalid
	}
	return preparePlan{Manifest: m, Writes: []TextWrite{{WriteID: newID(), FileID: entryID, Text: r.EntryText, MIME: kind}}, ManifestWriteID: newID(), Refs: []ContentRef{}, Invalidated: []string{}}, nil
}
func changed(previous Manifest, r CommitRequest) (preparePlan, error) {
	if (r.Changes == nil) == (r.Manifest == nil) {
		return preparePlan{}, ErrInvalid
	}
	if r.Changes != nil && len(r.Reselected) > 0 {
		return preparePlan{}, ErrInvalid
	}
	// Clone all inventories: preparation must never mutate a published revision.
	raw, err := json.Marshal(previous)
	if err != nil {
		return preparePlan{}, ErrInvalid
	}
	var m Manifest
	if json.Unmarshal(raw, &m) != nil {
		return preparePlan{}, ErrInvalid
	}
	writes := []TextWrite{}
	pending := map[string]bool{}
	reselected := append([]string{}, r.Reselected...)
	if r.Manifest != nil {
		raw, err = json.Marshal(r.Manifest)
		if err != nil {
			return preparePlan{}, ErrInvalid
		}
		if json.Unmarshal(raw, &m) != nil {
			return preparePlan{}, ErrInvalid
		}
		// New IDs in a complete client manifest are local placeholders. Allocate
		// stable server IDs and rewrite all references in this preparation plan.
		existing := map[string]bool{}
		for _, f := range previous.Files {
			existing[f.ID] = true
		}
		seen, replacements := map[string]bool{}, map[string]string{}
		for i := range m.Files {
			old := m.Files[i].ID
			if !ValidID(old) || seen[old] {
				return preparePlan{}, ErrInvalid
			}
			seen[old] = true
			if !existing[old] {
				replacements[old] = newID()
				m.Files[i].ID = replacements[old]
			}
		}
		if replacement, ok := replacements[m.EntryDocumentID]; ok {
			m.EntryDocumentID = replacement
		}
		for i := range m.Selections {
			for j, id := range m.Selections[i].FileIDs {
				if replacement, ok := replacements[id]; ok {
					m.Selections[i].FileIDs[j] = replacement
				}
			}
		}
		for i := range m.DomainDocuments {
			if replacement, ok := replacements[m.DomainDocuments[i].FileID]; ok {
				m.DomainDocuments[i].FileID = replacement
			}
		}
	} else {
		c := r.Changes
		if c.Metadata != nil {
			v := c.Metadata
			if v.Name != nil {
				m.Name = *v.Name
			}
			if v.Category != nil {
				m.Category = *v.Category
			}
			if v.Status != nil {
				m.Status = *v.Status
			}
			if v.EntryDocumentID != nil {
				m.EntryDocumentID = *v.EntryDocumentID
			}
		}
		remove := map[string]bool{}
		if c.RemoveFileIDs != nil {
			for _, id := range *c.RemoveFileIDs {
				if !ValidID(id) || remove[id] {
					return preparePlan{}, ErrInvalid
				}
				remove[id] = true
			}
		}
		existing := map[string]File{}
		for _, f := range m.Files {
			existing[f.ID] = f
		}
		for id := range remove {
			if _, ok := existing[id]; !ok {
				return preparePlan{}, ErrInvalid
			}
		}
		updates := map[string]File{}
		if c.UpsertFiles != nil {
			for _, f := range *c.UpsertFiles {
				if (f.Content == nil) == (f.Text == nil) {
					return preparePlan{}, ErrInvalid
				}
				id := f.ID
				if id == "" {
					id = newID()
				} else {
					if !ValidID(id) {
						return preparePlan{}, ErrInvalid
					}
					if _, ok := existing[id]; !ok {
						return preparePlan{}, ErrInvalid
					}
				}
				if _, ok := updates[id]; ok || remove[id] {
					return preparePlan{}, ErrInvalid
				}
				v := File{ID: id, Path: f.Path, Role: f.Role}
				if f.Content != nil {
					if f.MIME != "" || !f.Content.Valid() {
						return preparePlan{}, ErrInvalid
					}
					v.Content = *f.Content
				} else {
					if !validInline(*f.Text, f.MIME) {
						return preparePlan{}, ErrInvalid
					}
					pending[id] = true
					writes = append(writes, TextWrite{WriteID: newID(), FileID: id, Text: *f.Text, MIME: f.MIME})
				}
				updates[id] = v
			}
		}
		files := []File{}
		for _, f := range m.Files {
			if remove[f.ID] {
				continue
			}
			if v, ok := updates[f.ID]; ok {
				files = append(files, v)
				delete(updates, f.ID)
			} else {
				files = append(files, f)
			}
		}
		// Preserve request order for new entries rather than ranging over a map.
		for _, w := range writes {
			if f, ok := updates[w.FileID]; ok {
				files = append(files, f)
				delete(updates, w.FileID)
			}
		}
		// Reference-only new entries carry server IDs too; stable order is by ID.
		ids := sortedFileIDs(updates)
		for _, id := range ids {
			files = append(files, updates[id])
		}
		m.Files = files
		if c.Selections != nil {
			m.Selections = append([]Selection{}, (*c.Selections)...)
			for _, s := range m.Selections {
				reselected = append(reselected, s.Purpose)
			}
		}
		if c.AssetRefs != nil {
			m.AssetRefs = append([]AssetRef{}, (*c.AssetRefs)...)
		}
		if c.RunRefs != nil {
			m.RunRefs = append([]RunRef{}, (*c.RunRefs)...)
		}
		if c.DomainDocuments != nil {
			m.DomainDocuments = append([]DomainDocument{}, (*c.DomainDocuments)...)
		}
	}
	m, invalid, err := reconcileSelections(previous, m, reselected, pending)
	if err != nil {
		return preparePlan{}, ErrInvalid
	}
	b, err := json.Marshal(m)
	if err != nil || len(b) > MaxManifestBytes {
		return preparePlan{}, ErrInvalid
	}
	return preparePlan{Manifest: m, Writes: writes, ManifestWriteID: newID(), Refs: []ContentRef{}, Invalidated: invalid}, nil
}
func sortedFileIDs(files map[string]File) []string {
	ids := make([]string, 0, len(files))
	for id := range files {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}
