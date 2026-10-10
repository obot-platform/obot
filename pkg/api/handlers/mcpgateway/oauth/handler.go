package oauth

import (
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/accesscontrolrule"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/api/handlers"
	"github.com/obot-platform/obot/pkg/api/server"
	"github.com/obot-platform/obot/pkg/jwt/persistent"
	"github.com/obot-platform/obot/pkg/mcp"
	"github.com/obot-platform/obot/pkg/safehttp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	vmcpconfig "github.com/obot-platform/obot/pkg/vmcp"
)

type handler struct {
	oauthChecker     *MCPOAuthHandlerFactory
	tokenService     *persistent.TokenService
	oauthConfig      handlers.OAuthAuthorizationServerConfig
	tokenStore       mcp.GlobalTokenStore
	acrHelper        *accesscontrolrule.Helper
	baseURL          string
	clientExpiration time.Duration

	clientMetadataHTTPClient *http.Client
	clientMetadataCache      map[string]clientMetadataCacheEntry
	clientMetadataCacheLock  sync.Mutex
	clientIDNativeExceptions map[string]struct{}
}

func SetupHandlers(oauthChecker *MCPOAuthHandlerFactory, tokenStore mcp.GlobalTokenStore, tokenService *persistent.TokenService, oauthConfig handlers.OAuthAuthorizationServerConfig, mcpSessionManager *mcp.SessionManager, acrHelper *accesscontrolrule.Helper, baseURL string, clientSecretExpiration time.Duration, additionalClientIDNativeExceptions []string, mux *server.Server) {
	remoteURLValidationConfig := mcpSessionManager.RemoteMCPURLValidationConfig()
	h := &handler{
		tokenStore:   tokenStore,
		tokenService: tokenService,
		oauthConfig:  oauthConfig,
		clientMetadataHTTPClient: safehttp.NewClient(safehttp.Options{
			BlockLoopback:  !remoteURLValidationConfig.AllowLocalhostMCP,
			BlockPrivateIP: !remoteURLValidationConfig.AllowPrivateIPMCP,
			BlockLinkLocal: !remoteURLValidationConfig.AllowLinkLocalMCP,
			Timeout:        clientMetadataFetchTimeout,
		}),
		baseURL:                  baseURL,
		oauthChecker:             oauthChecker,
		acrHelper:                acrHelper,
		clientExpiration:         clientSecretExpiration,
		clientMetadataCache:      map[string]clientMetadataCacheEntry{},
		clientIDNativeExceptions: newClientIDNativeExceptions(additionalClientIDNativeExceptions),
	}
	// Reuse the handler's SSRF-safe client metadata resolver when forwarding a
	// downstream client's registered name to the upstream OAuth server.
	oauthChecker.resolveOAuthClient = h.resolveOAuthClient

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

	mux.HandleFunc("GET /api/oauth/vmcp/{mcp_id}", h.checkVMCPAuth)
	mux.HandleFunc("GET /api/oauth/vmcp/{mcp_id}/components/{component_mcp_id}", h.checkVMCPComponentAuth)

	mux.HandleFunc("GET /oauth/userinfo", h.userInfo)
}

// OAuth connections must resolve to a vMCP or instance, including migrated aliases.
func resolveOAuthConnection(req api.Context, id string) (*v1.VMCP, *v1.VMCPInstance, error) {
	vmcp, instance, err := vmcpconfig.ResolveID(req.Context(), req.Storage, id)
	if err != nil {
		return nil, nil, err
	}
	if vmcp == nil {
		return nil, nil, types.NewErrBadRequest("mcp_id must identify a vMCP or vMCP instance")
	}
	return vmcp, instance, nil
}

func validOAuthConnection(req api.Context, mcpID, audience, resource, userID string) (bool, error) {
	u, err := url.Parse(resource)
	if err != nil {
		return false, nil
	}
	resourceID, ok := strings.CutPrefix(u.Path, "/mcp-connect/")
	if !ok || resourceID == "" {
		return false, nil
	}
	ids := []string{mcpID, resourceID}
	if audience != "" {
		ids = append(ids, audience)
	}
	for _, id := range ids {
		vmcp, _, err := vmcpconfig.ResolveConnectID(req.Context(), req.Storage, id, userID)
		if err != nil || vmcp == nil {
			return false, err
		}
	}
	return true, nil
}
