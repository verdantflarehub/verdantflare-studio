package navigation

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"go.etcd.io/etcd/api/v3/etcdserverpb"
	"go.etcd.io/etcd/api/v3/mvccpb"
	clientv3 "go.etcd.io/etcd/client/v3"
)

type preferenceKV struct {
	clientv3.KV
	data     map[string]*mvccpb.KeyValue
	revision int64
	fail     bool
}

func (m *preferenceKV) Get(_ context.Context, key string, _ ...clientv3.OpOption) (*clientv3.GetResponse, error) {
	if m.fail {
		return nil, errors.New("unavailable")
	}
	out := &clientv3.GetResponse{}
	if item := m.data[key]; item != nil {
		out.Kvs = []*mvccpb.KeyValue{item}
	}
	return out, nil
}
func (m *preferenceKV) Txn(context.Context) clientv3.Txn { return &preferenceTxn{kv: m} }

type preferenceTxn struct {
	kv   *preferenceKV
	cmps []clientv3.Cmp
	ops  []clientv3.Op
}

func (t *preferenceTxn) If(cmps ...clientv3.Cmp) clientv3.Txn { t.cmps = cmps; return t }
func (t *preferenceTxn) Then(ops ...clientv3.Op) clientv3.Txn { t.ops = ops; return t }
func (t *preferenceTxn) Else(...clientv3.Op) clientv3.Txn     { panic("unexpected else") }
func (t *preferenceTxn) Commit() (*clientv3.TxnResponse, error) {
	if t.kv.fail {
		return nil, errors.New("unavailable")
	}
	if len(t.cmps) != 1 || len(t.ops) != 1 {
		panic("expected single CAS")
	}
	cmp := t.cmps[0].GetCompare()
	op := t.ops[0]
	if cmp.Target != etcdserverpb.Compare_MOD || cmp.Result != etcdserverpb.Compare_EQUAL || !op.IsPut() || string(op.KeyBytes()) != string(cmp.Key) {
		panic("bad CAS")
	}
	var revision int64
	if old := t.kv.data[string(cmp.Key)]; old != nil {
		revision = old.ModRevision
	}
	if revision != cmp.GetModRevision() {
		return &clientv3.TxnResponse{Succeeded: false}, nil
	}
	t.kv.revision++
	t.kv.data[string(cmp.Key)] = &mvccpb.KeyValue{Key: cmp.Key, Value: op.ValueBytes(), ModRevision: t.kv.revision}
	return &clientv3.TxnResponse{Succeeded: true, Header: &etcdserverpb.ResponseHeader{Revision: t.kv.revision}}, nil
}
func preferenceScope() Scope {
	return Scope{UserID: uuid.NewString(), OrganizationID: uuid.NewString(), StationID: uuid.NewString(), AuthorizationEpoch: "epoch"}
}
func TestPreferencePersistenceIsolationAndCAS(t *testing.T) {
	kv := &preferenceKV{data: map[string]*mvccpb.KeyValue{}}
	store := &EtcdPreferences{Client: func() clientv3.KV { return kv }}
	scope := preferenceScope()
	ctx := context.Background()
	prefs, revision, err := store.Load(ctx, scope)
	if err != nil || revision != "0" || len(prefs.Entries) != 0 {
		t.Fatal("initial state", err)
	}
	prefs.Entries = append(prefs.Entries, Preference{AppID: "sample", EntryID: "main", Pinned: false, Order: 9})
	revision, err = store.Save(ctx, scope, prefs, revision)
	if err != nil || revision != "1" {
		t.Fatal("save", err)
	}
	if _, err = store.Save(ctx, scope, prefs, "0"); !errors.Is(err, ErrConflict) {
		t.Fatal("lost update accepted", err)
	}
	// Another store/authorization lifetime reads the durable explicit unpin.
	second := &EtcdPreferences{Client: func() clientv3.KV { return kv }}
	scope.AuthorizationEpoch = "new-epoch"
	loaded, revision, err := second.Load(ctx, scope)
	if err != nil || revision != "1" || loaded.Entries[0].Pinned || loaded.Entries[0].Order != 9 {
		t.Fatal("preference reset", err)
	}
	for _, field := range []string{"user", "org", "station"} {
		other := scope
		switch field {
		case "user":
			other.UserID = uuid.NewString()
		case "org":
			other.OrganizationID = uuid.NewString()
		case "station":
			other.StationID = uuid.NewString()
		}
		p, r, e := store.Load(ctx, other)
		if e != nil || r != "0" || len(p.Entries) != 0 {
			t.Fatal("scope leaked")
		}
	}
	for k, v := range kv.data {
		if strings.Contains(k, scope.UserID) || strings.Contains(string(v.Value), "epoch") {
			t.Fatal("unstable key or authorization persisted")
		}
	}
}
func TestPreferenceUnavailableAndCorruptionFailClosed(t *testing.T) {
	kv := &preferenceKV{data: map[string]*mvccpb.KeyValue{}}
	store := &EtcdPreferences{Client: func() clientv3.KV { return kv }}
	scope := preferenceScope()
	ctx := context.Background()
	kv.fail = true
	if _, _, err := store.Load(ctx, scope); !errors.Is(err, ErrUnavailable) {
		t.Fatal("unavailable returned defaults")
	}
	kv.fail = false
	foreign := scope
	foreign.UserID = uuid.NewString()
	prefs := EmptyPreferences(foreign)
	if _, err := store.Save(ctx, scope, prefs, "0"); !errors.Is(err, ErrInvalid) {
		t.Fatal("foreign write accepted")
	}
	kv.data[preferenceKey(scope)] = &mvccpb.KeyValue{ModRevision: 1, Value: []byte(`{"schemaVersion":1,"schemaVersion":1}`)}
	if _, _, err := store.Load(ctx, scope); !errors.Is(err, ErrInvalid) {
		t.Fatal("corruption returned defaults")
	}
}
