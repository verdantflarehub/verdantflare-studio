package project

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func id(n int) string { return fmt.Sprintf("0199b501-0000-7000-8000-%012x", n) }
func example() Manifest {
	return Manifest{SchemaVersion: 1, Kind: "project", ProjectID: id(1), Name: "小月", Category: "image", Status: "active", EntryDocumentID: id(2),
		Files:      []File{{ID: id(2), Path: "review.md", Role: "review", Content: ContentRef{id(3), id(4), id(5)}}, {ID: id(6), Path: "images/front.png", Role: "character-front", Content: ContentRef{id(3), id(7), id(8)}}},
		Selections: []Selection{{"character-reference", []string{id(6)}}}, AssetRefs: []AssetRef{}, RunRefs: []RunRef{{"image", "native/job:1"}}, DomainDocuments: []DomainDocument{}}
}

func TestPortablePaths(t *testing.T) {
	for _, p := range []string{"../outside.md", "/root.md", "C:/file.md", "a\\b.png", "a//b", "a/", "a/./b", ".VF/project.json", "NUL.txt", "a/COM1.wav", "LPT¹.txt", "a./b", "a /b", "a\x00b", "e\u0301.png"} {
		t.Run(fmt.Sprintf("%q", p), func(t *testing.T) {
			if PortablePath(p) == nil {
				t.Fatal("accepted unsafe path")
			}
		})
	}
	for _, p := range []string{"制作审核记录.md", "images/小月.png", "é.png", "nested/.vfile", "reviews/review.v1.md"} {
		if err := PortablePath(p); err != nil {
			t.Errorf("%q: %v", p, err)
		}
	}
}

func TestManifestReferentialIntegrity(t *testing.T) {
	cases := map[string]func(*Manifest){
		"duplicate ID":        func(m *Manifest) { m.Files[1].ID = m.Files[0].ID },
		"case collision":      func(m *Manifest) { m.Files[1].Path = "REVIEW.MD" },
		"missing entry":       func(m *Manifest) { m.EntryDocumentID = id(99) },
		"binary entry":        func(m *Manifest) { m.EntryDocumentID = id(6) },
		"dangling selection":  func(m *Manifest) { m.Selections[0].FileIDs = []string{id(99)} },
		"duplicate purpose":   func(m *Manifest) { m.Selections = append(m.Selections, m.Selections[0]) },
		"domain missing file": func(m *Manifest) { m.DomainDocuments = []DomainDocument{{"music", id(99)}} },
		"unversioned asset":   func(m *Manifest) { m.AssetRefs = []AssetRef{{id(40), "latest", "dance"}} },
		"duplicate run":       func(m *Manifest) { m.RunRefs = append(m.RunRefs, m.RunRefs[0]) },
		"missing array":       func(m *Manifest) { m.AssetRefs = nil },
		"extension namespace": func(m *Manifest) { m.Extensions = map[string]json.RawMessage{"password": json.RawMessage(`"example"`)} },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			m := example()
			mutate(&m)
			if m.Validate() == nil {
				t.Fatal("accepted invalid inventory")
			}
		})
	}
	if err := example().Validate(); err != nil {
		t.Fatal(err)
	}
	if PathKey("Σ.png") != PathKey("ς.png") {
		t.Fatal("Unicode case-equivalent paths must collide")
	}
}

func TestStrictDecode(t *testing.T) {
	b, err := json.Marshal(example())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Decode(b); err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string][]byte{
		"duplicate top level": []byte(strings.Replace(string(b), `"name":"小月"`, `"name":"小月","name":"other"`, 1)),
		"duplicate nested":    []byte(strings.Replace(string(b), `"role":"review"`, `"role":"review","role":"other"`, 1)),
		"case variant":        []byte(strings.Replace(string(b), `"project_id"`, `"Project_ID"`, 1)),
		"nested case variant": []byte(strings.Replace(string(b), `"file_id"`, `"FILE_ID"`, 1)),
		"trailing JSON":       append(append([]byte{}, b...), []byte(` {}`)...),
		"too large":           []byte(strings.Repeat(" ", MaxManifestBytes+1)),
		"invalid UTF8":        append([]byte{0xff}, b...),
		"auth field":          []byte(strings.Replace(string(b), `"kind":"project"`, `"kind":"project","organization_id":"client"`, 1)),
		"null extensions":     []byte(strings.Replace(string(b), `"kind":"project"`, `"kind":"project","extensions":null`, 1)),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Decode(data); err == nil {
				t.Fatal("accepted ambiguous/invalid JSON")
			}
		})
	}
}

func TestSelectionRemainsBoundToReviewedContent(t *testing.T) {
	old := example()
	next := example()
	next.Files[1].Content.VersionID = id(99)
	updated, cleared, err := ReconcileSelections(old, next, nil)
	if err != nil || len(updated.Selections) != 0 || len(cleared) != 1 {
		t.Fatalf("replacement retained old review: %v %v", cleared, err)
	}
	if len(old.Selections) != 1 || len(next.Selections) != 1 {
		t.Fatal("input manifest mutated")
	}
	updated, _, err = ReconcileSelections(old, next, []string{"character-reference"})
	if err != nil || len(updated.Selections) != 1 {
		t.Fatal("explicit reselection was lost", err)
	}
	next = example()
	next.Files[1].Path = "references/front.png"
	updated, cleared, err = ReconcileSelections(old, next, nil)
	if err != nil || len(updated.Selections) != 1 || len(cleared) != 0 {
		t.Fatal("rename changed review", err)
	}
	next = example()
	next.Files = next.Files[:1]
	updated, _, err = ReconcileSelections(old, next, nil)
	if err != nil || len(updated.Selections) != 0 {
		t.Fatal("removed file left selection", err)
	}
	if _, _, err = ReconcileSelections(old, next, []string{"character-reference"}); err == nil {
		t.Fatal("reselected deleted file")
	}
}

// Run against the central contracts when developing in the full workspace.
// Standalone source builds have no copy of the design repository.
func TestCentralContractProjectExamples(t *testing.T) {
	path := filepath.Join("..", "..", "..", "docs", "design", "contracts", "v2", "examples.md")
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		t.Skip("central contracts available only in design workspace")
	}
	if err != nil {
		t.Fatal(err)
	}
	blocks := regexp.MustCompile("(?s)## ProjectManifest\\r?\\n\\r?\\n```json\\r?\\n(.*?)\\r?\\n```").FindAllSubmatch(b, -1)
	if len(blocks) != 2 {
		t.Fatalf("expected two project fixtures, got %d", len(blocks))
	}
	for _, block := range blocks {
		if _, err := Decode(block[1]); err != nil {
			t.Fatal("contract example rejected:", err)
		}
	}
}
