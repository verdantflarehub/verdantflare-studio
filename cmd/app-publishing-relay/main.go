package main

import (
	"bytes"
	"errors"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const maxBody = 5 << 20
const maxResponse = 12 << 20

var forwardedHeaders = []string{
	"X-App-Publish-User", "X-App-Publish-Organization", "X-App-Publish-Timestamp",
	"X-App-Publish-Nonce", "X-App-Publish-Signature", "X-Request-Id",
}

type relay struct {
	endpoint *url.URL
	client   *http.Client
}

func newRelay(raw string, client *http.Client) (*relay, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "/api/control/internal/studio-publish/mcp" {
		return nil, errors.New("invalid control endpoint")
	}
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	return &relay{endpoint: u, client: client}, nil
}

func (r *relay) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	mux.HandleFunc("POST /mcp", r.forward)
	return mux
}

func (r *relay) forward(w http.ResponseWriter, req *http.Request) {
	if !strings.HasPrefix(req.Header.Get("Content-Type"), "application/json") {
		http.Error(w, "invalid content type", http.StatusUnsupportedMediaType)
		return
	}
	for _, key := range forwardedHeaders[:5] {
		if req.Header.Get(key) == "" {
			http.Error(w, "missing signed identity", http.StatusForbidden)
			return
		}
	}
	if req.ContentLength > maxBody {
		http.Error(w, "request too large", http.StatusRequestEntityTooLarge)
		return
	}
	data, err := io.ReadAll(http.MaxBytesReader(w, req.Body, maxBody))
	if err != nil {
		http.Error(w, "request too large", http.StatusRequestEntityTooLarge)
		return
	}
	upstream, err := http.NewRequestWithContext(req.Context(), http.MethodPost, r.endpoint.String(), bytes.NewReader(data))
	if err != nil {
		http.Error(w, "upstream unavailable", http.StatusBadGateway)
		return
	}
	upstream.Header.Set("Content-Type", "application/json")
	upstream.Header.Set("Accept", "application/json, text/event-stream")
	for _, key := range forwardedHeaders {
		if value := req.Header.Get(key); value != "" {
			upstream.Header.Set(key, value)
		}
	}
	response, err := r.client.Do(upstream)
	if err != nil {
		http.Error(w, "upstream unavailable", http.StatusBadGateway)
		return
	}
	defer response.Body.Close()
	result, err := io.ReadAll(io.LimitReader(response.Body, maxResponse+1))
	if err != nil || len(result) > maxResponse {
		http.Error(w, "invalid upstream response", http.StatusBadGateway)
		return
	}
	if kind := response.Header.Get("Content-Type"); kind != "" {
		w.Header().Set("Content-Type", kind)
	}
	w.WriteHeader(response.StatusCode)
	_, _ = w.Write(result)
}

func main() {
	r, err := newRelay(os.Getenv("APP_PUBLISH_CONTROL_ENDPOINT"), nil)
	if err != nil {
		log.Fatal(err)
	}
	listen := os.Getenv("APP_PUBLISH_LISTEN")
	if listen == "" {
		listen = "0.0.0.0:8090"
	}
	server := &http.Server{
		Addr: listen, Handler: r.handler(), ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout: 20 * time.Second, WriteTimeout: 35 * time.Second, IdleTimeout: 60 * time.Second,
	}
	log.Fatal(server.ListenAndServe())
}
