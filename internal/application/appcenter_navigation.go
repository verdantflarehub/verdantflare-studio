package application

import (
	"slices"

	"github.com/verdantflarehub/verdantflare-studio/internal/appcenter"
	"github.com/verdantflarehub/verdantflare-studio/internal/navigation"
)

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
		if !exists {
			continue
		}
		artifact := metadata[card.CardKey].ArtifactDigest
		if artifact == "" || artifact != binding.ArtifactDigest {
			return nil, navigation.ErrInvalid
		}
		binding.InstallationID, binding.State = installed.InstallationID, installed.State
		binding.OrganizationID, binding.StationID = view.Scope.OrganizationID, view.Scope.StationID
		binding.Availability = "unknown"
		if card.Entry != nil && card.Entry.Transport == "dashboard" && slices.ContainsFunc(card.Actions, func(a appcenter.Action) bool {
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
