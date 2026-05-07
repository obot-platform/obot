package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/gptscript-ai/go-gptscript"
	nmcp "github.com/obot-platform/nanobot/pkg/mcp"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/obot-platform/obot/pkg/validation"
	"github.com/obot-platform/obot/pkg/wait"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type MCPWebhookValidationHandler struct {
	mcpSessionManager *mcp.SessionManager
}

func NewMCPWebhookValidationHandler(mcpLoader *mcp.SessionManager) *MCPWebhookValidationHandler {
	return &MCPWebhookValidationHandler{mcpSessionManager: mcpLoader}
}

func (m *MCPWebhookValidationHandler) List(req api.Context) error {
	var list v1.MCPWebhookValidationList
	if err := req.List(&list); err != nil {
		return fmt.Errorf("failed to list mcp webhook validations: %w", err)
	}

	items := make([]types.MCPWebhookValidation, 0, len(list.Items))
	for _, item := range list.Items {
		credEnv, err := getCredentialsForWebhookValidation(req.Context(), req.GPTClient, item)
		if err != nil {
			return err
		}
		items = append(items, convertMCPWebhookValidation(item, credEnv))
	}

	return req.Write(types.MCPWebhookValidationList{Items: items})
}

func (m *MCPWebhookValidationHandler) Get(req api.Context) error {
	var webhookValidation v1.MCPWebhookValidation
	if err := req.Get(&webhookValidation, req.PathValue("mcp_webhook_validation_id")); err != nil {
		return err
	}

	credEnv, err := getCredentialsForWebhookValidation(req.Context(), req.GPTClient, webhookValidation)
	if err != nil {
		return err
	}

	return req.Write(convertMCPWebhookValidation(webhookValidation, credEnv))
}

func (m *MCPWebhookValidationHandler) Create(req api.Context) error {
	var manifest types.MCPWebhookValidationManifest
	if err := req.Read(&manifest); err != nil {
		return types.NewErrBadRequest("failed to read manifest: %v", err)
	}

	if err := m.resolveManifestFromCatalogEntry(req, &manifest); err != nil {
		return err
	}

	if err := validateManifest(&manifest); err != nil {
		return types.NewErrBadRequest("invalid manifest: %v", err)
	}

	var secretCred map[string]string
	if manifest.Secret != "" {
		secretCred = map[string]string{
			"secret": manifest.Secret,
		}

		// Don't save the secrets in the database.
		manifest.Secret = ""
	}

	webhookValidation := v1.MCPWebhookValidation{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: system.MCPWebhookValidationPrefix,
			Namespace:    req.Namespace(),
		},
		Spec: v1.MCPWebhookValidationSpec{
			Manifest: manifest,
		},
	}

	if err := req.Create(&webhookValidation); err != nil {
		return fmt.Errorf("failed to create mcp webhook validation: %w", err)
	}

	if err := req.GPTClient.CreateCredential(req.Context(), gptscript.Credential{
		Context:  system.MCPWebhookValidationCredentialContext,
		ToolName: webhookValidation.Name,
		Type:     gptscript.CredentialTypeTool,
		Env:      secretCred,
	}); err != nil {
		_ = req.Delete(&webhookValidation)
		return fmt.Errorf("failed to create credential: %w", err)
	}

	return req.Write(convertMCPWebhookValidation(webhookValidation, nil))
}

