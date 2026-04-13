package wellknown

import (
	"fmt"
	"net/http"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/api/handlers"
)

// resolveBaseURL returns the internal base URL if the request comes from an internal cluster client,
// otherwise returns the configured external base URL.
func (h *handler) resolveBaseURL(req api.Context) string {
	if h.internalBaseURL != "" && handlers.HostnameMatches(req.Host, h.internalHost) {
		return h.internalBaseURL
	}
	return h.baseURL
}

// oauthAuthorizationConfig returns metadata that matches the URL the client used to reach Obot.
func (h *handler) oauthAuthorizationConfig(req api.Context) handlers.OAuthAuthorizationServerConfig {
	baseURL := h.resolveBaseURL(req)
	if baseURL == h.baseURL {
		return h.config
	}

	config := h.config
	config.Issuer = baseURL
	config.AuthorizationEndpoint = fmt.Sprintf("%s/oauth/authorize", baseURL)
	config.TokenEndpoint = fmt.Sprintf("%s/oauth/token", baseURL)
	config.RegistrationEndpoint = fmt.Sprintf("%s/oauth/register", baseURL)
	config.JWKSURI = fmt.Sprintf("%s/oauth/jwks.json", baseURL)
	config.UserInfoEndpoint = fmt.Sprintf("%s/oauth/userinfo", baseURL)
	return config
}

// oauthAuthorization handles the /.well-known/oauth-authorization-server endpoint
func (h *handler) oauthAuthorization(req api.Context) error {
	return req.Write(h.oauthAuthorizationConfig(req))
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
