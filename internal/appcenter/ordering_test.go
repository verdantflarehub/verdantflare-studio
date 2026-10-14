package appcenter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
)

func TestPresentationOrderingValidation(t *testing.T) {
	record := PresentationRecord{AppID: "sample", Display: Display{Name: "Sample", GroupID: "other", Quality: "known", ResourceTags: []ResourceTag{}}, SourceRef: "docs/design/sample.md", SourceDigest: Digest("source")}
	reg := PresentationRegistry{SchemaVersion: 1, Revision: Digest("registry"), Groups: []Group{{ID: "other", Label: "Other", Order: 0}}, Records: []PresentationRecord{record}}
	base, err := json.Marshal(reg)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, ordering   string
		versioned, valid bool
	}{
		{name: "legacy", valid: true},
		{name: "numeric-order", ordering: `{"priorityClass":"normal","order":0}`, valid: true},
		{name: "missing-order", ordering: `{"priorityClass":"normal"}`, valid: true},
		{name: "maximum-order", ordering: `{"priorityClass":"normal","order":1000000}`, valid: true},
		{name: "null", ordering: `null`},
		{name: "missing-class", ordering: `{"order":0}`},
		{name: "unknown-class", ordering: `{"priorityClass":"official"}`},
		{name: "old-special-class", ordering: `{"priorityClass":"first_party_mcp","order":0}`},
		{name: "negative", ordering: `{"priorityClass":"normal","order":-1}`},
		{name: "fraction", ordering: `{"priorityClass":"normal","order":0.5}`},
		{name: "overflow", ordering: `{"priorityClass":"normal","order":1000001}`},
		{name: "null-order", ordering: `{"priorityClass":"normal","order":null}`},
		{name: "unknown-field", ordering: `{"priorityClass":"normal","allowOpen":true}`},
		{name: "duplicate-key", ordering: `{"priorityClass":"normal","priorityClass":"normal"}`},
		{name: "versioned", ordering: `{"priorityClass":"normal"}`, versioned: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := slices.Clone(base)
			if tc.ordering != "" {
				raw = bytes.Replace(raw, []byte(`"appId":"sample"`), []byte(`"ordering":`+tc.ordering+`,"appId":"sample"`), 1)
			}
			if tc.versioned {
				raw = bytes.Replace(raw, []byte(`"appVersion":null`), []byte(`"appVersion":"1.0.0"`), 1)
			}
			_, err := ReadPresentation(bytes.NewReader(raw))
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v err=%v", tc.valid, err)
			}
		})
	}
}

func TestAppCenterMCPOrderingAndStateIndependence(t *testing.T) {
	in := input(vectors(t)[0])
	baseFact := in.Facts.Items[0]
	baseMetadata := in.Metadata[baseFact.CardKey]
	in.Facts.Items = nil
	in.Metadata = map[string]Metadata{}
	in.Groups = []Group{{ID: "image", Label: "Image", Order: 0}, {ID: "video", Label: "Video", Order: 1}, {ID: "music", Label: "Music", Order: 2}}
	want := []string{}
	zero := 0
	for _, group := range in.Groups {
		for _, firstParty := range []bool{false, true} {
			fact := baseFact
			installation := *baseFact.Installation
			installation.InstallationID = fmt.Sprintf("00000000-0000-7000-8000-%012d", len(in.Facts.Items)+20)
			fact.Installation = &installation
			fact.CardKey = "installation:" + installation.InstallationID
			fact.AppID = group.ID + "-ordinary"
			meta := baseMetadata
			display := *baseMetadata.Display
			display.GroupID, display.Name = group.ID, "A third party MCP"
			meta.Display = &display
			if firstParty {
				fact.AppID = group.ID + "-mcp-server"
				display.Name = "Z first party MCP"
				meta.Ordering = &AppOrdering{PriorityClass: "normal", Order: &zero}
				want = append(want, fact.AppID, group.ID+"-ordinary")
			}
			in.Facts.Items = append(in.Facts.Items, fact)
			in.Metadata[fact.CardKey] = meta
		}
	}
	for _, state := range []string{"ready", "stopped", "degraded", "unknown", "starting"} {
		t.Run(state, func(t *testing.T) {
			for i := range in.Facts.Items {
				in.Facts.Items[i].Service.State = state
			}
			slices.Reverse(in.Facts.Items)
			view, err := Resolve(in)
			if err != nil {
				t.Fatal(err)
			}
			got := []string{}
			for _, card := range view.Items {
				got = append(got, card.AppID)
			}
			if !slices.Equal(got, want) {
				t.Fatalf("got %v want %v", got, want)
			}
			raw, _ := json.Marshal(view)
			if bytes.Contains(raw, []byte(`"ordering"`)) || bytes.Contains(raw, []byte(`"priorityClass"`)) {
				t.Fatal("server ordering leaked into public view")
			}
		})
	}
}

