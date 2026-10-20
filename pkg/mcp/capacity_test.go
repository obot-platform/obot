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

func TestStartUsage(t *testing.T) {
	existing := usageDeployment("a", "512Mi", "250m")
	changed := usageDeployment("a", "1Gi", "250m")

	for _, tt := range []struct {
		name              string
		desired, existing *appsv1.Deployment
		want              types.EntitlementsUsage
	}{
		{
			name:    "a new server",
			desired: &existing,
			want: types.EntitlementsUsage{
				HostedMCPServers: 1,
				MemoryMiB:        512,
				CPUMillicores:    250,
			},
		},
		{
			name:     "an unchanged server adds nothing",
			desired:  &existing,
			existing: &existing,
		},
		{
			name:     "a changed server's new pod runs beside the old one",
			desired:  &changed,
			existing: &existing,
			want: types.EntitlementsUsage{
				MemoryMiB:     1024,
				CPUMillicores: 250,
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := startUsage(tt.desired, tt.existing); got != tt.want {
				t.Fatalf("startUsage() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestPlanHasCapacity(t *testing.T) {
	limit := func(n int64) *int64 { return &n }

	for _, tt := range []struct {
		name    string
		limits  types.Entitlements
		running types.EntitlementsUsage
		start   types.EntitlementsUsage
		want    bool
	}{
		{
			name:  "unlimited",
			start: types.EntitlementsUsage{HostedMCPServers: 1, MemoryMiB: 512},
			want:  true,
		},
		{
			name:    "within the limits",
			limits:  types.Entitlements{HostedMCPServers: limit(3), MemoryMiB: limit(2048)},
			running: types.EntitlementsUsage{HostedMCPServers: 2, MemoryMiB: 1024},
			start:   types.EntitlementsUsage{HostedMCPServers: 1, MemoryMiB: 1024},
			want:    true,
		},
		{
			name:    "one server past the limit",
			limits:  types.Entitlements{HostedMCPServers: limit(3)},
			running: types.EntitlementsUsage{HostedMCPServers: 3},
			start:   types.EntitlementsUsage{HostedMCPServers: 1},
		},
		{
			name:    "memory past the limit",
			limits:  types.Entitlements{MemoryMiB: limit(1024)},
			running: types.EntitlementsUsage{MemoryMiB: 768},
			start:   types.EntitlementsUsage{MemoryMiB: 512},
		},
		{
			name:   "a zero limit allows none",
			limits: types.Entitlements{HostedMCPServers: limit(0)},
			start:  types.EntitlementsUsage{HostedMCPServers: 1},
		},
		{
			name:    "a start that doesn't grow a dimension isn't refused for it",
			limits:  types.Entitlements{MemoryMiB: limit(1024), CPUMillicores: limit(1000)},
			running: types.EntitlementsUsage{MemoryMiB: 2048, CPUMillicores: 500},
			start:   types.EntitlementsUsage{CPUMillicores: 250},
			want:    true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := planHasCapacity(tt.limits, tt.running, tt.start); got != tt.want {
				t.Fatalf("planHasCapacity() = %t, want %t", got, tt.want)
			}
		})
	}
}
