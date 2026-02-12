package systemmcpserver

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
)

type Handler struct {
	gptClient         *gptscript.GPTScript
	mcpSessionManager *mcp.SessionManager
}

func New(gptClient *gptscript.GPTScript, mcpLoader *mcp.SessionManager) *Handler {
	return &Handler{
		gptClient:         gptClient,
		mcpSessionManager: mcpLoader,
	}
}

// EnsureDeployment automatically deploys the server if Enabled=true and fully configured
func (h *Handler) EnsureDeployment(req router.Request, _ router.Response) error {
	systemServer := req.Object.(*v1.SystemMCPServer)

	slog.Info("EnsureDeployment called for system MCP server",
		"name", systemServer.Name,
		"enabled", systemServer.Spec.Manifest.Enabled,
		"runtime", systemServer.Spec.Manifest.Runtime)

	// Check if server should be deployed
	if !systemServer.Spec.Manifest.Enabled {
		slog.Info("System MCP server is disabled, shutting down any existing deployment", "name", systemServer.Name)
		// Server is disabled, ensure any existing deployment is removed
		err := h.mcpSessionManager.ShutdownServer(req.Ctx, systemServer.Name)
		if err != nil {
			return fmt.Errorf("failed to shutdown disabled system MCP server: %w", err)
		}
		return nil
	}

	// Check if server is fully configured
	if !isSystemServerConfigured(req.Ctx, h.gptClient, *systemServer) {
		slog.Info("System MCP server is not fully configured, shutting down any existing deployment", "name", systemServer.Name)
		// Server is not fully configured, ensure any existing deployment is removed
		err := h.mcpSessionManager.ShutdownServer(req.Ctx, systemServer.Name)
		if err != nil {
			return fmt.Errorf("failed to shutdown unconfigured system MCP server: %w", err)
		}
		return nil
	}

	// Get credentials for deployment
	credCtx := systemServer.Name
	creds, err := h.gptClient.ListCredentials(req.Ctx, gptscript.ListCredentialsOptions{
		CredentialContexts: []string{credCtx},
	})
	if err != nil {
		return fmt.Errorf("failed to list credentials: %w", err)
	}

	credEnv := make(map[string]string)
	for _, cred := range creds {
		// Get credential details
		credDetail, err := h.gptClient.RevealCredential(req.Ctx, []string{credCtx}, cred.ToolName)
		if err != nil {
			continue
		}
		for k, v := range credDetail.Env {
			credEnv[k] = v
		}
	}

	// Transform to ServerConfig
	serverConfig, missingRequired, err := mcp.SystemServerToServerConfig(*systemServer, credEnv)
	if err != nil {
		return fmt.Errorf("failed to transform system server to config: %w", err)
	}

	if len(missingRequired) > 0 {
		slog.Info("System MCP server still has missing required configuration",
			"name", systemServer.Name,
			"missingRequired", missingRequired)
		// Still missing required configuration
		return nil
	}

	slog.Info("Launching system MCP server",
		"name", systemServer.Name,
		"runtime", serverConfig.Runtime,
		"image", serverConfig.ContainerImage)

	// Deploy the system server via backend
	// System servers don't use webhooks, so pass nil
	_, err = h.mcpSessionManager.LaunchServer(req.Ctx, serverConfig)
	if err != nil {
		return fmt.Errorf("failed to deploy system MCP server: %w", err)
	}

	slog.Info("System MCP server launched successfully", "name", systemServer.Name)

	return nil
}

// CleanupDeployment handles cleanup when SystemMCPServer is deleted
func (h *Handler) CleanupDeployment(req router.Request, _ router.Response) error {
	systemServer := req.Object.(*v1.SystemMCPServer)

	// Shutdown deployment via backend
	// The backend's shutdownServer will remove the deployment (Docker container or K8s deployment)
	err := h.mcpSessionManager.ShutdownServer(req.Ctx, systemServer.Name)
	if err != nil {
		return fmt.Errorf("failed to shutdown system MCP server: %w", err)
	}

	return nil
}

// isSystemServerConfigured checks if all required configuration is present
func isSystemServerConfigured(ctx context.Context, gptClient *gptscript.GPTScript, server v1.SystemMCPServer) bool {
	// Check if all required env vars are configured
	credCtx := server.Name
	creds, err := gptClient.ListCredentials(ctx, gptscript.ListCredentialsOptions{
		CredentialContexts: []string{credCtx},
	})
	if err != nil {
		slog.Info("Failed to list credentials for system MCP server configuration check",
			"name", server.Name,
			"error", err)
		return false
	}

	credEnv := make(map[string]string)
	for _, cred := range creds {
		credDetail, err := gptClient.RevealCredential(ctx, []string{credCtx}, cred.ToolName)
		if err != nil {
			continue
		}
		for k, v := range credDetail.Env {
			credEnv[k] = v
		}
	}

	for _, env := range server.Spec.Manifest.Env {
		if env.Required && env.Value == "" && credEnv[env.Key] == "" {
			slog.Info("System MCP server missing required env var",
				"name", server.Name,
				"envKey", env.Key)
			return false
		}
	}

	return true
}
