package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/verdantflarehub/verdantflare-studio/internal/artifactclient"
	"github.com/verdantflarehub/verdantflare-studio/internal/mcp"
	"github.com/verdantflarehub/verdantflare-studio/internal/project"
	"github.com/verdantflarehub/verdantflare-studio/migrations"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if e := run(ctx, os.Args[1:]); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
func run(ctx context.Context, args []string) error {
	if len(args) != 1 || (args[0] != "migrate" && args[0] != "serve") {
		return errors.New("usage: studio-project migrate|serve")
	}
	dsn := os.Getenv("STUDIO_DATABASE_URL")
	if dsn == "" {
		return errors.New("STUDIO_DATABASE_URL is required")
	}
	startup, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	db, e := pgxpool.New(startup, dsn)
	if e != nil {
		return errors.New("invalid Studio database configuration")
	}
	defer db.Close()
	if e = db.Ping(startup); e != nil {
		return errors.New("Studio database unavailable")
	}
	if args[0] == "migrate" {
		if migrations.Apply(startup, db) != nil {
			return errors.New("Studio migration failed; check database and migration history")
		}
		return nil
	}
	content, e := artifactclient.New(os.Getenv("STUDIO_ARTIFACT_URL"), os.Getenv("STUDIO_ARTIFACT_SERVICE_TOKEN"))
	if e != nil {
		return errors.New("invalid Studio Artifact configuration")
	}
	service, e := project.NewService(startup, db, content, nil)
	if e != nil {
		return errors.New("Studio Project initialization failed; apply explicit migrations first")
	}
	token, authorityToken := os.Getenv("STUDIO_PROJECT_SERVICE_TOKEN"), os.Getenv("STUDIO_ARTIFACT_AUTHORITY_TOKEN")
	if token == os.Getenv("STUDIO_ARTIFACT_SERVICE_TOKEN") || authorityToken == os.Getenv("STUDIO_ARTIFACT_SERVICE_TOKEN") {
		return errors.New("Studio and Artifact service/authority credentials must be distinct")
	}
	handler, e := service.HTTPHandler(token, authorityToken)
	if e != nil {
		return errors.New("invalid Studio Project internal credentials")
	}
	addr := os.Getenv("STUDIO_PROJECT_LISTEN")
	if addr == "" {
		addr = "127.0.0.1:8095"
	}
	listener, e := net.Listen("tcp", addr)
	if e != nil {
		return errors.New("Studio Project listener unavailable")
	}
	defer listener.Close()
	mcpHandler, e := service.MCPHandler(token, authorityToken)
	if e != nil {
		return errors.New("Project MCP initialization failed")
	}
	mux := http.NewServeMux()
	mux.Handle("/mcp", mcpHandler)
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok","service":"studio-project-world"}`))
	})
	mux.Handle("/", handler)
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 10 * time.Minute, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10}
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	defer server.Close()
	if address := os.Getenv("STUDIO_PROJECT_MCP_ADVERTISE_URL"); address != "" {
		u, err := url.Parse(address)
		if err != nil || u.Path != "/mcp" {
			return errors.New("invalid Project MCP advertised address")
		}
		endpoints := []string{}
		for _, ep := range strings.Split(os.Getenv("ETCD_ENDPOINTS"), ",") {
			if strings.TrimSpace(ep) != "" {
				endpoints = append(endpoints, strings.TrimSpace(ep))
			}
		}
		if len(endpoints) == 0 {
			return errors.New("ETCD_ENDPOINTS required for Project MCP registration")
		}
		registry, err := mcp.NewGateway(endpoints)
		if err != nil {
			return err
		}
		defer registry.Close()
		u.Path = "/health"
		version := os.Getenv("STUDIO_PROJECT_SERVICE_VERSION")
		if version == "" {
			version = "0.1.0"
		}
		regs := []mcp.ServiceRegistration{}
		for _, domain := range []string{"project", "world"} {
			reg := mcp.ServiceRegistration{Domain: domain, Endpoint: address, HealthEndpoint: u.String(), Version: version, Tools: []mcp.ToolDefinition{}}
			for _, tool := range project.MCPTools() {
				if strings.HasPrefix(tool.Name, domain+".") {
					reg.Tools = append(reg.Tools, mcp.ToolDefinition{Name: tool.Name, Description: tool.Description, InputSchema: tool.InputSchema})
				}
			}
			regs = append(regs, reg)
		}
		publication, err := mcp.Register(ctx, registry.Client(), regs)
		if err != nil {
			return err
		}
		defer publication.Close()
	}
	select {
	case e := <-done:
		if errors.Is(e, http.ErrServerClosed) {
			return nil
		}
		return errors.New("Studio Project HTTP server failed")
	case <-ctx.Done():
		finish, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if server.Shutdown(finish) != nil {
			_ = server.Close()
			return errors.New("Studio Project shutdown deadline exceeded")
		}
		return nil
	}
}
