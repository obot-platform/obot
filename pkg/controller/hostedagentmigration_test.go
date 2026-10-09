package controller

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	schedulingv1 "k8s.io/api/scheduling/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestDeleteHostedAgentRuntime(t *testing.T) {
	var owned []kclient.Object
	for _, obj := range []kclient.Object{
		&appsv1.Deployment{}, &batchv1.Job{}, &appsv1.ReplicaSet{}, &corev1.Pod{},
		&corev1.Secret{}, &corev1.Service{}, &corev1.ResourceQuota{}, &corev1.PersistentVolumeClaim{}, &schedulingv1.PriorityClass{},
	} {
		obj.SetName("legacy")
		obj.SetNamespace("mcp")
		obj.SetLabels(map[string]string{"obot.ai/managed-by": "obot-agent-backend"})
		if _, ok := obj.(*schedulingv1.PriorityClass); ok {
			obj.SetNamespace("")
			obj.SetOwnerReferences([]metav1.OwnerReference{{APIVersion: "v1", Kind: "Namespace", Name: "mcp"}})
		}
		owned = append(owned, obj)
	}
	otherNamespace := owned[4].DeepCopyObject().(kclient.Object)
	otherNamespace.SetNamespace("other")
	otherPool := owned[8].DeepCopyObject().(kclient.Object)
	otherPool.SetName("other")
	otherPool.SetOwnerReferences([]metav1.OwnerReference{{APIVersion: "v1", Kind: "Namespace", Name: "other"}})
	mcp := &corev1.Secret{}
	mcp.SetName("mcp-secret")
	mcp.SetNamespace("mcp")
	kept := []kclient.Object{otherNamespace, otherPool, mcp}
	client := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(owned...).WithObjects(kept...).Build()

	require.NoError(t, deleteHostedAgentRuntime(t.Context(), client, "mcp"))
	for _, obj := range owned {
		require.True(t, apierrors.IsNotFound(client.Get(t.Context(), kclient.ObjectKeyFromObject(obj), obj)))
	}
	for _, obj := range kept {
		require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(obj), obj))
	}
	require.NoError(t, deleteHostedAgentRuntime(t.Context(), client, "mcp"))
}

func TestHostedAgentCleanupWaitsForWorkloads(t *testing.T) {
	pod := &corev1.Pod{}
	pod.SetName("blocked")
	pod.SetNamespace("mcp")
	pod.SetLabels(map[string]string{"obot.ai/managed-by": "obot-agent-backend"})
	pod.SetFinalizers([]string{"example.com/blocked"})
	secret := &corev1.Secret{}
	secret.SetName("credential")
	secret.SetNamespace("mcp")
	secret.SetLabels(pod.GetLabels())
	client := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(pod, secret).Build()
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()

	require.ErrorIs(t, deleteHostedAgentRuntime(ctx, client, "mcp"), context.DeadlineExceeded)
	require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(secret), secret))
	require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(pod), pod))
	require.Equal(t, []string{"example.com/blocked"}, pod.GetFinalizers())
}
