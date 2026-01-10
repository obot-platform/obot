package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/obot-platform/obot/pkg/validation"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type SystemMCPServerHandler struct {
	mcpSessionManager *mcp.SessionManager
}

func NewSystemMCPServerHandler(mcpLoader *mcp.SessionManager) *SystemMCPServerHandler {
	return &SystemMCPServerHandler{
		mcpSessionManager: mcpLoader,
	}
}

// List returns all system MCP servers
func (h *SystemMCPServerHandler) List(req api.Context) error {
	var list v1.SystemMCPServerList
	if err := req.List(&list); err != nil {
		return fmt.Errorf("failed to list system MCP servers: %w", err)
	}

	servers := make([]types.SystemMCPServer, 0, len(list.Items))
	for _, server := range list.Items {
		credEnv, err := getCredentialsForSystemServer(req.Context(), req.GPTClient, server)
		if err != nil {
			return err
		}
		servers = append(servers, convertSystemMCPServer(server, credEnv))
	}

	return req.Write(types.SystemMCPServerList{Items: servers})
}

// Get returns a specific system MCP server
func (h *SystemMCPServerHandler) Get(req api.Context) error {
	var systemServer v1.SystemMCPServer
	if err := req.Get(&systemServer, req.PathValue("id")); err != nil {
		return err
	}

	credEnv, err := getCredentialsForSystemServer(req.Context(), req.GPTClient, systemServer)
	if err != nil {
		return err
	}

	return req.Write(convertSystemMCPServer(systemServer, credEnv))
}

// Create creates a new system MCP server
func (h *SystemMCPServerHandler) Create(req api.Context) error {
	var manifest types.SystemMCPServerManifest
	if err := req.Read(&manifest); err != nil {
		return types.NewErrBadRequest("invalid request body: %v", err)
	}

	// Validate manifest
	if err := validation.ValidateSystemMCPServerManifest(manifest); err != nil {
		return types.NewErrBadRequest("validation failed: %v", err)
	}

	systemServer := v1.SystemMCPServer{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: system.SystemMCPServerPrefix,
			Namespace:    req.Namespace(),
			Finalizers:   []string{v1.SystemMCPServerFinalizer},
		},
		Spec: v1.SystemMCPServerSpec{
			Manifest: manifest,
		},
	}

	if err := req.Create(&systemServer); err != nil {
		return fmt.Errorf("failed to create system MCP server: %w", err)
	}

	return req.Write(convertSystemMCPServer(systemServer, nil)) // no credentials to check for a brand new server
}

// Update updates an existing system MCP server
func (h *SystemMCPServerHandler) Update(req api.Context) error {
	var manifest types.SystemMCPServerManifest
	if err := req.Read(&manifest); err != nil {
		return types.NewErrBadRequest("invalid request body: %v", err)
	}

	// Validate manifest
	if err := validation.ValidateSystemMCPServerManifest(manifest); err != nil {
		return types.NewErrBadRequest("validation failed: %v", err)
	}

	var systemServer v1.SystemMCPServer
	if err := req.Get(&systemServer, req.PathValue("id")); err != nil {
		return err
	}

	systemServer.Spec.Manifest = manifest

	if err := req.Update(&systemServer); err != nil {
		return fmt.Errorf("failed to update system MCP server: %w", err)
	}

	credEnv, err := getCredentialsForSystemServer(req.Context(), req.GPTClient, systemServer)
	if err != nil {
		return err
	}

	return req.Write(convertSystemMCPServer(systemServer, credEnv))
}

// Delete deletes a system MCP server
func (h *SystemMCPServerHandler) Delete(req api.Context) error {
	var systemServer v1.SystemMCPServer
	if err := req.Get(&systemServer, req.PathValue("id")); err != nil {
		return err
	}

	if err := req.Delete(&systemServer); err != nil {
		return fmt.Errorf("failed to delete system MCP server: %w", err)
	}

	return req.Write(map[string]string{"deleted": systemServer.Name})
}

