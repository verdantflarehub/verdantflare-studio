package project

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
)

func dramaID(n int) string { return fmt.Sprintf("01900000-0000-7000-8000-%012d", n) }
func dramaFixture(t *testing.T) (contentObject, Manifest) {
	t.Helper()
	m := Manifest{ProjectID: dramaID(1), Category: "ai_short_drama"}
	for i := 10; i <= 30; i++ {
		m.Files = append(m.Files, File{ID: dramaID(i), Path: fmt.Sprintf("files/%d.json", i), Content: ContentRef{StoreID: dramaID(100), ArtifactID: dramaID(i + 100), VersionID: dramaID(i + 200)}})
	}
	raw := `{"schema_version":1,"kind":"project-content","category":"ai_short_drama","ai_short_drama":{"globals":[{"key":"hall","kind":"location","name":"Hall","states":[{"key":"night","name":"Night, lamp off","definition_refs":[],"marks":[]}],"versions":[],"selected_versions":[]}],"episodes":[{"key":"ep01","title":"Episode","scenes":[{"key":"sc01","title":"Hall","shots":[{"key":"sh01","title":"Ring","segments":[{"key":"flashback","order":1,"label":"Rainy night"}]}]}]}],"usages":[{"key":"u01","global_key":"hall","state_key":"night","target":{"episode_key":"ep01","scene_key":"sc01","shot_key":"sh01"},"segment":{"key":"flashback","order":1,"label":"Rainy night"}}],"deliverables":[]}}`
	var root contentObject
	if e := json.Unmarshal([]byte(raw), &root); e != nil {
		t.Fatal(e)
	}
	g := objectOf(listOf(objectOf(root["ai_short_drama"])["globals"])[0])
	version := contentObject{"key": "v1", "label": "V1", "state_key": "night", "basis_revision_id": dramaID(2), "change_note": "Initial lighting proposal", "generation_record_file_ids": []any{dramaID(11)}, "lighting_record_file_id": dramaID(12), "review": contentObject{"state": "unreviewed"}}
	f := m.Files[0]
	version["file_refs"] = []any{contentObject{"file_id": f.ID, "content_ref": contentObject{"store_id": f.Content.StoreID, "artifact_id": f.Content.ArtifactID, "version_id": f.Content.VersionID}, "purpose": "lighting"}}
	g["versions"] = []any{version}
	g["selected_versions"] = []any{contentObject{"state_key": "night", "purpose": "lighting", "version_key": "v1"}}
	m.Selections = []Selection{{Purpose: "drama/globals/hall/night/lighting", FileIDs: []string{dramaID(10)}}}
	return root, m
}
func dramaBytes(t *testing.T, v any) []byte {
	t.Helper()
	b, e := json.Marshal(v)
	if e != nil {
		t.Fatal(e)
	}
	return b
}
func TestDramaContentSchemaAndSemanticValidation(t *testing.T) {
	root, m := dramaFixture(t)
	if err := ValidateDramaContent(dramaBytes(t, root), m); err != nil {
		t.Fatal(err)
	}
	cases := map[string]func(contentObject){
		"duplicate result under new key": func(d contentObject) {
			g := objectOf(listOf(d["globals"])[0])
			var clone contentObject
			json.Unmarshal(dramaBytes(t, listOf(g["versions"])[0]), &clone)
			clone["key"] = "duplicate"
			clone["label"] = "another name"
			g["versions"] = append(listOf(g["versions"]), clone)
		},
		"missing state": func(d contentObject) { objectOf(listOf(d["globals"])[0])["states"] = []any{} },
		"cross owner selection": func(d contentObject) {
			objectOf(listOf(objectOf(listOf(d["globals"])[0])["selected_versions"])[0])["version_key"] = "foreign"
		},
		"wrong selection state": func(d contentObject) {
			objectOf(listOf(objectOf(listOf(d["globals"])[0])["selected_versions"])[0])["state_key"] = "day"
		},
		"unknown field": func(d contentObject) { objectOf(listOf(d["globals"])[0])["latest_version"] = "v1" },
		"dangling lighting": func(d contentObject) {
			objectOf(listOf(objectOf(listOf(d["globals"])[0])["versions"])[0])["lighting_record_file_id"] = dramaID(99)
		},
		"missing lighting": func(d contentObject) {
			delete(objectOf(listOf(objectOf(listOf(d["globals"])[0])["versions"])[0]), "lighting_record_file_id")
		},
		"cyclic version": func(d contentObject) {
			objectOf(listOf(objectOf(listOf(d["globals"])[0])["versions"])[0])["parent_version_key"] = "v1"
		},
		"content substitution": func(d contentObject) {
			objectOf(objectOf(listOf(objectOf(listOf(objectOf(listOf(d["globals"])[0])["versions"])[0])["file_refs"])[0])["content_ref"])["version_id"] = dramaID(99)
		},
		"missing segment":     func(d contentObject) { objectOf(objectOf(listOf(d["usages"])[0])["segment"])["key"] = "missing" },
		"wrong segment order": func(d contentObject) { objectOf(objectOf(listOf(d["usages"])[0])["segment"])["order"] = float64(2) },
		"version without state": func(d contentObject) {
			u := objectOf(listOf(d["usages"])[0])
			delete(u, "state_key")
			u["version_key"] = "v1"
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			r, m := dramaFixture(t)
			mutate(objectOf(r["ai_short_drama"]))
			if e := ValidateDramaContent(dramaBytes(t, r), m); e == nil {
				t.Fatal("accepted invalid content")
			}
		})
	}
}
func TestDramaVersionHistoryPinsOutputsAndRecords(t *testing.T) {
	root, m := dramaFixture(t)
	prior := objectOf(root["ai_short_drama"])
	nextRoot, _ := dramaFixture(t)
	next := objectOf(nextRoot["ai_short_drama"])
	if e := checkVersionHistory(prior, next, m, m); e != nil {
		t.Fatal(e)
	}
	g := objectOf(listOf(next["globals"])[0])
	v := objectOf(listOf(g["versions"])[0])
	v["label"] = "Readable title"
	if e := checkVersionHistory(prior, next, m, m); e != nil {
		t.Fatal("renaming changed identity", e)
	}
	v["change_note"] = "Replaced facts"
	if e := checkVersionHistory(prior, next, m, m); e == nil {
		t.Fatal("history rewritten")
	}
	nextRoot, _ = dramaFixture(t)
	next = objectOf(nextRoot["ai_short_drama"])
	changed := m
	changed.Files = append([]File{}, m.Files...)
	changed.Files[2].Content.VersionID = dramaID(999)
	if e := checkVersionHistory(prior, next, m, changed); e == nil {
		t.Fatal("lighting record bytes changed")
	}
	objectOf(listOf(next["globals"])[0])["versions"] = []any{}
	if e := checkVersionHistory(prior, next, m, m); e == nil {
		t.Fatal("history removed")
	}
}
func TestDramaSchemaRejectsForeignCategoryAndReadsLegacy(t *testing.T) {
	r, m := dramaFixture(t)
	r["category"] = "video"
	if e := ValidateDramaContent(dramaBytes(t, r), m); e == nil {
		t.Fatal("category mismatch")
	}
	r, m = dramaFixture(t)
	g := objectOf(listOf(objectOf(r["ai_short_drama"])["globals"])[0])
	delete(g, "versions")
	delete(g, "selected_versions")
	m.Selections = []Selection{{Purpose: "drama/legacy", FileIDs: []string{dramaID(10)}}}
	if e := ValidateDramaContent(dramaBytes(t, r), m); e != nil {
		t.Fatal(e)
	}
	b := strings.Replace(string(dramaBytes(t, r)), `"marks":[]`, `"marks":null`, 1)
	if e := ValidateDramaContent([]byte(b), m); e == nil {
		t.Fatal("null marks silently accepted")
	}
}

