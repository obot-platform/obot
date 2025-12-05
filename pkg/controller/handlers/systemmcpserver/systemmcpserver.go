package systemmcpserver

import (
	"context"
	"fmt"

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

	// Check if server should be deployed
	if !systemServer.Spec.Manifest.Enabled {
		// Server is disabled, ensure it's not deployed
		return nil
	}

	// Check if server is fully configured
	if !isSystemServerConfigured(req.Ctx, h.gptClient, *systemServer) {
		// Server is not fully configured, cannot deploy
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
		// Still missing required configuration
		return nil
	}

	// TODO: use the backend to deploy

	return nil
}

// CleanupDeployment handles cleanup when SystemMCPServer is deleted
func (h *Handler) CleanupDeployment(req router.Request, _ router.Response) error {
	systemServer := req.Object.(*v1.SystemMCPServer)

	// Only cleanup if being deleted
	if systemServer.DeletionTimestamp == nil {
		return nil
	}

	// Shutdown deployment via backend
	// This would call the backend's shutdownServer method
	// For now, this is a placeholder as the backend integration would need more work
	// TODO: use the backend to remove the deployment

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
		return false
	}

	credMap := make(map[string]bool)
	for _, cred := range creds {
		credMap[cred.ToolName] = true
	}

	for _, env := range server.Spec.Manifest.Env {
		if env.Required && env.Sensitive && !credMap[env.Key] {
			return false
		}
	}

	if server.Spec.Manifest.RemoteConfig != nil {
		for _, header := range server.Spec.Manifest.RemoteConfig.Headers {
			if header.Required && header.Sensitive && !credMap[header.Key] {
				return false
			}
		}
	}

	return true
}
