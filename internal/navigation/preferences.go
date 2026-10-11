package navigation

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/verdantflarehub/verdantflare-studio/internal/strictjson"
	clientv3 "go.etcd.io/etcd/client/v3"
)

var ErrConflict = errors.New("NAVIGATION_PREFERENCE_CONFLICT")

type PreferenceStore interface {
	Load(context.Context, Scope) (Preferences, string, error)
	Save(context.Context, Scope, Preferences, string) (string, error)
}

type preferenceEntry struct {
	AppID   string `json:"appId"`
	EntryID string `json:"entryId"`
	Pinned  bool   `json:"pinned"`
	Order   int    `json:"order"`
}
type preferenceRecord struct {
	SchemaVersion  int               `json:"schemaVersion"`
	UserID         string            `json:"userId"`
	OrganizationID string            `json:"organizationId"`
	StationID      string            `json:"stationId"`
	Entries        []preferenceEntry `json:"entries"`
}

func EmptyPreferences(scope Scope) Preferences {
	return Preferences{UserID: scope.UserID, OrganizationID: scope.OrganizationID, StationID: scope.StationID, Entries: []Preference{}}
}
func validPreferences(scope Scope, prefs Preferences) bool {
	if !subject(scope.UserID) || !subject(scope.OrganizationID) || !subject(scope.StationID) || prefs.UserID != scope.UserID || prefs.OrganizationID != scope.OrganizationID || prefs.StationID != scope.StationID || prefs.Entries == nil || len(prefs.Entries) > 128 {
		return false
	}
	seen := map[string]bool{}
	for _, p := range prefs.Entries {
		k := key(p.AppID, p.EntryID)
		if seen[k] || !ident.MatchString(p.AppID) || !ident.MatchString(p.EntryID) || !orderValid(p.Order) {
			return false
		}
		seen[k] = true
	}
	return true
}

// ValidPreferenceRevision rejects aliases and values outside etcd's range.
func ValidPreferenceRevision(value string) bool {
	n, err := strconv.ParseInt(value, 10, 64)
	return err == nil && n >= 0 && strconv.FormatInt(n, 10) == value
}

func preferenceKey(scope Scope) string {
	data, _ := json.Marshal([]string{scope.UserID, scope.OrganizationID, scope.StationID})
	return "/verdantflare/studio/navigation/preferences/" + sum(data)[7:]
}

// EtcdPreferences has no process-memory fallback. Returning defaults on an
// unavailable store would silently lose explicit unpins and ordering choices.
type EtcdPreferences struct{ Client func() clientv3.KV }

func (s *EtcdPreferences) Load(ctx context.Context, scope Scope) (Preferences, string, error) {
	prefs := EmptyPreferences(scope)
	if !validPreferences(scope, prefs) {
		return prefs, "", ErrInvalid
	}
	if s.Client == nil {
		return prefs, "", ErrUnavailable
	}
	client := s.Client()
	if client == nil {
		return prefs, "", ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	response, err := client.Get(ctx, preferenceKey(scope))
	if err != nil || response == nil {
		return prefs, "", ErrUnavailable
	}
	if len(response.Kvs) == 0 {
		return prefs, "0", nil
	}
	if len(response.Kvs) != 1 || response.Kvs[0].ModRevision < 1 {
		return prefs, "", ErrInvalid
	}
	var record preferenceRecord
	if strictjson.Decode(bytes.NewReader(response.Kvs[0].Value), 64<<10, &record) != nil || record.SchemaVersion != 1 || record.Entries == nil {
		return prefs, "", ErrInvalid
	}
	prefs = Preferences{UserID: record.UserID, OrganizationID: record.OrganizationID, StationID: record.StationID, Entries: []Preference{}}
	for _, p := range record.Entries {
		prefs.Entries = append(prefs.Entries, Preference{AppID: p.AppID, EntryID: p.EntryID, Pinned: p.Pinned, Order: p.Order})
	}
	if !validPreferences(scope, prefs) {
		return prefs, "", ErrInvalid
	}
	return prefs, strconv.FormatInt(response.Kvs[0].ModRevision, 10), nil
}

func (s *EtcdPreferences) Save(ctx context.Context, scope Scope, prefs Preferences, revision string) (string, error) {
	if !validPreferences(scope, prefs) || !ValidPreferenceRevision(revision) {
		return "", ErrInvalid
	}
	if s.Client == nil {
		return "", ErrUnavailable
	}
	client := s.Client()
	if client == nil {
		return "", ErrUnavailable
	}
	record := preferenceRecord{SchemaVersion: 1, UserID: prefs.UserID, OrganizationID: prefs.OrganizationID, StationID: prefs.StationID, Entries: []preferenceEntry{}}
	for _, p := range prefs.Entries {
		record.Entries = append(record.Entries, preferenceEntry{AppID: p.AppID, EntryID: p.EntryID, Pinned: p.Pinned, Order: p.Order})
	}
	data, err := json.Marshal(record)
	if err != nil || len(data) > 64<<10 {
		return "", ErrInvalid
	}
	rev, _ := strconv.ParseInt(revision, 10, 64)
	k := preferenceKey(scope)
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	response, err := client.Txn(ctx).If(clientv3.Compare(clientv3.ModRevision(k), "=", rev)).Then(clientv3.OpPut(k, string(data))).Commit()
	if err != nil || response == nil {
		return "", ErrUnavailable
	}
	if !response.Succeeded {
		return "", ErrConflict
	}
	if response.Header == nil || response.Header.Revision < 1 {
		return "", ErrInvalid
	}
	return strconv.FormatInt(response.Header.Revision, 10), nil
}