// Configure configures environment variables for a system MCP server
func (h *SystemMCPServerHandler) Configure(req api.Context) error {
	var envVars map[string]string
	if err := req.Read(&envVars); err != nil {
		return types.NewErrBadRequest("invalid request body: %v", err)
	}

	var systemServer v1.SystemMCPServer
	if err := req.Get(&systemServer, req.PathValue("id")); err != nil {
		return err
	}

	credCtx := systemServer.Name

	// Allow for updating credentials. The only way to update a credential is to delete the existing one and recreate it.
	if err := DeleteCredentialIfExists(req.Context(), req.GPTClient, []string{credCtx}, systemServer.Name); err != nil {
		return err
	}

	// Remove empty values
	for key, val := range envVars {
		if val == "" {
			delete(envVars, key)
		}
	}

	// Store credentials using GPTScript
	if err := req.GPTClient.CreateCredential(req.Context(), gptscript.Credential{
		Context:  credCtx,
		ToolName: systemServer.Name,
		Type:     gptscript.CredentialTypeTool,
		Env:      envVars,
	}); err != nil {
		return fmt.Errorf("failed to create credential: %w", err)
	}

	// Update annotation to track configuration timestamp
	if systemServer.Annotations == nil {
		systemServer.Annotations = make(map[string]string, 1)
	}
	systemServer.Annotations["obot.obot.ai/configured-at"] = metav1.Now().Format(time.RFC3339)

	if err := req.Update(&systemServer); err != nil {
		return fmt.Errorf("failed to update system MCP server: %w", err)
	}

	credEnv, err := getCredentialsForSystemServer(req.Context(), req.GPTClient, systemServer)
	if err != nil {
		return err
	}

	return req.Write(convertSystemMCPServer(systemServer, credEnv))
}

// Deconfigure clears configuration for a system MCP server
func (h *SystemMCPServerHandler) Deconfigure(req api.Context) error {
	var systemServer v1.SystemMCPServer
	if err := req.Get(&systemServer, req.PathValue("id")); err != nil {
		return err
	}

	credCtx := systemServer.Name

	// Delete credentials using GPTScript
	if err := DeleteCredentialIfExists(req.Context(), req.GPTClient, []string{credCtx}, systemServer.Name); err != nil {
		return err
	}

	// Remove configuration annotation
	if systemServer.Annotations != nil {
		delete(systemServer.Annotations, "obot.obot.ai/configured-at")
	}

	if err := req.Update(&systemServer); err != nil {
		return fmt.Errorf("failed to update system MCP server: %w", err)
	}

	credEnv, err := getCredentialsForSystemServer(req.Context(), req.GPTClient, systemServer)
	if err != nil {
		return err
	}

	return req.Write(convertSystemMCPServer(systemServer, credEnv))
}

// Restart restarts a system MCP server deployment
func (h *SystemMCPServerHandler) Restart(req api.Context) error {
	var systemServer v1.SystemMCPServer
	if err := req.Get(&systemServer, req.PathValue("id")); err != nil {
		return err
	}

	// Check if server is both enabled and configured
	if err := h.checkEnabledAndConfigured(req.Context(), req.GPTClient, systemServer); err != nil {
		return err
	}

	credEnv, err := getCredentialsForSystemServer(req.Context(), req.GPTClient, systemServer)
	if err != nil {
		return err
	}

	// Transform to ServerConfig
	serverConfig, _, err := mcp.SystemServerToServerConfig(systemServer, credEnv)
	if err != nil {
		return types.NewErrBadRequest("failed to transform system server to config: %v", err)
	}

	// Restart the deployment via the session manager
	if err := h.mcpSessionManager.RestartServerDeployment(req.Context(), serverConfig); err != nil {
		if nse := (*mcp.ErrNotSupportedByBackend)(nil); errors.As(err, &nse) {
			return types.NewErrNotFound(nse.Error())
		}
		return fmt.Errorf("failed to restart system MCP server: %w", err)
	}

	req.WriteHeader(http.StatusNoContent)
	return nil
}