func TestDramaPublicationChoosesOlderVersionOnly(t *testing.T) {
	root, m := dramaFixture(t)
	g := objectOf(listOf(objectOf(root["ai_short_drama"])["globals"])[0])
	v1 := objectOf(listOf(g["versions"])[0])
	var v2 contentObject
	json.Unmarshal(dramaBytes(t, v1), &v2)
	v2["key"] = "v2"
	f := m.Files[3]
	v2["file_refs"] = []any{contentObject{"file_id": f.ID, "content_ref": contentObject{"store_id": f.Content.StoreID, "artifact_id": f.Content.ArtifactID, "version_id": f.Content.VersionID}, "purpose": "lighting"}}
	g["versions"] = append(listOf(g["versions"]), v2)
	objectOf(listOf(g["selected_versions"])[0])["version_key"] = "v2"
	candidate := contentObject{"source": contentObject{"kind": "global", "key": "hall"}}
	record := contentObject{"schema_version": 1, "kind": "short-drama-publication", "source": candidate["source"], "versions": []any{contentObject{"version_key": "v1", "file_refs": v1["file_refs"]}}, "supporting_file_refs": []any{contentObject{"file_id": dramaID(12), "purpose": "lighting_record", "content_ref": m.Files[2].Content}}}
	request := WorldRegisterRequest{FileIDs: []string{dramaID(10), dramaID(30), dramaID(12)}}
	if e := validateDramaPublication(dramaBytes(t, record), candidate, g, m, dramaID(30), request); e != nil {
		t.Fatal("older version cannot be published", e)
	}
	request.FileIDs = append(request.FileIDs, dramaID(13))
	if e := validateDramaPublication(dramaBytes(t, record), candidate, g, m, dramaID(30), request); e == nil {
		t.Fatal("unselected version leaked")
	}
	request.FileIDs = request.FileIDs[:3]
	objectOf(listOf(record["versions"])[0])["version_key"] = "v2"
	if e := validateDramaPublication(dramaBytes(t, record), candidate, g, m, dramaID(30), request); e == nil {
		t.Fatal("mislabeled version accepted")
	}
}

