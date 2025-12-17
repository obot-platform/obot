package wellknown

import (
	"fmt"
	"net/http"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
)

// oauthAuthorization handles the /.well-known/oauth-authorization-server endpoint
func (h *handler) oauthAuthorization(req api.Context) error {
	mcpID := req.PathValue("mcp_id")
	if mcpID != "" {
		mcpID = "/" + mcpID
	}
	config := h.config
	config.Issuer = fmt.Sprintf("%s%s", h.baseURL, mcpID)
	config.AuthorizationEndpoint = fmt.Sprintf("%s/oauth/authorize%s", h.baseURL, mcpID)
	config.TokenEndpoint = fmt.Sprintf("%s/oauth/token%s", h.baseURL, mcpID)
	config.RegistrationEndpoint = fmt.Sprintf("%s/oauth/register%s", h.baseURL, mcpID)
	return req.Write(config)
}

func (h *handler) oauthProtectedResource(req api.Context) error {
	mcpID := req.PathValue("mcp_id")
	if mcpID != "" {
		return req.Write(fmt.Sprintf(`{
	"resource_name": "Obot MCP Gateway",
	"resource": "%s/mcp-connect/%s",
	"authorization_servers": ["%[1]s/%[2]s"],
	"bearer_methods_supported": ["header"]
}`, h.baseURL, mcpID))
	}

	// The client is hitting the "generic" metadata endpoint and is not supplying an MCP ID. Server the generic metadata.
	return req.Write(fmt.Sprintf(`{
	"resource_name": "Obot MCP Gateway",
	"resource": "%s/mcp-connect",
	"authorization_servers": ["%[1]s"],
	"bearer_methods_supported": ["header"]
}`, h.baseURL))
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

func (h *handler) acrRegistryOAuthProtectedResource(req api.Context) error {
	// Return 404 if registry is in no-auth mode
	if h.registryNoAuth {
		return &types.ErrHTTP{
			Code:    http.StatusNotFound,
			Message: "Registry OAuth is not available when registry authentication is disabled",
		}
	}

	acrID := req.PathValue("acr_id")
	if acrID == "" {
		return &types.ErrHTTP{
			Code:    http.StatusNotFound,
			Message: "ACR ID is required",
		}
	}

	// Return the same OAuth metadata as root registry, but scoped to the ACR path
	return req.Write(fmt.Sprintf(`{
	"resource": "%s/mcp-registry/%s/v0.1/servers",
	"authorization_servers": ["%[1]s"],
	"bearer_methods_supported": ["header"]
}`, h.baseURL, acrID))
}
