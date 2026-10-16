// Package appstatehints transports disposable refresh hints, never App state.
package appstatehints

import (
	"bytes"
	"context"
	"errors"
	"regexp"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/verdantflarehub/verdantflare-studio/internal/strictjson"
	clientv3 "go.etcd.io/etcd/client/v3"
)

var ErrUnavailable = errors.New("application state hints unavailable")
var revisionToken = regexp.MustCompile(`^[A-Za-z0-9:._-]{1,128}$`)

func Key(station, organization string) (string, error) {
	for _, value := range []string{station, organization} {
		id, err := uuid.Parse(value)
		if err != nil || id == uuid.Nil || id.String() != value {
			return "", ErrUnavailable
		}
	}
	return NotificationPrefix + station + "/" + organization, nil
}

func decode(raw []byte) RefreshEvent {
	event := RefreshEvent{SchemaVersion: 1}
	var hint Notification
	if strictjson.Decode(bytes.NewReader(raw), 1024, &hint) != nil || hint.SchemaVersion != 1 || !revisionToken.MatchString(hint.Revision) {
		return event
	}
	if _, err := time.Parse(time.RFC3339Nano, hint.ChangedAt); err == nil {
		event.Revision = hint.Revision
	}
	return event
}

// Subscribe establishes the Get/Watch barrier before requesting a fresh view.
// The caller must cancel ctx; errors close the channel and require a new barrier.
func Subscribe(ctx context.Context, client *clientv3.Client, station, organization string) (<-chan RefreshEvent, error) {
	return SubscribeWithExtras(ctx, client, station, organization)
}

// SubscribeWithExtras watches the Station/Organization notification key as well
// as optional exact etcd keys (such as navigation catalog and user preference keys).
func SubscribeWithExtras(ctx context.Context, client *clientv3.Client, station, organization string, extraKeys ...string) (<-chan RefreshEvent, error) {
	key, err := Key(station, organization)
	if err != nil || client == nil {
		return nil, ErrUnavailable
	}
	readCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	first, err := client.Get(readCtx, key)
	cancel()
	if err != nil || first.Header == nil || first.Header.Revision < 1 {
		return nil, ErrUnavailable
	}
	watchCtx, stop := context.WithCancel(ctx)
	rev := first.Header.Revision + 1
	watch := client.Watch(watchCtx, key, clientv3.WithRev(rev))
	var extraWatches []clientv3.WatchChan
	for _, extra := range extraKeys {
		if extra != "" {
			extraWatches = append(extraWatches, client.Watch(watchCtx, extra, clientv3.WithRev(rev)))
		}
	}
	out := make(chan RefreshEvent, 1)
	initial := RefreshEvent{SchemaVersion: 1}
	if len(first.Kvs) == 1 {
		initial = decode(first.Kvs[0].Value)
	}
	out <- initial
	push := func(event RefreshEvent) bool {
		select {
		case <-out:
		default:
		}
		select {
		case out <- event:
			return true
		case <-ctx.Done():
			return false
		}
	}
	for _, ew := range extraWatches {
		ch := ew
		go func() {
			for {
				select {
				case <-watchCtx.Done():
					return
				case response, ok := <-ch:
					if !ok || response.Canceled || response.Err() != nil {
						stop()
						return
					}
					for _, change := range response.Events {
						event := RefreshEvent{SchemaVersion: 1}
						if change.Kv != nil && change.Kv.ModRevision > 0 {
							event.Revision = "nav:" + strconv.FormatInt(change.Kv.ModRevision, 10)
						}
						if !push(event) {
							return
						}
					}
				}
			}
		}()
	}
	go func() {
		defer stop()
		defer close(out)
		for {
			select {
			case <-watchCtx.Done():
				return
			case response, ok := <-watch:
				if !ok || response.Canceled || response.Err() != nil {
					return
				}
				for _, change := range response.Events {
					event := RefreshEvent{SchemaVersion: 1}
					if change.Kv != nil && string(change.Kv.Key) == key && change.Type == clientv3.EventTypePut {
						event = decode(change.Kv.Value)
					}
					if !push(event) {
						return
					}
				}
			}
		}
	}()
	return out, nil
}
