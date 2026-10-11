package preinstalled

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

const testUser = "019a00b0-0000-7000-8000-000000000001"
const testOrg = "019a00b0-0000-7000-8000-000000000002"
const testStation = "019a00b0-0000-7000-8000-000000000003"
const testOperation = "019a00b0-0000-7000-8000-000000000004"
const testInstallation = "019a00b0-0000-7000-8000-000000000005"

var testTemplate = "sha256:" + strings.Repeat("c", 64)

func fixture() (Actor, Config, Intent) {
	return Actor{testUser, testOrg, testStation, []string{"admin"}, []string{"app:read", "app:manage"}}, Config{1, "default", "1.0.0", testOrg, testStation, []Release{{"resources", "0.1.0", "resources-v1", "sha256:" + strings.Repeat("a", 64), "sha256:" + strings.Repeat("b", 64), []string{}}}}, Intent{ID: testUser, Execute: true}
}

type memoryStore struct {
	journal  Journal
	revision int64
}

func (m *memoryStore) Load(context.Context, string) (Journal, int64, error) {
	var out Journal
	raw, _ := json.Marshal(m.journal)
	_ = json.Unmarshal(raw, &out)
	return out, m.revision, nil
}
func (m *memoryStore) Save(_ context.Context, _ string, revision int64, journal Journal) (int64, error) {
	if revision != m.revision {
		return 0, ErrConflict
	}
	raw, _ := json.Marshal(journal)
	_ = json.Unmarshal(raw, &m.journal)
	m.revision++
	return m.revision, nil
}

type lifecycleFake struct {
	facts      map[string]Inventory
	commands   []Command
	operation  Operation
	trustError error
	unknown    bool
}

func (f *lifecycleFake) Verify(context.Context, Actor, Release) error { return f.trustError }
func (f *lifecycleFake) Observe(_ context.Context, _ Actor, id string) (Inventory, error) {
	return f.facts[id], nil
}
func (f *lifecycleFake) Submit(_ context.Context, c Command) (Operation, error) {
	f.commands = append(f.commands, c)
	if f.unknown {
		return Operation{}, ErrUnavailable
	}
	return f.operation, nil
}
func (f *lifecycleFake) Operation(context.Context, Actor, string) (Operation, error) {
	return f.operation, nil
}
func fake() *lifecycleFake {
	return &lifecycleFake{facts: map[string]Inventory{"resources": {Revision: "1", Complete: true, TemplateDigest: testTemplate}}, operation: Operation{testOperation, "resources", "0.1.0", "accepted", ""}}
}

