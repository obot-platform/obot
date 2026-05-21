package handlers

import (
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/mcp"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// MCPSecretBindingHandler serves admin lookup APIs for Kubernetes Secrets allowed for MCP secret bindings.
type MCPSecretBindingHandler struct {
	allowedLabel string
	backend      string
	k8sClient    kclient.Client
	namespace    string
}

// NewMCPSecretBindingHandler creates an MCP secret-binding lookup handler.
func NewMCPSecretBindingHandler(backend string, k8sClient kclient.Client, namespace, allowedLabel string) *MCPSecretBindingHandler {
	return &MCPSecretBindingHandler{
		allowedLabel: allowedLabel,
		backend:      backend,
		k8sClient:    k8sClient,
		namespace:    namespace,
	}
}

// ListAllowedSecrets lists bindable Kubernetes Secrets without exposing secret values.
func (h *MCPSecretBindingHandler) ListAllowedSecrets(req api.Context) error {
	if !req.UserIsAdmin() {
		return types.NewErrForbidden("only admins can list allowed MCP secret bindings")
	}
	if !mcp.IsKubernetesBackend(h.backend) || h.k8sClient == nil {
		return req.Write(types.MCPAllowedSecretBindingTargetList{})
	}

	targets, err := mcp.ListAllowedSecretBindingTargets(req.Context(), h.k8sClient, h.namespace, h.allowedLabel)
	if err != nil {
		return err
	}
	return req.Write(types.MCPAllowedSecretBindingTargetList{Items: targets})
}
