package mcp

import (
	"testing"

	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestParseResourceMaximums(t *testing.T) {
	maximums, err := ParseResourceMaximums(Options{
		MCPK8sMaxCPURequest:    "500m",
		MCPK8sMaxCPULimit:      "1",
		MCPK8sMaxMemoryRequest: "512Mi",
		MCPK8sMaxMemoryLimit:   "1Gi",
	})
	if err != nil {
		t.Fatalf("ParseResourceMaximums() error = %v", err)
	}
	if got, want := maximums.CPURequest.String(), "500m"; got != want {
		t.Fatalf("CPU request maximum = %q, want %q", got, want)
	}
	if got, want := maximums.CPULimit.String(), "1"; got != want {
		t.Fatalf("CPU limit maximum = %q, want %q", got, want)
	}
	if got, want := maximums.MemoryRequest.String(), "512Mi"; got != want {
		t.Fatalf("memory request maximum = %q, want %q", got, want)
	}
	if got, want := maximums.MemoryLimit.String(), "1Gi"; got != want {
		t.Fatalf("memory limit maximum = %q, want %q", got, want)
	}
}

func TestParseResourceMaximumsRejectsInvalidValues(t *testing.T) {
	if _, err := ParseResourceMaximums(Options{MCPK8sMaxCPURequest: "nope"}); err == nil {
		t.Fatal("expected invalid CPU request maximum to fail")
	}
	if _, err := ParseResourceMaximums(Options{MCPK8sMaxMemoryLimit: "-1Gi"}); err == nil {
		t.Fatal("expected negative memory limit maximum to fail")
	}
}

func TestValidateK8sSettingsResourceMaximumsUsesEffectiveDefaults(t *testing.T) {
	maximums := ResourceMaximums{MemoryRequest: new(resource.MustParse("256Mi"))}
	settings := v1.K8sSettingsSpec{
		Resources: &corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceMemory: resource.MustParse("128Mi"),
			},
		},
		NanobotAgentResources: &corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceMemory: resource.MustParse("512Mi"),
			},
		},
	}

	if err := ValidateK8sSettingsResourceMaximums(settings, maximums); err == nil {
		t.Fatal("expected nanobot agent default memory request to exceed maximum")
	}
}

func TestValidateK8sSettingsResourceMaximumsAllowsDefaultsBelowMaximum(t *testing.T) {
	maximums := ResourceMaximums{MemoryRequest: new(resource.MustParse("256Mi"))}
	settings := v1.K8sSettingsSpec{
		Resources: &corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceMemory: resource.MustParse("128Mi"),
			},
		},
		NanobotAgentResources: &corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceMemory: resource.MustParse("192Mi"),
			},
		},
	}

	if err := ValidateK8sSettingsResourceMaximums(settings, maximums); err != nil {
		t.Fatalf("expected defaults below maximum to pass: %v", err)
	}
}

func TestValidateConfiguredK8sSettingsResourceMaximumsSkipsUnconfiguredDefaults(t *testing.T) {
	maximums := ResourceMaximums{MemoryRequest: new(resource.MustParse("1Mi"))}

	if err := ValidateConfiguredK8sSettingsResourceMaximums(v1.K8sSettingsSpec{}, maximums); err != nil {
		t.Fatalf("expected unconfigured defaults to skip startup validation: %v", err)
	}
}

func TestValidateConfiguredK8sSettingsResourceMaximumsRejectsConfiguredDefaultAboveMaximum(t *testing.T) {
	maximums := ResourceMaximums{MemoryRequest: new(resource.MustParse("256Mi"))}
	settings := v1.K8sSettingsSpec{
		Resources: &corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceMemory: resource.MustParse("512Mi"),
			},
		},
	}

	if err := ValidateConfiguredK8sSettingsResourceMaximums(settings, maximums); err == nil {
		t.Fatal("expected configured default memory request to exceed maximum")
	}
}
