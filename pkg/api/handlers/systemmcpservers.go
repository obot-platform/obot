package handlers

import (
	"errors"
	"fmt"
	"net/url"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type SystemMCPServerHandler struct{}

func NewSystemMCPServerHandler() *SystemMCPServerHandler {
	return &SystemMCPServerHandler{}
}

func (h *SystemMCPServerHandler) List(req api.Context) error {
	var systemServers v1.SystemMCPServerList
	if err := req.List(&systemServers); err != nil {
		return err
	}

	// Build credential contexts for all servers
	credCtxs := make([]string, 0, len(systemServers.Items))
	for _, server := range systemServers.Items {
		credCtxs = append(credCtxs, fmt.Sprintf("system-%s", server.Name))
	}

	// Fetch all credentials at once
	creds, err := req.GPTClient.ListCredentials(req.Context(), gptscript.ListCredentialsOptions{
		CredentialContexts: credCtxs,
	})
	if err != nil {
		return fmt.Errorf("failed to list credentials: %w", err)
	}

	// Build credential map
	credMap := make(map[string]map[string]string, len(creds))
	for _, cred := range creds {
		if _, ok := credMap[cred.Context]; !ok {
			c, err := req.GPTClient.RevealCredential(req.Context(), []string{cred.Context}, cred.ToolName)
			if err != nil && !errors.As(err, &gptscript.ErrNotFound{}) {
				return fmt.Errorf("failed to find credential: %w", err)
			}
			credMap[cred.Context] = c.Env
		}
	}

	result := make([]types.SystemMCPServer, 0, len(systemServers.Items))
	for _, server := range systemServers.Items {
		credCtx := fmt.Sprintf("system-%s", server.Name)
		credEnv := credMap[credCtx]
		result = append(result, convertSystemMCPServer(server, credEnv))
	}

	return req.Write(types.SystemMCPServerList{Items: result})
}

func (h *SystemMCPServerHandler) Get(req api.Context) error {
	id := req.PathValue("id")

	var systemServer v1.SystemMCPServer
	if err := req.Get(&systemServer, id); err != nil {
		return err
	}

	// Fetch credentials for this server
	credCtx := fmt.Sprintf("system-%s", systemServer.Name)
	cred, err := req.GPTClient.RevealCredential(req.Context(), []string{credCtx}, systemServer.Name)
	credEnv := make(map[string]string)
	if err == nil {
		credEnv = cred.Env
	}

	return req.Write(convertSystemMCPServer(systemServer, credEnv))
}

func (h *SystemMCPServerHandler) Create(req api.Context) error {
	var input types.SystemMCPServerManifest
	if err := req.Read(&input); err != nil {
		return err
	}

	// Validate system type
	if input.SystemServerSettings.SystemType != types.SystemTypeHook {
		return types.NewErrBadRequest("invalid system type: %s", input.SystemServerSettings.SystemType)
	}

	// Validate runtime configuration
	if err := validateSystemMCPServerManifest(input.Manifest); err != nil {
		return err
	}

	systemServer := v1.SystemMCPServer{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: system.SystemMCPServerPrefix,
			Namespace:    req.Namespace(),
			Finalizers:   []string{v1.SystemMCPServerFinalizer},
		},
		Spec: v1.SystemMCPServerSpec{
			Manifest:             input.Manifest,
			SystemServerSettings: input.SystemServerSettings,
			Editable:             true,
		},
	}

	if err := req.Create(&systemServer); err != nil {
		return err
	}

	// Note: Configured status will be determined when credentials are set via Configure endpoint
	return req.WriteCreated(convertSystemMCPServer(systemServer, nil))
}

