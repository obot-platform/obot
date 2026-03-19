package deployment

import (
	"fmt"
	"slices"
	"strings"

	"github.com/obot-platform/nah/pkg/apply"
	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/mcp"
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
// to the corresponding MCPServer object based on the "app" label
func (h *Handler) UpdateMCPServerStatus(req router.Request, _ router.Response) error {
	deployment := req.Object.(*appsv1.Deployment)

	// Get the MCP server name from the deployment label
	mcpServerName, exists := deployment.Labels["app"]
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

	// Extract K8s settings hash from deployment annotation (only for Kubernetes runtime)
	k8sSettingsHash := deployment.Annotations["obot.ai/k8s-settings-hash"]

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

	// Manage NeedsK8sUpdate flag for K8s-compatible runtimes
	isK8sRuntime := mcpServer.Spec.Manifest.Runtime == types.RuntimeContainerized ||
		mcpServer.Spec.Manifest.Runtime == types.RuntimeUVX ||
		mcpServer.Spec.Manifest.Runtime == types.RuntimeNPX

	if isK8sRuntime {
		// Get current K8s settings to compare
		var k8sSettings v1.K8sSettings
		if err := h.storageClient.Get(req.Ctx, kclient.ObjectKey{
			Namespace: h.mcpNamespace,
			Name:      system.K8sSettingsName,
		}, &k8sSettings); err == nil {
			currentHash := mcp.ComputeK8sSettingsHash(k8sSettings.Spec)

			// Update K8sSettingsHash from deployment only if:
			// 1. The MCPServer has no hash yet (empty), OR
			// 2. The deployment's hash matches the current K8sSettings (deployment is up-to-date)
			// This prevents overwriting a hash that was set by the API handler during a redeploy
			// before the deployment has been updated.
			if k8sSettingsHash != "" {
				if mcpServer.Status.K8sSettingsHash == "" || k8sSettingsHash == currentHash {
					if mcpServer.Status.K8sSettingsHash != k8sSettingsHash {
						mcpServer.Status.K8sSettingsHash = k8sSettingsHash
						needsUpdate = true
					}
				}
			}

			// Only set NeedsK8sUpdate if:
			// 1. It's not already set
			// 2. The deployment has a hash (not initializing)
			// 3. The deployment's hash doesn't match current K8sSettings
			// 4. The MCPServer's expected hash also doesn't match current K8sSettings
			//    (if MCPServer already expects the current hash, a redeploy is pending)
			if !mcpServer.Status.NeedsK8sUpdate {
				if k8sSettingsHash != currentHash && mcpServer.Status.K8sSettingsHash != currentHash {
					mcpServer.Status.NeedsK8sUpdate = true
					needsUpdate = true
				}
			}
		}
	} else {
		// For non-K8s runtimes, just sync the hash from the deployment annotation
		if mcpServer.Status.K8sSettingsHash != k8sSettingsHash {
			mcpServer.Status.K8sSettingsHash = k8sSettingsHash
			needsUpdate = true
		}
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
	var availableCondition, progressingCondition *appsv1.DeploymentCondition

	// Collect both conditions
	for i := range deployment.Status.Conditions {
		condition := &deployment.Status.Conditions[i]
		switch condition.Type {
		case appsv1.DeploymentAvailable:
			availableCondition = condition
		case appsv1.DeploymentProgressing:
			progressingCondition = condition
		}
	}

	if progressingCondition != nil && progressingCondition.Status == corev1.ConditionFalse {
		if progressingCondition.Reason == "ProgressDeadlineExceeded" {
			// Rollout is stuck (after deadline)
			return "Needs Attention"
		}
		// Other failures (FailedCreate, FailedPlacement, etc.)
		return "Progressing"
	}

	if deployment.Status.UnavailableReplicas > 0 &&
		deployment.Status.UpdatedReplicas > 0 &&
		deployment.Generation == deployment.Status.ObservedGeneration {
		return "Progressing"
	}

	if availableCondition != nil {
		switch availableCondition.Status {
		case corev1.ConditionTrue:
			return "Available"
		case corev1.ConditionFalse:
			return "Unavailable"
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