func TestPreinstalledConfigurationClosedAndDependencyGraph(t *testing.T) {
	_, c, _ := fixture()
	raw, _ := json.Marshal(c)
	if _, e := DecodeConfig(raw); e != nil {
		t.Fatal(e)
	}
	if _, e := DecodeConfig(append(raw[:len(raw)-1], []byte(`,"trusted":true}`)...)); e == nil {
		t.Fatal("self-declared trust accepted")
	}
	c.Apps[0].Dependencies = []string{"resources"}
	if c.Validate() == nil {
		t.Fatal("cycle accepted")
	}
	c.Apps[0].Dependencies = []string{"missing"}
	if c.Validate() == nil {
		t.Fatal("missing dependency accepted")
	}
}
func TestPreinstalledLostSubmissionReplaysOriginalCommand(t *testing.T) {
	a, c, in := fixture()
	backend := fake()
	backend.unknown = true
	store := &memoryStore{}
	r := Reconciler{backend, store}
	first, e := r.Reconcile(context.Background(), a, c, in)
	if e != nil || first.Items[0].State != "unknown" || store.revision != 1 {
		t.Fatal(first, e)
	}
	backend.unknown = false
	in.ID = testStation
	second, e := r.Reconcile(context.Background(), a, c, in)
	if e != nil || second.Items[0].State != "accepted" || len(backend.commands) != 2 {
		t.Fatal(second, e)
	}
	left, _ := json.Marshal(backend.commands[0])
	right, _ := json.Marshal(backend.commands[1])
	if string(left) != string(right) {
		t.Fatal("uncertain operation changed its command")
	}
}
func TestPreinstalledRemovedAndExistingWorkloadNeedExplicitIntent(t *testing.T) {
	a, c, in := fixture()
	for _, kind := range []string{"removed", "existing", "untrusted", "incomplete"} {
		t.Run(kind, func(t *testing.T) {
			backend := fake()
			fact := backend.facts["resources"]
			want := "blocked"
			switch kind {
			case "removed":
				fact.RemovedInstallationID = testInstallation
				want = "suppressed"
			case "existing":
				fact.WorkloadUID = testInstallation
				fact.ManagementStatus, fact.ObservedAt = "unmanaged", "2026-10-11T00:00:00Z"
				want = "adoption_required"
			case "untrusted":
				backend.trustError = errors.New("invalid signature")
			case "incomplete":
				fact.Complete = false
			}
			backend.facts["resources"] = fact
			r := Reconciler{backend, &memoryStore{}}
			out, e := r.Reconcile(context.Background(), a, c, in)
			if e != nil || out.Items[0].State != want || len(backend.commands) != 0 {
				t.Fatal(out, e)
			}
		})
	}
	backend := fake()
	backend.facts["resources"] = Inventory{Revision: "1", Complete: true, WorkloadUID: testInstallation, TemplateDigest: testTemplate, ManagementStatus: "unmanaged", ObservedAt: "2026-10-11T00:00:00Z"}
	in.Adopt = map[string]string{"resources": testInstallation}
	in.AdoptTemplateDigests = map[string]string{"resources": testTemplate}
	r := Reconciler{backend, &memoryStore{}}
	_, e := r.Reconcile(context.Background(), a, c, in)
	if e != nil || len(backend.commands) != 1 || backend.commands[0].Mode != "adopt" || backend.commands[0].ExpectedWorkloadUID != testInstallation {
		t.Fatal("explicit evidence lost", e)
	}
}
func TestPreinstalledSuccessRequiresMatchingInstallationFact(t *testing.T) {
	a, c, in := fixture()
	backend := fake()
	backend.operation.Status = "succeeded"
	r := Reconciler{backend, &memoryStore{}}
	out, e := r.Reconcile(context.Background(), a, c, in)
	if e != nil || out.Items[0].State != "confirming" {
		t.Fatal(out, e)
	}
	backend.facts["resources"] = Inventory{Revision: "2", Complete: true, TemplateDigest: testTemplate, Current: &Fact{InstallationID: testInstallation, State: "installed", AppVersion: c.Apps[0].AppVersion, PackageDigest: c.Apps[0].PackageDigest, ChartDigest: c.Apps[0].ChartDigest, TemplateDigest: testTemplate, Confirmed: true}}
	out, e = r.Reconcile(context.Background(), a, c, in)
	if e != nil || out.Items[0].State != "ready" || len(backend.commands) != 1 {
		t.Fatal(out, e)
	}
}
func TestPreinstalledRetryChangesKeyOnlyAfterExplicitFailedIntent(t *testing.T) {
	a, c, in := fixture()
	backend := fake()
	backend.operation.Status = "failed"
	backend.operation.Code = "SERVICE_UNAVAILABLE"
	r := Reconciler{backend, &memoryStore{}}
	_, _ = r.Reconcile(context.Background(), a, c, in)
	in.Retry = []string{"resources"}
	_, _ = r.Reconcile(context.Background(), a, c, in)
	if len(backend.commands) != 1 {
		t.Fatal("same intent retried a failed command")
	}
	in.ID = testStation
	_, e := r.Reconcile(context.Background(), a, c, in)
	if e != nil || len(backend.commands) != 2 || backend.commands[0].IdempotencyKey == backend.commands[1].IdempotencyKey {
		t.Fatal("explicit retry did not create a new attempt", e)
	}
}
func TestPreinstalledDependenciesAndScopeFailClosed(t *testing.T) {
	a, c, in := fixture()
	dependency := c.Apps[0]
	dependency.AppID = "network"
	dependency.Dependencies = []string{"resources"}
	c.Apps = append(c.Apps, dependency)
	backend := fake()
	r := Reconciler{backend, &memoryStore{}}
	out, e := r.Reconcile(context.Background(), a, c, in)
	if e != nil || out.Items[1].State != "waiting" || len(backend.commands) != 1 {
		t.Fatal(out, e)
	}
	a.OrganizationID = testUser
	if _, e = r.Reconcile(context.Background(), a, c, in); !errors.Is(e, ErrDenied) {
		t.Fatal("cross organization allowed")
	}
}

func TestPreinstalledReadOnlyAdoptionAndChangedEvidence(t *testing.T) {
	for _, ownership := range []string{"unmanaged", "managed", "foreign", "unknown"} {
		t.Run(ownership, func(t *testing.T) {
			a, c, in := fixture()
			backend, store := fake(), &memoryStore{}
			fact := backend.facts["resources"]
			fact.WorkloadUID, fact.ManagementStatus, fact.ObservedAt = testInstallation, ownership, "2026-10-11T00:00:00Z"
			backend.facts["resources"] = fact
			r := Reconciler{backend, store}
			in.Execute = false
			out, err := r.Reconcile(context.Background(), a, c, in)
			eligible := ownership == "managed" || ownership == "unmanaged"
			if err != nil || (out.Items[0].AdoptionEvidence != nil) != eligible || len(backend.commands) != 0 || store.revision != 0 {
				t.Fatal(out, err)
			}
			if eligible && out.Items[0].State != "adoption_required" {
				t.Fatal("GET hid adoption requirement", out)
			}
			in.Execute = true
			in.Adopt = map[string]string{"resources": testInstallation}
			in.AdoptTemplateDigests = map[string]string{"resources": "sha256:" + strings.Repeat("d", 64)}
			out, err = r.Reconcile(context.Background(), a, c, in)
			if err != nil || len(backend.commands) != 0 || store.revision != 0 {
				t.Fatal("changed evidence submitted", out, err)
			}
		})
	}
}

