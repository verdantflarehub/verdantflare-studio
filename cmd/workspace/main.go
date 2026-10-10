package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/verdantflarehub/verdantflare-studio/internal/buildinfo"
	"github.com/verdantflarehub/verdantflare-studio/internal/project"
	"github.com/verdantflarehub/verdantflare-studio/internal/strictjson"
	"github.com/verdantflarehub/verdantflare-studio/internal/workspace"
	"github.com/verdantflarehub/verdantflare-studio/internal/workspacehttp"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Stdin, os.Stdout); err != nil {
		// Never print HTTP errors, credentials or upstream response bodies.
		_ = json.NewEncoder(os.Stderr).Encode(map[string]string{"error": err.Error()})
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, stdin io.Reader, stdout io.Writer) error {
	if len(args) == 1 && args[0] == "--version" {
		fmt.Fprintln(stdout, buildinfo.Version)
		return nil
	}
	if len(args) == 0 || args[0] == "--help" || args[0] == "help" {
		fmt.Fprintln(stdout, "studio-workspace tools (discover tools using the same user Bearer; no extra login)")
		fmt.Fprintln(stdout, "studio-workspace call --input request.json\nstudio-workspace open|status|fetch|save-texts|save-files|import|resume|switch-head|conflict|resolve --dir DIR --alias ALIAS --project-id ID [--input FILE] [--file-id ID] [--max-bytes N]\nConnection: STUDIO_MCP_URL, STUDIO_MCP_BEARER_TOKEN (a verified Core user session). JSON input may use '-' for stdin. Requests follow central Project/World contracts.")
		return nil
	}
	command := args[0]
	switch command {
	case "tools", "call", "open", "status", "fetch", "save-texts", "save-files", "import", "resume", "switch-head", "conflict", "resolve":
	default:
		return project.ErrInvalid
	}
	fs := flag.NewFlagSet("studio-workspace", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	directory := fs.String("dir", "", "")
	alias := fs.String("alias", "", "")
	pid := fs.String("project-id", "", "")
	input := fs.String("input", "", "")
	fileID := fs.String("file-id", "", "")
	limit := fs.Int64("max-bytes", 1<<30, "")
	if fs.Parse(args[1:]) != nil || fs.NArg() != 0 {
		return project.ErrInvalid
	}
	readInput := func(dst any) error {
		if *input == "" {
			return project.ErrInvalid
		}
		reader := stdin
		if *input != "-" {
			file, err := os.Open(*input)
			if err != nil {
				return errors.New("INPUT_FILE_UNAVAILABLE")
			}
			defer file.Close()
			reader = file
		}
		if strictjson.Decode(reader, project.MaxManifestBytes, dst) != nil {
			return project.ErrInvalid
		}
		return nil
	}
	remote, err := workspacehttp.New(os.Getenv("STUDIO_MCP_URL"), os.Getenv("STUDIO_MCP_BEARER_TOKEN"))
	if err != nil {
		return errors.New("STUDIO_SESSION_CONFIGURATION_REQUIRED")
	}
	var result any
	if command == "tools" {
		catalog, err := remote.Tools(ctx)
		if err != nil {
			return err
		}
		found := map[string]bool{}
		for _, tool := range catalog {
			found[tool.Name] = true
		}
		if !found["project.create"] || !found["project.list"] || !found["project.open"] {
			return errors.New("STUDIO_PROJECT_TOOLS_UNAVAILABLE: check server-side Bearer identity binding and Project service registration; no additional client login is required")
		}
		result = map[string]any{"tools": catalog}
	} else if command == "call" {
		var req struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err = readInput(&req); err != nil {
			return err
		}
		var raw json.RawMessage
		if err = remote.Call(ctx, req.Name, req.Arguments, &raw); err != nil {
			return err
		}
		result = raw
	} else {
		if *directory == "" || *alias == "" || !project.ValidID(*pid) {
			return project.ErrInvalid
		}
		w, err := workspace.Open(ctx, *directory, *alias, *pid, remote)
		if err != nil {
			return err
		}
		defer w.Close()
		switch command {
		case "status":
			result, err = w.Status(ctx)
		case "fetch":
			if !project.ValidID(*fileID) || *limit <= 0 || *limit > 1<<50 {
				return project.ErrInvalid
			}
			err = w.Fetch(ctx, *fileID, *limit)
			result = map[string]string{"file_id": *fileID, "state": "materialized"}
		case "save-texts":
			var ids []string
			if err = readInput(&ids); err == nil {
				result, err = w.SaveTexts(ctx, ids)
			}
		case "save-files":
			var files []workspace.FileInput
			if err = readInput(&files); err == nil {
				result, err = w.SaveFiles(ctx, files)
			}
		case "import":
			var files []workspace.ImportInput
			if err = readInput(&files); err == nil {
				result, err = w.ImportFiles(ctx, files)
			}
		case "resume":
			result, err = w.Resume(ctx)
		case "conflict":
			result, err = w.PreviewConflict(ctx)
		case "resolve":
			var resolution workspace.ConflictResolution
			if err = readInput(&resolution); err == nil {
				result, err = w.ResolveConflict(ctx, resolution)
			}
		case "open", "switch-head":
			var state workspace.State
			var manifest project.Manifest
			if command == "open" {
				state, manifest, err = w.Snapshot(ctx)
			} else {
				state, manifest, err = w.SwitchToHead(ctx)
			}
			result = struct {
				State    workspace.State  `json:"state"`
				Manifest project.Manifest `json:"manifest"`
			}{state, manifest}
		}
		if err != nil {
			return err
		}
	}
	return json.NewEncoder(stdout).Encode(result)
}
