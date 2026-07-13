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
	"github.com/obot-platform/obot/pkg/controller/handlers/systemmcpserver"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type Handler struct {
	mcpSessionManager         *mcp.SessionManager
	transport                 http.RoundTripper
	nanobot                   http.Handler
	secretBindingAllowedLabel string
}

var errMCPServerRequiresConfiguration = errors.New("mcp server requires configuration")

func NewHandler(ctx context.Context, mcpSessionManager *mcp.SessionManager, auditLogCollector auditlogs.Collector, serverURL, dsn, secretBindingAllowedLabel string) (*Handler, error) {
	sessionStore, err := session.NewStoreFromDSN(nanobotSessionStoreDSN(dsn))
	if err != nil {
		return nil, fmt.Errorf("failed to create session store: %w", err)
	}

	// TODO(thedadams): do we want to make this gcPeriod configurable?
	sessionManager := session.NewManager(ctx, sessionStore, 24*7*time.Hour)
	remoteValidationConfig, allowedHosts := mcpSessionManager.RemoteConfigForBackend()

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

	return &Handler{
		mcpSessionManager:         mcpSessionManager,
		transport:                 otelhttp.NewTransport(http.DefaultTransport),
		nanobot:                   nanobotHTTPServer,
		secretBindingAllowedLabel: secretBindingAllowedLabel,
	}, nil
}

func nanobotSessionStoreDSN(dsn string) string {
	if sqliteDSN, ok := strings.CutPrefix(dsn, "sqlite://"); ok {
		return "sqlite:" + sqliteDSN
	}
	return dsn
}

func (h *Handler) Proxy(req api.Context) error {
	if !req.UserIsAuthenticated() {
		writeMCPAuthRequired(req)
		return nil
	}

	serverConfig, err := h.ensureServerIsDeployed(req)
	if err != nil {
		if errors.Is(err, errMCPServerRequiresConfiguration) {
			return nil
		}
		return fmt.Errorf("failed to ensure server is deployed: %v", err)
	}

	if serverConfig.NanobotAgentName != "" {
		// We need to just reverse-proxy to the nanobot agent because the UI will make non-MCP requests
		u, err := url.Parse(serverConfig.URL)
		if err != nil {
			http.Error(req.ResponseWriter, err.Error(), http.StatusInternalServerError)
			return nil
		}

		(&httputil.ReverseProxy{
			Transport: h.transport,
			Director: func(r *http.Request) {
				r.Header.Set("X-Forwarded-Host", r.Host)
				scheme := "https"
				if strings.HasPrefix(r.Host, "localhost") || strings.HasPrefix(r.Host, "127.0.0.1") {
					scheme = "http"
				}
				r.Header.Set("X-Forwarded-Proto", scheme)

				r.Host = u.Host
				r.URL.Scheme = u.Scheme
				r.URL.Host = u.Host
				r.URL.Path = u.Path
				if rest := r.PathValue("rest"); rest != "" {
					if strings.HasPrefix(rest, "/") {
						r.URL.Path = rest
					} else {
						r.URL.Path = "/" + rest
					}
				}

				// Merge query parameters from the incoming request and the upstream URL.
				// Preserve all values; if a key exists in both, both values will be present.
				upstreamQuery := u.Query()
				origQuery := r.URL.Query()
				for k, vs := range origQuery {
					for _, v := range vs {
						upstreamQuery.Add(k, v)
					}
				}
				r.URL.RawQuery = upstreamQuery.Encode()
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

	nanobotCtx := ntypes.Context{
		Config: func(context.Context, string) (ntypes.Config, error) {
			return mcp.ServerNanobotConfig(serverConfig), nil
		},
	}

	ctx := req.Context()
	ctx = ntypes.WithNanobotContext(ctx, nanobotCtx)
	ctx = nmcp.WithAuditLogMetadata(ctx, serverConfig.AuditLogMetadata)
	ctx = nmcp.WithToken(ctx, strings.TrimPrefix(req.Request.Header.Get("Authorization"), "Bearer "))

	h.nanobot.ServeHTTP(req.ResponseWriter, req.WithContext(ctx))
	return nil
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
		writeMCPAuthRequired(req)
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

func writeMCPAuthRequired(req api.Context) {
	baseURL := strings.TrimSuffix(req.APIBaseURL, "/api")
	req.ResponseWriter.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer realm="Obot MCP Gateway", resource_metadata="%s/.well-known/oauth-protected-resource/mcp-connect/%s"`, baseURL, req.PathValue("mcp_id")))
	http.Error(req.ResponseWriter, "MCP server requires configuration", http.StatusUnauthorized)
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

	serverConfig, _, err := mcp.SystemServerToServerConfig(systemServer, audiences, baseURL, req.User.GetUID(), credEnv, secretsCred)
	if err != nil {
		return mcp.ServerConfig{}, fmt.Errorf("failed to convert system server to config: %w", err)
	}

	serverConfig, err = h.mcpSessionManager.LaunchServer(req.Context(), serverConfig)
	if err != nil {
		return mcp.ServerConfig{}, fmt.Errorf("failed to launch system MCP server: %w", err)
	}

	return serverConfig, nil
}
