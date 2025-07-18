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
	mux.HandleFunc("GET /.well-known/oauth-authorization-server/mcp-connect/{mcp_id}", h.oauthAuthorization)

	return nil
}