func (m *MCPWebhookValidationHandler) Update(req api.Context) error {
	var webhookValidation v1.MCPWebhookValidation
	if err := req.Get(&webhookValidation, req.PathValue("mcp_webhook_validation_id")); err != nil {
		return err
	}

	var manifest types.MCPWebhookValidationManifest
	if err := req.Read(&manifest); err != nil {
		return types.NewErrBadRequest("failed to read manifest: %v", err)
	}

	if err := m.resolveManifestFromCatalogEntry(req, &manifest); err != nil {
		return err
	}

	if err := validateManifest(&manifest); err != nil {
		return types.NewErrBadRequest("invalid manifest: %v", err)
	}

	var secretCred map[string]string
	if manifest.Secret != "" {
		secretCred = map[string]string{
			"secret": manifest.Secret,
		}
		// Don't save the secrets in the database.
		manifest.Secret = ""
	}

	webhookValidation.Spec.Manifest = manifest

	if secretCred != nil {
		if err := req.GPTClient.CreateCredential(req.Context(), gptscript.Credential{
			Context:  system.MCPWebhookValidationCredentialContext,
			ToolName: webhookValidation.Name,
			Type:     gptscript.CredentialTypeTool,
			Env:      secretCred,
		}); err != nil {
			return fmt.Errorf("failed to create credential: %w", err)
		}
	} else {
		cred, err := req.GPTClient.RevealCredential(req.Context(), []string{system.MCPWebhookValidationCredentialContext}, webhookValidation.Name)
		if err != nil && !errors.As(err, &gptscript.ErrNotFound{}) {
			return fmt.Errorf("failed to reveal credential: %w", err)
		}

		secretCred = cred.Env
	}

	if err := req.Update(&webhookValidation); err != nil {
		return fmt.Errorf("failed to update mcp webhook validation: %w", err)
	}

	return req.Write(convertMCPWebhookValidation(webhookValidation, secretCred))
}

func (m *MCPWebhookValidationHandler) Delete(req api.Context) error {
	var webhookValidation v1.MCPWebhookValidation
	if err := req.Get(&webhookValidation, req.PathValue("mcp_webhook_validation_id")); err != nil {
		return err
	}

	if err := req.GPTClient.DeleteCredential(req.Context(), system.MCPWebhookValidationCredentialContext, webhookValidation.Name); err != nil && !errors.As(err, &gptscript.ErrNotFound{}) {
		return fmt.Errorf("failed to delete credential: %w", err)
	}

	if err := req.Delete(&webhookValidation); err != nil {
		return fmt.Errorf("failed to delete mcp webhook validation: %w", err)
	}

	return req.Write(convertMCPWebhookValidation(webhookValidation, nil))
}

// Configure configures environment variables for a webhook validation.
func (m *MCPWebhookValidationHandler) Configure(req api.Context) error {
	var envVars map[string]string
	if err := req.Read(&envVars); err != nil {
		return types.NewErrBadRequest("invalid request body: %v", err)
	}

	var webhookValidation v1.MCPWebhookValidation
	if err := req.Get(&webhookValidation, req.PathValue("mcp_webhook_validation_id")); err != nil {
		return err
	}

	if err := applyRemoteURLTemplateToWebhookValidation(&webhookValidation, envVars); err != nil {
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
		Context:  system.MCPWebhookValidationCredentialContext,
		ToolName: webhookValidation.Name,
		Type:     gptscript.CredentialTypeTool,
		Env:      envVars,
	}); err != nil {
		return fmt.Errorf("failed to create credential: %w", err)
	}

	// Update annotation to track configuration timestamp
	if webhookValidation.Annotations == nil {
		webhookValidation.Annotations = make(map[string]string, 1)
	}
	webhookValidation.Annotations["obot.obot.ai/configured-at"] = metav1.Now().Format(time.RFC3339)

	if err := req.Update(&webhookValidation); err != nil {
		return fmt.Errorf("failed to update mcp webhook validation: %w", err)
	}

	return req.Write(convertMCPWebhookValidation(webhookValidation, envVars))
}

func (m *MCPWebhookValidationHandler) Deconfigure(req api.Context) error {
	var webhookValidation v1.MCPWebhookValidation
	if err := req.Get(&webhookValidation, req.PathValue("mcp_webhook_validation_id")); err != nil {
		return err
	}

	if err := DeleteCredentialIfExists(req.Context(), req.GPTClient, []string{system.MCPWebhookValidationCredentialContext}, webhookValidation.Name); err != nil {
		return err
	}

	if webhookValidation.Annotations != nil {
		delete(webhookValidation.Annotations, "obot.obot.ai/configured-at")
	}

	if err := req.Update(&webhookValidation); err != nil {
		return fmt.Errorf("failed to update mcp webhook validation: %w", err)
	}

	return req.Write(convertMCPWebhookValidation(webhookValidation, nil))
}

