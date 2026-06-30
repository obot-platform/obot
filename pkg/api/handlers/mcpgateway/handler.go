package mcpgateway

import (
	"errors"
	"fmt"
	"maps"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/api/handlers"
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
	secretBindingAllowedLabel string
}

var errMCPServerRequiresConfiguration = errors.New("mcp server requires configuration")

func NewHandler(mcpSessionManager *mcp.SessionManager, secretBindingAllowedLabel string) *Handler {
	return &Handler{
		mcpSessionManager:         mcpSessionManager,
		transport:                 otelhttp.NewTransport(http.DefaultTransport),
		secretBindingAllowedLabel: secretBindingAllowedLabel,
	}
}

func (h *Handler) Proxy(req api.Context) error {
	serverConfig, mcpURL, allowDifferentPaths, err := h.ensureServerIsDeployed(req)
	if err != nil {
		if errors.Is(err, errMCPServerRequiresConfiguration) {
			return nil
		}
		return fmt.Errorf("failed to ensure server is deployed: %v", err)
	}

	u, err := url.Parse(mcpURL)
	if err != nil {
		http.Error(req.ResponseWriter, err.Error(), http.StatusInternalServerError)
		return nil
	}

	// RFC 9728 §5.1: if the upstream MCP server replies 401 without pointing the
	// client at its protected-resource metadata, advertise the path-aware
	// metadata URL for this connect endpoint. Without this, clients that don't
	// already know the path fall back to the host-root metadata document, which
	// cannot identify this specific MCP server.
	mcpID := req.PathValue("mcp_id")
	connectBaseURL := strings.TrimSuffix(req.APIBaseURL, "/api")

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
			if rest := r.PathValue("rest"); allowDifferentPaths && rest != "" {
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

			for i := range serverConfig.PassthroughHeaderNames {
				if i < len(serverConfig.PassthroughHeaderValues) {
					r.Header.Set(serverConfig.PassthroughHeaderNames[i], serverConfig.PassthroughHeaderValues[i])
				}
			}
		},
		ModifyResponse: func(resp *http.Response) error {
			if resp.StatusCode == http.StatusUnauthorized && mcpID != "" && resp.Header.Get("WWW-Authenticate") == "" {
				resp.Header.Set("WWW-Authenticate", fmt.Sprintf(`Bearer resource_metadata="%s/.well-known/oauth-protected-resource/mcp-connect/%s"`, connectBaseURL, mcpID))
			}
			return nil
		},
	}).ServeHTTP(req.ResponseWriter, req.Request)

	return nil
}

func (h *Handler) ensureServerIsDeployed(req api.Context) (mcp.ServerConfig, string, bool, error) {
	mcpID := req.PathValue("mcp_id")

	if system.IsSystemMCPServerID(mcpID) {
		return h.ensureSystemServerIsDeployed(req, mcpID)
	}

	connectID := mcpID
	mcpID, mcpServer, mcpServerConfig, missingConfig, err := handlers.ServerForActionWithConnectIDAllowMissingConfig(req, mcpID, h.secretBindingAllowedLabel)
	if err != nil {
		return mcp.ServerConfig{}, "", false, fmt.Errorf("failed to get mcp server config: %w", err)
	}
	if mcpServer.Spec.Template {
		return mcp.ServerConfig{}, "", false, apierrors.NewNotFound(schema.GroupResource{Group: "obot.obot.ai", Resource: "mcpserver"}, mcpID)
	}
	if len(missingConfig) > 0 {
		writeMCPAuthRequired(req, connectID)
		return mcp.ServerConfig{}, "", false, errMCPServerRequiresConfiguration
	}

	// Add-hoc authorization for nanobot agents
	if mcpServerConfig.NanobotAgentName != "" {
		var agent v1.NanobotAgent
		if err = req.Get(&agent, mcpServerConfig.NanobotAgentName); err != nil {
			return mcp.ServerConfig{}, "", false, fmt.Errorf("failed to get nanobot agent %q: %w", mcpServerConfig.NanobotAgentName, err)
		}
		if agent.Spec.UserID != req.User.GetUID() && (!req.UserCanImpersonate() || !req.UserIsAdmin()) {
			return mcp.ServerConfig{}, "", false, types.NewErrForbidden("user is not authorized to access nanobot agent %q", mcpServerConfig.NanobotAgentName)
		}
	}

	url, err := h.mcpSessionManager.LaunchServer(req.Context(), mcpServerConfig)
	if err != nil {
		return mcp.ServerConfig{}, "", false, fmt.Errorf("failed to launch mcp server: %w", err)
	}

	return mcpServerConfig, url, mcpServerConfig.NanobotAgentName != "", nil
}

func writeMCPAuthRequired(req api.Context, mcpID string) {
	baseURL := strings.TrimSuffix(req.APIBaseURL, "/api")
	req.ResponseWriter.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer realm="Obot MCP Gateway", resource_metadata="%s/.well-known/oauth-protected-resource/mcp-connect/%s"`, baseURL, url.PathEscape(mcpID)))
	http.Error(req.ResponseWriter, "MCP server requires configuration", http.StatusUnauthorized)
}

func (h *Handler) ensureSystemServerIsDeployed(req api.Context, mcpID string) (mcp.ServerConfig, string, bool, error) {
	var systemServer v1.SystemMCPServer
	if err := req.Get(&systemServer, mcpID); err != nil {
		return mcp.ServerConfig{}, "", false, fmt.Errorf("failed to get system MCP server %q: %w", mcpID, err)
	}

	if systemServer.Spec.Manifest.Enabled != nil && !*systemServer.Spec.Manifest.Enabled {
		return mcp.ServerConfig{}, "", false, apierrors.NewNotFound(schema.GroupResource{Group: "obot.obot.ai", Resource: "systemmcpserver"}, mcpID)
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
			return mcp.ServerConfig{}, "", false, fmt.Errorf("failed to list credentials for system server: %w", err)
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
		return mcp.ServerConfig{}, "", false, fmt.Errorf("failed to resolve secret bindings: %w", err)
	}

	baseURL := strings.TrimSuffix(req.APIBaseURL, "/api")
	audiences := systemServer.ValidConnectURLs(baseURL)

	serverConfig, _, err := mcp.SystemServerToServerConfig(systemServer, audiences, baseURL, credEnv, secretsCred)
	if err != nil {
		return mcp.ServerConfig{}, "", false, fmt.Errorf("failed to convert system server to config: %w", err)
	}

	mcpURL, err := h.mcpSessionManager.LaunchServer(req.Context(), serverConfig)
	if err != nil {
		return mcp.ServerConfig{}, "", false, fmt.Errorf("failed to launch system MCP server: %w", err)
	}

	return serverConfig, mcpURL, false, nil
}