func (h *SystemMCPServerHandler) Update(req api.Context) error {
	id := req.PathValue("id")

	var input types.SystemMCPServerManifest
	if err := req.Read(&input); err != nil {
		return err
	}

	var existing v1.SystemMCPServer
	if err := req.Get(&existing, id); err != nil {
		return err
	}

	// Prevent editing git-synced servers
	if !existing.Spec.Editable {
		return types.NewErrBadRequest("cannot edit system server synced from git source")
	}

	// Validate system type
	if input.SystemServerSettings.SystemType != types.SystemTypeHook {
		return types.NewErrBadRequest("invalid system type: %s", input.SystemServerSettings.SystemType)
	}

	// Validate runtime configuration
	if err := validateSystemMCPServerManifest(input.Manifest); err != nil {
		return err
	}

	existing.Spec.Manifest = input.Manifest
	existing.Spec.SystemServerSettings = input.SystemServerSettings

	if err := req.Update(&existing); err != nil {
		return err
	}

	// Fetch existing credentials to compute configuration status
	credCtx := fmt.Sprintf("system-%s", existing.Name)
	cred, err := req.GPTClient.RevealCredential(req.Context(), []string{credCtx}, existing.Name)
	credEnv := make(map[string]string)
	if err == nil {
		credEnv = cred.Env
	}

	return req.Write(convertSystemMCPServer(existing, credEnv))
}

func (h *SystemMCPServerHandler) Delete(req api.Context) error {
	id := req.PathValue("id")

	// TODO(g-linville): get the server and reject the deletion if it is not Editable

	// Delete associated credentials
	var systemServer v1.SystemMCPServer
	if err := req.Get(&systemServer, id); err == nil {
		credCtx := fmt.Sprintf("system-%s", systemServer.Name)
		_ = req.GPTClient.DeleteCredential(req.Context(), credCtx, systemServer.Name)
	}

	return req.Delete(&v1.SystemMCPServer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      id,
			Namespace: req.Namespace(),
		},
	})
}

func (h *SystemMCPServerHandler) Configure(req api.Context) error {
	id := req.PathValue("id")

	var systemServer v1.SystemMCPServer
	if err := req.Get(&systemServer, id); err != nil {
		return err
	}

	// Note: Even git-synced servers (Editable: false) can be configured via credentials.
	// The Editable flag only prevents editing the manifest, not setting credentials.

	// Read environment variables from request
	var envVars map[string]string
	if err := req.Read(&envVars); err != nil {
		return err
	}

	// Remove empty values
	for key, val := range envVars {
		if val == "" {
			delete(envVars, key)
		}
	}

	// Credential context for system servers is "system-{serverName}"
	credCtx := fmt.Sprintf("system-%s", systemServer.Name)

	// Delete existing credential to allow update
	_ = req.GPTClient.DeleteCredential(req.Context(), credCtx, systemServer.Name)

	// Create new credential with updated environment variables
	if err := req.GPTClient.CreateCredential(req.Context(), gptscript.Credential{
		Context:  credCtx,
		ToolName: systemServer.Name,
		Type:     gptscript.CredentialTypeTool,
		Env:      envVars,
	}); err != nil {
		return fmt.Errorf("failed to create credential: %w", err)
	}

	return req.Write(convertSystemMCPServer(systemServer, envVars))
}

func (h *SystemMCPServerHandler) Deconfigure(req api.Context) error {
	id := req.PathValue("id")

	var systemServer v1.SystemMCPServer
	if err := req.Get(&systemServer, id); err != nil {
		return err
	}

	// Note: Even git-synced servers (Editable: false) can be deconfigured.
	// The Editable flag only prevents editing the manifest, not removing credentials.

	// Delete credentials
	credCtx := fmt.Sprintf("system-%s", systemServer.Name)
	if err := req.GPTClient.DeleteCredential(req.Context(), credCtx, systemServer.Name); err != nil {
		return fmt.Errorf("failed to delete credential: %w", err)
	}

	return req.Write(convertSystemMCPServer(systemServer, nil))
}

// Helper functions