func (m *MCPWebhookValidationHandler) Reveal(req api.Context) error {
	var webhookValidation v1.MCPWebhookValidation
	if err := req.Get(&webhookValidation, req.PathValue("mcp_webhook_validation_id")); err != nil {
		return err
	}

	cred, err := req.GPTClient.RevealCredential(req.Context(), []string{system.MCPWebhookValidationCredentialContext}, webhookValidation.Name)
	if err != nil && !errors.As(err, &gptscript.ErrNotFound{}) {
		return fmt.Errorf("failed to find credential: %w", err)
	} else if err == nil {
		return req.Write(cred.Env)
	}

	return types.NewErrNotFound("no credential found for %q", webhookValidation.Name)
}

func convertMCPWebhookValidation(validation v1.MCPWebhookValidation, credEnv map[string]string) types.MCPWebhookValidation {
	result := types.MCPWebhookValidation{
		Metadata:                     MetadataFrom(&validation),
		MCPWebhookValidationManifest: validation.Spec.Manifest,
		HasSecret:                    credEnv["secret"] != "" || credEnv["WEBHOOK_SECRET"] != "",
		Configured:                   validation.Status.Configured,
	}

	if manifest := validation.Spec.Manifest.SystemMCPServerManifest; manifest != nil {
		result.Configured = true
		for _, env := range manifest.Env {
			if env.Required && env.Value == "" && credEnv[env.Key] == "" {
				result.MissingRequiredEnvVars = append(result.MissingRequiredEnvVars, env.Key)
				result.Configured = false
			}
		}

		sort.Strings(result.MissingRequiredEnvVars)
	}

	return result
}

func getCredentialsForWebhookValidation(ctx context.Context, gptClient *gptscript.GPTScript, webhookValidation v1.MCPWebhookValidation) (map[string]string, error) {
	cred, err := gptClient.RevealCredential(ctx, []string{system.MCPWebhookValidationCredentialContext}, webhookValidation.Name)
	if err != nil && !errors.As(err, &gptscript.ErrNotFound{}) {
		return nil, err
	}

	return cred.Env, nil
}

func (m *MCPWebhookValidationHandler) getSystemServerForWebhookValidation(req api.Context) (v1.SystemMCPServer, error) {
	systemServer := &v1.SystemMCPServer{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: req.Namespace(),
			Name:      system.SystemMCPServerPrefix + req.PathValue("mcp_webhook_validation_id"),
		},
	}

	systemServer, err := wait.For(req.Context(), req.Storage, systemServer, func(s *v1.SystemMCPServer) (bool, error) {
		return s.UID != "" && s.Name == systemServer.Name, nil
	}, wait.Option{
		Timeout:       10 * time.Second,
		WaitForExists: true,
	})
	if err != nil {
		return v1.SystemMCPServer{}, err
	}

	return *systemServer, nil
}

func (m *MCPWebhookValidationHandler) Restart(req api.Context) error {
	systemServer, err := m.getSystemServerForWebhookValidation(req)
	if err != nil {
		return err
	}

	if err := checkEnabledAndConfigured(req.Context(), req.GPTClient, systemServer); err != nil {
		return err
	}

	serverConfig, _, err := systemServerToServerConfig(req, systemServer)
	if err != nil {
		return types.NewErrBadRequest("failed to transform system server to config: %v", err)
	}

	if err := m.mcpSessionManager.RestartServerDeployment(req.Context(), serverConfig); err != nil {
		if nse := (*mcp.ErrNotSupportedByBackend)(nil); errors.As(err, &nse) {
			return types.NewErrNotFound(nse.Error())
		}
		return fmt.Errorf("failed to restart mcp webhook validation: %w", err)
	}

	req.WriteHeader(http.StatusNoContent)
	return nil
}

