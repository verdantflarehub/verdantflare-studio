package appstatehints

import (
	"context"
	"strings"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
)

// WatchInvalidations follows every Station/Organization notification key and
// reports which scope changed. Each (re)subscription first calls reset, since
// changes during a gap cannot be replayed from disposable hints.
func WatchInvalidations(ctx context.Context, client *clientv3.Client, changed func(station, organization string), reset func()) {
	if client == nil {
		return
	}
	for ctx.Err() == nil {
		watchCtx, cancel := context.WithCancel(clientv3.WithRequireLeader(ctx))
		reset()
		for response := range client.Watch(watchCtx, NotificationPrefix, clientv3.WithPrefix()) {
			if response.Canceled || response.Err() != nil {
				break
			}
			for _, event := range response.Events {
				if event.Kv == nil {
					continue
				}
				station, organization, ok := strings.Cut(strings.TrimPrefix(string(event.Kv.Key), NotificationPrefix), "/")
				if !ok {
					continue
				}
				if _, err := Key(station, organization); err == nil {
					changed(station, organization)
				}
			}
		}
		cancel()
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Second):
		}
	}
}
