// Package preinstalled coordinates explicit installation requests through the
// trusted lifecycle boundary. It never installs workloads or invents facts.
package preinstalled

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"regexp"

	"github.com/google/uuid"
	"github.com/verdantflarehub/verdantflare-studio/internal/strictjson"
)

var ErrInvalid = errors.New("INVALID_PREINSTALLED_CONFIG")
var ErrConflict = errors.New("PREINSTALLED_STATE_CONFLICT")
var ErrUnavailable = errors.New("PREINSTALLED_DEPENDENCY_UNAVAILABLE")
var ErrDenied = errors.New("PREINSTALLED_DENIED")
var identifier = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,62}$`)
var appIdentifier = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]{0,61}[a-z0-9])?$`)
var versionPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)
var digestPattern = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)

type Release struct {
	AppID         string   `json:"appId"`
	AppVersion    string   `json:"appVersion"`
	ReleaseRef    string   `json:"releaseRef"`
	PackageDigest string   `json:"packageDigest"`
	ChartDigest   string   `json:"chartDigest"`
	Dependencies  []string `json:"dependencies"`
}
type Config struct {
	SchemaVersion  int       `json:"schemaVersion"`
	ConfigID       string    `json:"configId"`
	Revision       string    `json:"revision"`
	OrganizationID string    `json:"organizationId"`
	StationID      string    `json:"stationId"`
	Apps           []Release `json:"apps"`
}

func validID(value string) bool {
	id, err := uuid.Parse(value)
	return err == nil && id != uuid.Nil && id.String() == value
}
func (c Config) Validate() error {
	if c.SchemaVersion != 1 || !identifier.MatchString(c.ConfigID) || !versionPattern.MatchString(c.Revision) || !validID(c.OrganizationID) || !validID(c.StationID) || len(c.Apps) == 0 || len(c.Apps) > 128 {
		return ErrInvalid
	}
	apps := map[string]Release{}
	for _, app := range c.Apps {
		if !appIdentifier.MatchString(app.AppID) || !versionPattern.MatchString(app.AppVersion) || !identifier.MatchString(app.ReleaseRef) || !digestPattern.MatchString(app.PackageDigest) || !digestPattern.MatchString(app.ChartDigest) || app.Dependencies == nil || len(app.Dependencies) > 128 {
			return ErrInvalid
		}
		if _, ok := apps[app.AppID]; ok {
			return ErrInvalid
		}
		apps[app.AppID] = app
	}
	seen, active := map[string]bool{}, map[string]bool{}
	var visit func(string) error
	visit = func(id string) error {
		if active[id] {
			return ErrInvalid
		}
		if seen[id] {
			return nil
		}
		app, ok := apps[id]
		if !ok {
			return ErrInvalid
		}
		active[id] = true
		deps := map[string]bool{}
		for _, dep := range app.Dependencies {
			if deps[dep] {
				return ErrInvalid
			}
			deps[dep] = true
			if err := visit(dep); err != nil {
				return err
			}
		}
		delete(active, id)
		seen[id] = true
		return nil
	}
	for id := range apps {
		if err := visit(id); err != nil {
			return err
		}
	}
	return nil
}
func (c Config) Ordered() []Release {
	apps := map[string]Release{}
	for _, app := range c.Apps {
		apps[app.AppID] = app
	}
	out := []Release{}
	seen := map[string]bool{}
	var visit func(string)
	visit = func(id string) {
		if seen[id] {
			return
		}
		seen[id] = true
		for _, dep := range apps[id].Dependencies {
			visit(dep)
		}
		out = append(out, apps[id])
	}
	for _, app := range c.Apps {
		visit(app.AppID)
	}
	return out
}
func (c Config) Digest() string {
	raw, _ := json.Marshal(c)
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:])
}
func DecodeConfig(raw []byte) (Config, error) {
	var c Config
	if strictjson.Decode(bytes.NewReader(raw), 1<<20, &c) != nil || c.Validate() != nil {
		return Config{}, ErrInvalid
	}
	return c, nil
}
func LoadConfig(path string) (Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return Config{}, ErrUnavailable
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, (1<<20)+1))
	if err != nil || len(raw) > 1<<20 {
		return Config{}, ErrInvalid
	}
	return DecodeConfig(raw)
}
