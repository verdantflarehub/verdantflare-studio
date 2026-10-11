package preinstalled

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"time"
)

// Actor must come from a freshly authenticated Core identity, never JSON input.
type Actor struct {
	UserID, OrganizationID, StationID string
	Roles, Scopes                     []string
}
type Intent struct {
	ID                   string            `json:"intentId"`
	Execute              bool              `json:"-"`
	Retry                []string          `json:"retry"`
	Reinstall            []string          `json:"reinstall"`
	Adopt                map[string]string `json:"adopt"`
	AdoptTemplateDigests map[string]string `json:"adoptTemplateDigests,omitempty"`
}
type Fact struct {
	InstallationID, State, AppVersion, PackageDigest, ChartDigest, LastOperationID string
	TemplateDigest                                                                 string
	Confirmed                                                                      bool
}
type Inventory struct {
	// Revision includes current installation and removed history. Submit must
	// atomically enforce this precondition under the Core Station/App lock.
	Revision              string
	Complete              bool
	Current               *Fact
	RemovedInstallationID string
	WorkloadUID           string
	TemplateDigest        string
	ManagementStatus      string
	ObservedAt            string
}
type Command struct {
	Actor                                                                                       Actor
	Release                                                                                     Release
	RequestID, IdempotencyKey, InventoryRevision, Mode, ExpectedWorkloadUID, ReinstallRemovedID string
	TemplateDigest                                                                              string
}
type Operation struct{ ID, AppID, AppVersion, Status, Code string }
type Lifecycle interface {
	// Verify checks the actual signed package, Chart and executable mapping.
	Verify(context.Context, Actor, Release) error
	Observe(context.Context, Actor, string) (Inventory, error)
	Submit(context.Context, Command) (Operation, error)
	Operation(context.Context, Actor, string) (Operation, error)
}
type Record struct {
	AppID        string  `json:"appId"`
	UserID       string  `json:"userId"`
	Release      Release `json:"release"`
	Attempt      int     `json:"attempt"`
	IntentID     string  `json:"intentId"`
	IntentDigest string  `json:"intentDigest"`
	Command      Command `json:"command"`
	OperationID  string  `json:"operationId"`
	Status       string  `json:"status"`
	Code         string  `json:"code"`
}
type Journal struct {
	SchemaVersion  int               `json:"schemaVersion"`
	OrganizationID string            `json:"organizationId"`
	StationID      string            `json:"stationId"`
	ConfigID       string            `json:"configId"`
	ConfigDigest   string            `json:"configDigest"`
	Records        map[string]Record `json:"records"`
}
type Store interface {
	Load(context.Context, string) (Journal, int64, error)
	Save(context.Context, string, int64, Journal) (int64, error)
}
type Item struct {
	AppID            string            `json:"appId"`
	State            string            `json:"state"`
	Code             string            `json:"code"`
	OperationID      string            `json:"operationId"`
	InstallationID   string            `json:"installationId"`
	AdoptionEvidence *AdoptionEvidence `json:"adoptionEvidence,omitempty"`
}

// Evidence describes observed ownership and the signed target. Runtime must
// still compare the full live spec and Service before any metadata write.
type AdoptionEvidence struct {
	ExpectedWorkloadUID string `json:"expectedWorkloadUid"`
	TemplateDigest      string `json:"templateDigest"`
	ManagementStatus    string `json:"managementStatus"`
	ObservedAt          string `json:"observedAt"`
}

func adoptionEvidence(f Inventory) *AdoptionEvidence {
	if !validID(f.WorkloadUID) || !digestPattern.MatchString(f.TemplateDigest) || !slices.Contains([]string{"unmanaged", "managed"}, f.ManagementStatus) {
		return nil
	}
	observed, err := time.Parse(time.RFC3339Nano, f.ObservedAt)
	if err != nil || observed.IsZero() {
		return nil
	}
	return &AdoptionEvidence{f.WorkloadUID, f.TemplateDigest, f.ManagementStatus, f.ObservedAt}
}