func TestDramaSelectionProjectionAndSpatialRecords(t *testing.T) {
	root, m := dramaFixture(t)
	d := objectOf(root["ai_short_drama"])
	shot := dramaOwners(d)["episodes/ep01/scenes/sc01/shots/sh01"]
	version := contentObject{"key": "video1", "label": "Video 1", "purpose": "shot_video", "basis_revision_id": dramaID(2), "change_note": "Initial whole shot", "change_record_file_id": dramaID(11), "generation_record_file_ids": []any{}, "review": contentObject{"state": "unreviewed"}}
	f := m.Files[4]
	version["file_refs"] = []any{contentObject{"file_id": f.ID, "content_ref": contentObject{"store_id": f.Content.StoreID, "artifact_id": f.Content.ArtifactID, "version_id": f.Content.VersionID}, "purpose": "video"}}
	shot["versions"] = []any{version}
	shot["selected_versions"] = []any{contentObject{"purpose": "shot_video", "version_key": "video1"}}
	shot["result_file_ids"] = []any{f.ID}
	shot["selected_result_file_id"] = f.ID
	shot["review"] = version["review"]
	m.Selections = append(m.Selections, Selection{Purpose: "drama/episodes/ep01/scenes/sc01/shots/sh01/shot_video/whole", FileIDs: []string{f.ID}})
	if e := ValidateDramaContent(dramaBytes(t, root), m); e != nil {
		t.Fatal(e)
	}
	t.Run("manifest disagreement", func(t *testing.T) {
		bad := m
		bad.Selections = append([]Selection{}, m.Selections...)
		bad.Selections[1].FileIDs = []string{dramaID(12)}
		if e := ValidateDramaContent(dramaBytes(t, root), bad); e == nil {
			t.Fatal("different manifest selection accepted")
		}
	})
	t.Run("image masquerading as video", func(t *testing.T) {
		objectOf(listOf(version["file_refs"])[0])["purpose"] = "keyframe"
		if e := ValidateDramaContent(dramaBytes(t, root), m); e == nil {
			t.Fatal("no primary video accepted")
		}
		objectOf(listOf(version["file_refs"])[0])["purpose"] = "video"
	})
	t.Run("duplicate selection", func(t *testing.T) {
		shot["selected_versions"] = append(listOf(shot["selected_versions"]), listOf(shot["selected_versions"])[0])
		if e := ValidateDramaContent(dramaBytes(t, root), m); e == nil {
			t.Fatal("duplicate selection accepted")
		}
		shot["selected_versions"] = listOf(shot["selected_versions"])[:1]
	})
	t.Run("whitebox without record", func(t *testing.T) {
		v := contentObject{"key": "wb", "label": "WB", "purpose": "whitebox", "basis_revision_id": dramaID(2), "change_note": "Initial space", "change_record_file_id": dramaID(11), "generation_record_file_ids": []any{}, "review": contentObject{"state": "unreviewed"}, "file_refs": version["file_refs"]}
		shot["versions"] = append(listOf(shot["versions"]), v)
		if e := ValidateDramaContent(dramaBytes(t, root), m); e == nil {
			t.Fatal("whitebox has no spatial record")
		}
		shot["versions"] = listOf(shot["versions"])[:1]
	})
	t.Run("unknown delivery selection", func(t *testing.T) {
		d["deliverables"] = []any{contentObject{"key": "cut", "title": "Cut", "selected_version_key": "missing"}}
		if e := ValidateDramaContent(dramaBytes(t, root), m); e == nil {
			t.Fatal("missing delivery version accepted")
		}
	})
}

func TestDramaValidationHTTPDetails(t *testing.T) {
	response := httptest.NewRecorder()
	httpError(response, Principal{RequestID: dramaID(1)}, contentError("globals/hall/versions/v1", "missing state"))
	var result map[string]any
	if json.Unmarshal(response.Body.Bytes(), &result) != nil || response.Code != 400 || result["code"] != "INVALID_ARGUMENT" {
		t.Fatal("invalid response", response.Body.String())
	}
	details := objectOf(result["details"])
	if details["path"] != "globals/hall/versions/v1" || details["document_type"] != "project-content" {
		t.Fatal("missing domain location", details)
	}
}
