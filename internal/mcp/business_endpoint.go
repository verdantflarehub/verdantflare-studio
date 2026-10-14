package mcp

import (
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
)

// BusinessEndpoint resolves controlled non-MCP origins. Separate listeners must
// never inherit the port advertised for MCP. Callers still authorize each route.
func (g *Gateway) BusinessEndpoint(domain string) (string, bool) {
	key := ""
	switch domain {
	case "project":
		key = "STUDIO_PROJECT_SERVICE_URL"
	case "world":
		key = "STUDIO_WORLD_SERVICE_URL"
	case "artifact":
		key = "STUDIO_ARTIFACT_URL"
	default:
		return "", false
	}
	switch os.Getenv("STUDIO_CONTENT_SERVICE_MODE") {
	case "split":
		return businessOrigin(os.Getenv(key), true)
	case "", "legacy":
		// Artifact can separate its MCP and binary listeners independently of
		// the Project/World authority migration. Keep their legacy routes intact.
		if domain == "artifact" && os.Getenv(key) != "" {
			return businessOrigin(os.Getenv(key), true)
		}
		endpoint, ok := g.ServiceEndpoint(domain)
		if !ok {
			return "", false
		}
		u, err := url.Parse(endpoint)
		if err != nil || u.Path != "/mcp" || u.RawPath != "" {
			return "", false
		}
		u.Path = ""
		// Preserve rejection of even an empty fragment before URL.String drops it.
		if strings.Contains(endpoint, "#") {
			return "", false
		}
		return businessOrigin(u.String(), false)
	default:
		return "", false
	}
}

func businessOrigin(raw string, secure bool) (string, bool) {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.Hostname() == "" || u.User != nil || u.Opaque != "" || u.RawPath != "" || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.ForceQuery || strings.Contains(raw, "#") || (u.Scheme != "https" && u.Scheme != "http") {
		return "", false
	}
	if strings.HasSuffix(u.Host, ":") {
		return "", false
	}
	if port := u.Port(); port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return "", false
		}
	}
	if secure && u.Scheme == "http" {
		ip := net.ParseIP(u.Hostname())
		if ip == nil || !ip.IsLoopback() {
			return "", false
		}
	}
	u.Path = ""
	return u.String(), true
}