func TestAppCenterOrderingTiesAndDefaults(t *testing.T) {
	in := input(vectors(t)[0])
	baseFact, baseMetadata := in.Facts.Items[0], in.Metadata[in.Facts.Items[0].CardKey]
	in.Facts.Items, in.Metadata = nil, map[string]Metadata{}
	zero, five := 0, 5
	for i, tc := range []struct {
		name     string
		ordering *AppOrdering
	}{
		{"A", nil},
		{"B", &AppOrdering{PriorityClass: "normal"}},
		{"Z", &AppOrdering{PriorityClass: "normal", Order: &zero}},
		{"A", &AppOrdering{PriorityClass: "normal"}},
		{"Z", &AppOrdering{PriorityClass: "normal", Order: &five}},
		{"B", &AppOrdering{PriorityClass: "normal", Order: &zero}},
		{"A", &AppOrdering{PriorityClass: "normal", Order: &zero}},
		{"A", &AppOrdering{PriorityClass: "normal", Order: &zero}},
	} {
		fact, meta := baseFact, baseMetadata
		installation, display := *baseFact.Installation, *baseMetadata.Display
		installation.InstallationID = fmt.Sprintf("00000000-0000-7000-8000-%012d", i+20)
		fact.Installation, fact.AppID = &installation, fmt.Sprintf("sample-%d", i)
		fact.CardKey = "installation:" + installation.InstallationID
		display.Name = tc.name
		meta.Display, meta.Ordering = &display, tc.ordering
		in.Facts.Items = append(in.Facts.Items, fact)
		in.Metadata[fact.CardKey] = meta
	}
	want := []string{"sample-6", "sample-7", "sample-5", "sample-2", "sample-4", "sample-0", "sample-3", "sample-1"}
	slices.Reverse(in.Facts.Items)
	view, err := Resolve(in)
	if err != nil {
		t.Fatal(err)
	}
	got := []string{}
	for _, card := range view.Items {
		got = append(got, card.AppID)
	}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestOrderingConfigRevisionPreservesBindingsAndActions(t *testing.T) {
	in := input(vectors(t)[0])
	reg := PresentationRegistry{SchemaVersion: 1, Revision: Digest("registry"), Groups: in.Groups, Records: []PresentationRecord{{AppID: "sample", Display: *in.Metadata[in.Facts.Items[0].CardKey].Display, SourceRef: "docs/design/sample.md", SourceDigest: Digest("source")}}}
	path := filepath.Join(t.TempDir(), "presentation.json")
	read := func() View {
		t.Helper()
		raw, err := json.Marshal(reg)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, raw, 0600); err != nil {
			t.Fatal(err)
		}
		groups, metadata, source, err := LoadPresentation(context.Background(), path, in.Facts)
		if err != nil {
			t.Fatal(err)
		}
		in.Groups, in.Metadata, in.Sources = groups, metadata, []Source{source}
		view, err := Resolve(in)
		if err != nil {
			t.Fatal(err)
		}
		return view
	}
	before := read()
	reg.Records[0].Ordering = &AppOrdering{PriorityClass: "normal"}
	after := read()
	if before.Revision == after.Revision {
		t.Fatal("config change did not revise view even when resulting order was unchanged")
	}
	if !reflect.DeepEqual(before.Items, after.Items) {
		t.Fatal("ordering changed card facts, permissions, bindings or actions")
	}
	reg.Records[0].Ordering = nil
	reverted := read()
	if reverted.Revision != before.Revision {
		t.Fatal("poll times or prior ordering leaked into revision")
	}
}
