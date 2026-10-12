package appcenter

// Commands are fixed, credential-free values defined by the central schemas.
type CommandPrepare struct {
	SchemaVersion           int    `json:"schemaVersion"`
	RequestID               string `json:"requestId"`
	IdempotencyKey          string `json:"idempotencyKey"`
	CardKey                 string `json:"cardKey"`
	ActionID                string `json:"actionId"`
	ExpectedBindingRevision string `json:"expectedBindingRevision"`
}
type Command struct {
	RequestID      string `json:"request_id"`
	IdempotencyKey string `json:"idempotency_key"`
	OrganizationID string `json:"organization_id"`
	StationID      string `json:"station_id"`
	AppID          string `json:"app_id"`
	AppVersion     string `json:"app_version"`
	Action         string `json:"action"`
}
type CommandExpected struct {
	InstallationID       *string `json:"installationId"`
	InstallationRevision *int64  `json:"installationRevision"`
	AppVersion           string  `json:"appVersion"`
	ReleaseDigest        *string `json:"releaseDigest"`
	WorkloadUID          *string `json:"workloadUID"`
	PolicyRevision       string  `json:"policyRevision"`
}
type CommandCore struct {
	SchemaVersion int             `json:"schemaVersion"`
	Command       Command         `json:"command"`
	Expected      CommandExpected `json:"expected"`
}
type CommandSubmit struct {
	SchemaVersion int         `json:"schemaVersion"`
	Prepared      CommandCore `json:"prepared"`
}
