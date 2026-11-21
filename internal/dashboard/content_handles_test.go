package dashboard

import (
	"errors"
	"fmt"
	"testing"
	"time"
)

func handleFixture() ContentHandle {
	return ContentHandle{Actor: Actor{UserID: "reader", OrganizationID: "org", StationID: "station"}, SessionID: "page", ExpiresAt: time.Now().Add(time.Hour)}
}

func TestContentHandlesRecycleLeastRecentlyUsedWithinSession(t *testing.T) {
	var h ContentHandles
	in := handleFixture()
	ids := make([]string, 128)
	for i := range ids {
		in.Version.VersionID = fmt.Sprint(i)
		var err error
		ids[i], err = h.Add(in)
		if err != nil {
			t.Fatal(err)
		}
	}
	// A real read and an identical descriptor both protect recently used paths.
	if _, err := h.Get(in.Actor, in.SessionID, ids[0]); err != nil {
		t.Fatal(err)
	}
	in.Version.VersionID = "1"
	if id, err := h.Add(in); err != nil || id != ids[1] {
		t.Fatal("identical content did not reuse its handle", err)
	}
	other := in.Actor
	other.UserID = "other"
	if _, err := h.Get(other, in.SessionID, ids[2]); !errors.Is(err, ErrDenied) {
		t.Fatal("cross-subject request accepted", err)
	}
	in.Version.VersionID = "new"
	if _, err := h.Add(in); err != nil || len(h.items) != 128 {
		t.Fatal("capacity was not recycled within the bound", err)
	}
	if _, err := h.Get(in.Actor, in.SessionID, ids[2]); !errors.Is(err, ErrExpired) {
		t.Fatal("least recently used path was not retired", err)
	}
	for _, id := range ids[:2] {
		if _, err := h.Get(in.Actor, in.SessionID, id); err != nil {
			t.Fatal("recent handle was displaced", err)
		}
	}
	// Access scope is part of reuse identity even for the same immutable bytes.
	in.Access.AssetVersionID = "different-scope"
	if _, err := h.Add(in); err != nil || len(h.items) != 128 {
		t.Fatal("new access scope was not allocated separately", err)
	}
	h.Close(other, in.SessionID)
	if len(h.items) != 128 {
		t.Fatal("another subject closed the session's handles")
	}
	h.Close(in.Actor, in.SessionID)
	if len(h.items) != 0 {
		t.Fatal("close did not release handles")
	}
}

func TestContentHandlesGlobalBoundDoesNotDisplaceOtherSessions(t *testing.T) {
	var h ContentHandles
	in := handleFixture()
	// Seed a full process with distinct sessions; this exercises the global
	// boundary without spending test time filling each session separately.
	h.items = make(map[string]contentHandleEntry, 4096)
	for i := 0; i < 4096; i++ {
		entry := in
		entry.SessionID = fmt.Sprintf("page-%d", i)
		h.touch(fmt.Sprint(i), contentHandleEntry{ContentHandle: entry})
	}
	if _, err := h.Add(in); !errors.Is(err, ErrUnavailable) || len(h.items) != 4096 {
		t.Fatal("a new session displaced another session", err)
	}
	in.SessionID = "page-0"
	in.Version.VersionID = "new"
	if _, err := h.Add(in); err != nil || len(h.items) != 4096 {
		t.Fatal("own handle could not be recycled at global capacity", err)
	}
	if _, ok := h.items["0"]; ok {
		t.Fatal("old own handle retained")
	}
	if _, ok := h.items["1"]; !ok {
		t.Fatal("another session was displaced")
	}
	in.Actor.UserID = "another-subject"
	if _, err := h.Add(in); !errors.Is(err, ErrUnavailable) {
		t.Fatal("another subject recycled a handle", err)
	}
	in.ExpiresAt = time.Now().Add(-time.Second)
	if _, err := h.Add(in); !errors.Is(err, ErrExpired) {
		t.Fatal("expired allocation accepted", err)
	}
}