type Result struct {
	ConfigDigest string `json:"configDigest"`
	Items        []Item `json:"items"`
}
type Reconciler struct {
	Lifecycle Lifecycle
	Store     Store
}

func key(c Config) string { return c.OrganizationID + "/" + c.StationID + "/" + c.ConfigID }
func attemptKey(c Config, app Release, user string, attempt int) string {
	sum := sha256.Sum256([]byte(c.Digest() + "/" + app.AppID + "/" + user + fmt.Sprintf("/%d", attempt)))
	return "preinstalled-" + hex.EncodeToString(sum[:])
}
func intentDigest(in Intent) string {
	raw, _ := json.Marshal(in)
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:])
}
func operationValid(op Operation, app Release) bool {
	return validID(op.ID) && op.AppID == app.AppID && op.AppVersion == app.AppVersion && slices.Contains([]string{"accepted", "running", "succeeded", "failed"}, op.Status)
}

// Reconcile advances only explicitly requested work. Durable intents are written
// before Submit; uncertain calls replay the exact command and original key.
func (r *Reconciler) Reconcile(ctx context.Context, actor Actor, c Config, intent Intent) (Result, error) {
	out := Result{ConfigDigest: c.Digest(), Items: []Item{}}
	if c.Validate() != nil || !validID(intent.ID) {
		return out, ErrInvalid
	}
	if !validID(actor.UserID) || actor.OrganizationID != c.OrganizationID || actor.StationID != c.StationID || !slices.Contains(actor.Roles, "admin") || !slices.Contains(actor.Scopes, "app:read") || intent.Execute && !slices.Contains(actor.Scopes, "app:manage") {
		return out, ErrDenied
	}
	if r.Lifecycle == nil || r.Store == nil {
		return out, ErrUnavailable
	}
	known := map[string]bool{}
	for _, app := range c.Apps {
		known[app.AppID] = true
	}
	for _, list := range [][]string{intent.Retry, intent.Reinstall} {
		seen := map[string]bool{}
		for _, id := range list {
			if !known[id] || seen[id] {
				return out, ErrInvalid
			}
			seen[id] = true
		}
	}
	for id, uid := range intent.Adopt {
		if !known[id] || !validID(uid) || !digestPattern.MatchString(intent.AdoptTemplateDigests[id]) {
			return out, ErrInvalid
		}
	}
	for id := range intent.AdoptTemplateDigests {
		if intent.Adopt[id] == "" {
			return out, ErrInvalid
		}
	}
	journal, revision, err := r.Store.Load(ctx, key(c))
	if err != nil {
		return out, err
	}
	if revision == 0 {
		journal = Journal{SchemaVersion: 1, OrganizationID: c.OrganizationID, StationID: c.StationID, ConfigID: c.ConfigID, ConfigDigest: c.Digest(), Records: map[string]Record{}}
	}
	if journal.SchemaVersion != 1 || journal.OrganizationID != c.OrganizationID || journal.StationID != c.StationID || journal.ConfigID != c.ConfigID || journal.Records == nil {
		return out, ErrConflict
	}
	if journal.ConfigDigest != c.Digest() {
		return out, ErrConflict
	}
	for appID, record := range journal.Records {
		var release *Release
		for i := range c.Apps {
			if c.Apps[i].AppID == appID {
				release = &c.Apps[i]
				break
			}
		}
		cmd := record.Command
		if !digestPattern.MatchString(record.IntentDigest) || intent.Execute && record.IntentID == intent.ID && record.IntentDigest != intentDigest(intent) {
			return out, ErrConflict
		}
		if release == nil || record.AppID != appID || !validID(record.UserID) || record.Attempt < 1 || !validID(record.IntentID) || !reflect.DeepEqual(record.Release, *release) || !reflect.DeepEqual(cmd.Release, *release) || cmd.Actor.UserID != record.UserID || cmd.Actor.OrganizationID != c.OrganizationID || cmd.Actor.StationID != c.StationID || cmd.RequestID != record.IntentID || cmd.IdempotencyKey != attemptKey(c, *release, record.UserID, record.Attempt) || !digestPattern.MatchString(cmd.TemplateDigest) || !slices.Contains([]string{"install", "adopt"}, cmd.Mode) || cmd.InventoryRevision == "" || cmd.Mode == "adopt" && !validID(cmd.ExpectedWorkloadUID) || cmd.Mode == "install" && cmd.ExpectedWorkloadUID != "" || cmd.ReinstallRemovedID != "" && !validID(cmd.ReinstallRemovedID) || record.OperationID != "" && !validID(record.OperationID) || !slices.Contains([]string{"submitting", "accepted", "running", "succeeded", "failed"}, record.Status) || record.OperationID == "" && record.Status != "submitting" {
			return out, ErrConflict
		}
	}
	save := func(record Record) error {
		journal.Records[record.AppID] = record
		next, e := r.Store.Save(ctx, key(c), revision, journal)
		if e == nil {
			revision = next
		}
		return e
	}
	states := map[string]string{}
	for _, app := range c.Ordered() {
		item := Item{AppID: app.AppID, State: "pending"}
		complete := func() { out.Items = append(out.Items, item); states[app.AppID] = item.State }
		blocked := false
		for _, dep := range app.Dependencies {
			if states[dep] != "ready" {
				blocked = true
			}
		}
		if blocked {
			item.State, item.Code = "waiting", "DEPENDENCY_NOT_INSTALLED"
			complete()
			continue
		}
		if err := r.Lifecycle.Verify(ctx, actor, app); err != nil {
			item.State, item.Code = "blocked", "TRUSTED_RELEASE_UNAVAILABLE"
			complete()
			continue
		}
		facts, err := r.Lifecycle.Observe(ctx, actor, app.AppID)
		if err != nil || !facts.Complete || facts.Revision == "" || !digestPattern.MatchString(facts.TemplateDigest) {
			item.State, item.Code = "blocked", "INSTALLATION_INVENTORY_UNAVAILABLE"
			complete()
			continue
		}
		if f := facts.Current; f != nil {
			if !validID(f.InstallationID) {
				item.State, item.Code = "blocked", "INVALID_INSTALLATION_FACT"
				complete()
				continue
			}
			item.InstallationID = f.InstallationID
			if f.AppVersion != app.AppVersion {
				item.State, item.Code = "upgrade_required", "VERSION_CHANGE_REQUIRES_UPGRADE"
				complete()
				continue
			}
			if f.PackageDigest != "" && (f.PackageDigest != app.PackageDigest || f.ChartDigest != app.ChartDigest || f.TemplateDigest != facts.TemplateDigest) {
				item.State, item.Code = "blocked", "INSTALLATION_BINDING_MISMATCH"
				complete()
				continue
			}
			if f.State == "installed" && f.Confirmed && f.PackageDigest == app.PackageDigest && f.ChartDigest == app.ChartDigest {
				item.State = "ready"
				complete()
				continue
			}
		}
		record, exists := journal.Records[app.AppID]
		if exists && record.UserID != actor.UserID {
			item.State, item.Code = "blocked", "RESUME_REQUIRES_ORIGINAL_ACTOR"
			complete()
			continue
		}
		if exists && record.Command.TemplateDigest != facts.TemplateDigest {
			item.State, item.Code = "blocked", "PERSISTED_RELEASE_BINDING_CHANGED"
			complete()
			continue
		}
		reinstall := facts.Current == nil && facts.RemovedInstallationID != "" && (!exists || record.Command.ReinstallRemovedID != facts.RemovedInstallationID)
		if reinstall && (!intent.Execute || !slices.Contains(intent.Reinstall, app.AppID) || exists && record.IntentID == intent.ID) {
			item.State, item.Code = "suppressed", "USER_REMOVED_APPLICATION"
			complete()
			continue
		}
		if exists && record.OperationID != "" {
			op, e := r.Lifecycle.Operation(ctx, actor, record.OperationID)
			if e != nil || !operationValid(op, app) {
				item.State, item.Code, item.OperationID = "unknown", "OPERATION_RESULT_UNAVAILABLE", record.OperationID
				complete()
				continue
			}
			record.Status, record.Code = op.Status, op.Code
			item.OperationID = op.ID
			if op.Status != "failed" && !(reinstall && op.Status == "succeeded") {
				item.State = op.Status
				if op.Status == "succeeded" {
					item.State = "confirming"
				}
				complete()
				continue
			}
			if !intent.Execute || (!reinstall && !slices.Contains(intent.Retry, app.AppID) && intent.Adopt[app.AppID] == "") || record.IntentID == intent.ID {
				item.State, item.Code = "failed", op.Code
				complete()
				continue
			}
			record.OperationID = ""
			record.Status = "failed"
		}
		if !exists && facts.Current != nil && facts.Current.LastOperationID != "" {
			op, e := r.Lifecycle.Operation(ctx, actor, facts.Current.LastOperationID)
			if e != nil || !operationValid(op, app) {
				item.State, item.Code, item.OperationID = "unknown", "EXISTING_OPERATION_RESULT_UNAVAILABLE", facts.Current.LastOperationID
				complete()
				continue
			}
			if op.Status == "accepted" || op.Status == "running" || op.Status == "succeeded" && facts.Current.PackageDigest != "" {
				item.State, item.OperationID = op.Status, op.ID
				if op.Status == "succeeded" {
					item.State = "confirming"
				}
				complete()
				continue
			}
		}
		if (!exists || record.Status == "failed") && facts.WorkloadUID != "" {
			item.AdoptionEvidence = adoptionEvidence(facts)
			if item.AdoptionEvidence == nil {
				item.State, item.Code = "blocked", "WORKLOAD_OWNERSHIP_UNAVAILABLE"
				complete()
				continue
			}
			if !intent.Execute || intent.Adopt[app.AppID] != facts.WorkloadUID || intent.AdoptTemplateDigests[app.AppID] != facts.TemplateDigest {
				item.State, item.Code = "adoption_required", "EXPLICIT_ADOPTION_EVIDENCE_REQUIRED"
				complete()
				continue
			}
		}
		if !intent.Execute {
			item.State = "pending"
			if exists && record.OperationID == "" {
				item.State, item.Code = "unknown", "SUBMISSION_RESULT_UNAVAILABLE"
			}
			complete()
			continue
		}
		if !exists || record.Status == "failed" {
			mode := "install"
			expectedUID := ""
			if facts.WorkloadUID != "" {
				expectedUID = intent.Adopt[app.AppID]
				if expectedUID == "" || expectedUID != facts.WorkloadUID {
					item.State, item.Code = "adoption_required", "EXPLICIT_ADOPTION_EVIDENCE_REQUIRED"
					complete()
					continue
				}
				mode = "adopt"
			}
			attempt := record.Attempt + 1
			command := Command{Actor: actor, Release: app, RequestID: intent.ID, IdempotencyKey: attemptKey(c, app, actor.UserID, attempt), InventoryRevision: facts.Revision, Mode: mode, ExpectedWorkloadUID: expectedUID, TemplateDigest: facts.TemplateDigest}
			if slices.Contains(intent.Reinstall, app.AppID) {
				command.ReinstallRemovedID = facts.RemovedInstallationID
			}
			record = Record{AppID: app.AppID, UserID: actor.UserID, Release: app, Attempt: attempt, IntentID: intent.ID, IntentDigest: intentDigest(intent), Command: command, Status: "submitting"}
			if err := save(record); err != nil {
				return out, err
			}
		}
		op, e := r.Lifecycle.Submit(ctx, record.Command)
		if e != nil || !operationValid(op, app) {
			item.State, item.Code = "unknown", "SUBMISSION_RESULT_UNAVAILABLE"
			complete()
			continue
		}
		record.OperationID, record.Status, record.Code = op.ID, op.Status, op.Code
		if err := save(record); err != nil {
			return out, err
		}
		item.State, item.OperationID, item.Code = op.Status, op.ID, op.Code
		if op.Status == "succeeded" {
			item.State = "confirming"
		}
		complete()
	}
	return out, nil
}
