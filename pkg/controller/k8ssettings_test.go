package controller

import (
	"testing"

	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func TestEnsureK8sSettingsRejectsExistingConfiguredResourcesAboveStartupMaximum(t *testing.T) {
	ctx := t.Context()
	client := newFakeClient(t, &v1.K8sSettings{
		Name:      system.K8sSettingsName,
		Namespace: system.DefaultNamespace,
		Spec: v1.K8sSettingsSpec{
			Resources: &corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse("512Mi"),
				},
			},
		},
	})

	err := ensureK8sSettings(ctx, client, nil, nil, mcp.ResourceMaximums{
		MemoryRequest: new(resource.MustParse("256Mi")),
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "configured K8s settings resource defaults exceed configured MCP Kubernetes resource maximums")
	require.Contains(t, err.Error(), "resources.requests.memory 512Mi exceeds configured maximum 256Mi")
}

func TestEnsureK8sSettingsRejectsHelmConfiguredResourcesAboveStartupMaximum(t *testing.T) {
	ctx := t.Context()
	client := newFakeClient(t)

	err := ensureK8sSettings(ctx, client, &v1.K8sSettingsSpec{
		Resources: &corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceMemory: resource.MustParse("512Mi"),
			},
		},
	}, nil, mcp.ResourceMaximums{
		MemoryRequest: new(resource.MustParse("256Mi")),
	})
	require.Error(t, err)

	var settings v1.K8sSettings
	err = client.Get(ctx, kclient.ObjectKey{
		Namespace: system.DefaultNamespace,
		Name:      system.K8sSettingsName,
	}, &settings)
	require.True(t, apierrors.IsNotFound(err), "expected invalid Helm K8s settings to fail before create, got %v", err)
}

func TestEnsureK8sSettingsAllowsStartupMaximumsWithoutConfiguredResources(t *testing.T) {
	ctx := t.Context()
	client := newFakeClient(t)

	err := ensureK8sSettings(ctx, client, nil, nil, mcp.ResourceMaximums{
		MemoryRequest: new(resource.MustParse("1Mi")),
	})
	require.NoError(t, err)

	var settings v1.K8sSettings
	require.NoError(t, client.Get(ctx, kclient.ObjectKey{
		Namespace: system.DefaultNamespace,
		Name:      system.K8sSettingsName,
	}, &settings))
	require.Nil(t, settings.Spec.Resources)
	require.Nil(t, settings.Spec.NanobotAgentResources)
}

func TestEnsureK8sSettingsLocksMaximumsConfiguredThroughHelm(t *testing.T) {
	ctx := t.Context()
	existingResources := &corev1.ResourceRequirements{
		Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("250m")},
	}
	client := newFakeClient(t, &v1.K8sSettings{
		Name:      system.K8sSettingsName,
		Namespace: system.DefaultNamespace,
		Spec:      v1.K8sSettingsSpec{Resources: existingResources},
	})
	maximum := resource.MustParse("2Gi")

	require.NoError(t, ensureK8sSettings(ctx, client, &v1.K8sSettingsSpec{
		MaximumsSetViaHelm: true,
		MaxMemoryLimit:     &maximum,
	}, nil, mcp.ResourceMaximums{MemoryLimit: &maximum}))

	var settings v1.K8sSettings
	require.NoError(t, client.Get(ctx, kclient.ObjectKey{
		Namespace: system.DefaultNamespace,
		Name:      system.K8sSettingsName,
	}, &settings))
	require.False(t, settings.Spec.SetViaHelm)
	require.True(t, settings.Spec.MaximumsSetViaHelm)
	require.Equal(t, existingResources, settings.Spec.Resources)
	require.NotNil(t, settings.Spec.MaxMemoryLimit)
	require.Zero(t, settings.Spec.MaxMemoryLimit.Cmp(maximum))

	require.NoError(t, ensureK8sSettings(ctx, client, nil, nil, mcp.ResourceMaximums{}))
	require.NoError(t, client.Get(ctx, kclient.ObjectKey{
		Namespace: system.DefaultNamespace,
		Name:      system.K8sSettingsName,
	}, &settings))
	require.False(t, settings.Spec.MaximumsSetViaHelm)
	require.Nil(t, settings.Spec.MaxMemoryLimit)
	require.Equal(t, existingResources, settings.Spec.Resources)
}
