package kubernetes

import (
	"fmt"

	"github.com/obot-platform/obot/pkg/agentbackend"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

// classifyDeployment reduces Kubernetes workload status to the three states the
// agent backend contract exposes. Everything a provider would otherwise want a
// new state for is carried as a stable Reason plus a human-readable Message.
//
// "error" does not mean permanent. It means the runtime cannot currently
// realize the desired configuration, which is the distinction the contract
// draws. Obot keeps reconciling, so a transient image-pull failure recovers on
// its own while still being visible to the user rather than sitting silently in
// "pending".
func classifyDeployment(deployment *appsv1.Deployment, pods []corev1.Pod) (agentbackend.State, string, string) {
	// A quota rejection surfaces here rather than on the write that created the
	// Deployment: the Deployment is admitted, and its ReplicaSet is what fails
	// to create pods. Without this the sandbox would report "pending" forever
	// against a full pool.
	for _, condition := range deployment.Status.Conditions {
		if condition.Type == appsv1.DeploymentReplicaFailure && condition.Status == corev1.ConditionTrue {
			return agentbackend.StateError, reasonOr(condition.Reason, "ReplicaFailure"), condition.Message
		}
	}

	if state, reason, message, ok := classifyPods(pods); ok {
		return state, reason, message
	}

	if deployment.Status.ReadyReplicas > 0 {
		return agentbackend.StateReady, "", ""
	}

	for _, condition := range deployment.Status.Conditions {
		if condition.Type == appsv1.DeploymentProgressing &&
			condition.Status == corev1.ConditionFalse &&
			condition.Reason == "ProgressDeadlineExceeded" {
			return agentbackend.StateError, "ProgressDeadlineExceeded", condition.Message
		}
	}

	return agentbackend.StatePending, "Starting", "the sandbox is starting"
}

// classifyPods reports the first pod condition worth surfacing. The bool
// distinguishes "nothing notable" from a deliberate pending verdict.
func classifyPods(pods []corev1.Pod) (agentbackend.State, string, string, bool) {
	newest := newestPod(pods)
	if newest == nil {
		return "", "", "", false
	}

	if newest.Status.Reason == "Evicted" {
		return agentbackend.StateError, "Evicted", messageOr(newest.Status.Message, "the sandbox was evicted from its node"), true
	}

	switch newest.Status.Phase {
	case corev1.PodFailed:
		return agentbackend.StateError, "PodFailed", messageOr(newest.Status.Message, "the sandbox pod failed"), true
	case corev1.PodUnknown:
		return agentbackend.StateError, "PodUnknown", "the sandbox pod state cannot be determined", true
	}

	for _, condition := range newest.Status.Conditions {
		if condition.Type == corev1.PodScheduled &&
			condition.Status == corev1.ConditionFalse &&
			condition.Reason == corev1.PodReasonUnschedulable {
			// A pool is confined to one node by its shared volume, so an
			// unschedulable sandbox cannot spill elsewhere and will stay pending
			// indefinitely. Reporting it as an error is what makes a full pool
			// visible instead of silent.
			return agentbackend.StateError, "Unschedulable", messageOr(condition.Message, "the sandbox cannot be scheduled onto the pool's node"), true
		}
	}

	for _, status := range newest.Status.ContainerStatuses {
		if waiting := status.State.Waiting; waiting != nil {
			switch waiting.Reason {
			case "CrashLoopBackOff", "InvalidImageName", "CreateContainerConfigError",
				"CreateContainerError", "RunContainerError", "ImagePullBackOff", "ErrImagePull":
				return agentbackend.StateError, waiting.Reason, messageOr(waiting.Message, waiting.Reason), true
			case "ContainerCreating", "PodInitializing":
				return agentbackend.StatePending, waiting.Reason, "the sandbox container is starting", true
			}
		}

		if terminated := status.State.Terminated; terminated != nil && terminated.ExitCode != 0 {
			return agentbackend.StateError, "ContainerTerminated",
				fmt.Sprintf("the sandbox exited with code %d after %d restart(s): %s",
					terminated.ExitCode, status.RestartCount, terminated.Reason), true
		}
	}

	return "", "", "", false
}

func newestPod(pods []corev1.Pod) *corev1.Pod {
	var newest *corev1.Pod
	for i := range pods {
		if !pods[i].DeletionTimestamp.IsZero() {
			continue
		}
		if newest == nil || pods[i].CreationTimestamp.After(newest.CreationTimestamp.Time) {
			newest = &pods[i]
		}
	}
	return newest
}

func reasonOr(reason, fallback string) string {
	if reason == "" {
		return fallback
	}
	return reason
}

func messageOr(message, fallback string) string {
	if message == "" {
		return fallback
	}
	return message
}
