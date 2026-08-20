package mcpgateway

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/obot-platform/nanobot/pkg/llm"
	nmcp "github.com/obot-platform/nanobot/pkg/mcp"
	"github.com/obot-platform/nanobot/pkg/runtime"
	"github.com/obot-platform/nanobot/pkg/server"
	"github.com/obot-platform/nanobot/pkg/session"
	ntypes "github.com/obot-platform/nanobot/pkg/types"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/controller/handlers/systemmcpserver"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/jwt/persistent"
	"github.com/obot-platform/obot/pkg/mcp"
	"github.com/obot-platform/obot/pkg/principal"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/obot-platform/obot/pkg/tunnel"
	"github.com/obot-platform/obot/pkg/utils"
	"golang.org/x/oauth2"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/authentication/user"
)

type Handler struct {
	mcpSessionManager         *mcp.SessionManager
	globalTokenStore          mcp.GlobalTokenStore
	tokenService              *persistent.TokenService
	auditLogCollector         proxyAuditCollector
	nanobot                   http.Handler
	hookRunner                mcp.HookRunner
	tunnelManager             *tunnel.Manager
	secretBindingAllowedLabel string
	serverURL                 string
}

func auditLogMetadataForPrincipal(metadata map[string]string, user user.Info) map[string]string {
	result := maps.Clone(metadata)
	attribution, ok := principal.APIKeyAttributionFromUser(user)
	if !ok {
		return result
	}
	if result == nil {
		result = map[string]string{}
	}
	result[principal.APIKeyIDExtra] = strconv.FormatUint(uint64(attribution.ID), 10)
	result[principal.APIKeyNameExtra] = attribution.Name
	return result
}

var errMCPServerRequiresConfiguration = errors.New("mcp server requires configuration")

func compositeLoopbackURLs(serverURL, mcpServerName string, transform func(string) string) (audienceURL, targetURL string) {
	audienceURL = fmt.Sprintf("%s/mcp-connect-composite/%s", strings.TrimSuffix(serverURL, "/"), mcpServerName)
	return audienceURL, transform(audienceURL)
}

