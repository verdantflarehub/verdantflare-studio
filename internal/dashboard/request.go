package dashboard

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/verdantflarehub/verdantflare-studio/internal/strictjson"
)

type AppRequest struct {
	Method string          `json:"method"`
	Path   string          `json:"path"`
	Body   json.RawMessage `json:"body,omitempty"`
}

// DecodeRequest closes the host envelope while preserving App-owned JSON bytes.
func DecodeRequest(reader io.Reader) (AppRequest, error) {
	data, err := strictjson.ReadValue(reader, (1<<20)+4096)
	if err != nil {
		return AppRequest{}, ErrInvalid
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(data, &fields) != nil || len(fields) < 2 || len(fields) > 3 {
		return AppRequest{}, ErrInvalid
	}
	for key := range fields {
		if key != "method" && key != "path" && key != "body" {
			return AppRequest{}, ErrInvalid
		}
	}
	var request AppRequest
	if json.Unmarshal(fields["method"], &request.Method) != nil || json.Unmarshal(fields["path"], &request.Path) != nil {
		return AppRequest{}, ErrInvalid
	}
	request.Body = fields["body"]
	if !ValidRequest(request) {
		return AppRequest{}, ErrInvalid
	}
	return request, nil
}

func ValidRequest(in AppRequest) bool {
	if !validPath(in.Path) || !slices.Contains([]string{"GET", "POST", "PUT", "PATCH", "DELETE"}, in.Method) || len(in.Body) > 1<<20 || (in.Method == "GET" && len(in.Body) != 0) {
		return false
	}
	if len(in.Body) > 0 {
		if _, err := strictjson.ReadValue(bytes.NewReader(in.Body), 1<<20); err != nil {
			return false
		}
	}
	return true
}

func ruleTarget(rule Rule, method, path string, bodySize int) (string, bool) {
	if method != rule.Method || bodySize < 0 || bodySize > rule.MaxBody || !validPath(path) {
		return "", false
	}
	pattern, parts := strings.Split(rule.Path, "/"), strings.Split(path, "/")
	if len(pattern) != len(parts) {
		return "", false
	}
	parameters := map[string]string{}
	for i, part := range pattern {
		if strings.HasPrefix(part, ":") {
			if !identifier.MatchString(parts[i]) {
				return "", false
			}
			parameters[part] = parts[i]
		} else if part != parts[i] {
			return "", false
		}
	}
	target := strings.Split(rule.UpstreamPath, "/")
	for i, part := range target {
		if strings.HasPrefix(part, ":") {
			value, ok := parameters[part]
			if !ok {
				return "", false
			}
			target[i] = value
		}
	}
	path = strings.Join(target, "/")
	return path, validPath(path)
}

var appErrorCode = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,63}$`)

// Request requires the current Binding returned by Resolver.AuthorizeRequest.
// It sends at most once. Transport cancellation cannot undo an accepted write.
func (s *RegisteredSource) Request(ctx context.Context, actor Actor, binding Binding, in AppRequest, requestID string) ([]byte, int, error) {
	if !ValidRequest(in) || binding.Backend == nil || !identifier.MatchString(requestID) {
		return nil, 0, ErrDenied
	}
	grants, err := validate(actor, binding, binding.InstallationID)
	if err != nil || !slices.Contains(grants, "app.request") {
		return nil, 0, ErrDenied
	}
	return s.requestBound(ctx, actor, binding, in, requestID)
}

// requestBound is shared by separately authorized Dashboard and workspace APIs.
func (s *RegisteredSource) requestBound(ctx context.Context, actor Actor, binding Binding, in AppRequest, requestID string) ([]byte, int, error) {
	path := ""
	timeout := 15 * time.Second
	for _, rule := range binding.Rules {
		if target, ok := ruleTarget(rule, in.Method, in.Path, len(in.Body)); ok {
			path = target
			if rule.TimeoutSeconds > 0 {
				timeout = time.Duration(rule.TimeoutSeconds) * time.Second
			}
			break
		}
	}
	if path == "" {
		return nil, 0, ErrDenied
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := s.internalRequest(ctx, actor, *binding.Backend, in.Method, path, in.Body, "application/json")
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("X-Request-Id", requestID)
	if reviewOperation(binding.AppID, in.Method, in.Path) && path == in.Path {
		origin := "agent"
		if interactive, _ := ctx.Value(interactiveReviewKey{}).(bool); interactive {
			origin = "human_interactive"
		}
		req.Header.Set("X-Review-Origin", origin)
	}
	client := *s.client
	client.Timeout = timeout
	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, 0, ctx.Err()
		}
		return nil, 0, ErrUnavailable
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNoContent {
		body, err := io.ReadAll(io.LimitReader(resp.Body, 1))
		if err != nil || len(body) != 0 {
			return nil, 0, ErrInvalid
		}
		return nil, resp.StatusCode, nil
	}
	if !slices.Contains([]int{200, 201, 202, 400, 401, 403, 404, 409, 422, 429}, resp.StatusCode) {
		return nil, 0, ErrUnavailable
	}
	if strings.TrimSpace(strings.Split(resp.Header.Get("Content-Type"), ";")[0]) != "application/json" {
		return nil, 0, ErrInvalid
	}
	data, err := strictjson.ReadValue(resp.Body, 4<<20)
	if err != nil {
		return nil, 0, ErrInvalid
	}
	if resp.StatusCode >= 400 {
		var fields map[string]json.RawMessage
		if json.Unmarshal(data, &fields) != nil {
			return nil, 0, ErrInvalid
		}
		var code string
		if json.Unmarshal(fields["code"], &code) != nil || !appErrorCode.MatchString(code) {
			return nil, 0, ErrInvalid
		}
		data, _ = json.Marshal(map[string]string{"code": code})
	}
	return data, resp.StatusCode, nil
}
