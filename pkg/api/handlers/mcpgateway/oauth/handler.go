package oauth

import (
	"github.com/gptscript-ai/go-gptscript"
	"github.com/obot-platform/obot/pkg/api/handlers/mcpgateway"
	"github.com/obot-platform/obot/pkg/api/server"
	"github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/mcp"
	"github.com/obot-platform/obot/pkg/services"
)

type handler struct {
	gptClient         *gptscript.GPTScript
	gatewayClient     *client.Client
	oauthConfig       services.OAuthAuthorizationServerConfig
	mcpSessionManager *mcp.SessionManager
	baseURL           string
	stateCache        *stateCache
	tokenStore        mcpgateway.GlobalTokenStore
}

func SetupHandlers(gptClient *gptscript.GPTScript, gatewayClient *client.Client, mcpSessionManager *mcp.SessionManager, oauthConfig services.OAuthAuthorizationServerConfig, baseURL string, mux *server.Server) {
	h := &handler{
		gptClient:         gptClient,
		gatewayClient:     gatewayClient,
		oauthConfig:       oauthConfig,
		mcpSessionManager: mcpSessionManager,
		baseURL:           baseURL,
		stateCache:        newStateCache(gatewayClient),
		tokenStore:        mcpgateway.NewGlobalTokenStore(gatewayClient),
	}

	mux.HandleFunc("POST /oauth/register/{mcp_server_instance_id}", h.register)
	mux.HandleFunc("GET /oauth/register/{client}", h.readClient)
	mux.HandleFunc("PUT /oauth/register/{client}", h.updateClient)
	mux.HandleFunc("DELETE /oauth/register/{client}", h.deleteClient)
	mux.HandleFunc("GET /oauth/authorize/{mcp_server_instance_id}", h.authorize)
	mux.HandleFunc("GET /oauth/callback/{oauth_auth_request}/{mcp_server_instance_id}", h.callback)
	mux.HandleFunc("POST /oauth/token/{mcp_server_instance_id}", h.token)
	mux.HandleFunc("GET /oauth/mcp/callback/{oauth_auth_request}/{mcp_server_instance_id}", h.oauthCallback)
}