func (m *MCPWebhookValidationHandler) Launch(req api.Context) error {
	systemServer, err := m.getSystemServerForWebhookValidation(req)
	if err != nil {
		return err
	}

	if err := checkEnabledAndConfigured(req.Context(), req.GPTClient, systemServer); err != nil {
		return err
	}

	serverConfig, _, err := systemServerToServerConfig(req, systemServer)
	if err != nil {
		return types.NewErrBadRequest("failed to transform system server to config: %v", err)
	}

	if serverConfig.Runtime != types.RuntimeRemote {
		_, err = m.mcpSessionManager.ListTools(req.Context(), serverConfig)
	} else {
		_, err = m.mcpSessionManager.LaunchServer(req.Context(), serverConfig)
	}
	if err != nil {
		if errors.Is(err, mcp.ErrHealthCheckFailed) || errors.Is(err, mcp.ErrHealthCheckTimeout) {
			return types.NewErrHTTP(http.StatusServiceUnavailable, "MCP webhook validation is not healthy, check configuration for errors")
		}
		if errors.Is(err, nmcp.ErrNoResult) || strings.HasSuffix(err.Error(), nmcp.ErrNoResult.Error()) {
			return types.NewErrHTTP(http.StatusServiceUnavailable, "No response from MCP webhook validation, check configuration for errors")
		}
		if errors.Is(err, mcp.ErrInsufficientCapacity) {
			return types.NewErrHTTP(http.StatusServiceUnavailable, "Insufficient capacity to deploy MCP webhook validation. Please contact your administrator.")
		}
		if nse := (*mcp.ErrNotSupportedByBackend)(nil); errors.As(err, &nse) {
			return types.NewErrHTTP(http.StatusBadRequest, nse.Error())
		}
		return fmt.Errorf("failed to launch mcp webhook validation: %w", err)
	}

	return nil
}

func (m *MCPWebhookValidationHandler) Logs(req api.Context) error {
	systemServer, err := m.getSystemServerForWebhookValidation(req)
	if err != nil {
		return err
	}

	if err := checkEnabledAndConfigured(req.Context(), req.GPTClient, systemServer); err != nil {
		return err
	}

	serverConfig, _, err := systemServerToServerConfig(req, systemServer)
	if err != nil {
		return types.NewErrBadRequest("failed to transform system server to config: %v", err)
	}

	logs, err := m.mcpSessionManager.StreamServerLogs(req.Context(), serverConfig)
	if err != nil {
		if nse := (*mcp.ErrNotSupportedByBackend)(nil); errors.As(err, &nse) {
			return types.NewErrNotFound(nse.Error())
		}
		return err
	}

	return StreamLogs(req.Context(), req.ResponseWriter, logs, StreamLogsOptions{
		SendKeepAlive:  true,
		SendDisconnect: true,
		SendEnded:      true,
	})
}

func (m *MCPWebhookValidationHandler) GetDetails(req api.Context) error {
	systemServer, err := m.getSystemServerForWebhookValidation(req)
	if err != nil {
		return err
	}

	if err := checkEnabledAndConfigured(req.Context(), req.GPTClient, systemServer); err != nil {
		return err
	}

	serverConfig, _, err := systemServerToServerConfig(req, systemServer)
	if err != nil {
		return types.NewErrBadRequest("failed to transform system server to config: %v", err)
	}

	details, err := m.mcpSessionManager.GetServerDetails(req.Context(), serverConfig)
	if err != nil {
		if nse := (*mcp.ErrNotSupportedByBackend)(nil); errors.As(err, &nse) {
			return types.NewErrNotFound(nse.Error())
		}
		return fmt.Errorf("failed to get server details: %w", err)
	}

	return req.Write(details)
}

func (m *MCPWebhookValidationHandler) resolveManifestFromCatalogEntry(req api.Context, manifest *types.MCPWebhookValidationManifest) error {
	if manifest.SystemMCPServerCatalogEntryID == "" {
		return nil
	}
	if manifest.URL != "" {
		return types.NewErrBadRequest("webhook URL and system MCP server catalog entry ID are mutually exclusive")
	}
	if manifest.SystemMCPServerManifest != nil {
		return types.NewErrBadRequest("system MCP server manifest and system MCP server catalog entry ID are mutually exclusive")
	}

	var entry v1.SystemMCPServerCatalogEntry
	if err := req.Get(&entry, manifest.SystemMCPServerCatalogEntryID); err != nil {
		return err
	}

	if entry.Spec.Manifest.SystemMCPServerType != types.SystemMCPServerTypeFilter {
		return types.NewErrBadRequest("system MCP server catalog entry %q must have systemMCPServerType %q", manifest.SystemMCPServerCatalogEntryID, types.SystemMCPServerTypeFilter)
	}

	if entry.Spec.Manifest.FilterConfig == nil || entry.Spec.Manifest.FilterConfig.ToolName == "" {
		return types.NewErrBadRequest("system MCP server catalog entry %q must have filterConfig.toolName", manifest.SystemMCPServerCatalogEntryID)
	}

	serverManifest := systemMCPServerManifestFromCatalogEntry(entry.Spec.Manifest, manifest.Disabled)
	if err := validation.ValidateSystemMCPServerManifest(serverManifest); err != nil {
		return types.NewErrBadRequest("invalid system MCP server catalog entry manifest: %v", err)
	}

	manifest.SystemMCPServerManifest = &serverManifest
	manifest.ToolName = entry.Spec.Manifest.FilterConfig.ToolName
	return nil
}

