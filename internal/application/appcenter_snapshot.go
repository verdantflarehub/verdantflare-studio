package application

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/verdantflarehub/verdantflare-studio/internal/appcenter"
)

// snapshotMaxAge bounds how long a cached scope may go without any Core
// notification before one background-free re-read on the next request.
const snapshotMaxAge = 60 * time.Second

// snapshotInput is everything a view needs except the evaluation time. It is
// read from Core and the host registries once, then shared read-only.
type snapshotInput struct {
	fetched      time.Time
	facts        appcenter.Facts
	groups       []appcenter.Group
	metadata     map[string]appcenter.Metadata
	presentation appcenter.Source
	entries      appcenter.Source
}

// at replays the cached facts as of their Core read. Observation freshness
// stays as Core evaluated it; later expiry or state changes arrive as Core
// notifications, which invalidate the cache instead of a Studio timer.
func (in snapshotInput) at(now time.Time, requestID string) appcenter.Facts {
	facts := in.facts
	facts.RequestID = requestID
	delta := now.Sub(in.fetched)
	if delta < 0 {
		delta = 0
	}
	facts.GeneratedAt = in.facts.GeneratedAt.Add(delta)
	facts.Items = make([]appcenter.Fact, len(in.facts.Items))
	for i, item := range in.facts.Items {
		if at := item.Service.ObservedAt; at != nil {
			shifted := at.Add(delta)
			item.Service.ObservedAt = &shifted
		}
		facts.Items[i] = item
	}
	return facts
}

type snapshotEntry struct {
	input      snapshotInput
	generation uint64
	files      string
}

type snapshotFlight struct {
	done  chan struct{}
	input snapshotInput
	out   Result
}

// snapshotCache holds one input per authorized scope. A generation per
// Station/Organization is advanced by Core notifications and local commands.
type snapshotCache struct {
	mu          sync.Mutex
	entries     map[appcenter.Scope]snapshotEntry
	flights     map[appcenter.Scope]*snapshotFlight
	generations map[string]uint64
}

func organizationKey(station, organization string) string { return station + "/" + organization }

// InvalidateOrganization drops cached snapshots after a Core notification or
// a lifecycle command for the Station/Organization.
func (a *AppCenter) InvalidateOrganization(station, organization string) {
	if a == nil {
		return
	}
	c := &a.snapshots
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.generations == nil {
		c.generations = map[string]uint64{}
	}
	c.generations[organizationKey(station, organization)]++
}

// InvalidateAll drops every cached snapshot, used when notifications may have
// been missed (for example while the etcd watch reconnects).
func (a *AppCenter) InvalidateAll() {
	if a == nil {
		return
	}
	c := &a.snapshots
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = nil
}

func (a *AppCenter) registrySignature() string {
	dashboardRegistry := ""
	if a.dashboards != nil && a.dashboards.source != nil {
		dashboardRegistry = a.dashboards.source.Path()
	}
	sig := fileSignature(a.presentationPath, a.preinstalledPath, a.legacyPath, a.declarationsDir, dashboardRegistry)
	if a.legacyBackends != nil {
		sig += appcenter.Digest(a.legacyBackends())
	}
	return sig
}

// WithSnapshotCache serves views, navigation and Open from the per-scope
// snapshot. Configure only during startup, before serving requests.
func (a *AppCenter) WithSnapshotCache() *AppCenter {
	a.snapshotCache = true
	return a
}

func (a *AppCenter) snapshot(ctx context.Context, token, id string, scope appcenter.Scope) (snapshotInput, Result) {
	if !a.snapshotCache {
		return a.loadSnapshot(ctx, token, id, scope)
	}
	c := &a.snapshots
	files, now := a.registrySignature(), a.now().UTC()
	org := organizationKey(scope.StationID, scope.OrganizationID)
	c.mu.Lock()
	generation := c.generations[org]
	if entry, ok := c.entries[scope]; ok && entry.generation == generation && entry.files == files && now.Sub(entry.input.fetched) < snapshotMaxAge {
		c.mu.Unlock()
		return entry.input, Result{Status: 200, RequestID: id}
	}
	flight, waiting := c.flights[scope]
	if !waiting {
		if c.flights == nil {
			c.flights = map[appcenter.Scope]*snapshotFlight{}
		}
		flight = &snapshotFlight{done: make(chan struct{})}
		c.flights[scope] = flight
	}
	c.mu.Unlock()
	if waiting {
		select {
		case <-flight.done:
			if flight.out.Status != 200 {
				return snapshotInput{}, appCenterError(flight.out.Status, resultCode(flight.out), id)
			}
			return flight.input, Result{Status: 200, RequestID: id}
		case <-ctx.Done():
			return snapshotInput{}, appCenterError(503, "APPCENTER_DEPENDENCY_UNAVAILABLE", id)
		}
	}
	input, out := a.loadSnapshot(ctx, token, id, scope)
	c.mu.Lock()
	delete(c.flights, scope)
	if out.Status == 200 {
		if c.entries == nil {
			c.entries = map[appcenter.Scope]snapshotEntry{}
		}
		if len(c.entries) >= 1024 {
			for key, entry := range c.entries {
				if now.Sub(entry.input.fetched) >= snapshotMaxAge {
					delete(c.entries, key)
				}
			}
		}
		if len(c.entries) < 1024 {
			// Keep the generation observed before the read: a notification that
			// raced with it leaves this entry stale for the next request.
			c.entries[scope] = snapshotEntry{input: input, generation: generation, files: files}
		}
	}
	flight.input, flight.out = input, out
	close(flight.done)
	c.mu.Unlock()
	return input, out
}

// resultCode recovers the stable error code from a failed shared read so that
// waiters report it under their own request ID.
func resultCode(out Result) string {
	var payload struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(out.Data, &payload); err != nil || payload.Code == "" {
		return "APPCENTER_DEPENDENCY_UNAVAILABLE"
	}
	return payload.Code
}
