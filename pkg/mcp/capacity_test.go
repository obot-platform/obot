package mcp

import (
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func usageRequests(memory, cpu string) corev1.ResourceRequirements {
	return corev1.ResourceRequirements{Requests: corev1.ResourceList{
		corev1.ResourceMemory: resource.MustParse(memory),
		corev1.ResourceCPU:    resource.MustParse(cpu),
	}}
}

func usageDeployment(name, memory, cpu string) appsv1.Deployment {
	return appsv1.Deployment{
		Name:       name,
		Generation: 1,
		Spec: appsv1.DeploymentSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "mcp", Resources: usageRequests(memory, cpu)}},
		}}},
		Status: appsv1.DeploymentStatus{ObservedGeneration: 1, UpdatedReplicas: 1},
	}
}

func usagePod(app, memory, cpu string, phase corev1.PodPhase) corev1.Pod {
	return corev1.Pod{
		Labels: map[string]string{"app": app},
		Spec:   corev1.PodSpec{Containers: []corev1.Container{{Name: "mcp", Resources: usageRequests(memory, cpu)}}},
		Status: corev1.PodStatus{Phase: phase},
	}
}

func TestRunningUsage(t *testing.T) {
	for _, tt := range []struct {
		name        string
		deployments []appsv1.Deployment
		pods        []corev1.Pod
		want        types.EntitlementsUsage
	}{
		{
			name: "nothing deployed",
		},
		{
			name:        "deployment without pods counts its template",
			deployments: []appsv1.Deployment{usageDeployment("a", "512Mi", "250m")},
			want: types.EntitlementsUsage{
				HostedMCPServers: 1,
				MemoryMiB:        512,
				CPUMillicores:    250,
			},
		},
		{
			name:        "a rollout's surge pod counts",
			deployments: []appsv1.Deployment{usageDeployment("a", "512Mi", "250m")},
			pods: []corev1.Pod{
				usagePod("a", "512Mi", "250m", corev1.PodRunning),
				usagePod("a", "512Mi", "250m", corev1.PodPending),
			},
			want: types.EntitlementsUsage{
				HostedMCPServers: 1,
				MemoryMiB:        1024,
				CPUMillicores:    500,
			},
		},
		{
			name:        "finished pods and pods of other deployments don't count",
			deployments: []appsv1.Deployment{usageDeployment("a", "512Mi", "250m")},
			pods: []corev1.Pod{
				usagePod("a", "512Mi", "250m", corev1.PodRunning),
				usagePod("a", "512Mi", "250m", corev1.PodSucceeded),
				usagePod("a", "512Mi", "250m", corev1.PodFailed),
				usagePod("gone", "4Gi", "2", corev1.PodRunning),
			},
			want: types.EntitlementsUsage{
				HostedMCPServers: 1,
				MemoryMiB:        512,
				CPUMillicores:    250,
			},
		},
		{
			name: "memory rounds up to MiB per server",
			deployments: []appsv1.Deployment{
				usageDeployment("a", "100M", "100m"),
				usageDeployment("b", "100M", "100m"),
			},
			want: types.EntitlementsUsage{
				HostedMCPServers: 2,
				MemoryMiB:        192,
				CPUMillicores:    200,
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := RunningUsage(tt.deployments, tt.pods); got != tt.want {
				t.Fatalf("RunningUsage() = %+v, want %+v", got, tt.want)
			}
		})
	}
}