func TestPreinstalledRecoveryNeverExecutesDuringGetAndRejectsChangedIntent(t *testing.T) {
	a, c, in := fixture()
	backend, store := fake(), &memoryStore{}
	backend.unknown = true
	r := Reconciler{backend, store}
	_, err := r.Reconcile(context.Background(), a, c, in)
	if err != nil {
		t.Fatal(err)
	}
	read := Intent{ID: testStation}
	out, err := r.Reconcile(context.Background(), a, c, read)
	if err != nil || out.Items[0].State != "unknown" || len(backend.commands) != 1 || store.revision != 1 {
		t.Fatal(out, err)
	}
	changed := in
	changed.Retry = []string{"resources"}
	if _, err = r.Reconcile(context.Background(), a, c, changed); !errors.Is(err, ErrConflict) {
		t.Fatal("changed intent accepted", err)
	}
	updated := c
	updated.Revision = "1.0.1"
	if _, err = r.Reconcile(context.Background(), a, updated, in); !errors.Is(err, ErrConflict) {
		t.Fatal("pending intent lost to new config", err)
	}
	store.journal.Records["resources"] = func() Record {
		v := store.journal.Records["resources"]
		v.Command.Actor.OrganizationID = testUser
		return v
	}()
	if _, err = r.Reconcile(context.Background(), a, c, in); !errors.Is(err, ErrConflict) {
		t.Fatal("malformed saved scope replayed", err)
	}
	if len(backend.commands) != 1 {
		t.Fatal("rejected request submitted")
	}
}

func TestPreinstalledExistingActiveOperationAndUnconfirmedBinding(t *testing.T) {
	a, c, in := fixture()
	backend := fake()
	fact := backend.facts["resources"]
	fact.Current = &Fact{InstallationID: testInstallation, State: "installed", AppVersion: "0.1.0", PackageDigest: c.Apps[0].PackageDigest, ChartDigest: c.Apps[0].ChartDigest, TemplateDigest: testTemplate, LastOperationID: testOperation}
	backend.facts["resources"] = fact
	r := Reconciler{backend, &memoryStore{}}
	out, err := r.Reconcile(context.Background(), a, c, in)
	if err != nil || out.Items[0].State != "accepted" || len(backend.commands) != 0 {
		t.Fatal("unconfirmed binding treated ready or duplicated", out, err)
	}
	backend.operation.Status = "succeeded"
	out, err = r.Reconcile(context.Background(), a, c, in)
	if err != nil || out.Items[0].State != "confirming" || len(backend.commands) != 0 {
		t.Fatal(out, err)
	}
}

func TestPreinstalledReinstallUsesNewIntentThenExactLostResponseReplay(t *testing.T) {
	a, c, in := fixture()
	backend, store := fake(), &memoryStore{}
	backend.operation.Status = "succeeded"
	r := Reconciler{backend, store}
	if _, err := r.Reconcile(context.Background(), a, c, in); err != nil {
		t.Fatal(err)
	}
	fact := backend.facts["resources"]
	fact.RemovedInstallationID = testInstallation
	backend.facts["resources"] = fact
	out, err := r.Reconcile(context.Background(), a, c, Intent{ID: testStation})
	if err != nil || out.Items[0].State != "suppressed" || len(backend.commands) != 1 {
		t.Fatal(out, err)
	}
	in.ID, in.Reinstall = testStation, []string{"resources"}
	backend.unknown = true
	out, err = r.Reconcile(context.Background(), a, c, in)
	if err != nil || out.Items[0].State != "unknown" || len(backend.commands) != 2 || backend.commands[1].ReinstallRemovedID != testInstallation || backend.commands[0].IdempotencyKey == backend.commands[1].IdempotencyKey {
		t.Fatal(out, err)
	}
	out, err = r.Reconcile(context.Background(), a, c, Intent{ID: testUser})
	if err != nil || out.Items[0].State != "unknown" || len(backend.commands) != 2 {
		t.Fatal("GET replayed reinstall", out, err)
	}
	backend.unknown, backend.operation.Status = false, "accepted"
	out, err = r.Reconcile(context.Background(), a, c, in)
	if err != nil || out.Items[0].State != "accepted" || len(backend.commands) != 3 || !reflect.DeepEqual(backend.commands[1], backend.commands[2]) {
		t.Fatal("reinstall replay changed command", out, err)
	}
}
