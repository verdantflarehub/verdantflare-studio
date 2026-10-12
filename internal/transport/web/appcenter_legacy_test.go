package web

import (
	"testing"

	"github.com/verdantflarehub/verdantflare-studio/internal/appcenter"
)

func TestLegacyBackendsTrackConfiguredProxyAvailability(t *testing.T) {
	t.Setenv("STUDIO_BLENDER_SERVICE_TOKEN", "")
	endpoint, available := "http://sample.invalid:8000/internal/mcp", true
	s := &Server{blenderEndpoint: func() (string, bool) { return endpoint, available }}
	if got := s.LegacyBackends(ImageConfig{}, VideoConfig{})(); len(got) != 0 {
		t.Fatal("missing credentials advertised a backend")
	}
	t.Setenv("STUDIO_BLENDER_SERVICE_TOKEN", "test-credential")
	read := s.LegacyBackends(ImageConfig{URL: "http://image.invalid:8000", Token: "test-credential"}, VideoConfig{URL: "https://video.invalid", Token: "test-credential"})
	first := read()
	if len(first) != 3 || first["blender-host-v1"] != appcenter.LegacyBackendRevision("blender-host-v1", endpoint) {
		t.Fatal("configured proxy identity missing")
	}
	endpoint = "http://replacement.invalid:8000/internal/mcp"
	if read()["blender-host-v1"] == first["blender-host-v1"] {
		t.Fatal("service discovery change reused the old proof")
	}
	available = false
	if read()["blender-host-v1"] != "" {
		t.Fatal("unavailable service advertised a backend")
	}
	bad := s.LegacyBackends(ImageConfig{URL: "http://image.invalid?target=other", Token: "test-credential"}, VideoConfig{URL: "https://user:secret@video.invalid", Token: "test-credential"})()
	if bad["image-iframe-v1"] != "" || bad["video-iframe-v1"] != "" {
		t.Fatal("invalid upstream produced a backend proof")
	}
}
