package oauth

import (
	"crypto/ecdsa"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/obot-platform/obot/pkg/api/server"
	"github.com/obot-platform/obot/pkg/services"
)

type handler struct {
	gptClient   *gptscript.GPTScript
	baseURL     string
	oauthConfig services.OAuthAuthorizationServerConfig
	key         *ecdsa.PrivateKey
}

func SetupHandlers(gptClient *gptscript.GPTScript, oauthConfig services.OAuthAuthorizationServerConfig, baseURL string, key *ecdsa.PrivateKey, mux *server.Server) {
	h := &handler{
		gptClient:   gptClient,
		baseURL:     baseURL,
		oauthConfig: oauthConfig,
		key:         key,
	}

	mux.HandleFunc("POST /oauth/register", h.register)
	mux.HandleFunc("GET /oauth/register/{client}", h.readClient)
	mux.HandleFunc("PUT /oauth/register/{client}", h.updateClient)
	mux.HandleFunc("DELETE /oauth/register/{client}", h.deleteClient)
	mux.HandleFunc("GET /oauth/authorize", h.authorize)
	mux.HandleFunc("GET /oauth/callback", h.callback)
	mux.HandleFunc("POST /oauth/token", h.token)
	mux.HandleFunc("POST /oauth/mcp-token", h.mcpToken)
	mux.HandleFunc("POST /oauth/mcp-config", h.mcpConfig)
}
