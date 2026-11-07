package markup

import (
	"github.com/88250/lute/html"
	"strings"
)

// Original util.EscapeHTML/UnescapeHTML preserve whitespace-only input.
func EscapeHTML(s string) string {
	if strings.TrimSpace(s) == "" {
		return s
	}
	return html.EscapeString(s)
}
func UnescapeHTML(s string) string {
	if strings.TrimSpace(s) == "" {
		return s
	}
	return html.UnescapeString(s)
}
