package application

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/verdantflarehub/verdantflare-studio/internal/dashboard"
	"github.com/verdantflarehub/verdantflare-studio/internal/project"
	"github.com/verdantflarehub/verdantflare-studio/internal/strictjson"
)

// Dashboards is the server-side boundary. Desktop transports must use the
// controlled Studio service, never connect directly to internal App origins.
type Dashboards struct {
	station    *Station
	source     *dashboard.RegisteredSource
	resolver   *dashboard.Resolver
	workspaces *dashboard.Workspaces
	content    dashboard.ContentHandles
}

type DashboardContext struct {
	UserID          string  `json:"userId"`
	OrganizationID  string  `json:"organizationId"`
	StationID       string  `json:"stationId"`
	ProjectID       *string `json:"projectId"`
	AppID           string  `json:"appId"`
	InstallationID  string  `json:"installationId"`
	ContextRevision string  `json:"contextRevision"`
}

type DashboardOpened struct {
	Launch  dashboard.Launch `json:"launch"`
	Context DashboardContext `json:"context"`
}

func NewDashboards(station *Station, registryPath string) *Dashboards {
	d := &Dashboards{station: station}
	d.source = dashboard.NewRegisteredSource(registryPath, d.readInstallation)
	d.resolver = dashboard.New(d.source)
	d.workspaces = dashboard.NewWorkspaces(d.source)
	return d
}

func (d *Dashboards) Shutdown() { d.source.Close() }

// Request credentials live only in an unexported, service-bound context value.
// Actor is deliberately insufficient to impersonate an authenticated request.
type dashboardCredentialsKey struct{}
type dashboardCredentials struct {
	service          *Dashboards
	token, requestID string
	actor            dashboard.Actor
	canManage        bool
}

func (d *Dashboards) begin(ctx context.Context, token string) (context.Context, dashboard.Actor, Result) {
	id := uuid.Must(uuid.NewV7()).String()
	if d.station == nil {
		return ctx, dashboard.Actor{}, Error(503, "DASHBOARD_UNAVAILABLE")
	}
	identity, status := d.station.verifiedIdentity(ctx, token, id)
	if status != 200 {
		code := "DASHBOARD_UNAVAILABLE"
		if status == 401 {
			code = "UNAUTHENTICATED"
		} else if status == 403 {
			code = "DASHBOARD_DENIED"
		}
		return ctx, dashboard.Actor{}, Error(status, code)
	}
	if !slices.Contains(identity.Roles, "admin") || !slices.Contains(identity.Scopes, "app:read") {
		return ctx, dashboard.Actor{}, Error(403, "DASHBOARD_DENIED")
	}
	actor := dashboardActorFromIdentity(identity)
	credentials := dashboardCredentials{service: d, token: token, requestID: id, actor: actor, canManage: slices.Contains(identity.Scopes, "app:manage")}
	return context.WithValue(ctx, dashboardCredentialsKey{}, credentials), actor, Result{Status: 200, RequestID: id}
}

type dashboardInstallation struct {
	InstallationID  string    `json:"installation_id"`
	AppID           string    `json:"app_id"`
	AppVersion      string    `json:"app_version"`
	StationID       string    `json:"station_id"`
	OrganizationID  string    `json:"organization_id"`
	State           string    `json:"state"`
	Revision        int64     `json:"revision"`
	LastOperationID string    `json:"last_operation_id"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

var dashboardAppID = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._:-]{0,127}$`)
var dashboardAppVersion = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)

