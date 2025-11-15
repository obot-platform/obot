package systemmcpserver

import (
	"errors"
	"fmt"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
)

type Handler struct {
	sessionManager *mcp.SessionManager
	gptClient      *gptscript.GPTScript
}

func New(sessionManager *mcp.SessionManager, gptClient *gptscript.GPTScript) *Handler {
	return &Handler{
		sessionManager: sessionManager,
		gptClient:      gptClient,
	}
}

// EnsureDeployment ensures that enabled and configured SystemMCPServers are deployed,
// and that disabled or misconfigured servers are not deployed.
func (h *Handler) EnsureDeployment(req router.Request, _ router.Response) error {
	systemServer := req.Object.(*v1.SystemMCPServer)

	if systemServer.Spec.Manifest.Runtime == types.RuntimeRemote {
		// Nothing to deploy for remote runtime
		// TODO(g-linville): test to see if you can update an existing sysms1 to remote after it is local first, leaving behind an orphaned deployment.
		// If that works, check if normal MCP servers can do that too.
		return nil
	}

	// Skip deployment if not enabled
	if !systemServer.Spec.SystemServerSettings.IsEnabled {
		// Ensure any existing deployment is shut down
		if systemServer.Status.DeploymentStatus != "" {
			serverConfig, err := h.systemServerToServerConfig(systemServer, nil)
			if err == nil {
				_ = h.sessionManager.ShutdownServer(req.Ctx, serverConfig)
			}
			// Clear deployment status
			systemServer.Status.DeploymentStatus = ""
			systemServer.Status.DeploymentConditions = nil
			systemServer.Status.DeploymentAvailableReplicas = nil
			systemServer.Status.DeploymentReadyReplicas = nil
			systemServer.Status.DeploymentReplicas = nil
			return req.Client.Status().Update(req.Ctx, systemServer)
		}
		return nil
	}

	// Fetch credentials to check if server is fully configured
	credCtx := fmt.Sprintf("system-%s", systemServer.Name)
	cred, err := h.gptClient.RevealCredential(req.Ctx, []string{credCtx}, systemServer.Name)
	credEnv := make(map[string]string)
	if err == nil {
		credEnv = cred.Env
	}

	// Check for missing required configuration
	for _, env := range systemServer.Spec.Manifest.Env {
		if env.Required {
			if _, ok := credEnv[env.Key]; !ok {
				// Server is enabled but missing required configuration - don't deploy
				return nil
			}
		}
	}

	// Convert SystemMCPServer to ServerConfig with credentials
	serverConfig, err := h.systemServerToServerConfig(systemServer, credEnv)
	if err != nil {
		return fmt.Errorf("failed to convert system server to config: %w", err)
	}

	// Ensure deployment using session manager's backend
	// Use "system" as userID and server name as both display name and server name
	if _, err := h.sessionManager.EnsureDeployment(req.Ctx, serverConfig, "system", systemServer.Spec.Manifest.Name, systemServer.Name); err != nil {
		return fmt.Errorf("failed to ensure deployment: %w", err)
	}

	return nil
}

