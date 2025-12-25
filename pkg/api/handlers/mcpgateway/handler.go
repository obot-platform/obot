package mcpgateway

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/api/handlers"
	"github.com/obot-platform/obot/pkg/mcp"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type Handler struct {
	storageClient     kclient.Client
	mcpSessionManager *mcp.SessionManager
	webhookHelper     *mcp.WebhookHelper
	scope             string
}

func NewHandler(storageClient kclient.Client, mcpSessionManager *mcp.SessionManager, webhookHelper *mcp.WebhookHelper, scopesSupported []string) *Handler {
	var scope string
	if len(scopesSupported) > 0 {
		scope = fmt.Sprintf(", scope=\"%s\"", strings.Join(scopesSupported, " "))
	}
	return &Handler{
		storageClient:     storageClient,
		mcpSessionManager: mcpSessionManager,
		webhookHelper:     webhookHelper,
		scope:             scope,
	}
}

func (h *Handler) Proxy(req api.Context) error {
	if req.User.GetUID() == "anonymous" {
		req.ResponseWriter.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer error="invalid_request", error_description="Invalid access token", resource_metadata="%s/.well-known/oauth-protected-resource%s"%s`, strings.TrimSuffix(req.APIBaseURL, "/api"), req.URL.Path, h.scope))
		return apierrors.NewUnauthorized("user is not authenticated")
	}

	mcpURL, err := h.ensureServerIsDeployed(req)
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

			r.URL.Scheme = u.Scheme
			r.URL.Host = u.Host
			r.Host = u.Host
		},
	}).ServeHTTP(req.ResponseWriter, req.Request)

	return nil
}

func (h *Handler) ensureServerIsDeployed(req api.Context) (string, error) {
	mcpID, mcpServer, mcpServerConfig, err := handlers.ServerForActionWithConnectID(req, req.PathValue("mcp_id"))
	if err != nil {
		return "", fmt.Errorf("failed to get mcp server config: %w", err)
	}

	if mcpServer.Spec.Template {
		return "", apierrors.NewNotFound(schema.GroupResource{Group: "obot.obot.ai", Resource: "mcpserver"}, mcpID)
	}

	return h.mcpSessionManager.LaunchServer(req.Context(), mcpServerConfig)
}