func convertSystemMCPServer(server v1.SystemMCPServer, credEnv map[string]string) types.SystemMCPServer {
	// Compute configuration status based on credentials
	configured, missingEnvVars, missingHeaders := checkConfiguration(server.Spec.Manifest, credEnv)

	return types.SystemMCPServer{
		Metadata:                    MetadataFrom(&server),
		Manifest:                    server.Spec.Manifest,
		SystemServerSettings:        server.Spec.SystemServerSettings,
		SourceURL:                   server.Spec.SourceURL,
		Editable:                    server.Spec.Editable,
		Configured:                  configured,
		MissingRequiredEnvVars:      missingEnvVars,
		MissingRequiredHeaders:      missingHeaders,
		DeploymentStatus:            server.Status.DeploymentStatus,
		DeploymentAvailableReplicas: server.Status.DeploymentAvailableReplicas,
		DeploymentReadyReplicas:     server.Status.DeploymentReadyReplicas,
		DeploymentReplicas:          server.Status.DeploymentReplicas,
		DeploymentConditions:        convertDeploymentConditions(server.Status.DeploymentConditions),
		K8sSettingsHash:             server.Status.K8sSettingsHash,
	}
}

func validateSystemMCPServerManifest(manifest types.MCPServerManifest) error {
	// TODO(g-linville): can we lean upon more of the pre-existing validation functions here?
	// Validate runtime is set
	if manifest.Runtime == "" {
		return types.NewErrBadRequest("runtime is required")
	}

	// Validate runtime-specific config
	switch manifest.Runtime {
	case types.RuntimeUVX:
		if manifest.UVXConfig == nil || manifest.UVXConfig.Package == "" {
			return types.NewErrBadRequest("uvxConfig with package is required for uvx runtime")
		}
	case types.RuntimeNPX:
		if manifest.NPXConfig == nil || manifest.NPXConfig.Package == "" {
			return types.NewErrBadRequest("npxConfig with package is required for npx runtime")
		}
	case types.RuntimeContainerized:
		if manifest.ContainerizedConfig == nil {
			return types.NewErrBadRequest("containerizedConfig is required for containerized runtime")
		}
		if manifest.ContainerizedConfig.Image == "" {
			return types.NewErrBadRequest("containerizedConfig.image is required")
		}
		if manifest.ContainerizedConfig.Port == 0 {
			return types.NewErrBadRequest("containerizedConfig.port is required")
		}
		if manifest.ContainerizedConfig.Path == "" {
			return types.NewErrBadRequest("containerizedConfig.path is required")
		}
	case types.RuntimeRemote:
		if manifest.RemoteConfig == nil || manifest.RemoteConfig.URL == "" {
			return types.NewErrBadRequest("remoteConfig with URL is required for remote runtime")
		}
		// Validate URL format
		if _, err := url.Parse(manifest.RemoteConfig.URL); err != nil {
			return types.NewErrBadRequest("invalid remote URL: %v", err)
		}
	default:
		return types.NewErrBadRequest("unsupported runtime: %s", manifest.Runtime)
	}

	return nil
}

func checkConfiguration(manifest types.MCPServerManifest, credEnv map[string]string) (bool, []string, []string) {
	var missingEnvVars []string
	var missingHeaders []string

	// Check for required env vars that haven't been configured via credentials
	for _, env := range manifest.Env {
		if env.Required {
			if _, ok := credEnv[env.Key]; !ok {
				missingEnvVars = append(missingEnvVars, env.Key)
			}
		}
	}

	// Check for required headers (for remote runtime)
	if manifest.RemoteConfig != nil {
		for _, header := range manifest.RemoteConfig.Headers {
			if header.Required {
				if _, ok := credEnv[header.Key]; !ok {
					missingHeaders = append(missingHeaders, header.Key)
				}
			}
		}
	}

	configured := len(missingEnvVars) == 0 && len(missingHeaders) == 0
	return configured, missingEnvVars, missingHeaders
}

func convertDeploymentConditions(conditions []v1.DeploymentCondition) []types.DeploymentCondition {
	result := make([]types.DeploymentCondition, len(conditions))
	for i, cond := range conditions {
		result[i] = types.DeploymentCondition{
			Type:               string(cond.Type),
			Status:             string(cond.Status),
			Reason:             cond.Reason,
			Message:            cond.Message,
			LastTransitionTime: *types.NewTime(cond.LastTransitionTime.Time),
			LastUpdateTime:     *types.NewTime(cond.LastUpdateTime.Time),
		}
	}
	return result
}
