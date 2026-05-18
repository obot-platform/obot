package wellknown

import (
	"fmt"
	"net/http"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
)

// oauthAuthorization handles the /.well-known/oauth-authorization-server endpoint
func (h *handler) oauthAuthorization(req api.Context) error {
	return req.Write(h.config)
}

func (h *handler) oauthProtectedResource(req api.Context) error {
	mcpID := req.PathValue("mcp_id")
	if mcpID != "" {
		return req.Write(map[string]any{
			"resource_name":            "Obot MCP Gateway",
			"resource":                 fmt.Sprintf("%s/mcp-connect/%s", h.baseURL, mcpID),
			"authorization_servers":    []string{h.baseURL},
			"bearer_methods_supported": []string{"header"},
		})
	}

	// The client is hitting the "generic" metadata endpoint and is not supplying an MCP ID. Serve the generic metadata.
	return req.Write(map[string]any{
		"resource_name":            "Obot MCP Gateway",
		"resource":                 fmt.Sprintf("%s/mcp-connect", h.baseURL),
		"authorization_servers":    []string{h.baseURL},
		"bearer_methods_supported": []string{"header"},
	})
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
