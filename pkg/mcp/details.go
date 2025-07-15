package mcp

import (
	"context"
	"fmt"
	"io"
	"sort"

	"github.com/obot-platform/obot/apiclient/types"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/fields"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (sm *SessionManager) GetServerDetails(ctx context.Context, serverConfig ServerConfig) (types.MCPServerDetails, error) {
	if serverConfig.URL != "" {
		return types.MCPServerDetails{}, fmt.Errorf("getting server details is not supported for remote servers")
	}

	if !sm.KubernetesEnabled() {
		return types.MCPServerDetails{}, fmt.Errorf("kubernetes is not enabled")
	}

	id := sessionID(serverConfig)

	var deployment appsv1.Deployment
	if err := sm.client.Get(ctx, client.ObjectKey{Name: id, Namespace: sm.mcpNamespace}, &deployment); err != nil {
		if apierrors.IsNotFound(err) {
			return types.MCPServerDetails{}, fmt.Errorf("mcp server %s is not running", id)
		}

		return types.MCPServerDetails{}, fmt.Errorf("failed to get deployment %s: %w", id, err)
	}

	var (
		lastRestart types.Time
		pods        corev1.PodList
		podEvents   []corev1.Event
	)
	if err := sm.client.List(ctx, &pods, client.InNamespace(sm.mcpNamespace), client.MatchingLabels(deployment.Spec.Selector.MatchLabels)); err != nil {
		return types.MCPServerDetails{}, fmt.Errorf("failed to get pods: %w", err)
	}

	for _, pod := range pods.Items {
		if pod.Status.Phase == corev1.PodRunning {
			lastRestart = types.Time{Time: pod.CreationTimestamp.Time}
		}

		var eventList corev1.EventList
		if err := sm.client.List(ctx, &eventList, client.InNamespace(sm.mcpNamespace), client.MatchingFieldsSelector{
			Selector: fields.SelectorFromSet(map[string]string{
				"involvedObject.kind":      "Pod",
				"involvedObject.name":      pod.Name,
				"involvedObject.namespace": pod.Namespace,
			}),
		}); err != nil {
			return types.MCPServerDetails{}, fmt.Errorf("failed to get events: %w", err)
		}

		podEvents = append(podEvents, eventList.Items...)
	}

	var deploymentEvents corev1.EventList
	if err := sm.client.List(ctx, &deploymentEvents, client.InNamespace(sm.mcpNamespace), client.MatchingFieldsSelector{
		Selector: fields.SelectorFromSet(map[string]string{
			"involvedObject.kind":      "Deployment",
			"involvedObject.name":      deployment.Name,
			"involvedObject.namespace": deployment.Namespace,
		}),
	}); err != nil {
		return types.MCPServerDetails{}, fmt.Errorf("failed to get events: %w", err)
	}

	allEvents := append(deploymentEvents.Items, podEvents...)
	sort.Slice(allEvents, func(i, j int) bool {
		return allEvents[i].CreationTimestamp.Before(&allEvents[j].CreationTimestamp)
	})

	var mcpEvents []types.MCPServerEvent
	for _, event := range allEvents {
		mcpEvents = append(mcpEvents, types.MCPServerEvent{
			Time:         types.Time{Time: event.CreationTimestamp.Time},
			Reason:       event.Reason,
			Message:      event.Message,
			EventType:    event.Type,
			Action:       event.Action,
			Count:        event.Count,
			ResourceName: event.InvolvedObject.Name,
			ResourceKind: event.InvolvedObject.Kind,
		})
	}

	return types.MCPServerDetails{
		DeploymentName: deployment.Name,
		Namespace:      deployment.Namespace,
		LastRestart:    lastRestart,
		ReadyReplicas:  deployment.Status.ReadyReplicas,
		Replicas:       deployment.Status.Replicas,
		IsAvailable:    deployment.Status.ReadyReplicas > 0,
		Events:         mcpEvents,
	}, nil
}

func (sm *SessionManager) StreamServerLogs(ctx context.Context, serverConfig ServerConfig) (io.ReadCloser, error) {
	if serverConfig.URL != "" {
		return nil, fmt.Errorf("streaming logs is not supported for remote servers")
	}

	if !sm.KubernetesEnabled() {
		return nil, fmt.Errorf("kubernetes is not enabled")
	}

	id := sessionID(serverConfig)

	var deployment appsv1.Deployment
	if err := sm.client.Get(ctx, client.ObjectKey{Name: id, Namespace: sm.mcpNamespace}, &deployment); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("mcp server %s is not running", id)
		}

		return nil, fmt.Errorf("failed to get deployment %s: %w", id, err)
	}

	var pods corev1.PodList
	if err := sm.client.List(ctx, &pods, client.InNamespace(sm.mcpNamespace), client.MatchingLabels(deployment.Spec.Selector.MatchLabels)); err != nil {
		return nil, fmt.Errorf("failed to get pods: %w", err)
	}

	if len(pods.Items) == 0 {
		return nil, fmt.Errorf("no pods found for deployment %s", id)
	}

	tailLines := int64(100)
	logs, err := sm.clientset.CoreV1().Pods(sm.mcpNamespace).GetLogs(pods.Items[0].Name, &corev1.PodLogOptions{
		Follow:     true,
		Timestamps: true,
		TailLines:  &tailLines,
	}).Stream(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get logs: %w", err)
	}

	return logs, nil
}
