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

	mux.HandleFunc("GET /.well-known/oauth-protected-resource", h.oauthProtectedResource)
	mux.HandleFunc("GET /.well-known/oauth-authorization-server", h.oauthAuthorization)

	return nil
}
