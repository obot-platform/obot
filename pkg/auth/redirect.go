package auth

import (
	"strings"
	"unicode"
)

// SafeRedirectPath sanitizes a caller-supplied redirect so that it can only point back into Obot,
// returning "/" for anything else. Browsers treat "\" as "/" when resolving against an http(s)
// base, so "/\evil.com" is another spelling of "//evil.com" and must be rejected too. Browsers
// also strip tabs and newlines from URLs, so "/\t/evil.com" is rejected along with any other
// control character.
func SafeRedirectPath(rd string) string {
	if strings.IndexFunc(rd, unicode.IsControl) >= 0 {
		return "/"
	}
	normalized := strings.ReplaceAll(rd, `\`, "/")
	if !strings.HasPrefix(normalized, "/") || strings.HasPrefix(normalized, "//") {
		return "/"
	}
	return rd
}
