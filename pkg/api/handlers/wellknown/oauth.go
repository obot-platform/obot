package wellknown

import (
	"fmt"
	"net"
	"net/http"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
)

// resolveBaseURL returns the internal base URL if the request comes from an internal cluster client,
// otherwise returns the configured external base URL.
func (h *handler) resolveBaseURL(req api.Context) string {
	if h.internalBaseURL != "" && h.internalHost != "" {
		host := req.Host
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		if host == h.internalHost {
			return h.internalBaseURL
		}
	}
	return h.baseURL
}

// oauthAuthorization handles the /.well-known/oauth-authorization-server endpoint
func (h *handler) oauthAuthorization(req api.Context) error {
	return req.Write(h.config)
}

func (h *handler) oauthProtectedResource(req api.Context) error {
	baseURL := h.resolveBaseURL(req)
	mcpID := req.PathValue("mcp_id")
	if mcpID != "" {
		return req.Write(fmt.Sprintf(`{
	"resource_name": "Obot MCP Gateway",
	"resource": "%s/mcp-connect/%s",
	"authorization_servers": ["%[1]s"],
	"bearer_methods_supported": ["header"]
}`, baseURL, mcpID))
	}

	// The client is hitting the "generic" metadata endpoint and is not supplying an MCP ID. Server the generic metadata.
	return req.Write(fmt.Sprintf(`{
	"resource_name": "Obot MCP Gateway",
	"resource": "%s/mcp-connect",
	"authorization_servers": ["%[1]s"],
	"bearer_methods_supported": ["header"]
}`, baseURL))
}

func (h *handler) registryOAuthProtectedResource(req api.Context) error {
	// Return 404 if registry is in no-auth mode
	if h.registryNoAuth {
		return &types.ErrHTTP{
			Code:    http.StatusNotFound,
			Message: "Registry OAuth is not available when registry authentication is disabled",
		}
	}

	return req.Write(fmt.Sprintf(`{
	"resource": "%s",
	"authorization_servers": ["%[1]s"],
	"bearer_methods_supported": ["header"]
}`, h.baseURL))
}
