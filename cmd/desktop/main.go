package main

import (
	"github.com/verdantflarehub/verdantflare-studio/frontend"
	core "github.com/verdantflarehub/verdantflare-studio/internal/application"
	"github.com/verdantflarehub/verdantflare-studio/internal/transport/desktop"
	"github.com/wailsapp/wails/v3/pkg/application"
	"log"
	"os"
)

func main() {
	endpoint := os.Getenv("STATION_CORE_URL")
	if endpoint == "" {
		endpoint = "http://127.0.0.1:5050"
	}
	station, err := core.NewStation(endpoint)
	if err != nil {
		log.Fatal(err)
	}
	app := application.New(application.Options{Name: "VerdantFlare Studio", Services: []application.Service{application.NewService(desktop.New(station))}, Assets: application.AssetOptions{Handler: application.AssetFileServerFS(frontend.Assets())}})
	app.Window.NewWithOptions(application.WebviewWindowOptions{Title: "VerdantFlare Studio", URL: "/?host=desktop", Width: 1440, Height: 960})
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
