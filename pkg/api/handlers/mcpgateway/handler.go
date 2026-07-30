package mcpgateway

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/obot-platform/nanobot/pkg/llm"
	nmcp "github.com/obot-platform/nanobot/pkg/mcp"
	"github.com/obot-platform/nanobot/pkg/mcp/auditlogs"
	"github.com/obot-platform/nanobot/pkg/runtime"
	"github.com/obot-platform/nanobot/pkg/server"
	"github.com/obot-platform/nanobot/pkg/session"
	ntypes "github.com/obot-platform/nanobot/pkg/types"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/auth"
	"github.com/obot-platform/obot/pkg/controller/handlers/systemmcpserver"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/jwt/persistent"
	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/obot-platform/obot/pkg/tunnel"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/authentication/user"
)

type Handler struct {
	mcpSessionManager         *mcp.SessionManager
	transport                 http.RoundTripper
	nanobot                   http.Handler
	mintToken                 func(context.Context, persistent.TokenContext) (string, error)
	resolveServer             func(api.Context) (mcp.ServerConfig, error)
	secretBindingAllowedLabel string
	tunnelManager             *tunnel.Manager
}

var errMCPServerRequiresConfiguration = errors.New("mcp server requires configuration")

const gatewayTokenExpiration = 5 * time.Minute

