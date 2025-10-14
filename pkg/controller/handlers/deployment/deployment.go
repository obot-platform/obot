package deployment

import (
	"fmt"
	"slices"
	"strings"

	"github.com/obot-platform/nah/pkg/apply"
	"github.com/obot-platform/nah/pkg/router"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type Handler struct {
	mcpDeploymentNamespace string
	mcpNamespace           string
	storageClient          kclient.Client
}

func New(mcpNamespace string, storageClient kclient.Client) *Handler {
	return &Handler{
		mcpDeploymentNamespace: mcpNamespace,
		mcpNamespace:           system.DefaultNamespace,
		storageClient:          storageClient,
	}
}

// UpdateMCPServerStatus watches for Deployment changes and copies status information
// to the corresponding MCPServer object based on the "mcp-server-name" label
func (h *Handler) UpdateMCPServerStatus(req router.Request, _ router.Response) error {
	deployment := req.Object.(*appsv1.Deployment)

	// Get the MCP server name from the deployment label
	mcpServerName, exists := deployment.Labels["mcp-server-name"]
	if !exists {
		// This deployment is not associated with an MCP server, skip it
		return nil
	}

	// Find the corresponding MCPServer object by name using the storage client
	var mcpServer v1.MCPServer
	if err := h.storageClient.Get(req.Ctx, kclient.ObjectKey{
		Name:      mcpServerName,
		Namespace: h.mcpNamespace,
	}, &mcpServer); apierrors.IsNotFound(err) {
		return nil
	} else if err != nil {
		return fmt.Errorf("failed to get MCPServer %s: %w", mcpServerName, err)
	}

	// Extract deployment status information
	deploymentStatus := getDeploymentStatus(deployment)
	availableReplicas := deployment.Status.AvailableReplicas
	readyReplicas := deployment.Status.ReadyReplicas
	replicas := deployment.Spec.Replicas
	conditions := getDeploymentConditions(deployment)

	// Check if we need to update the MCPServer status
	var needsUpdate bool
	if mcpServer.Status.DeploymentStatus != deploymentStatus {
		mcpServer.Status.DeploymentStatus = deploymentStatus
		needsUpdate = true
	}
	if !int32PtrEqual(mcpServer.Status.DeploymentAvailableReplicas, &availableReplicas) {
		mcpServer.Status.DeploymentAvailableReplicas = &availableReplicas
		needsUpdate = true
	}
	if !int32PtrEqual(mcpServer.Status.DeploymentReadyReplicas, &readyReplicas) {
		mcpServer.Status.DeploymentReadyReplicas = &readyReplicas
		needsUpdate = true
	}
	if !int32PtrEqual(mcpServer.Status.DeploymentReplicas, replicas) {
		mcpServer.Status.DeploymentReplicas = replicas
		needsUpdate = true
	}
	if !slices.Equal(mcpServer.Status.DeploymentConditions, conditions) {
		mcpServer.Status.DeploymentConditions = conditions
		needsUpdate = true
	}

	// Update the MCPServer status if needed
	if needsUpdate {
		return h.storageClient.Status().Update(req.Ctx, &mcpServer)
	}

	return nil
}

// CleanupOldIDs will remove deployments with the old ID
func (h *Handler) CleanupOldIDs(req router.Request, _ router.Response) error {
	name := req.Object.GetName()
	if !strings.HasPrefix(name, "mcp") || len(name) < 16 {
		return nil
	}

	return apply.New(req.Client).WithNamespace(h.mcpDeploymentNamespace).WithOwnerSubContext(name).WithPruneTypes(
		new(appsv1.Deployment), new(corev1.Secret), new(corev1.Service),
	).Apply(req.Ctx, nil)
}

// getDeploymentStatus determines the overall deployment status based on conditions
func getDeploymentStatus(deployment *appsv1.Deployment) string {
	for _, condition := range deployment.Status.Conditions {
		switch condition.Type {
		case appsv1.DeploymentAvailable:
			switch condition.Status {
			case corev1.ConditionTrue:
				return "Available"
			case corev1.ConditionFalse:
				return "Unavailable"
			}
		}
	}

	if deployment.Status.ReadyReplicas > 0 {
		return "Progressing"
	}

	return "Unknown"
}

// getDeploymentConditions extracts key deployment conditions
func getDeploymentConditions(deployment *appsv1.Deployment) []v1.DeploymentCondition {
	conditions := make([]v1.DeploymentCondition, 0, len(deployment.Status.Conditions))
	for _, condition := range deployment.Status.Conditions {
		conditions = append(conditions, v1.DeploymentCondition{
			Type:               condition.Type,
			Status:             condition.Status,
			Reason:             condition.Reason,
			Message:            condition.Message,
			LastTransitionTime: condition.LastTransitionTime,
			LastUpdateTime:     condition.LastUpdateTime,
		})
	}
	return conditions
}

// Helper functions for comparing values
func int32PtrEqual(a, b *int32) bool {
	if (a == nil) != (b == nil) {
		return false
	}
	return a == nil || *a == *b
}
