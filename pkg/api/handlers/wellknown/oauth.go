package wellknown

import (
	"fmt"

	"github.com/obot-platform/obot/pkg/api"
)

// oauthAuthorization handles the /.well-known/oauth-authorization-server endpoint
func (h *handler) oauthAuthorization(req api.Context) error {
	config := h.config
	config.Issuer = fmt.Sprintf("%s/%s", h.baseURL, req.PathValue("mcp_server_instance_id"))
	config.AuthorizationEndpoint = fmt.Sprintf("%s/oauth/authorize/%s", h.baseURL, req.PathValue("mcp_server_instance_id"))
	config.TokenEndpoint = fmt.Sprintf("%s/oauth/token/%s", h.baseURL, req.PathValue("mcp_server_instance_id"))
	config.RegistrationEndpoint = fmt.Sprintf("%s/oauth/register/%s", h.baseURL, req.PathValue("mcp_server_instance_id"))
	return req.Write(config)
}

func (h *handler) oauthProtectedResource(req api.Context) error {
	return req.Write(fmt.Sprintf(`{
	"resource_name": "Obot MCP Gateway",
	"resource": "%s/mcp-connect/%s",
	"authorization_servers": ["%[1]s/%[2]s"],
	"bearer_methods_supported": ["header"]
}`, h.baseURL, req.PathValue("mcp_server_instance_id")))
}
