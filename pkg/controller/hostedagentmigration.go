package controller

import (
	"context"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	schedulingv1 "k8s.io/api/scheduling/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// Delete workloads before their volumes and credentials. Never clear Kubernetes
// finalizers: a failed teardown must leave the migration eligible for retry.
func deleteHostedAgentRuntime(ctx context.Context, client kclient.Client, namespace string) error {
	if client == nil {
		return nil // The former non-Kubernetes backend did not create real sandboxes.
	}
	if namespace == "" {
		return fmt.Errorf("MCP namespace is required for Hosted Agent cleanup")
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	for _, phase := range [][]kclient.ObjectList{
		{&appsv1.DeploymentList{}, &batchv1.JobList{}},
		{&appsv1.ReplicaSetList{}, &corev1.PodList{}},
		{&corev1.SecretList{}, &corev1.ServiceList{}, &corev1.ResourceQuotaList{}, &corev1.PersistentVolumeClaimList{}, &schedulingv1.PriorityClassList{}},
	} {
		if err := wait.PollUntilContextCancel(ctx, time.Second, true, func(ctx context.Context) (bool, error) {
			empty := true
			for _, list := range phase {
				opts := []kclient.ListOption{kclient.MatchingLabels{"obot.ai/managed-by": "obot-agent-backend"}}
				_, clusterScoped := list.(*schedulingv1.PriorityClassList)
				if !clusterScoped {
					opts = append(opts, kclient.InNamespace(namespace))
				}
				if err := client.List(ctx, list, opts...); err != nil {
					return false, fmt.Errorf("list legacy %T: %w", list, err)
				}
				items, err := meta.ExtractList(list)
				if err != nil {
					return false, err
				}
				for _, item := range items {
					obj := item.(kclient.Object)
					if clusterScoped {
						owned := false
						for _, owner := range obj.GetOwnerReferences() {
							if owner.APIVersion == "v1" && owner.Kind == "Namespace" && owner.Name == namespace {
								owned = true
							}
						}
						if !owned {
							continue
						}
					}
					empty = false
					if obj.GetDeletionTimestamp() != nil {
						continue
					}
					uid := obj.GetUID()
					err := client.Delete(ctx, obj, kclient.PropagationPolicy(metav1.DeletePropagationForeground), kclient.Preconditions{UID: &uid})
					if err := kclient.IgnoreNotFound(err); err != nil {
						return false, fmt.Errorf("delete legacy %T %s: %w", obj, obj.GetName(), err)
					}
				}
			}
			return empty, nil
		}); err != nil {
			return err
		}
	}
	return nil
}