func NewHandler(ctx context.Context, mcpSessionManager *mcp.SessionManager, globalTokenStore mcp.GlobalTokenStore, tokenService *persistent.TokenService, auditLogCollector proxyAuditCollector, serverURL, dsn, secretBindingAllowedLabel string, tunnelManager *tunnel.Manager) (*Handler, error) {
	sessionStore, err := session.NewStoreFromDSN(dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to create session store: %w", err)
	}

	// TODO(thedadams): do we want to make this gcPeriod configurable?
	sessionManager := session.NewManager(ctx, sessionStore, 24*7*time.Hour)
	remoteValidationConfig, allowedHosts := mcpSessionManager.RemoteConfigForBackend()
	if tunnelManager != nil {
		allowedHosts = append(allowedHosts, tunnelManager.BridgeHost())
	}

	nanobotRuntime, err := runtime.NewRuntime(ctx, llm.Config{}, runtime.Options{
		TokenExchangeEndpoint: mcpSessionManager.TransformObotHostname(fmt.Sprintf("%s/oauth/token", serverURL)),
		BlockLoopback:         !remoteValidationConfig.AllowLocalhostMCP,
		BlockPrivateIP:        !remoteValidationConfig.AllowPrivateIPMCP,
		BlockLinkLocal:        !remoteValidationConfig.AllowLinkLocalMCP,
		AllowedHosts:          allowedHosts,
		Store:                 sessionStore,
		AuditLogCollector:     auditLogCollector,
	})
	if err != nil {
		return nil, err
	}

	var mcpServer nmcp.MessageHandler = server.NewServer(nanobotRuntime, nil, sessionManager, server.Options{
		ForceFetchToolList: true,
	})

	otelEnv := mcp.OTELEnv("obot-proxy", serverURL)
	otelEnvMap := make(map[string]string, len(otelEnv))
	for k, v := range otelEnv {
		otelEnvMap[k] = string(v)
	}

	envProvider := func() (map[string]string, error) {
		return otelEnvMap, nil
	}

	nanobotHTTPServer, err := nmcp.NewHTTPServer(envProvider, mcpServer, nmcp.HTTPServerOptions{
		SessionStore:      sessionManager,
		AuditLogCollector: auditLogCollector,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP server: %w", err)
	}

	return &Handler{
		mcpSessionManager:         mcpSessionManager,
		globalTokenStore:          globalTokenStore,
		tokenService:              tokenService,
		auditLogCollector:         auditLogCollector,
		nanobot:                   nanobotHTTPServer,
		hookRunner:                mcp.NewHookRunner(mcpSessionManager),
		tunnelManager:             tunnelManager,
		secretBindingAllowedLabel: secretBindingAllowedLabel,
		serverURL:                 serverURL,
	}, nil
}

func (h *Handler) Proxy(req api.Context) error {
	if !req.UserIsAuthenticated() {
		writeMCPAuthRequired(req, false)
		return nil
	}

	serverConfig, err := h.ensureServerIsDeployed(req)
	if err != nil {
		if errors.Is(err, errMCPServerRequiresConfiguration) {
			return nil
		}
		return fmt.Errorf("failed to ensure server is deployed: %v", err)
	}

	var (
		token       string
		tokenSource oauth2.TokenSource
		now         = time.Now()
	)
	if !strings.HasSuffix(req.URL.Path, "/mcp-connect-composite/"+req.PathValue("mcp_id")) {
		// For composite runtimes, the /mcp-connect/{mcp_id} path only handles audit logs and hooks.
		// The URL is changed to /mcp-connect-composite/{mcp_id} which comes back here and handles
		// the multi-MCP server configuration (and calls no audit logs nor hooks).
		if serverConfig.Runtime == types.RuntimeComposite {
			compositeAudienceURL, compositeTargetURL := compositeLoopbackURLs(h.serverURL, serverConfig.MCPServerName, h.mcpSessionManager.TransformObotHostname)
			serverConfig.URL = compositeTargetURL

			// In order for the loopback to work, we need to authenticate as a composite MCP server.
			_, token, err = h.tokenService.NewToken(req.Context(), persistent.TokenContext{
				Audience:   compositeAudienceURL,
				IssuedAt:   persistent.NewTime(now),
				ExpiresAt:  persistent.NewTime(now.Add(10 * time.Minute)),
				UserID:     req.User.GetUID(),
				UserName:   req.User.GetName(),
				UserEmail:  utils.FirstSet(req.User.GetExtra()["email"]...),
				UserGroups: []string{types.GroupCompositeMCP, types.GroupAuthenticated},
				MCPID:      serverConfig.MCPServerName,
			})
			if err != nil {
				return err
			}
		} else if serverConfig.MCPServerName == system.ObotMCPServerName {
			// If this is contacting the Obot system MCP server, then we need to mint a token that has access
			// to the APIs the user has access to so the MCP server can list/connect to MCP servers.
			_, token, err = h.tokenService.NewToken(req.Context(), persistent.TokenContext{
				Audience:   h.serverURL,
				IssuedAt:   persistent.NewTime(now),
				ExpiresAt:  persistent.NewTime(now.Add(time.Hour)),
				UserID:     serverConfig.UserID,
				UserGroups: []string{types.GroupAPI, types.GroupAuthenticated},
				Namespace:  system.DefaultNamespace,
			})
			if err != nil {
				return fmt.Errorf("failed to generate token: %w", err)
			}
		} else {
			tokenSource, err = h.globalTokenStore.ForUserAndMCP(serverConfig.UserID, serverConfig.MCPServerName, serverConfig.URL).TokenSource(req.Context())
			if err != nil {
				return fmt.Errorf("failed to get token source: %w", err)
			}
		}

		if token != "" {
			serverConfig.Headers = append(serverConfig.Headers, "Authorization=Bearer "+token)
		}

		client, err := h.mcpSessionManager.HTTPClientForServer(serverConfig, mcp.HTTPClientOptions{TokenSource: tokenSource, DirectConnect: true})
		if err != nil {
			return fmt.Errorf("failed to get HTTP client for server: %w", err)
		}

		u, err := url.Parse(serverConfig.URL)
		if err != nil {
			return err
		}

		hookConfig, hookServers := mcp.ServerHookConfig(serverConfig)

		audit, err := newProxyAudit(req.Request, serverConfig.AuditLogMetadata, h.auditLogCollector, req.Storage)
		if err != nil {
			return fmt.Errorf("failed to prepare MCP request audit log: %w", err)
		}

		hooks, err := newHookProcessor(req.Request, h.hookRunner, hookConfig, hookServers, audit, newHookCorrelationStore(req.Storage, serverConfig.AuditLogMetadata))
		if err != nil {
			return fmt.Errorf("failed to prepare MCP request hooks: %w", err)
		}

		audit.recordRequest()

		if body, blocked, hookErr := hooks.blockedRequest(); blocked {
			audit.recordBlockedRequest(body, hookErr)
			req.ResponseWriter.Header().Set("Content-Type", "application/json")
			req.WriteHeader(http.StatusOK)
			_, _ = req.ResponseWriter.Write(body)
			return nil
		}

		(&httputil.ReverseProxy{
			Transport: client.Transport,
			Rewrite: func(r *httputil.ProxyRequest) {
				rewriteProxyRequest(r, u)
			},
			ModifyResponse: func(resp *http.Response) error {
				if err := hooks.filterResponse(resp); err != nil {
					return err
				}
				return audit.wrapResponse(resp)
			},
			ErrorHandler: func(w http.ResponseWriter, _ *http.Request, err error) {
				audit.recordTransportError(err, http.StatusBadGateway)
				if serverConfig.NanobotAgentName != "" {
					http.Error(w, fmt.Sprintf("failed to proxy request to Nanobot agent %s: %v", serverConfig.NanobotAgentName, err), http.StatusBadGateway)
				} else {
					mcpServerName := serverConfig.MCPServerDisplayName
					if mcpServerName == "" {
						mcpServerName = serverConfig.MCPServerName
					}
					http.Error(w, fmt.Sprintf("failed to proxy request to MCP server %s: %v", mcpServerName, err), http.StatusBadGateway)
				}
			},
		}).ServeHTTP(req.ResponseWriter, req.Request)

		return nil
	}

	for i := range serverConfig.PassthroughHeaderNames {
		if i < len(serverConfig.PassthroughHeaderValues) {
			req.Request.Header.Set(serverConfig.PassthroughHeaderNames[i], serverConfig.PassthroughHeaderValues[i])
		}
	}

	nanobotCtx := ntypes.Context{
		Config: func(context.Context, string) (ntypes.Config, error) {
			return mcp.ServerNanobotConfig(serverConfig), nil
		},
	}

	ctx := req.Context()
	ctx = ntypes.WithNanobotContext(ctx, nanobotCtx)
	// Don't audit log composite loopback requests, they are internal to the MCP gateway
	ctx = nmcp.WithAuditLogMetadata(ctx, map[string]string{mcp.AuditLogIgnore: "true"})
	ctx = nmcp.WithToken(ctx, strings.TrimPrefix(req.Request.Header.Get("Authorization"), "Bearer "))

	h.nanobot.ServeHTTP(req.ResponseWriter, req.WithContext(ctx))
	return nil
}

func rewriteProxyRequest(r *httputil.ProxyRequest, upstreamURL *url.URL) {
	// Authorization authenticates the client to Obot and must not be forwarded
	// to the upstream MCP server. The transport adds any configured or OAuth
	// Authorization header after this rewrite.
	r.Out.Header.Del("Authorization")

	// SetXForwarded preserves the X-Forwarded-For handling that ReverseProxy
	// applied automatically under the deprecated Director. It also writes
	// X-Forwarded-Host and X-Forwarded-Proto, so the values this handler cares
	// about are re-applied afterwards: the scheme is derived from the inbound
	// host rather than from whether this hop happens to be TLS.
	r.SetXForwarded()

	r.Out.Header.Set("X-Forwarded-Host", r.In.Host)
	scheme := "https"
	if strings.HasPrefix(r.In.Host, "localhost") || strings.HasPrefix(r.In.Host, "127.0.0.1") || strings.HasPrefix(r.In.Host, "[::1]") {
		scheme = "http"
	}
	r.Out.Header.Set("X-Forwarded-Proto", scheme)

	r.Out.Host = upstreamURL.Host
	r.Out.URL.Scheme = upstreamURL.Scheme
	r.Out.URL.Host = upstreamURL.Host
	r.Out.URL.Path = upstreamURL.Path
	if rest := r.In.PathValue("rest"); rest != "" {
		if strings.HasPrefix(rest, "/") {
			r.Out.URL.Path = rest
		} else {
			r.Out.URL.Path = "/" + rest
		}
	}

	// Merge query parameters from the incoming request and the upstream URL.
	// Preserve all values; if a key exists in both, both values will be present.
	upstreamQuery := upstreamURL.Query()
	origQuery := r.In.URL.Query()
	for k, vs := range origQuery {
		for _, v := range vs {
			upstreamQuery.Add(k, v)
		}
	}
	r.Out.URL.RawQuery = upstreamQuery.Encode()
}

func (h *Handler) ensureServerIsDeployed(req api.Context) (mcp.ServerConfig, error) {
	mcpID := req.PathValue("mcp_id")

	if system.IsSystemMCPServerID(mcpID) {
		return h.ensureSystemServerIsDeployed(req, mcpID)
	}

	mcpID, mcpServer, mcpServerConfig, missingConfig, err := h.mcpSessionManager.ServerForActionWithConnectIDAllowMissingConfig(req.Context(), mcpID, principal.ResourceOwnerID(req.User))
	if err != nil {
		return mcp.ServerConfig{}, fmt.Errorf("failed to get mcp server config: %w", err)
	}
	if mcpServer.Spec.Template {
		return mcp.ServerConfig{}, apierrors.NewNotFound(schema.GroupResource{Group: "obot.obot.ai", Resource: "mcpserver"}, mcpID)
	}
	if len(missingConfig) > 0 {
		writeMCPAuthRequired(req, true)
		return mcp.ServerConfig{}, errMCPServerRequiresConfiguration
	}

	// Add-hoc authorization for nanobot agents
	if mcpServerConfig.NanobotAgentName != "" {
		var agent v1.NanobotAgent
		if err = req.Get(&agent, mcpServerConfig.NanobotAgentName); err != nil {
			return mcp.ServerConfig{}, fmt.Errorf("failed to get nanobot agent %q: %w", mcpServerConfig.NanobotAgentName, err)
		}
		if agent.Spec.UserID != req.User.GetUID() && (!req.UserCanImpersonate() || !req.UserIsAdmin()) {
			return mcp.ServerConfig{}, types.NewErrForbidden("user is not authorized to access nanobot agent %q", mcpServerConfig.NanobotAgentName)
		}
	}

	mcpServerConfig, err = h.mcpSessionManager.LaunchServer(req.Context(), mcpServerConfig)
	if err != nil {
		return mcp.ServerConfig{}, fmt.Errorf("failed to launch mcp server: %w", err)
	}

	return mcpServerConfig, nil
}

func writeMCPAuthRequired(req api.Context, requiresConfig bool) {
	baseURL := strings.TrimSuffix(req.APIBaseURL, "/api")
	connectPath := "mcp-connect"
	if strings.HasPrefix(req.URL.Path, "/mcp-connect-composite/") {
		connectPath = "mcp-connect-composite"
	}

	req.ResponseWriter.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer realm="Obot MCP Gateway", resource_metadata="%s/.well-known/oauth-protected-resource/%s/%s"`, baseURL, connectPath, req.PathValue("mcp_id")))
	if requiresConfig {
		http.Error(req.ResponseWriter, "MCP server requires configuration", http.StatusUnauthorized)
	} else {
		http.Error(req.ResponseWriter, "MCP server requires authentication", http.StatusUnauthorized)
	}
}

func (h *Handler) ensureSystemServerIsDeployed(req api.Context, mcpID string) (mcp.ServerConfig, error) {
	var systemServer v1.SystemMCPServer
	if err := req.Get(&systemServer, mcpID); err != nil {
		return mcp.ServerConfig{}, fmt.Errorf("failed to get system MCP server %q: %w", mcpID, err)
	}

	if systemServer.Spec.Manifest.Enabled != nil && !*systemServer.Spec.Manifest.Enabled {
		return mcp.ServerConfig{}, apierrors.NewNotFound(schema.GroupResource{Group: "obot.obot.ai", Resource: "systemmcpserver"}, mcpID)
	}

	// Only look up credentials if the manifest has env vars without static values.
	// This avoids expensive credential lookups on the hot path for servers like
	// obot-mcp-server where all env vars have static values.
	credEnv := make(map[string]string)
	var needsCredentials bool
	for _, env := range systemServer.Spec.Manifest.Env {
		if env.Value == "" {
			needsCredentials = true
			break
		}
	}

	if needsCredentials {
		credCtx := systemServer.Name
		creds, err := req.GatewayClient.ListCredentials(req.Context(), gateway.ListCredentialsOptions{
			CredentialContexts: []string{credCtx},
		})
		if err != nil {
			return mcp.ServerConfig{}, fmt.Errorf("failed to list credentials for system server: %w", err)
		}

		secretToolName := systemmcpserver.SecretInfoToolName(systemServer.Name)
		for _, cred := range creds {
			// Skip the secret info credential — those vars go to the shim only, not the MCP server.
			if cred.Name == secretToolName {
				continue
			}
			credDetail, err := req.GatewayClient.RevealCredential(req.Context(), []string{credCtx}, cred.Name)
			if err != nil {
				continue
			}
			maps.Copy(credEnv, credDetail.Secrets)
		}
	}

	// Retrieve the token exchange credential
	var secretsCred map[string]string
	tokenExchangeCred, err := req.GatewayClient.RevealCredential(req.Context(), []string{systemServer.Name}, systemmcpserver.SecretInfoToolName(systemServer.Name))
	if err == nil {
		secretsCred = tokenExchangeCred.Secrets
	}

	credEnv, err = mcp.MergeBoundCreds(req.Context(), req.LocalK8sClient, req.ObotNamespace, systemServer.Spec.Manifest.Env, systemServer.Spec.Manifest.RemoteConfig, credEnv, h.secretBindingAllowedLabel)
	if err != nil {
		return mcp.ServerConfig{}, fmt.Errorf("failed to resolve secret bindings: %w", err)
	}

	baseURL := strings.TrimSuffix(req.APIBaseURL, "/api")
	audiences := systemServer.ValidConnectURLs(baseURL)

	// Ownership, not the acting identity: a system server deployed for an agent
	// belongs to the person who created that agent.
	serverConfig, _, err := mcp.SystemServerToServerConfig(systemServer, audiences, principal.ResourceOwnerID(req.User), credEnv, secretsCred)
	if err != nil {
		return mcp.ServerConfig{}, fmt.Errorf("failed to convert system server to config: %w", err)
	}

	serverConfig, err = h.mcpSessionManager.LaunchServer(req.Context(), serverConfig)
	if err != nil {
		return mcp.ServerConfig{}, fmt.Errorf("failed to launch system MCP server: %w", err)
	}

	return serverConfig, nil
}
