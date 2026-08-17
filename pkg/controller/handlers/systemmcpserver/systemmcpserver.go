package systemmcpserver

import (
	"context"
	"fmt"
	"log/slog"
	"maps"

	"github.com/obot-platform/nah/pkg/router"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
)

type Handler struct {
	gatewayClient     *gateway.Client
	mcpSessionManager *mcp.SessionManager
	serverURL         string
}

func New(gatewayClient *gateway.Client, mcpLoader *mcp.SessionManager, serverURL string) *Handler {
	return &Handler{
		gatewayClient:     gatewayClient,
		mcpSessionManager: mcpLoader,
		serverURL:         serverURL,
	}
}

// EnsureDeployment automatically deploys the server if Enabled=true and fully configured
func (h *Handler) EnsureDeployment(req router.Request, _ router.Response) error {
	systemServer := req.Object.(*v1.SystemMCPServer)

	slog.Info("EnsureDeployment called for system MCP server",
		"server", systemServer.Name, "enabled", systemServer.Spec.Manifest.Enabled, "runtime", systemServer.Spec.Manifest.Runtime)

	// Check if server should be deployed
	if systemServer.Spec.Manifest.Enabled != nil && !*systemServer.Spec.Manifest.Enabled {
		slog.Info("System MCP server is disabled, shutting down any existing deployment", "server", systemServer.Name)
		// Server is disabled, ensure any existing deployment is removed
		err := h.mcpSessionManager.ShutdownIdleServer(req.Ctx, systemServer.Name)
		if err != nil {
			return fmt.Errorf("failed to shutdown disabled system MCP server: %w", err)
		}
		return nil
	}

	// Check if server is fully configured
	if !IsSystemServerConfigured(req.Ctx, h.gatewayClient, *systemServer) {
		slog.Info("System MCP server is not fully configured, shutting down any existing deployment", "server", systemServer.Name)
		// Server is not fully configured, ensure any existing deployment is removed
		err := h.mcpSessionManager.ShutdownIdleServer(req.Ctx, systemServer.Name)
		if err != nil {
			return fmt.Errorf("failed to shutdown unconfigured system MCP server: %w", err)
		}
		return nil
	}

	// Get credentials for deployment
	credCtx := systemServer.Name
	creds, err := h.gatewayClient.ListCredentials(req.Ctx, gateway.ListCredentialsOptions{
		CredentialContexts: []string{credCtx},
	})
	if err != nil {
		return fmt.Errorf("failed to list credentials: %w", err)
	}

	credEnv := make(map[string]string)
	for _, cred := range creds {
		// Get credential details
		credDetail, err := h.gatewayClient.RevealCredential(req.Ctx, []string{credCtx}, cred.Name)
		if err != nil {
			continue
		}

		maps.Copy(credEnv, credDetail.Secrets)
	}

	audiences := systemServer.ValidConnectURLs(h.serverURL)

	// Transform to ServerConfig
	serverConfig, missingRequired, err := mcp.SystemServerToServerConfig(*systemServer, audiences, "", credEnv)
	if err != nil {
		return fmt.Errorf("failed to transform system server to config: %w", err)
	}

	if len(missingRequired) > 0 {
		slog.Info("System MCP server still has missing required configuration",
			"server", systemServer.Name, "missingRequired", missingRequired)
		// Still missing required configuration
		return nil
	}

	slog.Info("Launching system MCP server",
		"server", systemServer.Name, "runtime", serverConfig.Runtime, "image", serverConfig.ContainerImage)

	// Deploy the system server via backend
	// System servers don't use webhooks, so pass nil
	_, err = h.mcpSessionManager.LaunchServer(req.Ctx, serverConfig)
	if err != nil {
		return fmt.Errorf("failed to deploy system MCP server: %w", err)
	}

	slog.Info("System MCP server launched successfully", "server", systemServer.Name)

	return nil
}

// CleanupDeployment handles cleanup when SystemMCPServer is deleted
func (h *Handler) CleanupDeployment(req router.Request, _ router.Response) error {
	systemServer := req.Object.(*v1.SystemMCPServer)
	creds, err := h.gatewayClient.ListCredentials(req.Ctx, gateway.ListCredentialsOptions{
		CredentialContexts: []string{systemServer.Name},
	})
	if err != nil {
		return fmt.Errorf("failed to list credentials for %s system server cleanup: %w", systemServer.Name, err)
	}

	for _, cred := range creds {
		if _, err := h.gatewayClient.DeleteCredential(req.Ctx, cred.Context, cred.Name); err != nil {
			return fmt.Errorf("failed to delete credential %s: %w", cred.Name, err)
		}
	}

	// Shutdown deployment via backend
	// The backend's shutdownServer will remove the deployment (Docker container or K8s deployment)
	if err = h.mcpSessionManager.ShutdownServer(req.Ctx, systemServer.Name); err != nil {
		return fmt.Errorf("failed to shutdown system MCP server %s: %w", systemServer.Name, err)
	}

	return nil
}

// IsSystemServerConfigured checks if all required configuration is present
func IsSystemServerConfigured(ctx context.Context, gatewayClient *gateway.Client, server v1.SystemMCPServer) bool {
	credEnv, err := GetCredentialsForSystemServer(ctx, gatewayClient, server)
	if err != nil {
		slog.Error("Failed to get credentials for system MCP server", "server", server.Name, "error", err)
		return false
	}

	for _, env := range server.Spec.Manifest.Env {
		if env.Required && env.Value == "" && credEnv[env.Key] == "" {
			slog.Info("System MCP server missing required env var",
				"server", server.Name, "envVar", env.Key)
			return false
		}
	}

	return true
}

// GetCredentialsForSystemServer retrieves all credentials for the given system MCP server and returns them as a single map of env vars.
func GetCredentialsForSystemServer(ctx context.Context, gatewayClient *gateway.Client, server v1.SystemMCPServer) (map[string]string, error) {
	credCtx := server.Name
	creds, err := gatewayClient.ListCredentials(ctx, gateway.ListCredentialsOptions{
		CredentialContexts: []string{credCtx},
	})
	if err != nil {
		return nil, err
	}

	credEnv := make(map[string]string)
	for _, cred := range creds {
		credDetail, err := gatewayClient.RevealCredential(ctx, []string{credCtx}, cred.Name)
		if err != nil {
			continue
		}

		maps.Copy(credEnv, credDetail.Secrets)
	}

	return credEnv, nil
}
