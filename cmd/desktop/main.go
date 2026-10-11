package main

import (
	"context"
	"github.com/verdantflarehub/verdantflare-studio/frontend"
	core "github.com/verdantflarehub/verdantflare-studio/internal/application"
	"github.com/verdantflarehub/verdantflare-studio/internal/mcp"
	"github.com/verdantflarehub/verdantflare-studio/internal/studioremote"
	"github.com/verdantflarehub/verdantflare-studio/internal/transport/desktop"
	"github.com/wailsapp/wails/v3/pkg/application"
	"log"
	"os"
	"strings"
	"time"
)

func main() {
	if origin := os.Getenv("STUDIO_SERVER_URL"); origin != "" {
		remote, err := studioremote.New(origin)
		if err != nil {
			log.Fatal("invalid Studio server configuration")
		}
		defer remote.Close()
		service := desktop.NewRemote(remote)
		var app *application.App
		bridge := desktop.NewDashboardWorkspaces(service, desktop.WorkspaceDialogFuncs{
			ChooseDirectory: func(ctx context.Context) (string, error) {
				if err := ctx.Err(); err != nil {
					return "", err
				}
				return app.Dialog.OpenFile().CanChooseDirectories(true).CanChooseFiles(false).PromptForSingleSelection()
			},
			ChooseFiles: func(ctx context.Context) ([]string, error) {
				if err := ctx.Err(); err != nil {
					return nil, err
				}
				return app.Dialog.OpenFile().CanChooseDirectories(false).CanChooseFiles(true).PromptForMultipleSelection()
			},
		})
		defer bridge.Close()
		handler := bridge.Wrap(desktop.RemoteAssetHandler(service, application.AssetFileServerFS(frontend.Assets())))
		app = application.New(application.Options{Name: "VerdantFlare Studio", Services: []application.Service{application.NewService(service)}, Assets: application.AssetOptions{Handler: handler}})
		app.Window.NewWithOptions(application.WebviewWindowOptions{Title: "VerdantFlare Studio", URL: "/?host=desktop", Width: 1440, Height: 960})
		if err := app.Run(); err != nil {
			log.Fatal(err)
		}
		return
	}
	endpoint := os.Getenv("STATION_CORE_URL")
	if endpoint == "" {
		endpoint = "http://127.0.0.1:5050"
	}
	station, err := core.NewStation(endpoint)
	if err != nil {
		log.Fatal(err)
	}
	gateway, stopGateway := configureGateway()
	defer stopGateway()
	service := desktop.NewWithGateway(station, gateway)
	app := application.New(application.Options{Name: "VerdantFlare Studio", Services: []application.Service{application.NewService(service)}, Assets: application.AssetOptions{Handler: desktop.RemoteAssetHandler(service, application.AssetFileServerFS(frontend.Assets()))}})
	app.Window.NewWithOptions(application.WebviewWindowOptions{Title: "VerdantFlare Studio", URL: "/?host=desktop", Width: 1440, Height: 960})
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}

// configureGateway is optional for offline development. When ETCD_ENDPOINTS
// is present, the desktop host discovers the same Project/World/Artifact MCP
// services as the web host and retries discovery until the registry is ready.
func configureGateway() (*mcp.Gateway, func()) {
	raw := strings.TrimSpace(os.Getenv("ETCD_ENDPOINTS"))
	if raw == "" {
		return nil, func() {}
	}
	endpoints := []string{}
	for _, endpoint := range strings.Split(raw, ",") {
		if endpoint = strings.TrimSpace(endpoint); endpoint != "" {
			endpoints = append(endpoints, endpoint)
		}
	}
	if len(endpoints) == 0 {
		return nil, func() {}
	}
	gateway, err := mcp.NewGateway(endpoints)
	if err != nil {
		log.Printf("[MCP Gateway Warning] invalid ETCD_ENDPOINTS: %v", err)
		return nil, func() {}
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		for ctx.Err() == nil {
			if err := gateway.StartDiscovery(ctx); err == nil {
				log.Printf("[MCP Gateway] desktop discovery started")
				return
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Second):
			}
		}
	}()
	return gateway, func() {
		cancel()
		_ = gateway.Close()
	}
}
