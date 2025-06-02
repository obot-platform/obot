package oauth

import (
	"github.com/gptscript-ai/go-gptscript"
	"github.com/obot-platform/obot/pkg/api/server"
	"github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/services"
)

type handler struct {
	gptClient     *gptscript.GPTScript
	gatewayClient *client.Client
	oauthConfig   services.OAuthAuthorizationServerConfig
	baseURL       string
}

func SetupHandlers(gptClient *gptscript.GPTScript, gatewayClient *client.Client, oauthConfig services.OAuthAuthorizationServerConfig, baseURL string, mux *server.Server) {
	h := &handler{
		gptClient:     gptClient,
		gatewayClient: gatewayClient,
		oauthConfig:   oauthConfig,
		baseURL:       baseURL,
	}

	mux.HandleFunc("POST /oauth/register", h.register)
	mux.HandleFunc("GET /oauth/register/{client}", h.readClient)
	mux.HandleFunc("PUT /oauth/register/{client}", h.updateClient)
	mux.HandleFunc("DELETE /oauth/register/{client}", h.deleteClient)
	mux.HandleFunc("GET /oauth/authorize", h.authorize)
	mux.HandleFunc("GET /oauth/callback/{oauth_auth_request}", h.callback)
	mux.HandleFunc("POST /oauth/token", h.token)
}
