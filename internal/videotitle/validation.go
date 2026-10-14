// Package videotitle validates the precise Video rename proxy payload.
package videotitle

import (
	"bytes"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/verdantflarehub/verdantflare-studio/internal/strictjson"
)

func ValidBody(body []byte) bool {
	var value struct {
		Title    string `json:"title"`
		Revision int64  `json:"expected_title_revision"`
	}
	if strictjson.Decode(bytes.NewReader(body), 65536, &value) != nil || value.Revision < 0 {
		return false
	}
	title := strings.TrimSpace(value.Title)
	if utf8.RuneCountInString(title) < 1 || utf8.RuneCountInString(title) > 64 || strings.ContainsAny(title, "<>") {
		return false
	}
	for _, char := range value.Title {
		if unicode.Is(unicode.C, char) || char == '\u2028' || char == '\u2029' {
			return false
		}
	}
	return true
}
