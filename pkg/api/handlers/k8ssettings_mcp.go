package handlers

import (
	"errors"
	"fmt"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"k8s.io/client-go/util/retry"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type k8sSettingsHelper struct {
	sessionManager      *mcp.SessionManager
	mcpImagePullSecrets []string
}

func newK8sSettingsHelper(sessionManager *mcp.SessionManager, mcpImagePullSecrets []string) *k8sSettingsHelper {
	return &k8sSettingsHelper{
		sessionManager:      sessionManager,
		mcpImagePullSecrets: mcpImagePullSecrets,
	}
}

func (h *k8sSettingsHelper) currentK8sSettingsHash(req api.Context, settings v1.K8sSettingsSpec, mcpServer v1.MCPServer) (string, error) {
	imagePullSecretNames, err := mcp.CurrentImagePullSecretNames(req.Context(), req.Storage, h.sessionManager.MCPRuntimeBackend(), h.mcpImagePullSecrets)
	if err != nil {
		return "", err
	}
	resources, err := mcp.CoreResourceRequirements(mcpServer.Spec.Manifest.Resources)
	if err != nil {
		return "", fmt.Errorf("failed to compute core resource requirements: %w", err)
	}
	return mcp.ComputeK8sSettingsHash(settings, resources, mcpServer.Spec.Manifest.Runtime, mcpServer.Spec.NanobotAgentID != "", imagePullSecretNames), nil
}

func (h *k8sSettingsHelper) checkK8sSettingsStatus(req api.Context, server v1.MCPServer) (types.K8sSettingsStatus, error) {
	deployedHash := server.Status.K8sSettingsHash
	if deployedHash == "" {
		return types.K8sSettingsStatus{}, types.NewErrBadRequest("K8s settings check is only supported for Kubernetes runtime")
	}

	var k8sSettings v1.K8sSettings
	if err := req.Storage.Get(req.Context(), kclient.ObjectKey{
		Namespace: req.Namespace(),
		Name:      system.K8sSettingsName,
	}, &k8sSettings); err != nil {
		return types.K8sSettingsStatus{}, err
	}

	currentHash, err := h.currentK8sSettingsHash(req, k8sSettings.Spec, server)
	if err != nil {
		return types.K8sSettingsStatus{}, err
	}

	currentSettings, err := convertK8sSettings(k8sSettings)
	if err != nil {
		return types.K8sSettingsStatus{}, err
	}

	return types.K8sSettingsStatus{
		NeedsK8sUpdate:       deployedHash != currentHash,
		CurrentSettings:      &currentSettings,
		DeployedSettingsHash: deployedHash,
	}, nil
}

func (h *k8sSettingsHelper) redeployWithK8sSettings(req api.Context, server v1.MCPServer, serverConfig mcp.ServerConfig) (v1.MCPServer, error) {
	if !mcp.IsKubernetesBackend(h.sessionManager.MCPRuntimeBackend()) {
		return server, types.NewErrBadRequest("Redeployment with K8s settings is only supported for Kubernetes backend")
	}

	deployedHash := server.Status.K8sSettingsHash

	var k8sSettings v1.K8sSettings
	if err := req.Storage.Get(req.Context(), kclient.ObjectKey{
		Namespace: req.Namespace(),
		Name:      system.K8sSettingsName,
	}, &k8sSettings); err != nil {
		return server, err
	}

	currentHash, err := h.currentK8sSettingsHash(req, k8sSettings.Spec, server)
	if err != nil {
		return server, err
	}
	hashDrift := deployedHash != currentHash

	if hashDrift || server.Status.NeedsK8sUpdate {
		if err := h.sessionManager.RestartServerDeployment(req.Context(), serverConfig); err != nil {
			if nse := (*mcp.ErrNotSupportedByBackend)(nil); errors.As(err, &nse) {
				return server, types.NewErrBadRequest("Restart is not supported by the current backend")
			}
			return server, fmt.Errorf("failed to redeploy server: %w", err)
		}
	}

	if !server.Status.NeedsK8sUpdate && !hashDrift {
		return server, nil
	}

	if err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		var latest v1.MCPServer
		if err := req.Storage.Get(req.Context(), kclient.ObjectKey{
			Namespace: req.Namespace(),
			Name:      server.Name,
		}, &latest); err != nil {
			return err
		}
		latest.Status.NeedsK8sUpdate = false
		latest.Status.K8sSettingsHash = currentHash
		if err := req.Storage.Status().Update(req.Context(), &latest); err != nil {
			return err
		}
		server = latest
		return nil
	}); err != nil {
		return server, fmt.Errorf("failed to update server status: %w", err)
	}

	return server, nil
}

func validateMCPServerCatalogOrWorkspace(server v1.MCPServer, catalogID, workspaceID, entryID string) error {
	if entryID != "" {
		if server.Spec.MCPServerCatalogEntryName != entryID {
			return types.NewErrNotFound("MCP server not found")
		}
		return nil
	}
	if server.Spec.MCPCatalogID != catalogID || server.Spec.PowerUserWorkspaceID != workspaceID {
		return types.NewErrNotFound("MCP server not found")
	}
	return nil
}

func validateMCPServerCatalogOrWorkspaceEntry(req api.Context, server v1.MCPServer, catalogID, workspaceID, entryID string) error {
	if err := validateMCPServerCatalogOrWorkspace(server, catalogID, workspaceID, entryID); err != nil {
		return err
	}
	if entryID == "" {
		return nil
	}

	var entry v1.MCPServerCatalogEntry
	if err := req.Get(&entry, entryID); err != nil {
		return types.NewErrNotFound("MCP server not found")
	}
	if entry.Spec.MCPCatalogName != catalogID || entry.Spec.PowerUserWorkspaceID != workspaceID {
		return types.NewErrNotFound("MCP server not found")
	}
	return nil
}
