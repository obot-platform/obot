package kubernetes

import (
	"testing"

	"github.com/obot-platform/obot/pkg/agentbackend"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func waitingPod(reason, message string) corev1.Pod {
	return corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "sandbox"},
		Status: corev1.PodStatus{
			Phase: corev1.PodPending,
			ContainerStatuses: []corev1.ContainerStatus{{
				Name:  "agent",
				State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: reason, Message: message}},
			}},
		},
	}
}

func TestClassifyDeploymentReadyWhenReplicaAvailable(t *testing.T) {
	deployment := &appsv1.Deployment{Status: appsv1.DeploymentStatus{ReadyReplicas: 1}}

	state, reason, message := classifyDeployment(deployment, nil)

	if state != agentbackend.StateReady {
		t.Fatalf("expected ready, got %s (%s: %s)", state, reason, message)
	}
}

// A quota rejection never reaches the caller that created the Deployment; it
// lands on the ReplicaSet and is surfaced as a Deployment condition.
func TestClassifyDeploymentReportsQuotaRejection(t *testing.T) {
	deployment := &appsv1.Deployment{
		Status: appsv1.DeploymentStatus{
			Conditions: []appsv1.DeploymentCondition{{
				Type:    appsv1.DeploymentReplicaFailure,
				Status:  corev1.ConditionTrue,
				Reason:  "FailedCreate",
				Message: `pods "x" is forbidden: exceeded quota: obot-pool-a`,
			}},
		},
	}

	state, reason, message := classifyDeployment(deployment, nil)

	if state != agentbackend.StateError {
		t.Fatalf("expected error, got %s", state)
	}
	if reason != "FailedCreate" {
		t.Errorf("expected FailedCreate reason, got %q", reason)
	}
	if message == "" {
		t.Error("expected the quota message to be preserved")
	}
}

// A pool is pinned to one node by its shared volume, so an unschedulable
// sandbox never recovers by moving. Reporting pending would hide a full pool.
func TestClassifyDeploymentTreatsUnschedulableAsError(t *testing.T) {
	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "sandbox"},
		Status: corev1.PodStatus{
			Phase: corev1.PodPending,
			Conditions: []corev1.PodCondition{{
				Type:    corev1.PodScheduled,
				Status:  corev1.ConditionFalse,
				Reason:  corev1.PodReasonUnschedulable,
				Message: "0/1 nodes are available: insufficient memory",
			}},
		},
	}

	state, reason, _ := classifyDeployment(&appsv1.Deployment{}, []corev1.Pod{pod})

	if state != agentbackend.StateError || reason != "Unschedulable" {
		t.Fatalf("expected Unschedulable error, got %s/%s", state, reason)
	}
}

func TestClassifyDeploymentPodReasons(t *testing.T) {
	for _, tt := range []struct {
		reason string
		state  agentbackend.State
	}{
		{"CrashLoopBackOff", agentbackend.StateError},
		{"ImagePullBackOff", agentbackend.StateError},
		{"InvalidImageName", agentbackend.StateError},
		{"CreateContainerConfigError", agentbackend.StateError},
		{"ContainerCreating", agentbackend.StatePending},
		{"PodInitializing", agentbackend.StatePending},
	} {
		t.Run(tt.reason, func(t *testing.T) {
			pod := waitingPod(tt.reason, "detail")

			state, reason, _ := classifyDeployment(&appsv1.Deployment{}, []corev1.Pod{pod})

			if state != tt.state {
				t.Fatalf("expected %s, got %s", tt.state, state)
			}
			if reason != tt.reason {
				t.Errorf("expected reason %q, got %q", tt.reason, reason)
			}
		})
	}
}

// A Recreate rollout leaves the outgoing pod behind briefly. The newest pod is
// the one describing the revision being reconciled.
func TestClassifyDeploymentUsesNewestPod(t *testing.T) {
	old := waitingPod("CrashLoopBackOff", "old failure")
	old.CreationTimestamp = metav1.Unix(100, 0)
	current := waitingPod("ContainerCreating", "starting")
	current.CreationTimestamp = metav1.Unix(200, 0)

	state, reason, _ := classifyDeployment(&appsv1.Deployment{}, []corev1.Pod{old, current})

	if state != agentbackend.StatePending || reason != "ContainerCreating" {
		t.Fatalf("expected the newest pod to decide, got %s/%s", state, reason)
	}
}

func TestClassifyDeploymentIgnoresTerminatingPods(t *testing.T) {
	terminating := waitingPod("CrashLoopBackOff", "going away")
	terminating.CreationTimestamp = metav1.Unix(300, 0)
	now := metav1.Unix(400, 0)
	terminating.DeletionTimestamp = &now

	live := waitingPod("ContainerCreating", "starting")
	live.CreationTimestamp = metav1.Unix(200, 0)

	state, _, _ := classifyDeployment(&appsv1.Deployment{}, []corev1.Pod{terminating, live})

	if state != agentbackend.StatePending {
		t.Fatalf("expected pending from the live pod, got %s", state)
	}
}
