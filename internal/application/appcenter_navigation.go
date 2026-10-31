package application

import (
	"slices"
	"strconv"
	"strings"

	"github.com/verdantflarehub/verdantflare-studio/internal/appcenter"
	"github.com/verdantflarehub/verdantflare-studio/internal/navigation"
)

func compareAppVersion(a, b string) int {
	pa := strings.Split(a, ".")
	pb := strings.Split(b, ".")
	for i := 0; i < max(len(pa), len(pb)); i++ {
		var va, vb int
		if i < len(pa) {
			va, _ = strconv.Atoi(pa[i])
		}
		if i < len(pb) {
			vb, _ = strconv.Atoi(pb[i])
		}
		if va != vb {
			if va > vb {
				return 1
			}
			return -1
		}
	}
	return strings.Compare(a, b)
}

// Navigation metadata contributes presentation and preferences, never another
// installation or service-state inference. Missing/disabled entries stay visible
// when their original trusted navigation declaration and installation survive.
func appCenterNavigationBindings(view appcenter.View, metadata map[string]appcenter.Metadata, catalog navigation.Catalog) ([]navigation.Binding, error) {
	bindings := []navigation.Binding{}
	for _, card := range view.Items {
		installed := card.Installation
		if installed == nil || installed.State != "installed" {
			continue
		}
		binding, exists := catalog.Releases[card.AppID+"@"+installed.AppVersion]
		if !exists && card.Helm != nil {
			artifact := metadata[card.CardKey].ArtifactDigest
			bestVer := ""
			for key, candidate := range catalog.Releases {
				if ver, found := strings.CutPrefix(key, card.AppID+"@"); found {
					if artifact != "" && candidate.ArtifactDigest != artifact {
						continue
					}
					if !exists || compareAppVersion(ver, bestVer) > 0 {
						binding = candidate
						bestVer = ver
						exists = true
					}
				}
			}
		}
		if !exists {
			continue
		}
		artifact := metadata[card.CardKey].ArtifactDigest
		if artifact == "" || artifact != binding.ArtifactDigest {
			return nil, navigation.ErrInvalid
		}
		binding.InstallationID, binding.State = installed.InstallationID, installed.State
		binding.OrganizationID, binding.StationID = view.Scope.OrganizationID, view.Scope.StationID
		// Availability is display only: it follows the observed service, while
		// opening relies on the static entry binding alone.
		binding.Availability = "unknown"
		if card.Entry != nil && card.Entry.Transport == "dashboard" && card.Service.Quality == "known" && card.Service.State == "ready" && slices.ContainsFunc(card.Actions, func(a appcenter.Action) bool {
			return a.Kind == "open" && a.Enabled && a.EntryID != nil && *a.EntryID == card.Entry.EntryID
		}) {
			binding.Availability = "ready"
		} else if card.Service.Quality == "known" && slices.Contains([]string{"absent", "stopped", "starting", "stopping", "degraded"}, card.Service.State) {
			binding.Availability = "offline"
		}
		bindings = append(bindings, binding)
	}
	return bindings, nil
}
