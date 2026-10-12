package desktop

import (
	"strings"
	"testing"
)

func TestBlenderViewerPreservesPreparedResources(t *testing.T) {
	input := `<!doctype html><html><head><script src="studio-embed.js"></script><link rel="stylesheet" href="assets/client.css"><script type="module" src="assets/client.js"></script></head><body><div id="root"></div></body></html>`
	prefix := "/studio/legacy-blender/01980000-0000-4000-8000-000000000001"
	result, err := blenderViewer([]byte(input), prefix, "blenderA", strings.Repeat("s", 43))
	if err != nil {
		t.Fatal(err)
	}
	text := string(result)
	start := strings.Index(text, `<script src="`+prefix)
	end := strings.Index(text, `<script src="studio-embed.js">`)
	if start < 0 || end <= start || text[:start]+text[end:] != input {
		t.Fatal("original viewer bytes changed")
	}
	if !strings.Contains(text[start:end], `data-session="`+strings.Repeat("s", 43)+`"`) {
		t.Fatal("edit binding missing")
	}
}

func TestBlenderViewerRejectsUnexpectedBootstrap(t *testing.T) {
	for _, input := range []string{
		`<html><head></head></html>`,
		`<script src="studio-embed.js"></script><head></head>`,
		`<head></head><body><script src="studio-embed.js"></script></body>`,
		`<head><script>alert(1)</script><script src="studio-embed.js"></script></head>`,
		`<head><script src="studio-embed.js" async></script></head>`,
		`<head><script src="studio-embed.js"></script><BASE href="https://other.invalid"></head>`,
		`<head></head><head><script src="studio-embed.js"></script></head>`,
	} {
		if _, err := blenderViewer([]byte(input), "/workspace", "blenderA", strings.Repeat("s", 43)); err == nil {
			t.Fatal("accepted unexpected viewer")
		}
	}
}

func TestBlenderViewerCSPAuthorizesOriginalInlineScript(t *testing.T) {
	policy := blenderViewerCSP("http://wails.localhost", []byte("<head><script>window.ok=true;\r\n</script></head>"))
	if !strings.Contains(policy, "script-src http://wails.localhost 'unsafe-eval' 'sha256-KOmGoNF9UFx5+R/Z3PHG1Xf+PQzXwberIw7fZaxvIOY=';") {
		t.Fatal("inline bootstrap hash differs from browser-normalized text")
	}
	if !strings.Contains(policy, "connect-src http://wails.localhost;") || !strings.Contains(policy, "base-uri 'none'") {
		t.Fatal("viewer policy widened")
	}
}
