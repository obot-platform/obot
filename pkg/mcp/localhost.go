package mcp

import (
	"net/url"
	"path"
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
)

const (
	DefaultLocalhostCallbackPath = "/oauth/callback"
)

// ValidateLocalhostCallbackPath keeps the provider callback separate from the
// CLI's own OAuth callback and from URL query/authority components.
func ValidateLocalhostCallbackPath(value string) error {
	if value == "" {
		return nil
	}
	u, err := url.Parse(value)
	if err != nil || !strings.HasPrefix(value, "/") || strings.HasPrefix(value, "//") ||
		strings.ContainsAny(value, "?#\\") || strings.TrimSpace(value) != value ||
		u.Host != "" || u.Path != path.Clean(u.Path) || u.Path == "/oauth/obot/callback" {
		return types.RuntimeValidationError{
			Runtime: types.RuntimeRemote,
			Field:   "localhostCallbackPath",
			Message: "localhost callback must be an absolute path without query or fragment, and cannot be /oauth/obot/callback",
		}
	}
	return nil
}
