package appcenter

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"
)

//go:embed testdata/rules.generated.json
var ruleVectors []byte

type vector struct {
	Name     string              `json:"name"`
	Facts    Facts               `json:"facts"`
	Metadata map[string]Metadata `json:"metadata"`
	Expected struct {
		Status, Primary string
		Enabled         []string
	} `json:"expected"`
}

func vectors(t *testing.T) []vector {
	t.Helper()
	var list []vector
	if err := json.Unmarshal(ruleVectors, &list); err != nil {
		t.Fatal(err)
	}
	return list
}
func input(v vector) Input {
	return Input{Facts: v.Facts, Scope: v.Facts.Scope, Now: v.Facts.GeneratedAt, Metadata: v.Metadata, Groups: []Group{{ID: "other", Label: "Other", Order: 1}}}
}
func TestAppCenterSharedRules(t *testing.T) {
	for _, v := range vectors(t) {
		t.Run(v.Name, func(t *testing.T) {
			for _, app := range []string{"comfyui", "blender", "new-application"} {
				in := input(v)
				in.Facts.Items = slices.Clone(in.Facts.Items)
				in.Facts.Items[0].AppID = app
				out, err := Resolve(in)
				if err != nil {
					t.Fatal(err)
				}
				c := out.Items[0]
				if c.StatusCode != v.Expected.Status || c.PrimaryActionID != v.Expected.Primary {
					t.Fatalf("%s: status=%s primary=%s", app, c.StatusCode, c.PrimaryActionID)
				}
				enabled := []string{}
				for _, a := range c.Actions {
					if a.Enabled {
						enabled = append(enabled, a.Kind)
					} else if a.ReasonCode == nil {
						t.Fatal("disabled action has no reason")
					}
				}
				if !slices.Equal(enabled, v.Expected.Enabled) {
					t.Fatalf("enabled=%v want=%v", enabled, v.Expected.Enabled)
				}
			}
		})
	}
}
func TestAppCenterScopeFreshnessAndRevision(t *testing.T) {
	v := vectors(t)[0]
	in := input(v)
	a, err := Resolve(in)
	if err != nil {
		t.Fatal(err)
	}
	in.Now = in.Now.Add(time.Second)
	in.Facts.RequestID = "next-request"
	b, err := Resolve(in)
	if err != nil || a.Revision != b.Revision || a.Items[0].BindingRevision != b.Items[0].BindingRevision {
		t.Fatal("poll invalidated unchanged facts", err)
	}
	in.Scope.UserID = "00000000-0000-7000-8000-000000000099"
	if _, err := Resolve(in); err == nil {
		t.Fatal("scope mismatch accepted")
	}
	in = input(v)
	in.Now = in.Now.Add(15 * time.Second)
	if _, err := Resolve(in); err == nil {
		t.Fatal("expired snapshot accepted")
	}
	in = input(v)
	stale := in.Now.Add(-16 * time.Second)
	in.Facts.Items[0].Service.ObservedAt = &stale
	b, err = Resolve(in)
	if err != nil || b.Items[0].PrimaryActionID != "refresh" {
		t.Fatal("stale service still enabled", err)
	}
}
func TestAppCenterDecoderRejectsAmbiguityAndLeaks(t *testing.T) {
	v := vectors(t)[0]
	raw, _ := json.Marshal(v.Facts)
	var out Facts
	if err := Decode(bytes.NewReader(raw), "facts", 4<<20, &out); err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []string{
		strings.Replace(string(raw), `"schemaVersion":1`, `"schemaVersion":1,"schemaVersion":1`, 1),
		strings.Replace(string(raw), `"catalog":null`, `"catalog":null,"token":"unexpected"`, 1),
		strings.Replace(string(raw), `"canRead":false`, `"canRead":true`, 1),
	} {
		if Decode(strings.NewReader(invalid), "facts", 4<<20, &out) == nil {
			t.Fatal("ambiguous or leaking protocol accepted")
		}
	}
	in := input(v)
	view, err := Resolve(in)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ = json.Marshal(view)
	if bytes.Contains(raw, []byte(`"workloadUID":`)) || bytes.Contains(raw, []byte(`"catalog":`)) {
		t.Fatal("internal facts leaked into view")
	}
}