func systemMCPServerManifestFromCatalogEntry(entry types.SystemMCPServerCatalogEntryManifest, disabled bool) types.SystemMCPServerManifest {
	manifest := types.SystemMCPServerManifest{
		Metadata:            entry.Metadata,
		Name:                entry.Name,
		ShortDescription:    entry.ShortDescription,
		Description:         entry.Description,
		Icon:                entry.Icon,
		Enabled:             new(!disabled),
		Runtime:             entry.Runtime,
		UVXConfig:           entry.UVXConfig,
		NPXConfig:           entry.NPXConfig,
		ContainerizedConfig: entry.ContainerizedConfig,
		Env:                 entry.Env,
	}

	if entry.RemoteConfig != nil {
		manifest.RemoteConfig = &types.RemoteRuntimeConfig{
			URL:                 entry.RemoteConfig.FixedURL,
			IsTemplate:          entry.RemoteConfig.URLTemplate != "",
			URLTemplate:         entry.RemoteConfig.URLTemplate,
			Hostname:            entry.RemoteConfig.Hostname,
			Headers:             entry.RemoteConfig.Headers,
			StaticOAuthRequired: entry.RemoteConfig.StaticOAuthRequired,
		}
	}

	return manifest
}

func applyRemoteURLTemplateToWebhookValidation(webhookValidation *v1.MCPWebhookValidation, envVars map[string]string) error {
	manifest := webhookValidation.Spec.Manifest.SystemMCPServerManifest
	if manifest == nil || manifest.Runtime != types.RuntimeRemote || manifest.RemoteConfig == nil || manifest.RemoteConfig.URLTemplate == "" {
		return nil
	}

	finalURL, err := applyURLTemplate(manifest.RemoteConfig.URLTemplate, envVars)
	if err != nil {
		return fmt.Errorf("failed to apply URL template: %w", err)
	}

	manifest.RemoteConfig.URL = finalURL
	if err := validation.ValidateSystemMCPServerManifest(*manifest); err != nil {
		return types.NewErrBadRequest("validation failed: %v", err)
	}

	return nil
}

func validateManifest(m *types.MCPWebhookValidationManifest) error {
	var sources int
	if m.URL != "" {
		sources++
	}
	if m.SystemMCPServerManifest != nil && m.SystemMCPServerCatalogEntryID == "" {
		sources++
	}
	if m.SystemMCPServerCatalogEntryID != "" {
		sources++
	}

	if sources == 0 {
		return fmt.Errorf("webhook URL, system MCP server manifest, or system MCP server catalog entry ID is required")
	}

	if sources > 1 {
		return fmt.Errorf("webhook URL, system MCP server manifest, and system MCP server catalog entry ID are mutually exclusive")
	}

	if m.SystemMCPServerManifest != nil {
		if m.ToolName == "" {
			return fmt.Errorf("tool name is required when using system MCP server manifest")
		}
		if err := validation.ValidateSystemMCPServerManifest(*m.SystemMCPServerManifest); err != nil {
			return fmt.Errorf("invalid system MCP server manifest: %w", err)
		}
	}

	for _, resource := range m.Resources {
		if err := resource.Validate(); err != nil {
			return fmt.Errorf("invalid resource: %v", err)
		}
	}

	for i, filter := range m.Selectors {
		if filter.Method == "*" {
			m.Selectors = []types.MCPSelector{{Method: filter.Method}}
			break
		}
		if slices.Contains(filter.Identifiers, "*") {
			m.Selectors[i].Identifiers = []string{"*"}
		}
	}

	return nil
}
