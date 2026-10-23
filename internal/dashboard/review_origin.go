package dashboard

import "context"

type interactiveReviewKey struct{}

// WithInteractiveReview is set only after the browser transport verifies the
// existing session, same-origin request and dashboard envelope. It is never
// populated from App JSON, caller headers or an MCP token.
func WithInteractiveReview(ctx context.Context) context.Context {
	return context.WithValue(ctx, interactiveReviewKey{}, true)
}

func reviewOperation(appID, method, path string) bool {
	if method != "POST" || (appID != "project" && appID != "world") {
		return false
	}
	prefix := "/api/v1/" + appID + "/"
	return path == prefix+"review/open" || path == prefix+"review/append" || path == prefix+"review/status"
}

// Content service operations authorize the actual Project/World object. App
// installation management is not a substitute for that business permission.
// Keep this exception exact; a remapped upstream or management-only rule is
// deliberately excluded.
func contentBusinessRule(appID string, rule Rule) bool {
	if rule.RequireManage || rule.Method != "POST" || rule.Path != rule.UpstreamPath {
		return false
	}
	if reviewOperation(appID, rule.Method, rule.Path) {
		return true
	}
	switch appID {
	case "project":
		return rule.Path == "/api/v1/project/list" || rule.Path == "/api/v1/project/open"
	case "world":
		return rule.Path == "/api/v1/world/list" || rule.Path == "/api/v1/world/get"
	}
	return false
}
