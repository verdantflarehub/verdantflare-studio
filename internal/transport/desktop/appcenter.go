package desktop

import (
	"context"
	"encoding/json"
	"time"

	"github.com/verdantflarehub/verdantflare-studio/internal/appcenter"
	"github.com/verdantflarehub/verdantflare-studio/internal/application"
)

// AppCenterView always uses the authenticated Studio gateway, including Wails
// development mode. Desktop never synthesizes an empty local catalog.
func (s *Service) AppCenterView() application.Result {
	if s.remote == nil {
		return application.Error(503, "STUDIO_REMOTE_REQUIRED")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return s.remoteCall(ctx, "/studio/appcenter/v1/view", "GET", nil)
}

func (s *Service) AppCenterOpen(input appcenter.OpenRequest) application.Result {
	if s.remote == nil {
		return application.Error(503, "STUDIO_REMOTE_REQUIRED")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	body, err := json.Marshal(input)
	if err != nil {
		return application.Error(400, "INVALID_APPCENTER_REQUEST")
	}
	return s.remoteCall(ctx, "/studio/appcenter/v1/open", "POST", body)
}