func (d *Dashboards) readInstallation(ctx context.Context, actor dashboard.Actor, id string) (dashboard.InstalledRelease, error) {
	credentials, ok := ctx.Value(dashboardCredentialsKey{}).(dashboardCredentials)
	if !ok || credentials.service != d || credentials.actor != actor || !installationPath.MatchString("installations/"+id) {
		return dashboard.InstalledRelease{}, dashboard.ErrDenied
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, d.station.base+"/app-installations/"+id, nil)
	if err != nil {
		return dashboard.InstalledRelease{}, dashboard.ErrUnavailable
	}
	req.Header.Set("Authorization", "Bearer "+credentials.token)
	req.Header.Set("X-Request-ID", credentials.requestID)
	req.Header.Set("Accept", "application/json")
	resp, err := d.station.client.Do(req)
	if err != nil {
		return dashboard.InstalledRelease{}, dashboard.ErrUnavailable
	}
	defer resp.Body.Close()
	if resp.StatusCode == 401 || resp.StatusCode == 403 || resp.StatusCode == 404 {
		return dashboard.InstalledRelease{}, dashboard.ErrDenied
	}
	if resp.StatusCode != 200 {
		return dashboard.InstalledRelease{}, dashboard.ErrUnavailable
	}
	var payload struct {
		RequestID    string                `json:"request_id"`
		Installation dashboardInstallation `json:"installation"`
	}
	if strictjson.Decode(resp.Body, 64<<10, &payload) != nil {
		return dashboard.InstalledRelease{}, dashboard.ErrInvalid
	}
	i := payload.Installation
	if payload.RequestID != credentials.requestID || i.InstallationID != id || i.StationID != actor.StationID || i.OrganizationID != actor.OrganizationID {
		return dashboard.InstalledRelease{}, dashboard.ErrDenied
	}
	if !validDashboardInstallation(i) {
		return dashboard.InstalledRelease{}, dashboard.ErrInvalid
	}
	return dashboard.InstalledRelease{InstallationID: id, AppID: i.AppID, AppVersion: i.AppVersion, StationID: i.StationID,
		OrganizationID: i.OrganizationID, State: i.State, Revision: i.Revision, CanManage: credentials.canManage}, nil
}

func dashboardError(err error, requestID string) Result {
	status, code := 503, "DASHBOARD_UNAVAILABLE"
	switch {
	case errors.Is(err, dashboard.ErrDenied):
		status, code = 403, "DASHBOARD_DENIED"
	case errors.Is(err, dashboard.ErrExpired):
		status, code = 410, "DASHBOARD_SESSION_EXPIRED"
	case errors.Is(err, dashboard.ErrInvalid):
		status, code = 502, "INVALID_DASHBOARD_BINDING"
	case errors.Is(err, dashboard.ErrUnsupported):
		status, code = 409, "DASHBOARD_UNSUPPORTED"
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		status, code = 408, "DASHBOARD_REQUEST_CANCELED"
	}
	result := Error(status, code)
	result.RequestID = requestID
	return result
}

func (d *Dashboards) Open(ctx context.Context, token, installationID string) Result {
	ctx, actor, result := d.begin(ctx, token)
	if result.Status != 200 {
		return result
	}
	launch, err := d.resolver.Open(ctx, actor, installationID)
	if err != nil {
		return dashboardError(err, result.RequestID)
	}
	result.Data = dashboardOpened(actor, launch)
	return result
}

func dashboardOpened(actor dashboard.Actor, launch dashboard.Launch) json.RawMessage {
	data, _ := json.Marshal(DashboardOpened{Launch: launch, Context: DashboardContext{
		UserID: actor.UserID, OrganizationID: actor.OrganizationID, StationID: actor.StationID,
		AppID: launch.AppID, InstallationID: launch.InstallationID, ContextRevision: actor.AuthorizationEpoch,
	}})
	return data
}

func (d *Dashboards) Inspect(ctx context.Context, token, sessionID string) Result {
	ctx, actor, result := d.begin(ctx, token)
	if result.Status != 200 {
		return result
	}
	launch, err := d.resolver.Inspect(ctx, actor, sessionID)
	if err != nil {
		return dashboardError(err, result.RequestID)
	}
	result.Data = dashboardOpened(actor, launch)
	return result
}

type DashboardInstallation struct {
	InstallationID string `json:"installationId"`
	AppID          string `json:"appId"`
	AppVersion     string `json:"appVersion"`
}

