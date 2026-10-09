package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/verdantflarehub/verdantflare-studio/frontend"
	"github.com/verdantflarehub/verdantflare-studio/internal/application"
	"github.com/verdantflarehub/verdantflare-studio/internal/mcp"
	"github.com/verdantflarehub/verdantflare-studio/internal/transport/web"
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

	handler, webServer := web.NewServer(s, env("STUDIO_PUBLIC_ORIGIN", "http://127.0.0.1:8000"), frontend.Assets(), web.VideoConfig{URL: os.Getenv("STUDIO_VIDEO_URL"), Token: os.Getenv("STUDIO_VIDEO_TOKEN")})

	webServer.EnableImage(handler, web.ImageConfig{
		URL:   env("STUDIO_IMAGE_URL", "http://image-mcp-server.verdantflare-image.svc.cluster.local:8000"),
		Token: os.Getenv("STUDIO_IMAGE_TOKEN"),
	})

	etcdRaw := env("ETCD_ENDPOINTS", "http://etcd.verdantflare-station.svc.cluster.local:2379")
	var etcdEndpoints []string
	for _, ep := range strings.Split(etcdRaw, ",") {
		if trimmed := strings.TrimSpace(ep); trimmed != "" {
			etcdEndpoints = append(etcdEndpoints, trimmed)
		}
	}

	if len(etcdEndpoints) > 0 {
		gw, err := mcp.NewGateway(etcdEndpoints)
		if err != nil {
			log.Printf("[MCP Gateway Warning] failed to connect to etcd (%v): %v", etcdEndpoints, err)
		} else {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			defer gw.Close()
			// Mount the empty gateway before serving. A registry startup race must
			// recover without restarting Studio or replacing the handler pointer.
			webServer.SetMCPGateway(gw)
			go func() {
				for ctx.Err() == nil {
					if err := gw.StartDiscovery(ctx); err == nil {
						log.Printf("[MCP Gateway] Registry discovery started")
						return
					}
					log.Printf("[MCP Gateway Warning] Registry unavailable; retrying discovery")
					select {
					case <-ctx.Done():
						return
					case <-time.After(time.Second):
					}
				}
			}()
			if cli := gw.Client(); cli != nil {
				log.Printf("[Session] Connected to etcd for persistent sessions")
				webServer.SetEtcdClient(cli)
			}
		}
	}

	server := &http.Server{
		Addr:              env("STUDIO_LISTEN", "127.0.0.1:8000"),
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       20 * time.Second,
		WriteTimeout:      120 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	if addr := os.Getenv("STUDIO_BLENDER_CONTENT_LISTEN"); addr != "" {
		content := &http.Server{Addr: addr, Handler: webServer.BlenderContentHandler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 5 * time.Minute, WriteTimeout: 5 * time.Minute, IdleTimeout: 30 * time.Second}
		go func() { log.Fatal(content.ListenAndServe()) }()
	}
	log.Fatal(server.ListenAndServe())
}
