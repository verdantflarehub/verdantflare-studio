package project

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGenerationApprovalSeparateCredentialAndNoMCPRoute(t *testing.T) {
	s := &Service{}
	serviceToken, approvalToken, artifactToken := strings.Repeat("s", 32), strings.Repeat("h", 32), strings.Repeat("a", 32)
	if _, e := s.GenerationControlHandler(serviceToken, serviceToken); e == nil {
		t.Fatal("shared approval credential")
	}
	h, e := s.GenerationControlHandler(serviceToken, approvalToken)
	if e != nil {
		t.Fatal(e)
	}
	for _, token := range []string{"", serviceToken, artifactToken} {
		r := httptest.NewRequest(http.MethodPost, "/internal/v1/generation/approve", strings.NewReader(`{}`))
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set("X-Organization-Id", newID())
		r.Header.Set("X-User-Id", newID())
		r.Header.Set("X-Request-Id", newID())
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 403 {
			t.Fatal("non-approval credential accepted", w.Code)
		}
	}
	for _, tool := range MCPTools() {
		if strings.Contains(tool.Name, "approve") || strings.Contains(tool.Name, "reserve") || strings.Contains(tool.Name, "transition") || tool.Name == "project.generation_submit" {
			t.Fatal("private control exposed", tool.Name)
		}
	}
	mcp, e := s.MCPHandler(serviceToken, artifactToken)
	if e != nil {
		t.Fatal(e)
	}
	r := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewBufferString(`{"jsonrpc":"2.0","id":"test","method":"tools/call","params":{"name":"project.generation_approve","arguments":{}}}`))
	r.Header.Set("Authorization", "Bearer "+serviceToken)
	r.Header.Set("X-Organization-Id", newID())
	r.Header.Set("X-User-Id", newID())
	r.Header.Set("X-Request-Id", newID())
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mcp.ServeHTTP(w, r)
	if w.Code != 404 {
		t.Fatal("MCP approval route exposed", w.Code)
	}
}
