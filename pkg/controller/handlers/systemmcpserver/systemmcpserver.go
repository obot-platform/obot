package systemmcpserver

import (
	"context"
	"fmt"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/logger"
	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
)

var log = logger.Package()

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

	log.Infof("EnsureDeployment called for system MCP server %s (enabled=%v, runtime=%s)",
		systemServer.Name, systemServer.Spec.Manifest.Enabled, systemServer.Spec.Manifest.Runtime)

	// Check if server should be deployed
	if !systemServer.Spec.Manifest.Enabled {
		log.Infof("System MCP server %s is disabled, shutting down any existing deployment", systemServer.Name)
		// Server is disabled, ensure any existing deployment is removed
		err := h.mcpSessionManager.ShutdownServer(req.Ctx, systemServer.Name)
		if err != nil {
			return fmt.Errorf("failed to shutdown disabled system MCP server: %w", err)
		}
		return nil
	}

	// Check if server is fully configured
	if !isSystemServerConfigured(req.Ctx, h.gptClient, *systemServer) {
		log.Infof("System MCP server %s is not fully configured, shutting down any existing deployment", systemServer.Name)
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
		log.Infof("System MCP server %s still has missing required configuration: %v",
			systemServer.Name, missingRequired)
		// Still missing required configuration
		return nil
	}

	log.Infof("Launching system MCP server %s (runtime=%s, image=%s)",
		systemServer.Name, serverConfig.Runtime, serverConfig.ContainerImage)

	// Deploy the system server via backend
	// System servers don't use webhooks, so pass nil
	_, err = h.mcpSessionManager.LaunchServer(req.Ctx, serverConfig)
	if err != nil {
		return fmt.Errorf("failed to deploy system MCP server: %w", err)
	}

	log.Infof("System MCP server %s launched successfully", systemServer.Name)

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
		log.Infof("Failed to list credentials for system MCP server %s configuration check: %v",
			server.Name, err)
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
			log.Infof("System MCP server %s missing required env var %s",
				server.Name, env.Key)
			return false
		}
	}

	return true
}
