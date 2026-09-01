package auth

import (
	"strings"
)

// SafeRedirectPath sanitizes a caller-supplied redirect so that it can only point back into Obot,
// returning "/" for anything else. Browsers treat "\" as "/" when resolving against an http(s)
// base, so "/\evil.com" is another spelling of "//evil.com" and must be rejected too.
func SafeRedirectPath(rd string) string {
	normalized := strings.ReplaceAll(rd, `\`, "/")
	if !strings.HasPrefix(normalized, "/") || strings.HasPrefix(normalized, "//") {
		return "/"
	}
	return rd
}
