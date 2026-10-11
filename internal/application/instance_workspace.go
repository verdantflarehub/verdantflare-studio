package application

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/verdantflarehub/verdantflare-studio/internal/dashboard"
)

func (d *Dashboards) PublicInstanceAsset(ctx context.Context, artifact, path string) ([]byte, string, Result) {
	data, mime, err := d.source.PublicWorkspaceAsset(ctx, artifact, path)
	if err != nil {
		return nil, "", dashboardError(err, "")
	}
	return data, mime, Result{Status: 200}
}

func (d *Dashboards) OpenInstance(ctx context.Context, token, sessionID string, input dashboard.InstanceOpen) Result {
	ctx, actor, result := d.begin(ctx, token)
	if result.Status != 200 {
		return result
	}
	if !dashboard.ValidInstanceOpen(input) {
		return Error(400, "INVALID_INSTANCE_ENTRY")
	}
	binding, err := d.resolver.Check(ctx, actor, sessionID)
	if err != nil {
		return dashboardError(err, result.RequestID)
	}
	launch, err := d.workspaces.Open(ctx, actor, binding.InstallationID, input)
	if err != nil {
		return dashboardError(err, result.RequestID)
	}
	// Closing the opener while resolution is in flight cancels this unfinished
	// open. After publication, Dashboard and workspace lifetimes are independent.
	if _, err = d.resolver.Check(ctx, actor, sessionID); err != nil {
		_ = d.workspaces.Close(actor, launch.WorkspaceSessionID)
		return dashboardError(err, result.RequestID)
	}
	result.Data, _ = json.Marshal(launch)
	return result
}

func (d *Dashboards) InspectWorkspace(ctx context.Context, token, id string) Result {
	ctx, actor, result := d.begin(ctx, token)
	if result.Status != 200 {
		return result
	}
	launch, _, _, err := d.workspaces.Check(ctx, actor, id)
	if err != nil {
		return dashboardError(err, result.RequestID)
	}
	result.Data, _ = json.Marshal(launch)
	return result
}

func (d *Dashboards) WorkspaceAsset(ctx context.Context, token, id, path string) ([]byte, string, Result) {
	ctx, actor, result := d.begin(ctx, token)
	if result.Status != 200 {
		return nil, "", result
	}
	_, binding, entry, err := d.workspaces.Check(ctx, actor, id)
	if err != nil {
		return nil, "", dashboardError(err, result.RequestID)
	}
	data, mime, err := d.source.WorkspaceAsset(ctx, actor, binding, entry, path)
	if err != nil {
		if errors.Is(err, dashboard.ErrDenied) || errors.Is(err, dashboard.ErrInvalid) {
			_ = d.workspaces.Close(actor, id)
		}
		return nil, "", dashboardError(err, result.RequestID)
	}
	return data, mime, result
}

func (d *Dashboards) WorkspaceRequest(ctx context.Context, token, id string, input dashboard.AppRequest) Result {
	ctx, actor, result := d.begin(ctx, token)
	if result.Status != 200 {
		return result
	}
	if !dashboard.ValidRequest(input) {
		return Error(400, "INVALID_WORKSPACE_REQUEST")
	}
	launch, binding, entry, err := d.workspaces.Check(ctx, actor, id)
	if err != nil {
		return dashboardError(err, result.RequestID)
	}
	data, status, err := d.source.WorkspaceRequest(ctx, actor, binding, entry, launch.InstanceID, input, result.RequestID)
	if err != nil {
		return dashboardError(err, result.RequestID)
	}
	result.Data, result.Status = data, status
	return result
}

func (d *Dashboards) CloseWorkspace(ctx context.Context, token, id string) Result {
	ctx, actor, result := d.begin(ctx, token)
	if result.Status != 200 {
		return result
	}
	if err := ctx.Err(); err != nil {
		return dashboardError(err, result.RequestID)
	}
	if err := d.workspaces.Close(actor, id); err != nil {
		return dashboardError(err, result.RequestID)
	}
	result.Status, result.Data = 204, json.RawMessage(`{}`)
	return result
}