// Logs streams logs from a system MCP server
func (h *SystemMCPServerHandler) Logs(req api.Context) error {
	var systemServer v1.SystemMCPServer
	if err := req.Get(&systemServer, req.PathValue("id")); err != nil {
		return err
	}

	// Check if server is both enabled and configured
	if err := h.checkEnabledAndConfigured(req.Context(), req.GPTClient, systemServer); err != nil {
		return err
	}

	credEnv, err := getCredentialsForSystemServer(req.Context(), req.GPTClient, systemServer)
	if err != nil {
		return err
	}

	// Transform to ServerConfig
	serverConfig, _, err := mcp.SystemServerToServerConfig(systemServer, credEnv)
	if err != nil {
		return types.NewErrBadRequest("failed to transform system server to config: %v", err)
	}

	logs, err := h.mcpSessionManager.StreamServerLogs(req.Context(), serverConfig)
	if err != nil {
		if nse := (*mcp.ErrNotSupportedByBackend)(nil); errors.As(err, &nse) {
			return types.NewErrNotFound(nse.Error())
		}
		return err
	}

	// Stream logs using the helper (handles SSE formatting, Docker header stripping, etc.)
	return StreamLogs(req.Context(), req.ResponseWriter, logs, StreamLogsOptions{
		SendKeepAlive:  true,
		SendDisconnect: true,
		SendEnded:      true,
	})
}

// GetTools returns the tools provided by a system MCP server
func (h *SystemMCPServerHandler) GetTools(req api.Context) error {
	var systemServer v1.SystemMCPServer
	if err := req.Get(&systemServer, req.PathValue("id")); err != nil {
		return err
	}

	// Check if server is both enabled and configured
	if err := h.checkEnabledAndConfigured(req.Context(), req.GPTClient, systemServer); err != nil {
		return err
	}

	credEnv, err := getCredentialsForSystemServer(req.Context(), req.GPTClient, systemServer)
	if err != nil {
		return err
	}

	// Transform to ServerConfig
	serverConfig, _, err := mcp.SystemServerToServerConfig(systemServer, credEnv)
	if err != nil {
		return types.NewErrBadRequest("failed to transform system server to config: %v", err)
	}

	// Get server capabilities
	caps, err := h.mcpSessionManager.ServerCapabilities(req.Context(), serverConfig)
	if err != nil {
		if nse := (*mcp.ErrNotSupportedByBackend)(nil); errors.As(err, &nse) {
			return types.NewErrHTTP(http.StatusBadRequest, nse.Error())
		}
		return err
	}

	if caps.Tools == nil {
		return types.NewErrBadRequest("MCP server does not support tools")
	}

	// List tools from the server
	tools, err := h.mcpSessionManager.ListTools(req.Context(), serverConfig)
	if err != nil {
		if nse := (*mcp.ErrNotSupportedByBackend)(nil); errors.As(err, &nse) {
			return types.NewErrHTTP(http.StatusBadRequest, nse.Error())
		}
		return err
	}

	// Convert tools to API types
	convertedTools, err := mcp.ConvertTools(tools, nil, nil)
	if err != nil {
		return fmt.Errorf("failed to convert tools: %w", err)
	}

	return req.Write(convertedTools)
}

// GetDetails returns deployment details for a system MCP server
func (h *SystemMCPServerHandler) GetDetails(req api.Context) error {
	var systemServer v1.SystemMCPServer
	if err := req.Get(&systemServer, req.PathValue("id")); err != nil {
		return err
	}

	// Check if server is both enabled and configured
	if err := h.checkEnabledAndConfigured(req.Context(), req.GPTClient, systemServer); err != nil {
		return err
	}

	credEnv, err := getCredentialsForSystemServer(req.Context(), req.GPTClient, systemServer)
	if err != nil {
		return err
	}

	// Transform to ServerConfig
	serverConfig, _, err := mcp.SystemServerToServerConfig(systemServer, credEnv)
	if err != nil {
		return types.NewErrBadRequest("failed to transform system server to config: %v", err)
	}

	// Get server details from the session manager
	details, err := h.mcpSessionManager.GetServerDetails(req.Context(), serverConfig)
	if err != nil {
		if nse := (*mcp.ErrNotSupportedByBackend)(nil); errors.As(err, &nse) {
			return types.NewErrNotFound(nse.Error())
		}
		return fmt.Errorf("failed to get server details: %w", err)
	}

	return req.Write(details)
}

