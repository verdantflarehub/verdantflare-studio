package appcenter

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLegacyRoutesAreFiniteAndCanonical(t *testing.T) {
	for _, route := range []string{"/market?image=%2Fdashboard#", "/market?image=%2Fdash\tboard", "/market?blender=blender123&operation=00000000-0000-7000-0000-000000000001&panel=overview"} {
		if _, _, err := LegacyRoute(route); err == nil {
			t.Fatal("ambiguous route accepted")
		}
	}
	for _, route := range []string{"/market?image=%2Fdashboard%23tasks", "/market?video=%2Fdashboard%2Ftasks%2Fsample", "/market?blender=instances", "/market?blender=blender123&panel=overview"} {
		adapter, canonical, err := LegacyRoute(route)
		if err != nil || adapter == "" || canonical != route {
			t.Fatal("valid route rejected", route)
		}
	}
	for _, route := range []string{"https://other.invalid/market?blender=instances", "//other.invalid/market?blender=instances", "/market?image=%2Fdashboard&video=%2Fdashboard", "/market?image=%2Fdashboard&image=%2Fdashboard", "/market?image=%2Fdashboard&admin=true", "/market?image=%2Fdashboard%2F..%2Fapi", "/market?blender=instances&panel=overview", "/market?blender=blender123&panel=invalid", "/market?blender=blender123&operation=invalid", "/market?blender=instances#script", "/market?image=%2Fdashboard%23evil", "/market?image=%ZZ", "/market?image=%2Fdashboard;other=true"} {
		if _, _, err := LegacyRoute(route); err == nil {
			t.Fatal("unsafe route accepted", route)
		}
	}
}

func TestLegacyRegistryNeverCreatesInstallationsOrCrossScopeEntries(t *testing.T) {
	for _, scenario := range []string{"valid", "empty", "installed", "other-scope", "other-app", "duplicate-binding", "duplicate-object", "route-adapter-mismatch", "unknown-field", "nil-uuid", "canceled", "missing", "oversized"} {
		t.Run(scenario, func(t *testing.T) {
			scope := Scope{OrganizationID: "00000000-0000-7000-8000-000000000002", StationID: "00000000-0000-7000-8000-000000000003"}
			key := "obs:" + strings.Repeat("b", 64)
			binding := LegacyBinding{BindingID: "sample-old", AppID: "sample", OrganizationID: scope.OrganizationID, StationID: scope.StationID, ObservationKey: key, WorkloadUID: "00000000-0000-4000-8000-000000000005", AdapterID: "image-iframe-v1", BackendRevision: LegacyBackendRevision("image-iframe-v1", "http://sample.invalid:8000"), Route: "/market?image=%2Fdashboard%23tasks"}
			facts := Facts{Scope: scope, Items: []Fact{{AppID: "sample", Origin: "observation", CardKey: key}}}
			registry := LegacyRegistry{SchemaVersion: 1, Revision: "sha256:" + strings.Repeat("a", 64), Records: []LegacyBinding{binding}}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			switch scenario {
			case "empty":
				facts.Items = nil
			case "installed":
				facts.Items[0].Origin = "installation"
				facts.Items[0].Installation = &Installation{State: "installed"}
			case "other-scope":
				registry.Records[0].OrganizationID = "00000000-0000-7000-8000-000000000099"
			case "other-app":
				registry.Records[0].AppID = "another"
			case "duplicate-binding":
				other := binding
				other.ObservationKey = "obs:" + strings.Repeat("c", 64)
				registry.Records = append(registry.Records, other)
			case "duplicate-object":
				other := binding
				other.BindingID = "other"
				registry.Records = append(registry.Records, other)
			case "route-adapter-mismatch":
				registry.Records[0].AdapterID = "video-iframe-v1"
			case "nil-uuid":
				registry.Records[0].WorkloadUID = "00000000-0000-0000-0000-000000000000"
			case "canceled":
				cancel()
			}
			raw, _ := json.Marshal(registry)
			if scenario == "unknown-field" {
				raw = []byte(strings.Replace(string(raw), `"schemaVersion":1`, `"schemaVersion":1,"trusted":true`, 1))
			}
			if scenario == "oversized" {
				raw = make([]byte, (1<<20)+1)
			}
			path := filepath.Join(t.TempDir(), "legacy.json")
			if scenario != "missing" {
				if err := os.WriteFile(path, raw, 0600); err != nil {
					t.Fatal(err)
				}
			}
			bindings, revision, err := LoadLegacy(ctx, path, facts, map[string]string{binding.AdapterID: binding.BackendRevision})
			switch scenario {
			case "valid":
				if err != nil || len(bindings) != 1 || revision == "" {
					t.Fatal("missing legacy binding", err)
				}
			case "empty", "installed", "other-scope", "other-app":
				if err != nil || len(bindings) != 0 {
					t.Fatal("fabricated legacy object", err)
				}
			default:
				if err == nil {
					t.Fatal("invalid config accepted")
				}
			}
		})
	}
}

func TestLegacyRegistryRequiresExactInstalledResource(t *testing.T) {
	for _, scenario := range []string{"same-resource", "missing-uid", "other-uid", "missing-key", "other-key", "unknown", "conflict", "pending", "failed", "removed", "other-app", "other-scope"} {
		t.Run(scenario, func(t *testing.T) {
			scope := Scope{OrganizationID: "00000000-0000-7000-8000-000000000002", StationID: "00000000-0000-7000-8000-000000000003"}
			binding := LegacyBinding{BindingID: "original", AppID: "sample", OrganizationID: scope.OrganizationID, StationID: scope.StationID, ObservationKey: "obs:" + strings.Repeat("b", 64), WorkloadUID: "00000000-0000-4000-8000-000000000005", AdapterID: "image-iframe-v1", BackendRevision: LegacyBackendRevision("image-iframe-v1", "http://sample.invalid:8000"), Route: "/market?image=%2Fdashboard%23tasks"}
			fact := Fact{AppID: "sample", Origin: "installation", CardKey: "installation:original", InstallationQuality: "known", Installation: &Installation{State: "installed"}, Service: CoreService{Service: Service{ObservationKey: Text(binding.ObservationKey)}, WorkloadUID: Text(binding.WorkloadUID)}}
			switch scenario {
			case "missing-uid":
				fact.Service.WorkloadUID = nil
			case "other-uid":
				fact.Service.WorkloadUID = Text("00000000-0000-4000-8000-000000000099")
			case "missing-key":
				fact.Service.ObservationKey = nil
			case "other-key":
				fact.Service.ObservationKey = Text("obs:" + strings.Repeat("c", 64))
			case "unknown", "conflict":
				fact.InstallationQuality = scenario
			case "pending", "failed", "removed":
				fact.Installation.State = scenario
			case "other-app":
				fact.AppID = "other"
			case "other-scope":
				scope.OrganizationID = "00000000-0000-7000-8000-000000000099"
			}
			registry := LegacyRegistry{SchemaVersion: 1, Revision: "sha256:" + strings.Repeat("a", 64), Records: []LegacyBinding{binding}}
			raw, _ := json.Marshal(registry)
			path := filepath.Join(t.TempDir(), "legacy.json")
			if err := os.WriteFile(path, raw, 0600); err != nil {
				t.Fatal(err)
			}
			bindings, _, err := LoadLegacy(context.Background(), path, Facts{Scope: scope, Items: []Fact{fact}}, nil)
			if err != nil {
				t.Fatal(err)
			}
			_, matched := bindings[fact.CardKey]
			if matched != (scenario == "same-resource") {
				t.Fatal("installed resource binding mismatch")
			}
		})
	}
}
