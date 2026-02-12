package mcpgateway

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/api/handlers"
	"github.com/obot-platform/obot/pkg/controller/handlers/systemmcpserver"
	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type Handler struct {
	storageClient             kclient.Client
	gptClient                 *gptscript.GPTScript
	mcpSessionManager         *mcp.SessionManager
	webhookHelper             *mcp.WebhookHelper
	nanobotIntegrationEnabled bool
	scope                     string
}

func NewHandler(storageClient kclient.Client, gptClient *gptscript.GPTScript, mcpSessionManager *mcp.SessionManager, webhookHelper *mcp.WebhookHelper, scopesSupported []string, nanobotIntegrationEnabled bool) *Handler {
	var scope string
	if len(scopesSupported) > 0 {
		scope = fmt.Sprintf(", scope=\"%s\"", strings.Join(scopesSupported, " "))
	}
	return &Handler{
		storageClient:             storageClient,
		gptClient:                 gptClient,
		mcpSessionManager:         mcpSessionManager,
		webhookHelper:             webhookHelper,
		nanobotIntegrationEnabled: nanobotIntegrationEnabled,
		scope:                     scope,
	}
}

func (h *Handler) Proxy(req api.Context) error {
	if req.User.GetUID() == "anonymous" {
		req.ResponseWriter.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer error="invalid_request", error_description="Invalid access token", resource_metadata="%s/.well-known/oauth-protected-resource%s"%s`, strings.TrimSuffix(req.APIBaseURL, "/api"), req.URL.Path, h.scope))
		return apierrors.NewUnauthorized("user is not authenticated")
	}

	mcpURL, allowDifferentPaths, extraHeaders, err := h.ensureServerIsDeployed(req)
	if err != nil {
		return fmt.Errorf("failed to ensure server is deployed: %v", err)
	}

	u, err := url.Parse(mcpURL)
	if err != nil {
		http.Error(req.ResponseWriter, err.Error(), http.StatusInternalServerError)
	}

	(&httputil.ReverseProxy{
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

			// Apply extra headers (e.g., for direct proxy to remote SystemMCPServers).
			for k, v := range extraHeaders {
				r.Header.Set(k, v)
			}
		},
	}).ServeHTTP(req.ResponseWriter, req.Request)

	return nil
}

func (h *Handler) ensureServerIsDeployed(req api.Context) (string, bool, map[string]string, error) {
	mcpID := req.PathValue("mcp_id")

	if system.IsSystemMCPServerID(mcpID) {
		return h.ensureSystemServerIsDeployed(req, mcpID)
	}

	mcpID, mcpServer, mcpServerConfig, err := handlers.ServerForActionWithConnectID(req, mcpID)
	if err != nil {
		return "", false, nil, fmt.Errorf("failed to get mcp server config: %w", err)
	}

	if mcpServer.Spec.Template {
		return "", false, nil, apierrors.NewNotFound(schema.GroupResource{Group: "obot.obot.ai", Resource: "mcpserver"}, mcpID)
	}

	// Add-hoc authorization for nanobot agents
	if h.nanobotIntegrationEnabled && mcpServerConfig.NanobotAgentName != "" {
		var agent v1.NanobotAgent
		if err = req.Get(&agent, mcpServerConfig.NanobotAgentName); err != nil {
			return "", false, nil, fmt.Errorf("failed to get nanobot agent %q: %w", mcpServerConfig.NanobotAgentName, err)
		}
		if agent.Spec.UserID != req.User.GetUID() {
			return "", false, nil, types.NewErrForbidden("user is not authorized to access nanobot agent %q", mcpServerConfig.NanobotAgentName)
		}
	}

	url, err := h.mcpSessionManager.LaunchServer(req.Context(), mcpServerConfig)
	if err != nil {
		return "", false, nil, fmt.Errorf("failed to launch mcp server: %w", err)
	}

	return url, h.nanobotIntegrationEnabled && mcpServerConfig.NanobotAgentName != "", nil, nil
}

func (h *Handler) ensureSystemServerIsDeployed(req api.Context, mcpID string) (string, bool, map[string]string, error) {
	var systemServer v1.SystemMCPServer
	if err := h.storageClient.Get(req.Context(), kclient.ObjectKey{
		Namespace: req.Namespace(),
		Name:      mcpID,
	}, &systemServer); err != nil {
		return "", false, nil, fmt.Errorf("failed to get system MCP server %q: %w", mcpID, err)
	}

	if !systemServer.Spec.Manifest.Enabled {
		return "", false, nil, apierrors.NewNotFound(schema.GroupResource{Group: "obot.obot.ai", Resource: "systemmcpserver"}, mcpID)
	}

	// Only look up credentials if the manifest has env vars without static values.
	// This avoids expensive credential lookups on the hot path for servers like
	// obot-mcp-server where all env vars have static values.
	credEnv := make(map[string]string)
	needsCredentials := false
	for _, env := range systemServer.Spec.Manifest.Env {
		if env.Required && env.Value == "" {
			needsCredentials = true
			break
		}
	}

	if needsCredentials {
		credCtx := systemServer.Name
		creds, err := h.gptClient.ListCredentials(req.Context(), gptscript.ListCredentialsOptions{
			CredentialContexts: []string{credCtx},
		})
		if err != nil {
			return "", false, nil, fmt.Errorf("failed to list credentials for system server: %w", err)
		}

		for _, cred := range creds {
			credDetail, err := h.gptClient.RevealCredential(req.Context(), []string{credCtx}, cred.ToolName)
			if err != nil {
				continue
			}
			for k, v := range credDetail.Env {
				credEnv[k] = v
			}
		}
	}

	// Retrieve the token exchange credential
	var secretsCred map[string]string
	tokenExchangeCred, err := h.gptClient.RevealCredential(req.Context(), []string{systemServer.Name}, systemmcpserver.SecretInfoToolName(systemServer.Name))
	if err == nil {
		secretsCred = tokenExchangeCred.Env
	}

	baseURL := strings.TrimSuffix(req.APIBaseURL, "/api")
	audiences := systemServer.ValidConnectURLs(baseURL)

	serverConfig, _, err := mcp.SystemServerToServerConfig(systemServer, audiences, baseURL, credEnv, secretsCred)
	if err != nil {
		return "", false, nil, fmt.Errorf("failed to convert system server to config: %w", err)
	}

	mcpURL, err := h.mcpSessionManager.LaunchServer(req.Context(), serverConfig)
	if err != nil {
		return "", false, nil, fmt.Errorf("failed to launch system MCP server: %w", err)
	}

	return mcpURL, false, nil, nil
}
