package web

import (
	"os"

	"github.com/verdantflarehub/verdantflare-studio/internal/appcenter"
)

// LegacyBackends identifies existing host-owned proxies. No endpoint or token
// is returned to clients, and a missing credential never advertises an opener.
func (s *Server) LegacyBackends(image ImageConfig, video VideoConfig) func() map[string]string {
	return func() map[string]string {
		out := map[string]string{}
		if image.Token != "" {
			out["image-iframe-v1"] = appcenter.LegacyBackendRevision("image-iframe-v1", image.URL)
		}
		if video.Token != "" {
			out["video-iframe-v1"] = appcenter.LegacyBackendRevision("video-iframe-v1", video.URL)
		}
		if os.Getenv("STUDIO_BLENDER_SERVICE_TOKEN") != "" {
			if endpoint, ok := s.blenderOrigin(); ok {
				out["blender-host-v1"] = appcenter.LegacyBackendRevision("blender-host-v1", endpoint.String())
			}
		}
		return out
	}
}