func (d *Dashboards) Installations(ctx context.Context, token string) Result {
	ctx, actor, result := d.begin(ctx, token)
	if result.Status != 200 {
		return result
	}
	versions, err := d.source.Versions()
	if err != nil {
		return dashboardError(err, result.RequestID)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, d.station.base+"/app-installations", nil)
	if err != nil {
		return dashboardError(dashboard.ErrUnavailable, result.RequestID)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Request-ID", result.RequestID)
	resp, err := d.station.client.Do(req)
	if err != nil {
		return dashboardError(dashboard.ErrUnavailable, result.RequestID)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		if resp.StatusCode == 401 || resp.StatusCode == 403 {
			return dashboardError(dashboard.ErrDenied, result.RequestID)
		}
		return dashboardError(dashboard.ErrUnavailable, result.RequestID)
	}
	var payload struct {
		RequestID string                  `json:"request_id"`
		StationID string                  `json:"station_id"`
		Items     []dashboardInstallation `json:"items"`
	}
	if strictjson.Decode(resp.Body, 4<<20, &payload) != nil || payload.RequestID != result.RequestID || payload.StationID != actor.StationID || payload.Items == nil || len(payload.Items) > 1000 {
		return dashboardError(dashboard.ErrInvalid, result.RequestID)
	}
	items := []DashboardInstallation{}
	seen := map[string]bool{}
	for _, i := range payload.Items {
		if !installationPath.MatchString("installations/"+i.InstallationID) || seen[i.InstallationID] || i.StationID != actor.StationID || i.OrganizationID != actor.OrganizationID ||
			!validDashboardInstallation(i) {
			return dashboardError(dashboard.ErrInvalid, result.RequestID)
		}
		seen[i.InstallationID] = true
		if i.State == "installed" && versions[i.AppID+"@"+i.AppVersion] {
			items = append(items, DashboardInstallation{i.InstallationID, i.AppID, i.AppVersion})
		}
	}
	result.Data, _ = json.Marshal(struct {
		Items []DashboardInstallation `json:"items"`
	}{items})
	return result
}

func validDashboardInstallation(i dashboardInstallation) bool {
	return project.ValidID(i.LastOperationID) && dashboardAppID.MatchString(i.AppID) && dashboardAppVersion.MatchString(i.AppVersion) && i.Revision >= 1 &&
		slices.Contains([]string{"pending", "installed", "failed", "removed"}, i.State) && !i.CreatedAt.IsZero() && !i.UpdatedAt.Before(i.CreatedAt)
}

func (d *Dashboards) Asset(ctx context.Context, token, sessionID, path string) ([]byte, string, Result) {
	ctx, actor, result := d.begin(ctx, token)
	if result.Status != 200 {
		return nil, "", result
	}
	binding, err := d.resolver.Check(ctx, actor, sessionID)
	if err != nil {
		return nil, "", dashboardError(err, result.RequestID)
	}
	data, mime, err := d.source.Asset(ctx, actor, binding, path)
	if err != nil {
		if errors.Is(err, dashboard.ErrInvalid) || errors.Is(err, dashboard.ErrDenied) {
			_ = d.resolver.Close(actor, sessionID)
		}
		return nil, "", dashboardError(err, result.RequestID)
	}
	return data, mime, result
}

func (d *Dashboards) Close(ctx context.Context, token, sessionID string) Result {
	ctx, actor, result := d.begin(ctx, token)
	if result.Status != 200 {
		return result
	}
	if err := ctx.Err(); err != nil {
		return dashboardError(err, result.RequestID)
	}
	if err := d.resolver.Close(actor, sessionID); err != nil {
		return dashboardError(err, result.RequestID)
	}
	d.content.Close(actor, sessionID)
	result.Status = 204
	result.Data = json.RawMessage(`{}`)
	return result
}

func (d *Dashboards) Request(ctx context.Context, token, sessionID string, in dashboard.AppRequest) Result {
	ctx, actor, result := d.begin(ctx, token)
	if result.Status != 200 {
		return result
	}
	if !dashboard.ValidRequest(in) {
		return Error(400, "INVALID_DASHBOARD_REQUEST")
	}
	binding, err := d.resolver.AuthorizeRequest(ctx, actor, sessionID, in.Method, in.Path, len(in.Body))
	if err != nil {
		return dashboardError(err, result.RequestID)
	}
	data, status, err := d.source.Request(ctx, actor, binding, in, result.RequestID)
	if err != nil {
		return dashboardError(err, result.RequestID)
	}
	result.Status = status
	result.Data = data
	return result
}
