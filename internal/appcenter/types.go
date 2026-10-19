// Package appcenter implements the common AppCenter projection, without App
// names or network calls. Contracts are owned by the central design repository.
package appcenter

import (
	"encoding/json"
	"time"
)

type Scope struct {
	UserID             string `json:"userId"`
	OrganizationID     string `json:"organizationId"`
	StationID          string `json:"stationId"`
	AuthorizationEpoch string `json:"authorizationEpoch"`
}
type Source struct {
	Source     string     `json:"source"`
	Quality    string     `json:"quality"`
	Revision   *string    `json:"revision"`
	ObservedAt *time.Time `json:"observedAt"`
	ReasonCode *string    `json:"reasonCode"`
}
type Installation struct {
	InstallationID  string  `json:"installationId"`
	Revision        int64   `json:"revision"`
	AppVersion      string  `json:"appVersion"`
	State           string  `json:"state"`
	LastOperationID *string `json:"lastOperationId"`
}
type Service struct {
	Quality        string     `json:"quality"`
	State          string     `json:"state"`
	ObservedAt     *time.Time `json:"observedAt"`
	ReasonCode     *string    `json:"reasonCode"`
	ObservationKey *string    `json:"observationKey"`
}
type CoreService struct {
	Service
	WorkloadUID *string `json:"workloadUID"`
}
type Operation struct {
	Quality     string     `json:"quality"`
	Blocking    bool       `json:"blocking"`
	OperationID *string    `json:"operationId"`
	Protocol    *string    `json:"protocol"`
	Status      *string    `json:"status"`
	Phase       *string    `json:"phase"`
	CanRead     bool       `json:"canRead"`
	UpdatedAt   *time.Time `json:"updatedAt"`
}
type Candidate struct {
	Kind                string  `json:"kind"`
	Enabled             bool    `json:"enabled"`
	ReasonCode          *string `json:"reasonCode"`
	AppVersion          string  `json:"appVersion"`
	ReleaseDigest       *string `json:"releaseDigest"`
	ExpectedWorkloadUID *string `json:"expectedWorkloadUID"`
}
type Lifecycle struct {
	PolicyRevision string      `json:"policyRevision"`
	Candidates     []Candidate `json:"candidates"`
}
type Fact struct {
	CardKey             string          `json:"cardKey"`
	AppID               string          `json:"appId"`
	Origin              string          `json:"origin"`
	Catalog             json.RawMessage `json:"catalog"`
	Installation        *Installation   `json:"installation"`
	InstallationQuality string          `json:"installationQuality"`
	Service             CoreService     `json:"service"`
	Operation           Operation       `json:"operation"`
	Lifecycle           Lifecycle       `json:"lifecycle"`
}
type Facts struct {
	SchemaVersion int       `json:"schemaVersion"`
	RequestID     string    `json:"requestId"`
	Scope         Scope     `json:"scope"`
	Revision      string    `json:"revision"`
	GeneratedAt   time.Time `json:"generatedAt"`
	Sources       []Source  `json:"sources"`
	Items         []Fact    `json:"items"`
}
type ResourceTag struct {
	Kind      string  `json:"kind"`
	Value     float64 `json:"value"`
	Unit      string  `json:"unit"`
	SourceRef string  `json:"sourceRef"`
}
type Display struct {
	Name           string        `json:"name"`
	Description    string        `json:"description"`
	GroupID        string        `json:"groupId"`
	PublisherID    *string       `json:"publisherId"`
	IconRef        *string       `json:"iconRef"`
	ResourceTags   []ResourceTag `json:"resourceTags"`
	Quality        string        `json:"quality"`
	Models         *[]string     `json:"models,omitempty"`
	Capabilities   *[]string     `json:"capabilities,omitempty"`
	DisabledReason *string       `json:"disabledReason,omitempty"`
}
type Helm struct {
	OCIRef           string `json:"ociRef"`
	ChartName        string `json:"chartName"`
	ChartVersion     string `json:"chartVersion"`
	ManifestDigest   string `json:"manifestDigest"`
	PackageDigest    string `json:"packageDigest"`
	PackageSizeBytes int64  `json:"packageSizeBytes"`
	TargetNamespace  string `json:"targetNamespace"`
	ReleaseName      string `json:"releaseName"`
	ValuesSource     string `json:"valuesSource"`
}
type Entry struct {
	EntryID              string  `json:"entryId"`
	Transport            string  `json:"transport"`
	Quality              string  `json:"quality"`
	Availability         string  `json:"availability"`
	ReasonCode           *string `json:"reasonCode"`
	RequiresServiceReady bool    `json:"requiresServiceReady"`
	MinimumHostVersion   *string `json:"minimumHostVersion"`
}
type Action struct {
	ID           string  `json:"id"`
	Kind         string  `json:"kind"`
	Enabled      bool    `json:"enabled"`
	ReasonCode   *string `json:"reasonCode"`
	EntryID      *string `json:"entryId"`
	Confirmation string  `json:"confirmation"`
}
type Card struct {
	CardKey             string        `json:"cardKey"`
	AppID               string        `json:"appId"`
	Origin              string        `json:"origin"`
	Display             Display       `json:"display"`
	InstalledVersion    *string       `json:"installedVersion"`
	CandidateVersion    *string       `json:"candidateVersion"`
	Installation        *Installation `json:"installation"`
	InstallationQuality string        `json:"installationQuality"`
	Service             Service       `json:"service"`
	Operation           Operation     `json:"operation"`
	Entry               *Entry        `json:"entry"`
	BindingRevision     string        `json:"bindingRevision"`
	StatusCode          string        `json:"statusCode"`
	PrimaryActionID     string        `json:"primaryActionId"`
	Actions             []Action      `json:"actions"`
	Preinstalled        bool          `json:"preinstalled"`
	Helm                *Helm         `json:"helm,omitempty"`
}
type Group struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Order int    `json:"order"`
}
type View struct {
	SchemaVersion int       `json:"schemaVersion"`
	RequestID     string    `json:"requestId"`
	Scope         Scope     `json:"scope"`
	Revision      string    `json:"revision"`
	GeneratedAt   time.Time `json:"generatedAt"`
	ValidUntil    time.Time `json:"validUntil"`
	Partial       bool      `json:"partial"`
	Sources       []Source  `json:"sources"`
	Groups        []Group   `json:"groups"`
	Items         []Card    `json:"items"`
}
type OpenRequest struct {
	SchemaVersion           int    `json:"schemaVersion"`
	CardKey                 string `json:"cardKey"`
	EntryID                 string `json:"entryId"`
	ExpectedViewRevision    string `json:"expectedViewRevision"`
	ExpectedBindingRevision string `json:"expectedBindingRevision"`
}
