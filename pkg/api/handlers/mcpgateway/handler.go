package mcpgateway

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/nanobot-ai/nanobot/pkg/log"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/api/handlers"
	"github.com/obot-platform/obot/pkg/mcp"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type Handler struct {
	storageClient             kclient.Client
	mcpSessionManager         *mcp.SessionManager
	webhookHelper             *mcp.WebhookHelper
	updateRequestTimeInterval time.Duration
}

func NewHandler(storageClient kclient.Client, mcpSessionManager *mcp.SessionManager, webhookHelper *mcp.WebhookHelper, idleServerShutdownInterval time.Duration) *Handler {
	return &Handler{
		storageClient:             storageClient,
		mcpSessionManager:         mcpSessionManager,
		webhookHelper:             webhookHelper,
		updateRequestTimeInterval: min(idleServerShutdownInterval/4, 15*time.Minute),
	}
}

func (h *Handler) Proxy(req api.Context) error {
	if req.User.GetUID() == "anonymous" {
		req.ResponseWriter.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer error="invalid_request", error_description="Invalid access token", resource_metadata="%s/.well-known/oauth-protected-resource%s"`, strings.TrimSuffix(req.APIBaseURL, "/api"), req.URL.Path))
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

	go func() {
		// Best effort to update the last request time.
		// Don't update on every request, but one quarter of the time where we would shutdown the server.
		if time.Since(mcpServer.Status.LastRequestTime.Time) > h.updateRequestTimeInterval {
			mcpServer.Status.LastRequestTime = metav1.Now()
			if err := req.Storage.Status().Update(context.Background(), &mcpServer); err != nil {
				log.Errorf(req.Context(), "failed to update mcp server status: %v", err)
			}
		}
	}()

	return h.mcpSessionManager.LaunchServer(req.Context(), mcpServerConfig)
}
