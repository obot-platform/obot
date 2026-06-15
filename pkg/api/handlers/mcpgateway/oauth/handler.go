package oauth

import (
	"net/http"
	"sync"
	"time"

	"github.com/obot-platform/obot/pkg/api/handlers"
	"github.com/obot-platform/obot/pkg/api/server"
	"github.com/obot-platform/obot/pkg/jwt/persistent"
	"github.com/obot-platform/obot/pkg/mcp"
	"github.com/obot-platform/obot/pkg/system"
)

type handler struct {
	oauthChecker     *MCPOAuthHandlerFactory
	tokenService     *persistent.TokenService
	oauthConfig      handlers.OAuthAuthorizationServerConfig
	tokenStore       mcp.GlobalTokenStore
	baseURL          string
	clientExpiration time.Duration

	clientMetadataHTTPClient *http.Client
	clientMetadataCache      map[string]clientMetadataCacheEntry
	clientMetadataCacheLock  sync.Mutex
}

func SetupHandlers(oauthChecker *MCPOAuthHandlerFactory, tokenStore mcp.GlobalTokenStore, tokenService *persistent.TokenService, oauthConfig handlers.OAuthAuthorizationServerConfig, baseURL string, clientSecretExpiration time.Duration, mux *server.Server) {
	h := &handler{
		tokenStore:          tokenStore,
		tokenService:        tokenService,
		oauthConfig:         oauthConfig,
		baseURL:             baseURL,
		oauthChecker:        oauthChecker,
		clientExpiration:    clientSecretExpiration,
		clientMetadataCache: map[string]clientMetadataCacheEntry{},
	}

	// Expose two sets of endpoints: one for clients that look at the oauth-protected-resource metadata and one for clients that don't.
	// Clients that don't look at the metadata must use a resource parameter when authorizing.
	mux.HandleFunc("POST /oauth/register/{mcp_id}", h.register)
	mux.HandleFunc("POST /oauth/register", h.register)
	mux.HandleFunc("GET /oauth/authorize/{mcp_id}", h.authorize)
	mux.HandleFunc("GET /oauth/authorize", h.authorize)
	mux.HandleFunc("POST /oauth/token/{mcp_id}", h.token)
	mux.HandleFunc("POST /oauth/token", h.token)

	// This is the callback that Obot will redirect to after the user has authenticated.
	// It prepares the post-login consent screen before continuing to second-level OAuth
	// or returning the original redirect URI with the authorization code.
	mux.HandleFunc("GET /oauth/callback/{oauth_auth_request}", h.callback)
	mux.HandleFunc("GET /oauth/consent/{oauth_auth_request}", h.consent)
	mux.HandleFunc("POST /oauth/consent/{oauth_auth_request}/approve", h.approveConsent)
	mux.HandleFunc("POST /oauth/consent/{oauth_auth_request}/cancel", h.cancelConsent)
	mux.HandleFunc("GET /oauth/complete/{oauth_auth_request}", h.oauthComplete)

	mux.HandleFunc("GET /oauth/register/{client}", h.readClient)
	mux.HandleFunc("PUT /oauth/register/{client}", h.updateClient)
	mux.HandleFunc("DELETE /oauth/register/{client}", h.deleteClient)

	// This is the callback handler for second-level OAuth.
	// In other words, the third-party OAuth will redirect here.
	mux.HandleFunc("GET /oauth/mcp/callback", h.oauthCallback)

	mux.HandleFunc("GET /oauth/jwks.json", h.tokenService.ServeJWKS)
	mux.HandleFunc("POST /oauth/replace-jwks", h.tokenService.ReplaceJWK)
	mux.HandleFunc("GET "+system.OAuthClientIDMetadataPath, h.obotClientIDMetadata)

	mux.HandleFunc("GET /api/oauth/composite/{mcp_id}", h.checkCompositeAuth)

	mux.HandleFunc("GET /oauth/userinfo", h.userInfo)
}