// Reveal returns the configuration values (env vars) for a system MCP server
func (h *SystemMCPServerHandler) Reveal(req api.Context) error {
	var systemServer v1.SystemMCPServer
	if err := req.Get(&systemServer, req.PathValue("id")); err != nil {
		return err
	}

	// Check if server is both enabled and configured
	if err := h.checkEnabledAndConfigured(req.Context(), req.GPTClient, systemServer); err != nil {
		return err
	}

	credCtx := systemServer.Name

	// Reveal the credential
	cred, err := req.GPTClient.RevealCredential(req.Context(), []string{credCtx}, systemServer.Name)
	if err != nil && !errors.As(err, &gptscript.ErrNotFound{}) {
		return fmt.Errorf("failed to find credential: %w", err)
	} else if err == nil {
		return req.Write(cred.Env)
	}

	return types.NewErrNotFound("no credential found for %q", systemServer.Name)
}

// Helper functions

// checkEnabledAndConfigured verifies that a system MCP server is both enabled and configured
func (h *SystemMCPServerHandler) checkEnabledAndConfigured(ctx context.Context, gptClient *gptscript.GPTScript, server v1.SystemMCPServer) error {
	if !server.Spec.Manifest.Enabled {
		return types.NewErrBadRequest("system MCP server is not enabled")
	}

	if !isSystemServerConfigured(ctx, gptClient, server) {
		return types.NewErrBadRequest("system MCP server is not configured")
	}

	return nil
}

func convertSystemMCPServer(server v1.SystemMCPServer, credEnv map[string]string) types.SystemMCPServer {
	result := types.SystemMCPServer{
		Metadata:                    MetadataFrom(&server),
		SystemMCPServerManifest:     server.Spec.Manifest,
		DeploymentStatus:            server.Status.DeploymentStatus,
		DeploymentAvailableReplicas: server.Status.DeploymentAvailableReplicas,
		DeploymentReadyReplicas:     server.Status.DeploymentReadyReplicas,
		DeploymentReplicas:          server.Status.DeploymentReplicas,
		K8sSettingsHash:             server.Status.K8sSettingsHash,
	}

	// Convert deployment conditions
	for _, cond := range server.Status.DeploymentConditions {
		result.DeploymentConditions = append(result.DeploymentConditions, types.DeploymentCondition{
			Type:    string(cond.Type),
			Status:  string(cond.Status),
			Reason:  cond.Reason,
			Message: cond.Message,
		})
	}

	configured := true

	for _, env := range server.Spec.Manifest.Env {
		if env.Required && env.Value == "" && credEnv[env.Key] == "" {
			result.MissingRequiredEnvVars = append(result.MissingRequiredEnvVars, env.Key)
			configured = false
		}
	}

	if server.Spec.Manifest.RemoteConfig != nil {
		for _, header := range server.Spec.Manifest.RemoteConfig.Headers {
			if header.Required && header.Value == "" && credEnv[header.Key] == "" {
				result.MissingRequiredHeaders = append(result.MissingRequiredHeaders, header.Key)
				configured = false
			}
		}
	}

	result.Configured = configured
	return result
}

func isSystemServerConfigured(ctx context.Context, gptClient *gptscript.GPTScript, server v1.SystemMCPServer) bool {
	credEnv, err := getCredentialsForSystemServer(ctx, gptClient, server)
	if err != nil {
		return false
	}

	// Check if all required env vars are configured

	for _, env := range server.Spec.Manifest.Env {
		if env.Required && env.Value == "" && credEnv[env.Key] == "" {
			return false
		}
	}

	if server.Spec.Manifest.RemoteConfig != nil {
		for _, header := range server.Spec.Manifest.RemoteConfig.Headers {
			if header.Required && header.Value == "" && credEnv[header.Key] == "" {
				return false
			}
		}
	}

	return true
}

func getCredentialsForSystemServer(ctx context.Context, gptClient *gptscript.GPTScript, server v1.SystemMCPServer) (map[string]string, error) {
	credCtx := server.Name
	creds, err := gptClient.ListCredentials(ctx, gptscript.ListCredentialsOptions{
		CredentialContexts: []string{credCtx},
	})
	if err != nil {
		return nil, err
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

	return credEnv, nil
}
