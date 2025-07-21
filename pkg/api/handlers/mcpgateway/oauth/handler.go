package oauth

import (
	"github.com/obot-platform/obot/pkg/api/server"
	"github.com/obot-platform/obot/pkg/jwt"
	"github.com/obot-platform/obot/pkg/services"
)

type handler struct {
	oauthChecker *MCPOAuthHandlerFactory
	tokenService *jwt.TokenService
	oauthConfig  services.OAuthAuthorizationServerConfig
	baseURL      string
}

func SetupHandlers(oauthChecker *MCPOAuthHandlerFactory, tokenService *jwt.TokenService, oauthConfig services.OAuthAuthorizationServerConfig, baseURL string, mux *server.Server) {
	h := &handler{
		tokenService: tokenService,
		oauthConfig:  oauthConfig,
		baseURL:      baseURL,
		oauthChecker: oauthChecker,
	}

	mux.HandleFunc("POST /oauth/register/{mcp_id}", h.register)
	mux.HandleFunc("GET /oauth/register/{client}", h.readClient)
	mux.HandleFunc("PUT /oauth/register/{client}", h.updateClient)
	mux.HandleFunc("DELETE /oauth/register/{client}", h.deleteClient)
	mux.HandleFunc("GET /oauth/authorize/{mcp_id}", h.authorize)
	mux.HandleFunc("GET /oauth/callback/{oauth_auth_request}/{mcp_id}", h.callback)
	mux.HandleFunc("POST /oauth/token/{mcp_id}", h.token)
	mux.HandleFunc("GET /oauth/mcp/callback", h.oauthCallback)
}
