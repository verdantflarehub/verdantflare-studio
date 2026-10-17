package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const MusicGenerateTimeout = 65 * time.Minute
const musicContentPrefix = "/mcp-content/music"

func MusicContentPath(path string) bool {
	return strings.HasPrefix(path, musicContentPrefix+"/") && VideoContentPath(strings.TrimPrefix(path, musicContentPrefix))
}

func musicTool(name string) bool {
	switch name {
	case "music.generate", "music.redraw", "duet.plan", "duet.instrumental", "duet.vocal", "duet.preview", "stems.separate", "voice.prepare", "voice.train", "voice.convert", "lyrics.align", "mix.master", "asset.import":
		return true
	}
	return false
}

// Rewrite only legacy Music content links, including the JSON text fallback.
// Artifact IDs and integrity metadata remain unchanged.
func musicResult(raw json.RawMessage) json.RawMessage {
	var value any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if decoder.Decode(&value) != nil {
		return raw
	}
	var visit func(any)
	visit = func(v any) {
		switch node := v.(type) {
		case []any:
			for _, child := range node {
				visit(child)
			}
		case map[string]any:
			for key, child := range node {
				if text, ok := child.(string); ok {
					switch key {
					case "download_path", "download_url", "uri":
						u, err := url.Parse(text)
						if err == nil && u.RawPath == "" && u.RawQuery == "" && u.Fragment == "" && u.User == nil && VideoContentPath(u.Path) {
							u.Path = musicContentPrefix + u.Path
							node[key] = u.String()
						}
					case "text":
						if json.Valid([]byte(text)) {
							node[key] = string(musicResult(json.RawMessage(text)))
						}
					}
				} else {
					visit(child)
				}
			}
		}
	}
	visit(value)
	encoded, err := json.Marshal(value)
	if err != nil {
		return raw
	}
	return encoded
}

func (g *Gateway) MusicContent(ctx context.Context, path, byteRange string, identity map[string]string) (*http.Response, int, error) {
	if !MusicContentPath(path) {
		return nil, 400, errors.New("invalid music content path")
	}
	return g.legacyMediaContent(ctx, "music", "music.generate", strings.TrimPrefix(path, musicContentPrefix), byteRange, identity)
}
