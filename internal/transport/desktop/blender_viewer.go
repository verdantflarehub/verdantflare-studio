package desktop

import (
	"bytes"
	"crypto/sha256"
	_ "embed"
	"encoding/base64"
	"errors"
	"io"
	"strings"

	"golang.org/x/net/html"
)

//go:embed blender_signal.js
var blenderSignalScript []byte

// Keep the prepared viewer bytes and relative paths. The first executable script
// must be the App-owned relay/embedding adapter; insert the signal bridge before it.
func blenderViewer(data []byte, workspace, alias, session string) ([]byte, error) {
	z := html.NewTokenizer(bytes.NewReader(data))
	offset, insertion, heads, scripts := 0, -1, 0, 0
	inHead := false
	for {
		kind := z.Next()
		if kind == html.ErrorToken {
			if z.Err() != io.EOF {
				return nil, z.Err()
			}
			break
		}
		start := offset
		offset += len(z.Raw())
		if kind == html.EndTagToken {
			if token := z.Token(); token.Data == "head" {
				inHead = false
			}
			continue
		}
		if kind != html.StartTagToken && kind != html.SelfClosingTagToken {
			continue
		}
		token := z.Token()
		switch token.Data {
		case "base":
			return nil, errors.New("unexpected viewer base")
		case "head":
			heads++
			inHead = true
		case "script":
			scripts++
			if scripts == 1 {
				if heads != 1 || !inHead || kind != html.StartTagToken || len(token.Attr) != 1 || token.Attr[0].Key != "src" || token.Attr[0].Val != "studio-embed.js" {
					return nil, errors.New("unexpected viewer bootstrap")
				}
				insertion = start
			}
		}
	}
	if heads != 1 || insertion < 0 {
		return nil, errors.New("missing viewer bootstrap")
	}
	// All substituted values have already passed the host's strict path patterns.
	shim := `<script src="` + workspace + `/signal-shim.js" data-workspace="` + workspace + `" data-alias="` + alias + `" data-session="` + session + `"></script>`
	result := make([]byte, 0, len(data)+len(shim))
	result = append(result, data[:insertion]...)
	result = append(result, shim...)
	return append(result, data[insertion:]...), nil
}

func blenderViewerCSP(origin string, data []byte) string {
	// Authorize the original inline bootstrap by its parsed text, never unsafe-inline
	// for scripts. HTML parsing normalizes CRLF just as the browser does for CSP.
	// The pinned legacy Vue 2 viewer compiles its HTML template at runtime.
	// This exception applies to this trusted editor document, never the host page.
	scripts := origin + " 'unsafe-eval'"
	if document, err := html.Parse(bytes.NewReader(data)); err == nil {
		var visit func(*html.Node)
		visit = func(node *html.Node) {
			if node.Type == html.ElementNode && node.Data == "script" {
				var text strings.Builder
				for child := node.FirstChild; child != nil; child = child.NextSibling {
					if child.Type == html.TextNode {
						text.WriteString(child.Data)
					}
				}
				if text.Len() > 0 {
					digest := sha256.Sum256([]byte(text.String()))
					scripts += " 'sha256-" + base64.StdEncoding.EncodeToString(digest[:]) + "'"
				}
			}
			for child := node.FirstChild; child != nil; child = child.NextSibling {
				visit(child)
			}
		}
		visit(document)
	}
	// WebRTC stays constrained by the original studio-embed.js relay-only adapter.
	return strings.Join([]string{"default-src 'none'", "script-src " + scripts, "style-src " + origin + " 'unsafe-inline'", "img-src " + origin + " data: blob:", "font-src " + origin, "manifest-src " + origin,
		"connect-src " + origin, "media-src blob:", "worker-src " + origin + " blob:", "frame-ancestors 'self'", "base-uri 'none'", "form-action 'none'", "object-src 'none'"}, "; ")
}
