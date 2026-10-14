package appcenter

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestAppCenterPresentationPrecedenceAndIsolation(t *testing.T) {
	v := vectors(t)[0]
	display := *v.Metadata[v.Facts.Items[0].CardKey].Display
	general := PresentationRecord{AppID: "sample", Display: display, SourceRef: "docs/design/sample.md", SourceDigest: Digest("source")}
	exact := general
	exact.AppVersion = Text("1.0.0")
	exact.Display.Name = "Exact release"
	exact.Display.ResourceTags = []ResourceTag{{Kind: "cpu", Value: 2, Unit: "cores", SourceRef: "docs/design/release.md"}}
	general.Display.Models = &[]string{"App model"}
	general.Display.Capabilities = &[]string{"app.read"}
	general.Ordering = &AppOrdering{PriorityClass: "normal"}
	exact.Display.Models = &[]string{}
	exact.Display.Capabilities = &[]string{}
	reg := PresentationRegistry{SchemaVersion: 1, Revision: Digest("registry"), Groups: []Group{{ID: "other", Label: "Other", Order: 1}}, Records: []PresentationRecord{exact, general}}
	raw, _ := json.Marshal(reg)
	path := filepath.Join(t.TempDir(), "presentation.json")
	if os.WriteFile(path, raw, 0600) != nil {
		t.Fatal("write fixture")
	}
	_, metadata, source, err := LoadPresentation(context.Background(), path, v.Facts)
	if err != nil {
		t.Fatal(err)
	}
	m := metadata[v.Facts.Items[0].CardKey]
	if m.Display.Name != "Exact release" || len(m.Display.ResourceTags) != 1 || m.Entry != nil || source.Quality != "known" {
		t.Fatal("exact metadata or trust isolation lost")
	}
	if m.Ordering == nil || m.Ordering.PriorityClass != "normal" {
		t.Fatal("exact display overrode app-level ordering")
	}
	if m.Display.Models == nil || len(*m.Display.Models) != 0 || m.Display.Capabilities == nil || len(*m.Display.Capabilities) != 0 {
		t.Fatal("exact empty declarations must not inherit app-level terms")
	}
	v.Facts.Items[0].Installation.AppVersion = "2.0.0"
	_, metadata, _, err = LoadPresentation(context.Background(), path, v.Facts)
	if err != nil || len(metadata[v.Facts.Items[0].CardKey].Display.ResourceTags) != 0 {
		t.Fatal("old version resource tags leaked into another version")
	}
	if m := metadata[v.Facts.Items[0].CardKey]; m.Display.Models == nil || (*m.Display.Models)[0] != "App model" || m.Display.Capabilities == nil || (*m.Display.Capabilities)[0] != "app.read" {
		t.Fatal("app-level descriptive terms missing")
	}
	if metadata[v.Facts.Items[0].CardKey].Ordering == nil {
		t.Fatal("version change discarded app-level ordering")
	}
	_, metadata, source, err = LoadPresentation(context.Background(), "", v.Facts)
	if err != nil || len(metadata) != 0 || source.Quality != "unknown" {
		t.Fatal("unregistered metadata fabricated")
	}
}

func TestAppCenterPresentationRejectsAmbiguousOrExecutableData(t *testing.T) {
	display := Display{Name: "Sample", GroupID: "other", Quality: "known", ResourceTags: []ResourceTag{}}
	base := PresentationRecord{AppID: "sample", Display: display, SourceRef: "docs/design/sample.md", SourceDigest: Digest("source")}
	for _, kind := range []string{"duplicate", "wrong-group", "app-level-resources", "traversal", "url", "unknown-field"} {
		t.Run(kind, func(t *testing.T) {
			r := base
			reg := PresentationRegistry{SchemaVersion: 1, Revision: Digest("registry"), Groups: []Group{{ID: "other", Label: "Other", Order: 1}}, Records: []PresentationRecord{r}}
			switch kind {
			case "duplicate":
				reg.Records = append(reg.Records, r)
			case "wrong-group":
				reg.Records[0].Display.GroupID = "missing"
			case "app-level-resources":
				reg.Records[0].Display.ResourceTags = []ResourceTag{{Kind: "cpu", Value: 4, Unit: "cores", SourceRef: r.SourceRef}}
			case "traversal":
				reg.Records[0].SourceRef = "../outside.md"
			case "url":
				reg.Records[0].SourceRef = "https://untrusted.invalid/file"
			}
			raw, _ := json.Marshal(reg)
			if kind == "unknown-field" {
				raw = bytes.Replace(raw, []byte(`"appId":"sample"`), []byte(`"appId":"sample","allowOpen":true`), 1)
			}
			if _, err := ReadPresentation(bytes.NewReader(raw)); err == nil {
				t.Fatal("unsafe metadata accepted")
			}
		})
	}
}
