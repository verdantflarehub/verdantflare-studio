package dashboard

import (
	"reflect"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/verdantflarehub/verdantflare-studio/internal/project"
)

type ContentHandle struct {
	Actor     Actor
	SessionID string
	Version   project.ContentVersion
	Access    project.Access
	ExpiresAt time.Time
}

type ContentHandles struct {
	mu       sync.Mutex
	items    map[string]contentHandleEntry
	sequence uint64
}

type contentHandleEntry struct {
	ContentHandle
	lastUsed uint64
}

func (h *ContentHandles) touch(id string, entry contentHandleEntry) {
	h.sequence++
	entry.lastUsed = h.sequence
	h.items[id] = entry
}

func (h *ContentHandles) Add(in ContentHandle) (string, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	now := time.Now()
	if !in.ExpiresAt.After(now) {
		return "", ErrExpired
	}
	if h.items == nil {
		h.items = map[string]contentHandleEntry{}
	}
	count := 0
	var reusable, oldest string
	var oldestUse uint64
	for id, existing := range h.items {
		if !existing.ExpiresAt.After(now) {
			delete(h.items, id)
			continue
		}
		if existing.SessionID == in.SessionID {
			count++
			if existing.Actor == in.Actor {
				if oldest == "" || existing.lastUsed < oldestUse {
					oldest, oldestUse = id, existing.lastUsed
				}
				if reflect.DeepEqual(existing.Version, in.Version) && existing.Access == in.Access {
					reusable = id
				}
			}
		}
	}
	if reusable != "" {
		h.touch(reusable, h.items[reusable])
		return reusable, nil
	}
	if count >= 128 || len(h.items) >= 4096 {
		// Recycle only this actor's own session. Other sessions cannot displace
		// its handles. A Get already holding a value can finish its transfer;
		// later uses of the retired path must obtain a freshly authorized handle.
		if oldest == "" {
			return "", ErrUnavailable
		}
		delete(h.items, oldest)
	}
	id := uuid.NewString()
	h.touch(id, contentHandleEntry{ContentHandle: in})
	return id, nil
}

func (h *ContentHandles) Get(actor Actor, sessionID, id string) (ContentHandle, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	item, ok := h.items[id]
	if !ok || !item.ExpiresAt.After(time.Now()) {
		delete(h.items, id)
		return ContentHandle{}, ErrExpired
	}
	if item.Actor != actor || item.SessionID != sessionID {
		return ContentHandle{}, ErrDenied
	}
	h.touch(id, item)
	return item.ContentHandle, nil
}

func (h *ContentHandles) Close(actor Actor, sessionID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for id, item := range h.items {
		if item.Actor == actor && item.SessionID == sessionID {
			delete(h.items, id)
		}
	}
}
