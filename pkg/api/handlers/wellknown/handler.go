package wellknown

import (
	"github.com/obot-platform/obot/pkg/api/handlers"
	"github.com/obot-platform/obot/pkg/api/server"
)

type handler struct {
	baseURL        string
	config         handlers.OAuthAuthorizationServerConfig
	registryNoAuth bool
}

func SetupHandlers(baseURL string, config handlers.OAuthAuthorizationServerConfig, registryNoAuth bool, mux *server.Server) {
	h := &handler{
		baseURL:        baseURL,
		config:         config,
		registryNoAuth: registryNoAuth,
	}

	mux.HandleFunc("GET /.well-known/oauth-protected-resource/mcp-connect/{mcp_id}", h.oauthProtectedResource)
	mux.HandleFunc("GET /.well-known/oauth-protected-resource/v0.1/servers", h.registryOAuthProtectedResource)

	mux.HandleFunc("GET /.well-known/oauth-protected-resource", h.oauthProtectedResource)
	mux.HandleFunc("GET /.well-known/oauth-authorization-server", h.oauthAuthorization)
	mux.HandleFunc("GET /.well-known/oauth-authorization-server/oauth/authorize", h.oauthAuthorization)
}
