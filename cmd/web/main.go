package main

import (
	"github.com/verdantflarehub/verdantflare-studio/frontend"
	"github.com/verdantflarehub/verdantflare-studio/internal/application"
	"github.com/verdantflarehub/verdantflare-studio/internal/transport/web"
	"log"
	"net/http"
	"os"
	"time"
)

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
func main() {
	s, err := application.NewStation(env("STATION_CORE_URL", "http://127.0.0.1:5050"))
	if err != nil {
		log.Fatal(err)
	}
	server := &http.Server{Addr: env("STUDIO_LISTEN", "127.0.0.1:8000"), Handler: web.New(s, env("STUDIO_PUBLIC_ORIGIN", "http://127.0.0.1:8000"), frontend.Assets(), web.VideoConfig{URL: os.Getenv("STUDIO_VIDEO_URL"), Token: os.Getenv("STUDIO_VIDEO_TOKEN")}), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 20 * time.Second, WriteTimeout: 120 * time.Second, IdleTimeout: 60 * time.Second}
	log.Fatal(server.ListenAndServe())
}
