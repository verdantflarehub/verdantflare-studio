package project

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"mime"
	"net/http"
	"strings"

	"github.com/verdantflarehub/verdantflare-studio/internal/strictjson"
)

// GenerationControlHandler is intentionally NOT in MCPTools. Approval uses a
// distinct secret unavailable to callers of the ordinary Project service.
// Empty approval configuration disables approval without disabling recovery.
func (s *Service) GenerationControlHandler(dispatchToken, approvalToken string) (http.Handler, error) {
	valid := func(t string) bool { return len(t) >= 32 && !strings.ContainsAny(t, " \t\r\n") }
	if !valid(dispatchToken) || (approvalToken != "" && (!valid(approvalToken) || approvalToken == dispatchToken)) {
		return nil, ErrInvalid
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		p := Principal{OrganizationID: r.Header.Get("X-Organization-Id"), SubjectID: r.Header.Get("X-User-Id"), RequestID: r.Header.Get("X-Request-Id")}
		for _, k := range []string{"Authorization", "X-Organization-Id", "X-User-Id", "X-Request-Id"} {
			if len(r.Header.Values(k)) != 1 {
				httpError(w, p, ErrForbidden)
				return
			}
		}
		token := dispatchToken
		if r.URL.Path == "/internal/v1/generation/approve" {
			token = approvalToken
		}
		actual, want := sha256.Sum256([]byte(r.Header.Get("Authorization"))), sha256.Sum256([]byte("Bearer "+token))
		if token == "" || !p.Valid() || subtle.ConstantTimeCompare(actual[:], want[:]) != 1 {
			httpError(w, p, ErrForbidden)
			return
		}
		mt, _, e := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if e != nil || mt != "application/json" || r.Method != "POST" || r.URL.RawQuery != "" || r.Header.Get("Content-Encoding") != "" {
			httpError(w, p, ErrInvalid)
			return
		}
		decode := func(v any) bool {
			if strictjson.Decode(r.Body, 1<<20, v) != nil {
				httpError(w, p, ErrInvalid)
				return false
			}
			return true
		}
		var out any
		switch r.URL.Path {
		case "/internal/v1/generation/policy":
			var v GenerationPolicyRequest
			if !decode(&v) {
				return
			}
			out, e = s.GenerationPolicy(r.Context(), p, v.ProjectID)
		case "/internal/v1/generation/status":
			var v GenerationPlanKey
			if !decode(&v) {
				return
			}
			out, e = s.GenerationStatus(r.Context(), p, v)
		case "/internal/v1/generation/approve":
			var v GenerationDecision
			if !decode(&v) {
				return
			}
			out, e = s.GenerationApprove(r.Context(), p, v)
		case "/internal/v1/generation/reserve":
			var v GenerationOperationKey
			if !decode(&v) {
				return
			}
			out, e = s.GenerationReserve(r.Context(), p, v)
		case "/internal/v1/generation/transition":
			var v GenerationTransition
			if !decode(&v) {
				return
			}
			out, e = s.GenerationTransition(r.Context(), p, v)
		default:
			e = ErrNotFound
		}
		if e != nil {
			httpError(w, p, e)
			return
		}
		w.Header().Set("X-Request-Id", p.RequestID)
		_ = json.NewEncoder(w).Encode(out)
	}), nil
}
