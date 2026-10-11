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
	mu    sync.Mutex
	items map[string]ContentHandle
}

func (h *ContentHandles) Add(in ContentHandle) (string, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if !in.ExpiresAt.After(time.Now()) {
		return "", ErrExpired
	}
	if h.items == nil {
		h.items = map[string]ContentHandle{}
	}
	count := 0
	for id, existing := range h.items {
		if !existing.ExpiresAt.After(time.Now()) {
			delete(h.items, id)
			continue
		}
		if existing.SessionID == in.SessionID {
			count++
			if existing.Actor == in.Actor && reflect.DeepEqual(existing.Version, in.Version) && existing.Access == in.Access {
				return id, nil
			}
		}
	}
	if count >= 128 || len(h.items) >= 4096 {
		return "", ErrUnavailable
	}
	id := uuid.NewString()
	h.items[id] = in
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
	return item, nil
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
