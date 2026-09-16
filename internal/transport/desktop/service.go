package desktop

import (
	"context"
	"github.com/verdantflarehub/verdantflare-studio/internal/application"
	"sync"
)

// Service keeps the Station credential in Go memory, never in the WebView.
// Remember-login and OS credential storage are intentionally not implemented yet.
type Service struct {
	mu      sync.Mutex
	station *application.Station
	token   string
}

func New(s *application.Station) *Service { return &Service{station: s} }
func (s *Service) Call(in application.Request) application.Result {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := s.station.Call(context.Background(), s.token, in)
	if in.Path == "login" && out.Status == 201 {
		s.token = out.Token
	}
	if in.Path == "logout" || out.Status == 401 {
		s.token = ""
	}
	out.Token = ""
	return out
}
