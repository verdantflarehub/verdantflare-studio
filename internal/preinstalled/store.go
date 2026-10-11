package preinstalled

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"

	"github.com/verdantflarehub/verdantflare-studio/internal/strictjson"
	clientv3 "go.etcd.io/etcd/client/v3"
)

// EtcdStore persists only scope, immutable release references and operation
// recovery data. It stores no access token or service credential.
type EtcdStore struct{ Client func() clientv3.KV }

func journalPath(key string) (string, error) {
	parts := strings.Split(key, "/")
	if len(parts) != 3 || !validID(parts[0]) || !validID(parts[1]) || !identifier.MatchString(parts[2]) {
		return "", ErrInvalid
	}
	return "/studio/preinstalled/v1/" + key, nil
}
func (s *EtcdStore) Load(ctx context.Context, key string) (Journal, int64, error) {
	var out Journal
	path, err := journalPath(key)
	if err != nil {
		return out, 0, err
	}
	if s == nil || s.Client == nil {
		return out, 0, ErrUnavailable
	}
	client := s.Client()
	if client == nil {
		return out, 0, ErrUnavailable
	}
	res, err := client.Get(ctx, path)
	if err != nil {
		return out, 0, ErrUnavailable
	}
	if len(res.Kvs) == 0 {
		return out, 0, nil
	}
	if len(res.Kvs) != 1 {
		return out, 0, ErrConflict
	}
	if strictjson.Decode(bytes.NewReader(res.Kvs[0].Value), 2<<20, &out) != nil {
		return out, 0, ErrConflict
	}
	return out, res.Kvs[0].ModRevision, nil
}
func (s *EtcdStore) Save(ctx context.Context, key string, revision int64, journal Journal) (int64, error) {
	path, err := journalPath(key)
	if err != nil {
		return 0, err
	}
	if s == nil || s.Client == nil {
		return 0, ErrUnavailable
	}
	client := s.Client()
	if client == nil {
		return 0, ErrUnavailable
	}
	if revision < 0 {
		return 0, ErrConflict
	}
	raw, err := json.Marshal(journal)
	if err != nil || len(raw) > 2<<20 {
		return 0, ErrInvalid
	}
	condition := clientv3.Compare(clientv3.ModRevision(path), "=", revision)
	if revision == 0 {
		condition = clientv3.Compare(clientv3.CreateRevision(path), "=", 0)
	}
	res, err := client.Txn(ctx).If(condition).Then(clientv3.OpPut(path, string(raw))).Commit()
	if err != nil {
		return 0, ErrUnavailable
	}
	if !res.Succeeded {
		return 0, ErrConflict
	}
	return res.Header.Revision, nil
}
