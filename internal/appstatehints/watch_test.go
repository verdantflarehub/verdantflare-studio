package appstatehints

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.uber.org/zap"
)

func TestHintsScopeAndPayload(t *testing.T) {
	station, org := uuid.NewString(), uuid.NewString()
	if key, err := Key(station, org); err != nil || key != NotificationPrefix+station+"/"+org {
		t.Fatal("wrong scope key")
	}
	for _, bad := range []string{"", "../other", station + "/other", "00000000-0000-0000-0000-000000000000"} {
		if _, err := Key(station, bad); err == nil {
			t.Fatal("client-chosen invalid scope accepted")
		}
	}
	raw, _ := json.Marshal(Notification{SchemaVersion: 1, Revision: "boot:opaque:10", ChangedAt: time.Now().UTC().Format(time.RFC3339Nano)})
	if decode(raw).Revision != "boot:opaque:10" {
		t.Fatal("valid hint rejected")
	}
	for _, bad := range []string{
		`{"schemaVersion":1,"revision":"private","changedAt":"invalid"}`,
		`{"schemaVersion":1,"revision":"private","changedAt":"2026-10-12T00:00:00Z","state":"ready"}`,
		`{"schemaVersion":1,"revision":"private","revision":"duplicate","changedAt":"2026-10-12T00:00:00Z"}`,
		`{"schemaVersion":2,"revision":"private","changedAt":"2026-10-12T00:00:00Z"}`,
	} {
		if decode([]byte(bad)).Revision != "" {
			t.Fatal("invalid payload forwarded")
		}
	}
}

func TestHintsEtcdBarrierIsolationDeleteAndCancellation(t *testing.T) {
	endpoint := os.Getenv("STATION_TEST_ETCD_ENDPOINT")
	if endpoint == "" {
		t.Skip("5090 etcd required")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	client, err := clientv3.New(clientv3.Config{Endpoints: []string{endpoint}, DialTimeout: 3 * time.Second, Logger: zap.NewNop()})
	if err != nil {
		t.Fatal("etcd unavailable")
	}
	defer client.Close()
	station, org := uuid.NewString(), uuid.NewString()
	key, _ := Key(station, org)
	foreign, _ := Key(station, uuid.NewString())
	defer func() { _, _ = client.Delete(ctx, key); _, _ = client.Delete(ctx, foreign) }()
	watchCtx, stop := context.WithCancel(ctx)
	defer stop()
	events, err := Subscribe(watchCtx, client, station, org)
	if err != nil {
		t.Fatal(err)
	}
	receive := func() RefreshEvent {
		t.Helper()
		select {
		case event, ok := <-events:
			if !ok {
				t.Fatal("unexpected watch closure")
			}
			return event
		case <-ctx.Done():
			t.Fatal("missing hint")
			return RefreshEvent{}
		}
	}
	if receive().SchemaVersion != 1 {
		t.Fatal("missing initialization barrier")
	}
	put := func(target, revision string) {
		t.Helper()
		raw, _ := json.Marshal(Notification{SchemaVersion: 1, Revision: revision, ChangedAt: time.Now().UTC().Format(time.RFC3339Nano)})
		lease, err := client.Grant(ctx, 30)
		if err != nil {
			t.Fatal("test lease failed")
		}
		if _, err := client.Put(ctx, target, string(raw), clientv3.WithLease(lease.ID)); err != nil {
			t.Fatal("test put failed")
		}
	}
	put(foreign, "foreign-secret")
	put(key, "boot-z:10")
	if event := receive(); event.Revision != "boot-z:10" {
		t.Fatal("missed barrier event or leaked another organization")
	}
	put(key, "boot-a:1")
	if receive().Revision != "boot-a:1" {
		t.Fatal("opaque revision was ordered")
	}
	if _, err := client.Delete(ctx, key); err != nil {
		t.Fatal("test delete failed")
	}
	if receive().Revision != "" {
		t.Fatal("deletion did not request calibration")
	}
	stop()
	select {
	case _, ok := <-events:
		if ok {
			t.Fatal("watch survived cancellation")
		}
	case <-ctx.Done():
		t.Fatal("watch cancellation blocked")
	}
}
