package web

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"regexp"
	"time"

	"github.com/verdantflarehub/verdantflare-studio/internal/project"
)

var producerRunID = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.:-]{0,255}$`)

type producerCall func(context.Context, string, map[string]any, project.Principal, string) (any, int, error)

// Mounted exclusively on the private content listener. Tool names and read-only
// arguments are fixed here; the request cannot choose a URL or a write operation.
func worldProducerHandler(call producerCall) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		secret := os.Getenv("STUDIO_WORLD_SERVICE_TOKEN")
		got, want := sha256.Sum256([]byte(r.Header.Get("Authorization"))), sha256.Sum256([]byte("Bearer "+secret))
		p := project.Principal{SubjectID: r.Header.Get("X-User-Id"), OrganizationID: r.Header.Get("X-Organization-Id"), RequestID: r.Header.Get("X-Request-Id")}
		for _, h := range []string{"Authorization", "X-User-Id", "X-Organization-Id", "X-Request-Id"} {
			if len(r.Header.Values(h)) != 1 {
				http.Error(w, "Authentication required", 401)
				return
			}
		}
		if secret == "" || subtle.ConstantTimeCompare(got[:], want[:]) != 1 || !p.Valid() {
			http.Error(w, "Authentication required", 401)
			return
		}
		if r.Method != "POST" || r.URL.EscapedPath() != "/world/producer-result" || r.URL.RawQuery != "" || r.Header.Get("Content-Encoding") != "" {
			http.Error(w, "Route denied", 404)
			return
		}
		var in struct {
			ServiceID string `json:"service_id"`
			RunID     string `json:"run_id"`
			ProjectID string `json:"project_id"`
		}
		d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
		d.DisallowUnknownFields()
		if d.Decode(&in) != nil || d.Decode(new(any)) != io.EOF || !project.ValidID(in.ProjectID) || !producerRunID.MatchString(in.RunID) || (in.ServiceID != "image" && in.ServiceID != "video") {
			http.Error(w, "Invalid request", 400)
			return
		}
		if call == nil {
			http.Error(w, "Service unavailable", 503)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		result, status, err := call(ctx, "project.open", map[string]any{"project_id": in.ProjectID}, p, "")
		if err != nil || status != 200 {
			http.Error(w, "Project unavailable", 403)
			return
		}
		opened, err := producerStructured(result)
		if err != nil || opened["project_id"] != in.ProjectID {
			http.Error(w, "Project unavailable", 403)
			return
		}
		result, status, err = call(ctx, in.ServiceID+".result", map[string]any{"task_id": in.RunID, "execution_proof_only": true}, p, in.ProjectID)
		if err != nil || status != 200 {
			http.Error(w, "Producer unavailable", 503)
			return
		}
		obj, err := producerStructured(result)
		if err != nil {
			http.Error(w, "Proof unavailable", 503)
			return
		}
		proof, ok := obj["execution_proof"].(map[string]any)
		if !ok || proof["project_id"] != in.ProjectID || proof["run_id"] != in.RunID || proof["service_id"] != in.ServiceID {
			http.Error(w, "Proof unavailable", 503)
			return
		}
		// Whitelist even a producer response; never proxy its metadata or URLs.
		fields := []string{"schema_version", "service_id", "run_id", "project_id", "state", "model_id", "route", "prompt_sha256", "parameters", "inputs", "outputs"}
		if len(proof) != len(fields) {
			http.Error(w, "Proof unavailable", 503)
			return
		}
		for _, k := range fields {
			if _, ok := proof[k]; !ok {
				http.Error(w, "Proof unavailable", 503)
				return
			}
		}
		data, err := json.Marshal(proof)
		if err != nil || len(data) > 65536 {
			http.Error(w, "Proof unavailable", 503)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(data)
	})
}
func producerStructured(v any) (map[string]any, error) {
	raw, err := json.Marshal(v)
	if err != nil || len(raw) > 131072 {
		return nil, errors.New("invalid response")
	}
	var envelope map[string]any
	if json.Unmarshal(raw, &envelope) != nil || envelope["isError"] == true {
		return nil, errors.New("invalid response")
	}
	content, ok := envelope["structuredContent"].(map[string]any)
	if !ok {
		return nil, errors.New("missing structured content")
	}
	return content, nil
}