// systemServerToServerConfig converts a SystemMCPServer and credentials into a ServerConfig
// that can be used by the MCP backend for deployment.
func (h *Handler) systemServerToServerConfig(systemServer *v1.SystemMCPServer, credEnv map[string]string) (mcp.ServerConfig, error) {
	// TODO(g-linville): see if this can be DRY'd up
	manifest := systemServer.Spec.Manifest

	config := mcp.ServerConfig{
		Scope:   systemServer.Name, // Unique scope for system servers
		Runtime: manifest.Runtime,
		Env:     make([]string, 0, len(manifest.Env)),
		Files:   make([]mcp.File, 0),
	}

	// Process environment variables from credentials
	// Manifest defines which env vars are needed, credentials provide the values
	for _, env := range manifest.Env {
		value, ok := credEnv[env.Key]
		if !ok {
			if env.Required {
				// Skip required vars that are missing - deployment won't happen
				continue
			}
			// Non-required var is missing, skip it
			continue
		}

		if env.File {
			// File content
			config.Files = append(config.Files, mcp.File{
				Data:   value,
				EnvKey: env.Key,
			})
		} else {
			// Regular env var
			config.Env = append(config.Env, fmt.Sprintf("%s=%s", env.Key, value))
		}
	}

	// Runtime-specific configuration
	switch manifest.Runtime {
	case types.RuntimeUVX:
		if manifest.UVXConfig == nil {
			return config, fmt.Errorf("uvx config is required for uvx runtime")
		}
		config.Command = "uvx"
		if manifest.UVXConfig.Command != "" {
			config.Args = []string{"--from", manifest.UVXConfig.Package, manifest.UVXConfig.Command}
		} else {
			config.Args = []string{manifest.UVXConfig.Package}
		}
		config.Args = append(config.Args, manifest.UVXConfig.Args...)

	case types.RuntimeNPX:
		if manifest.NPXConfig == nil {
			return config, fmt.Errorf("npx config is required for npx runtime")
		}
		config.Command = "npx"
		config.Args = []string{manifest.NPXConfig.Package}
		config.Args = append(config.Args, manifest.NPXConfig.Args...)

	case types.RuntimeContainerized:
		if manifest.ContainerizedConfig == nil {
			return config, fmt.Errorf("containerized config is required for containerized runtime")
		}
		config.ContainerImage = manifest.ContainerizedConfig.Image
		config.ContainerPort = manifest.ContainerizedConfig.Port
		config.ContainerPath = manifest.ContainerizedConfig.Path
		config.Command = manifest.ContainerizedConfig.Command
		config.Args = manifest.ContainerizedConfig.Args

	case types.RuntimeRemote:
		if manifest.RemoteConfig == nil {
			return config, fmt.Errorf("remote config is required for remote runtime")
		}
		config.URL = manifest.RemoteConfig.URL
		// Convert headers to string array, using values from credentials
		config.Headers = make([]string, 0, len(manifest.RemoteConfig.Headers))
		for _, header := range manifest.RemoteConfig.Headers {
			value, ok := credEnv[header.Key]
			if !ok {
				if header.Required {
					// Skip required headers that are missing
					continue
				}
				// Non-required header is missing, skip it
				continue
			}
			if value != "" {
				config.Headers = append(config.Headers, fmt.Sprintf("%s=%s", header.Key, value))
			}
		}

	default:
		return config, fmt.Errorf("unsupported runtime: %s", manifest.Runtime)
	}

	return config, nil
}

// Cleanup is a finalizer that shuts down the deployment and removes credentials
// when a SystemMCPServer is deleted.
func (h *Handler) Cleanup(req router.Request, _ router.Response) error {
	systemServer := req.Object.(*v1.SystemMCPServer)

	// Get credentials to properly shut down the server
	credCtx := fmt.Sprintf("system-%s", systemServer.Name)
	cred, err := h.gptClient.RevealCredential(req.Ctx, []string{credCtx}, systemServer.Name)
	credEnv := make(map[string]string)
	if err == nil {
		credEnv = cred.Env
	}

	// Shut down the server deployment
	serverConfig, err := h.systemServerToServerConfig(systemServer, credEnv)
	if err == nil {
		if err := h.sessionManager.ShutdownServer(req.Ctx, serverConfig); err != nil {
			return fmt.Errorf("failed to shutdown server: %w", err)
		}
	}

	// Also try to shut down a server without credentials (in case it exists)
	serverConfig, err = h.systemServerToServerConfig(systemServer, nil)
	if err == nil {
		_ = h.sessionManager.ShutdownServer(req.Ctx, serverConfig)
	}

	// Delete credentials
	if err := h.gptClient.DeleteCredential(req.Ctx, credCtx, systemServer.Name); err != nil && !errors.As(err, &gptscript.ErrNotFound{}) {
		return fmt.Errorf("failed to delete credential: %w", err)
	}

	return nil
}
