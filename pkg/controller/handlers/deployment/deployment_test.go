package deployment

import (
	"context"
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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type liveSessionManager struct {
	calls    int
	maximums mcp.ResourceMaximums
}

func (*liveSessionManager) MCPRuntimeBackend() string {
	return mcp.RuntimeBackendKubernetes
}

func (m *liveSessionManager) EffectiveKubernetesResourceMaximums(
	context.Context,
	kclient.Client,
) (mcp.ResourceMaximums, error) {
	m.calls++
	return m.maximums, nil
}

func TestUpdateMCPServerStatusUsesCurrentResourceMaximums(t *testing.T) {
	oldMaximum := resource.MustParse("5m")
	newMaximum := resource.MustParse("10m")
	settingsSpec := v1.K8sSettingsSpec{}
	oldMaximums := mcp.ResourceMaximums{CPURequest: &oldMaximum}
	newMaximums := mcp.ResourceMaximums{CPURequest: &newMaximum}
	oldHash := mcp.ComputeK8sSettingsHash(
		settingsSpec,
		nil,
		types.RuntimeNPX,
		false,
		oldMaximums,
		nil,
	)
	newHash := mcp.ComputeK8sSettingsHash(
		settingsSpec,
		nil,
		types.RuntimeNPX,
		false,
		newMaximums,
		nil,
	)
	require.NotEqual(t, oldHash, newHash)

	server := &v1.MCPServer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mcp-server",
			Namespace: system.DefaultNamespace,
		},
		Spec: v1.MCPServerSpec{
			Manifest: types.MCPServerManifest{Runtime: types.RuntimeNPX},
		},
		Status: v1.MCPServerStatus{
			K8sSettingsHash: oldHash,
			NeedsK8sUpdate:  true,
		},
	}
	settings := &v1.K8sSettings{
		ObjectMeta: metav1.ObjectMeta{
			Name:      system.K8sSettingsName,
			Namespace: system.DefaultNamespace,
		},
		Spec: settingsSpec,
	}
	storageClient := fake.NewClientBuilder().
		WithScheme(storagescheme.Scheme).
		WithStatusSubresource(&v1.MCPServer{}).
		WithObjects(server, settings).
		Build()
	manager := &liveSessionManager{maximums: newMaximums}
	handler := &Handler{
		mcpDeploymentNamespace: "obot-mcp",
		mcpNamespace:           system.DefaultNamespace,
		storageClient:          storageClient,
		mcpRuntimeBackend:      mcp.RuntimeBackendKubernetes,
		mcpSessionManager:      manager,
	}
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:        server.Name,
			Annotations: map[string]string{"obot.ai/k8s-settings-hash": newHash},
			Labels:      map[string]string{"app": server.Name},
		},
	}

	err := handler.UpdateMCPServerStatus(router.Request{
		Client: storageClient,
		Ctx:    t.Context(),
		Object: deployment,
	}, &router.ResponseWrapper{})
	require.NoError(t, err)
	require.Equal(t, 1, manager.calls)

	var updated v1.MCPServer
	require.NoError(t, storageClient.Get(t.Context(), router.Key(server.Namespace, server.Name), &updated))
	require.Equal(t, newHash, updated.Status.K8sSettingsHash)
	require.False(t, updated.Status.NeedsK8sUpdate)
}
