package main

import (
	"errors"
	"net"
	"net/http"
)

func validateReadListener(business, peer string, tls bool) error {
	_, port, err := net.SplitHostPort(peer)
	_, original, originalErr := net.SplitHostPort(business)
	if err != nil || originalErr != nil || port == "" || port == original || (!tls && nonLoopbackListen(peer)) {
		return errors.New("separate authenticated read listener required")
	}
	return nil
}

// Project peers can check live write permission through Open.RequireWrite,
// but this listener exposes neither MCP nor any mutation/authority endpoints.
func projectReadHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawPath != "" || r.URL.RawQuery != "" || r.URL.ForceQuery ||
			!((r.Method == "POST" && r.URL.Path == "/internal/v1/project/open") || (r.Method == "GET" && r.URL.Path == "/health")) {
			http.NotFound(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}
