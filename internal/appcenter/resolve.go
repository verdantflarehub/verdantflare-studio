package appcenter

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"slices"
	"sort"
	"time"
)

// Metadata is supplied by the platform's verified registry reader, never by
// the browser or a running App. EntryRevision binds the exact trusted material.
type Metadata struct {
	Display        *Display
	Ordering       *AppOrdering
	Entry          *Entry
	EntryRevision  string
	ArtifactDigest string
	Legacy         *LegacyBinding
	Preinstalled   bool
}
type Input struct {
	Facts    Facts
	Scope    Scope
	Now      time.Time
	Groups   []Group
	Metadata map[string]Metadata
	Sources  []Source
}

func Text(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
func Digest(value any) string {
	raw, _ := json.Marshal(value)
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// Sorting is platform presentation policy. It never participates in entry
// binding or action permissions, and is not exposed in the public View.
func orderingValue(ordering *AppOrdering) int {
	if ordering != nil && ordering.Order != nil {
		return *ordering.Order
	}
	return 1000
}
func catalogDisplay(f Fact) (Display, *string) {
	d := Display{Name: f.AppID, GroupID: "other", ResourceTags: []ResourceTag{}, Quality: "unknown"}
	var data struct {
		Name    string `json:"display_name"`
		Group   string `json:"group_id"`
		Brand   string `json:"brand"`
		Version string `json:"version"`
	}
	if json.Unmarshal(f.Catalog, &data) != nil || data.Name == "" {
		return d, nil
	}
	d.Name, d.GroupID, d.PublisherID = data.Name, data.Group, Text(data.Brand)
	// An observation's labels are never an install candidate. The Core reader
	// emits registered catalog metadata only; it may still accompany an orphan.
	if f.Origin == "observation" {
		return d, nil
	}
	return d, Text(data.Version)
}
func action(kind string, enabled bool, reason string) Action {
	a := Action{ID: kind, Kind: kind, Enabled: enabled, Confirmation: "none"}
	if !enabled {
		a.ReasonCode = Text(reason)
	}
	switch kind {
	case "stop":
		a.Confirmation = "service-stop"
	case "restart":
		a.Confirmation = "service-restart"
	case "delete":
		a.Confirmation = "installation-delete"
	}
	return a
}
func known(value string) bool { return value == "known" }
func state(f Fact) string {
	if f.InstallationQuality == "conflict" || f.Service.Quality == "conflict" || f.Operation.Quality == "conflict" {
		return "BINDING_CONFLICT"
	}
	if !known(f.InstallationQuality) || !known(f.Operation.Quality) {
		return "STATE_UNKNOWN"
	}
	if f.Operation.Blocking {
		if f.Operation.Status != nil && *f.Operation.Status == "recovery_required" {
			return "RECOVERY_REQUIRED"
		}
		return "OPERATION_IN_PROGRESS"
	}
	if f.Installation == nil {
		if !known(f.Service.Quality) {
			return "STATE_UNKNOWN"
		}
		if f.Origin == "observation" {
			return "OBSERVED_DEPLOYMENT"
		}
		return "NOT_INSTALLED"
	}
	switch f.Installation.State {
	case "pending":
		return "INSTALLATION_PENDING"
	case "failed":
		return "INSTALLATION_FAILED"
	case "removed":
		return "INSTALLATION_REMOVED"
	}
	if !known(f.Service.Quality) {
		return "STATE_UNKNOWN"
	}
	switch f.Service.State {
	case "ready":
		return "READY"
	case "stopped":
		return "STOPPED"
	case "starting":
		return "STARTING"
	case "stopping":
		return "STOPPING"
	case "degraded":
		return "DEGRADED"
	case "absent":
		return "SERVICE_ABSENT"
	default:
		return "STATE_UNKNOWN"
	}
}
func unsafeReason(f Fact) string {
	if f.InstallationQuality == "conflict" || f.Service.Quality == "conflict" || f.Operation.Quality == "conflict" {
		return "BINDING_CONFLICT"
	}
	if !known(f.InstallationQuality) || !known(f.Service.Quality) || !known(f.Operation.Quality) {
		return "FACTS_UNAVAILABLE"
	}
	if f.Operation.Blocking {
		return "OPERATION_IN_PROGRESS"
	}
	return ""
}

func project(f Fact, meta Metadata, scope Scope) Card {
	display, candidateVersion := catalogDisplay(f)
	if meta.Display != nil {
		display = *meta.Display
	}
	c := Card{CardKey: f.CardKey, AppID: f.AppID, Origin: f.Origin, Display: display, CandidateVersion: candidateVersion, Installation: f.Installation, InstallationQuality: f.InstallationQuality, Service: f.Service.Service, Operation: f.Operation, Entry: meta.Entry, StatusCode: state(f), PrimaryActionID: "detail", Preinstalled: meta.Preinstalled, Actions: []Action{action("detail", true, ""), action("refresh", true, "")}}
	if f.Installation != nil {
		c.InstalledVersion = Text(f.Installation.AppVersion)
	}
	c.BindingRevision = Digest(struct {
		Scope                 Scope
		Key, App, Origin      string
		Installation          *Installation
		WorkloadUID           *string
		Policy, EntryRevision string
		Entry                 *Entry
	}{scope, f.CardKey, f.AppID, f.Origin, f.Installation, f.Service.WorkloadUID, f.Lifecycle.PolicyRevision, meta.EntryRevision, meta.Entry})
	denied := unsafeReason(f)
	if c.StatusCode == "BINDING_CONFLICT" || c.StatusCode == "STATE_UNKNOWN" {
		c.PrimaryActionID = "refresh"
	}
	if f.Operation.CanRead && f.Operation.OperationID != nil {
		c.Actions = append(c.Actions, action("progress", true, ""))
		if denied != "BINDING_CONFLICT" && known(f.InstallationQuality) && known(f.Operation.Quality) && (f.Operation.Blocking || (f.Installation != nil && f.Installation.State == "failed")) {
			c.PrimaryActionID = "progress"
		}
	}
	for _, candidate := range f.Lifecycle.Candidates {
		if !slices.Contains([]string{"install", "start", "stop", "restart", "delete"}, candidate.Kind) {
			continue
		}
		reason := denied
		if reason == "" && !candidate.Enabled {
			reason = "ACTION_UNSUPPORTED"
			if candidate.ReasonCode != nil {
				reason = *candidate.ReasonCode
			}
		}
		if reason == "" {
			if candidate.Kind == "install" {
				if f.Installation != nil || f.Origin != "catalog" || f.Service.State != "absent" || candidateVersion == nil || *candidateVersion != candidate.AppVersion {
					reason = "INSTALLATION_STATE_CONFLICT"
				}
			} else if f.Installation == nil || f.Installation.State != "installed" || f.Installation.AppVersion != candidate.AppVersion {
				reason = "INSTALLATION_STATE_CONFLICT"
			}
		}
		a := action(candidate.Kind, reason == "", reason)
		c.Actions = append(c.Actions, a)
		if a.Enabled && ((a.Kind == "install" && c.StatusCode == "NOT_INSTALLED") || (a.Kind == "start" && c.StatusCode == "STOPPED")) {
			c.PrimaryActionID = a.ID
		}
	}
	if meta.Entry != nil {
		e := meta.Entry
		reason := denied
		if reason == "" && (e.Quality != "known" || e.Availability != "available" || meta.EntryRevision == "") {
			reason = "ENTRY_UNAVAILABLE"
			if e.ReasonCode != nil {
				reason = *e.ReasonCode
			}
		}
		if reason == "" && e.RequiresServiceReady && f.Service.State != "ready" {
			reason = "SERVICE_NOT_READY"
		}
		if reason == "" && (f.Installation == nil || f.Installation.State != "installed") && !(f.Origin == "observation" && e.Transport == "legacy") {
			reason = "INSTALLATION_NOT_CONFIRMED"
		}
		a := action("open", reason == "", reason)
		a.EntryID = Text(e.EntryID)
		c.Actions = append(c.Actions, a)
		if a.Enabled {
			c.PrimaryActionID = a.ID
		}
	}
	return c
}

// Resolve is the only card/action rule projection. It accepts one validated
// scope and preserves partial observations instead of inventing ready states.
func Resolve(in Input) (View, error) {
	var empty View
	f := in.Facts
	if validate("facts", f) != nil || f.Scope != in.Scope || in.Now.IsZero() || f.GeneratedAt.After(in.Now.Add(2*time.Second)) {
		return empty, ErrInvalid
	}
	validUntil := f.GeneratedAt.Add(15 * time.Second)
	if !in.Now.Before(validUntil) {
		return empty, ErrUnavailable
	}
	sources := map[string]bool{}
	for _, s := range f.Sources {
		if sources[s.Source] {
			return empty, ErrInvalid
		}
		sources[s.Source] = true
		if s.Source != "observations" && s.Quality != "known" {
			return empty, ErrUnavailable
		}
	}
	for _, name := range []string{"catalog", "installations", "observations", "operations"} {
		if !sources[name] {
			return empty, ErrInvalid
		}
	}
	out := View{SchemaVersion: 1, RequestID: f.RequestID, Scope: f.Scope, GeneratedAt: in.Now.UTC(), ValidUntil: validUntil, Sources: append(slices.Clone(f.Sources), in.Sources...), Groups: slices.Clone(in.Groups), Items: []Card{}}
	if out.Groups == nil {
		out.Groups = []Group{}
	}
	seen := map[string]bool{}
	for _, item := range f.Items {
		if seen[item.CardKey] || (item.Origin == "installation") != (item.Installation != nil) {
			return empty, ErrInvalid
		}
		seen[item.CardKey] = true
		if item.Installation != nil && item.CardKey != "installation:"+item.Installation.InstallationID {
			return empty, ErrInvalid
		}
		if item.Origin == "catalog" && item.CardKey != "catalog:"+item.AppID {
			return empty, ErrInvalid
		}
		if item.Origin == "observation" && (item.Service.ObservationKey == nil || item.CardKey != *item.Service.ObservationKey || item.Service.WorkloadUID == nil) {
			return empty, ErrInvalid
		}
		if string(item.Catalog) != "null" {
			var catalog struct {
				AppID string `json:"app_id"`
			}
			if json.Unmarshal(item.Catalog, &catalog) != nil || catalog.AppID != item.AppID {
				return empty, ErrInvalid
			}
		}
		kinds := map[string]bool{}
		for _, c := range item.Lifecycle.Candidates {
			if kinds[c.Kind] {
				return empty, ErrInvalid
			}
			kinds[c.Kind] = true
		}
		if item.Service.Quality == "known" {
			at := item.Service.ObservedAt
			if at == nil || at.After(in.Now.Add(2*time.Second)) || !in.Now.Before(at.Add(15*time.Second)) {
				item.Service.Quality, item.Service.State, item.Service.ReasonCode = "unknown", "unknown", Text("OBSERVATION_EXPIRED")
			} else if at.Add(15 * time.Second).Before(out.ValidUntil) {
				out.ValidUntil = at.Add(15 * time.Second)
			}
		}
		card := project(item, in.Metadata[item.CardKey], f.Scope)
		if card.InstallationQuality != "known" || card.Service.Quality != "known" || card.Operation.Quality != "known" || card.Display.Quality != "known" || (card.Entry != nil && card.Entry.Quality != "known") {
			out.Partial = true
		}
		out.Items = append(out.Items, card)
	}
	for _, source := range out.Sources {
		if source.Quality != "known" {
			out.Partial = true
		}
	}
	groupOrder := map[string]int{}
	for _, group := range out.Groups {
		if _, ok := groupOrder[group.ID]; ok {
			return empty, ErrInvalid
		}
		groupOrder[group.ID] = group.Order
	}
	sort.Slice(out.Groups, func(i, j int) bool {
		a, b := out.Groups[i], out.Groups[j]
		if a.Order != b.Order {
			return a.Order < b.Order
		}
		return a.ID < b.ID
	})
	sort.Slice(out.Items, func(i, j int) bool {
		a, b := out.Items[i], out.Items[j]
		order := func(id string) int {
			if n, ok := groupOrder[id]; ok {
				return n
			}
			return 1000001
		}
		if order(a.Display.GroupID) != order(b.Display.GroupID) {
			return order(a.Display.GroupID) < order(b.Display.GroupID)
		}
		if a.Display.GroupID != b.Display.GroupID {
			return a.Display.GroupID < b.Display.GroupID
		}
		ao := orderingValue(in.Metadata[a.CardKey].Ordering)
		bo := orderingValue(in.Metadata[b.CardKey].Ordering)
		if ao != bo {
			return ao < bo
		}
		if a.Display.Name != b.Display.Name {
			return a.Display.Name < b.Display.Name
		}
		return a.CardKey < b.CardKey
	})
	// Poll time is not a semantic revision. A status, binding or permission
	// change remains in the digest and immediately invalidates stale actions.
	copy := out
	copy.GeneratedAt = time.Time{}
	copy.ValidUntil = time.Time{}
	copy.RequestID = ""
	copy.Sources = slices.Clone(out.Sources)
	copy.Items = slices.Clone(out.Items)
	for i := range copy.Sources {
		copy.Sources[i].ObservedAt = nil
	}
	for i := range copy.Items {
		copy.Items[i].Service.ObservedAt = nil
		copy.Items[i].Operation.UpdatedAt = nil
	}
	out.Revision = Digest(copy)
	if validate("view", out) != nil {
		return empty, ErrInvalid
	}
	return out, nil
}
