package wellknown

import (
	"github.com/obot-platform/obot/pkg/api/server"
	"github.com/obot-platform/obot/pkg/services"
)

type handler struct {
	baseURL string
	config  services.OAuthAuthorizationServerConfig
}

func SetupHandlers(baseURL string, config services.OAuthAuthorizationServerConfig, mux *server.Server) error {
	h := &handler{
		baseURL: baseURL,
		config:  config,
	}

	mux.HandleFunc("GET /.well-known/oauth-protected-resource/mcp-connect/{mcp_id}", h.oauthProtectedResource)
	// Some clients choose the wrong URL for oauth-authorization-server. It doesn't harm anything to serve both.
	mux.HandleFunc("GET /.well-known/oauth-authorization-server/{mcp_id}", h.oauthAuthorization)
	// This is the one we expect clients to hit.
	mux.HandleFunc("GET /.well-known/oauth-authorization-server/mcp-connect/{mcp_id}", h.oauthAuthorization)

	// These will allow clients that don't follow the WWW-Authenticate header to connect to the MCP gateway.
	// Such clients won't be able to do the second-level OAuth, but will be able to connect to all MCP servers
	// that don't require second-level OAuth.
	mux.HandleFunc("GET /.well-known/oauth-protected-resource", h.oauthProtectedResource)
	mux.HandleFunc("GET /.well-known/oauth-authorization-server", h.oauthAuthorization)

	return nil
}
