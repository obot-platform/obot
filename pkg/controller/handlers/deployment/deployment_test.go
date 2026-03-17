package deployment

import (
	"context"
	"testing"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestUpdateMCPServerStatusClearsNeedsK8sUpdateWhenDeploymentCatchesUp(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := appsv1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add apps scheme: %v", err)
	}
	if err := v1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add obot scheme: %v", err)
	}

	k8sSettings := &v1.K8sSettings{
		ObjectMeta: metav1.ObjectMeta{
			Name:      system.K8sSettingsName,
			Namespace: system.DefaultNamespace,
		},
		Spec: v1.K8sSettingsSpec{},
	}
	currentHash := mcp.ComputeK8sSettingsHash(k8sSettings.Spec)

	server := &v1.MCPServer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ms1agent",
			Namespace: system.DefaultNamespace,
		},
		Spec: v1.MCPServerSpec{
			Manifest: types.MCPServerManifest{
				Runtime: types.RuntimeContainerized,
				ContainerizedConfig: &types.ContainerizedRuntimeConfig{
					Image: "example.com/nanobot:v1",
				},
			},
		},
		Status: v1.MCPServerStatus{
			NeedsK8sUpdate:  true,
			K8sSettingsHash: "outdated-hash",
		},
	}

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      server.Name,
			Namespace: system.DefaultNamespace,
			Labels: map[string]string{
				"app": server.Name,
			},
			Annotations: map[string]string{
				"obot.ai/k8s-settings-hash": currentHash,
			},
		},
		Spec: appsv1.DeploymentSpec{},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&v1.MCPServer{}).
		WithObjects(server, deployment, k8sSettings).
		Build()

	h := &Handler{
		mcpNamespace:  system.DefaultNamespace,
		storageClient: client,
	}
	req := router.Request{
		Ctx:    context.Background(),
		Client: client,
		Object: deployment,
	}

	if err := h.UpdateMCPServerStatus(req, nil); err != nil {
		t.Fatalf("UpdateMCPServerStatus() error = %v", err)
	}

	var updated v1.MCPServer
	if err := client.Get(context.Background(), kclient.ObjectKeyFromObject(server), &updated); err != nil {
		t.Fatalf("failed to get updated server: %v", err)
	}

	if updated.Status.NeedsK8sUpdate {
		t.Fatal("expected NeedsK8sUpdate to be false after deployment catches up")
	}
	if updated.Status.K8sSettingsHash != currentHash {
		t.Fatalf("expected K8sSettingsHash %q, got %q", currentHash, updated.Status.K8sSettingsHash)
	}
}
