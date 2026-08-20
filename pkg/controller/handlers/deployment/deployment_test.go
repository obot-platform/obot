package deployment

import (
	"testing"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type liveSessionManager struct{}

func (*liveSessionManager) MCPRuntimeBackend() string {
	return mcp.RuntimeBackendKubernetes
}

func TestUpdateMCPServerStatusUsesCurrentResourceMaximums(t *testing.T) {
	oldMaximum := resource.MustParse("5m")
	newMaximum := resource.MustParse("10m")
	oldSettingsSpec := v1.K8sSettingsSpec{MaxCPURequest: &oldMaximum}
	newSettingsSpec := v1.K8sSettingsSpec{MaxCPURequest: &newMaximum}
	oldHash := mcp.ComputeK8sSettingsHash(
		oldSettingsSpec,
		nil,
		types.RuntimeNPX,
		false,
		nil,
	)
	newHash := mcp.ComputeK8sSettingsHash(
		newSettingsSpec,
		nil,
		types.RuntimeNPX,
		false,
		nil,
	)
	require.NotEqual(t, oldHash, newHash)

	server := &v1.MCPServer{
		Name:      "mcp-server",
		Namespace: system.DefaultNamespace,
		Spec: v1.MCPServerSpec{
			Manifest: types.MCPServerManifest{Runtime: types.RuntimeNPX},
		},
		Status: v1.MCPServerStatus{
			K8sSettingsHash: oldHash,
			NeedsK8sUpdate:  true,
		},
	}
	settings := &v1.K8sSettings{
		Name:      system.K8sSettingsName,
		Namespace: system.DefaultNamespace,
		Spec:      newSettingsSpec,
	}
	storageClient := fake.NewClientBuilder().
		WithScheme(storagescheme.Scheme).
		WithStatusSubresource(&v1.MCPServer{}).
		WithObjects(server, settings).
		Build()
	manager := &liveSessionManager{}
	handler := &Handler{
		mcpDeploymentNamespace: "obot-mcp",
		mcpNamespace:           system.DefaultNamespace,
		storageClient:          storageClient,
		mcpRuntimeBackend:      mcp.RuntimeBackendKubernetes,
		mcpSessionManager:      manager,
	}
	deployment := &appsv1.Deployment{
		Name:        server.Name,
		Annotations: map[string]string{"obot.ai/k8s-settings-hash": newHash},
		Labels:      map[string]string{"app": server.Name},
	}

	err := handler.UpdateMCPServerStatus(router.Request{
		Client: storageClient,
		Ctx:    t.Context(),
		Object: deployment,
	}, &router.ResponseWrapper{})
	require.NoError(t, err)

	var updated v1.MCPServer
	require.NoError(t, storageClient.Get(t.Context(), router.Key(server.Namespace, server.Name), &updated))
	require.Equal(t, newHash, updated.Status.K8sSettingsHash)
	require.False(t, updated.Status.NeedsK8sUpdate)
}
