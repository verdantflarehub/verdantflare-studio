package appstatehints

import (
	"context"
	"sync"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
)

// OrganizationWatcher keeps one exact-key watch per Station/Organization
// notification key and reports every change. Studio's etcd role grants only
// exact keys, so a prefix watch is denied; watches start lazily for the
// organizations that are actually cached.
type OrganizationWatcher struct {
	ctx      context.Context
	client   *clientv3.Client
	changed  func(station, organization string)
	mu       sync.Mutex
	watching map[string]bool
}

func NewOrganizationWatcher(ctx context.Context, client *clientv3.Client, changed func(station, organization string)) *OrganizationWatcher {
	return &OrganizationWatcher{ctx: ctx, client: client, changed: changed, watching: map[string]bool{}}
}

// Ensure starts the watch for one organization if it is not running yet.
func (w *OrganizationWatcher) Ensure(station, organization string) {
	if w == nil || w.client == nil {
		return
	}
	key, err := Key(station, organization)
	if err != nil {
		return
	}
	w.mu.Lock()
	if w.watching[key] || len(w.watching) >= 1000 {
		w.mu.Unlock()
		return
	}
	w.watching[key] = true
	w.mu.Unlock()
	go w.run(key, station, organization)
}

func (w *OrganizationWatcher) run(key, station, organization string) {
	backoff, logged := time.Second, false
	for w.ctx.Err() == nil {
		watchCtx, cancel := context.WithCancel(clientv3.WithRequireLeader(w.ctx))
		healthy := false
		// A successful unary read refreshes expired credentials. A private
		// watcher then opens a new stream instead of reusing another watch's
		// stream with its old authentication metadata.
		readCtx, finishRead := context.WithTimeout(watchCtx, 3*time.Second)
		first, err := w.client.Get(readCtx, key)
		finishRead()
		watcher := clientv3.NewWatcher(w.client)
		var responses clientv3.WatchChan
		if err == nil && first.Header != nil {
			responses = watcher.Watch(watchCtx, key, clientv3.WithRev(first.Header.Revision+1))
		} else {
			closed := make(chan clientv3.WatchResponse)
			close(closed)
			responses = closed
		}
		for response := range responses {
			if response.Canceled || response.Err() != nil {
				if !logged {
					logWatchEnd("invalidation", true, response)
					logged = true
				}
				break
			}
			healthy = true
			if len(response.Events) > 0 {
				w.changed(station, organization)
			}
		}
		cancel()
		_ = watcher.Close()
		// Changes during a gap cannot be replayed from disposable hints.
		w.changed(station, organization)
		if healthy {
			backoff, logged = time.Second, false
		} else if backoff < 30*time.Second {
			backoff *= 2
		}
		select {
		case <-w.ctx.Done():
			return
		case <-time.After(backoff):
		}
	}
}
