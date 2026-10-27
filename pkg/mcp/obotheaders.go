package mcp

import (
	"net/http"

	"github.com/obot-platform/obot/apiclient/types"
)

// StripObotHeaders reserves the attribution namespace for gateway-derived values.
// Iterate the map to also remove non-canonical keys supplied by in-process callers.
func StripObotHeaders(headers http.Header) {
	for name := range headers {
		if _, ok := types.CanonicalObotHeader(name); ok {
			delete(headers, name)
		}
	}
}