func NewHandler(ctx context.Context, mcpSessionManager *mcp.SessionManager, tokenService *persistent.TokenService, auditLogCollector auditlogs.Collector, serverURL, dsn, secretBindingAllowedLabel string, tunnelManager *tunnel.Manager) (*Handler, error) {
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

	runtime, err := runtime.NewRuntime(ctx, llm.Config{}, runtime.Options{
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

	var mcpServer nmcp.MessageHandler = server.NewServer(runtime, nil, sessionManager, server.Options{
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

	handler := &Handler{
		mcpSessionManager: mcpSessionManager,
		transport:         otelhttp.NewTransport(http.DefaultTransport),
		nanobot:           nanobotHTTPServer,
		mintToken: func(ctx context.Context, tokenContext persistent.TokenContext) (string, error) {
			_, token, err := tokenService.NewToken(ctx, tokenContext)
			return token, err
		},
		secretBindingAllowedLabel: secretBindingAllowedLabel,
		tunnelManager:             tunnelManager,
	}
	handler.resolveServer = handler.ensureServerIsDeployed
	return handler, nil
}

func (h *Handler) Proxy(req api.Context) error {
	if !req.UserIsAuthenticated() {
		writeMCPAuthRequired(req, false)
		return nil
	}

	serverConfig, err := h.resolveServer(req)
	if err != nil {
		if errors.Is(err, errMCPServerRequiresConfiguration) {
			return nil
		}
		return fmt.Errorf("failed to ensure server is deployed: %v", err)
	}

	var bridgeAuthorizationName, bridgeAuthorizationValue string
	if serverConfig.TunnelName != "" {
		if h.tunnelManager == nil {
			return fmt.Errorf("tunnel manager is not configured")
		}
		serverConfig.URL, err = h.tunnelManager.BridgeURL(serverConfig.TunnelName, serverConfig.URL)
		if err != nil {
			return fmt.Errorf("failed to prepare tunneled MCP server URL: %w", err)
		}
		bridgeAuthorizationName, bridgeAuthorizationValue = h.tunnelManager.BridgeAuthorization()
		serverConfig.Headers = append(serverConfig.Headers, fmt.Sprintf("%s=%s", bridgeAuthorizationName, bridgeAuthorizationValue))
	}

	gatewayToken, err := h.gatewayToken(req.Context(), req.User, serverConfig)
	if err != nil {
		return fmt.Errorf("failed to mint MCP gateway token: %w", err)
	}

	if serverConfig.NanobotAgentName != "" {
		// We need to just reverse-proxy to the nanobot agent because the UI will make non-MCP requests
		u, err := url.Parse(serverConfig.URL)
		if err != nil {
			http.Error(req.ResponseWriter, err.Error(), http.StatusInternalServerError)
			return nil
		}

		director := nanobotProxyDirector(u, gatewayToken)
		(&httputil.ReverseProxy{
			Transport: h.transport,
			Director: func(r *http.Request) {
				if bridgeAuthorizationName != "" {
					r.Header.Set(bridgeAuthorizationName, bridgeAuthorizationValue)
				}
				director(r)
			},
			ErrorHandler: func(w http.ResponseWriter, _ *http.Request, err error) {
				http.Error(w, fmt.Sprintf("failed to proxy request to Nanobot agent %s: %v", serverConfig.NanobotAgentName, err), http.StatusBadGateway)
			},
		}).ServeHTTP(req.ResponseWriter, req.Request)

		return nil
	}

	for i := range serverConfig.PassthroughHeaderNames {
		if i < len(serverConfig.PassthroughHeaderValues) {
			req.Request.Header.Set(serverConfig.PassthroughHeaderNames[i], serverConfig.PassthroughHeaderValues[i])
		}
	}

	isCompositeLoopback := strings.HasSuffix(req.URL.Path, "/mcp-connect-composite/"+req.PathValue("mcp_id"))
	nanobotCtx := ntypes.Context{
		Config: func(context.Context, string) (ntypes.Config, error) {
			return mcp.ServerNanobotConfig(serverConfig, isCompositeLoopback), nil
		},
	}

	ctx := req.Context()
	ctx = ntypes.WithNanobotContext(ctx, nanobotCtx)
	if isCompositeLoopback {
		// Don't audit log composite loopback requests, they are internal to the MCP gateway
		ctx = nmcp.WithAuditLogMetadata(ctx, map[string]string{mcp.AuditLogIgnore: "true"})
	} else {
		ctx = nmcp.WithAuditLogMetadata(ctx, serverConfig.AuditLogMetadata)
	}
	ctx = withGatewayToken(ctx, req.Request, gatewayToken)

	h.nanobot.ServeHTTP(req.ResponseWriter, req.WithContext(ctx))
	return nil
}

func (h *Handler) gatewayToken(ctx context.Context, authenticatedUser user.Info, server mcp.ServerConfig) (string, error) {
	if len(server.Audiences) == 0 {
		return "", nil
	}
	audience, err := gatewayAudience(server)
	if err != nil {
		return "", err
	}
	token, err := h.mintToken(ctx, gatewayTokenContext(authenticatedUser, server, audience, time.Now()))
	if err != nil {
		return "", err
	}
	return token, nil
}

func gatewayAudience(server mcp.ServerConfig) (string, error) {
	expectedPath := "/mcp-connect/" + server.MCPServerName
	for _, audience := range server.Audiences {
		parsed, err := url.Parse(audience)
		if err == nil && strings.HasSuffix(strings.TrimRight(parsed.Path, "/"), expectedPath) {
			return audience, nil
		}
	}
	return "", fmt.Errorf("no exact audience for MCP server %s", server.MCPServerName)
}

func withGatewayToken(ctx context.Context, request *http.Request, gatewayToken string) context.Context {
	if gatewayToken == "" {
		request.Header.Del("Authorization")
		return ctx
	}
	request.Header.Set("Authorization", "Bearer "+gatewayToken)
	return nmcp.WithToken(ctx, gatewayToken)
}

func gatewayTokenContext(authenticatedUser user.Info, server mcp.ServerConfig, audience string, now time.Time) persistent.TokenContext {
	extra := authenticatedUser.GetExtra()
	return persistent.TokenContext{
		Audience:              audience,
		IssuedAt:              persistent.NewTime(now),
		ExpiresAt:             persistent.NewTime(now.Add(gatewayTokenExpiration)),
		UserID:                authenticatedUser.GetUID(),
		UserName:              authenticatedUser.GetName(),
		UserEmail:             auth.FirstExtraValue(extra, "email"),
		UserGroups:            []string{types.GroupMCP, types.GroupAuthenticated},
		AuthProviderName:      auth.FirstExtraValue(extra, "auth_provider_name"),
		AuthProviderNamespace: auth.FirstExtraValue(extra, "auth_provider_namespace"),
		AuthProviderUserID:    auth.FirstExtraValue(extra, "auth_provider_user_id"),
		MCPID:                 server.MCPServerName,
	}
}

func nanobotProxyDirector(target *url.URL, gatewayToken string) func(*http.Request) {
	return func(request *http.Request) {
		if gatewayToken == "" {
			request.Header.Del("Authorization")
		} else {
			request.Header.Set("Authorization", "Bearer "+gatewayToken)
		}
		request.Header.Set("X-Forwarded-Host", request.Host)
		scheme := "https"
		if strings.HasPrefix(request.Host, "localhost") || strings.HasPrefix(request.Host, "127.0.0.1") {
			scheme = "http"
		}
		request.Header.Set("X-Forwarded-Proto", scheme)

		request.Host = target.Host
		request.URL.Scheme = target.Scheme
		request.URL.Host = target.Host
		request.URL.Path = target.Path
		if rest := request.PathValue("rest"); rest != "" {
			if strings.HasPrefix(rest, "/") {
				request.URL.Path = rest
			} else {
				request.URL.Path = "/" + rest
			}
		}

		// Merge query parameters from the incoming request and the upstream URL.
		// Preserve all values; if a key exists in both, both values will be present.
		upstreamQuery := target.Query()
		origQuery := request.URL.Query()
		for key, values := range origQuery {
			for _, value := range values {
				upstreamQuery.Add(key, value)
			}
		}
		request.URL.RawQuery = upstreamQuery.Encode()
	}
}

func (h *Handler) ensureServerIsDeployed(req api.Context) (mcp.ServerConfig, error) {
	mcpID := req.PathValue("mcp_id")

	if system.IsSystemMCPServerID(mcpID) {
		return h.ensureSystemServerIsDeployed(req, mcpID)
	}

	mcpID, mcpServer, mcpServerConfig, missingConfig, err := h.mcpSessionManager.ServerForActionWithConnectIDAllowMissingConfig(req.Context(), mcpID, req.User.GetUID())
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

	serverConfig, _, err := mcp.SystemServerToServerConfig(systemServer, audiences, req.User.GetUID(), credEnv, secretsCred)
	if err != nil {
		return mcp.ServerConfig{}, fmt.Errorf("failed to convert system server to config: %w", err)
	}

	serverConfig, err = h.mcpSessionManager.LaunchServer(req.Context(), serverConfig)
	if err != nil {
		return mcp.ServerConfig{}, fmt.Errorf("failed to launch system MCP server: %w", err)
	}

	return serverConfig, nil
}
