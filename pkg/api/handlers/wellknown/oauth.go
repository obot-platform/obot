package wellknown

import (
	"fmt"
	"net/http"
	"net/url"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
)

// oauthAuthorization handles the /.well-known/oauth-authorization-server and /.well-known/oauth-authorization-server/{mcp_id} endpoints
func (h *handler) oauthAuthorization(req api.Context) error {
	config := h.config
	if mcpID := req.PathValue("mcp_id"); mcpID != "" {
		config.AuthorizationEndpoint = appendPathSegment(config.AuthorizationEndpoint, mcpID)
		config.RegistrationEndpoint = appendPathSegment(config.RegistrationEndpoint, mcpID)
		config.TokenEndpoint = appendPathSegment(config.TokenEndpoint, mcpID)
	}
	return req.Write(config)
}

func appendPathSegment(rawURL, segment string) string {
	if rawURL == "" || segment == "" {
		return rawURL
	}

	joined, err := url.JoinPath(rawURL, segment)
	if err != nil {
		return rawURL
	}
	return joined
}

func (h *handler) oauthProtectedResource(req api.Context) error {
	mcpID := req.PathValue("mcp_id")
	if mcpID != "" {
		return req.Write(map[string]any{
			"resource_name":            "Obot MCP Gateway",
			"resource":                 fmt.Sprintf("%s/mcp-connect/%s", h.baseURL, mcpID),
			"authorization_servers":    []string{h.baseURL + "/" + mcpID},
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

	return req.Write(map[string]any{
		"resource":                 h.baseURL,
		"authorization_servers":    []string{h.baseURL},
		"bearer_methods_supported": []string{"header"},
	})
}
