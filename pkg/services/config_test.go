package services

import (
	"strings"
	"testing"

	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	corev1 "k8s.io/api/core/v1"
)

func TestParsePodSchedulingSettingsFromHelm(t *testing.T) {
	tests := []struct {
		name           string
		opts           mcp.Options
		expectError    bool
		errorContains  string
		expectNil      bool
		validateResult func(t *testing.T, spec *v1.K8sSettingsSpec)
	}{
		// Valid cases
		{
			name: "empty settings - all fields empty",
			opts: mcp.Options{
				MCPK8sSettingsAffinity:    "",
				MCPK8sSettingsTolerations: "",
				MCPK8sSettingsResources:   "",
			},
			expectNil: true,
		},
		{
			name: "valid affinity only",
			opts: mcp.Options{
				MCPK8sSettingsAffinity: `{"nodeAffinity":{"requiredDuringSchedulingIgnoredDuringExecution":{"nodeSelectorTerms":[{"matchExpressions":[{"key":"disktype","operator":"In","values":["ssd"]}]}]}}}`,
			},
			expectError: false,
			validateResult: func(t *testing.T, spec *v1.K8sSettingsSpec) {
				t.Helper()
				if spec.Affinity == nil {
					t.Error("expected affinity to be set")
					return
				}
				if spec.Affinity.NodeAffinity == nil {
					t.Error("expected node affinity to be set")
					return
				}
			},
		},
		{
			name: "valid tolerations only",
			opts: mcp.Options{
				MCPK8sSettingsTolerations: `[{"key":"key1","operator":"Equal","value":"value1","effect":"NoSchedule"}]`,
			},
			expectError: false,
			validateResult: func(t *testing.T, spec *v1.K8sSettingsSpec) {
				t.Helper()
				if len(spec.Tolerations) != 1 {
					t.Errorf("expected 1 toleration, got %d", len(spec.Tolerations))
					return
				}
				if spec.Tolerations[0].Key != "key1" {
					t.Errorf("expected key 'key1', got '%s'", spec.Tolerations[0].Key)
				}
			},
		},
		{
			name: "valid resources only",
			opts: mcp.Options{
				MCPK8sSettingsResources: `{"limits":{"cpu":"2","memory":"4Gi"},"requests":{"cpu":"1","memory":"2Gi"}}`,
			},
			expectError: false,
			validateResult: func(t *testing.T, spec *v1.K8sSettingsSpec) {
				t.Helper()
				if spec.Resources == nil {
					t.Error("expected resources to be set")
					return
				}
				cpuLimit := spec.Resources.Limits[corev1.ResourceCPU]
				if cpuLimit.String() != "2" {
					t.Errorf("expected cpu limit '2', got '%s'", cpuLimit.String())
				}
			},
		},
		{
			name: "all valid fields combined",
			opts: mcp.Options{
				MCPK8sSettingsAffinity:             `{"nodeAffinity":{"requiredDuringSchedulingIgnoredDuringExecution":{"nodeSelectorTerms":[{"matchExpressions":[{"key":"disktype","operator":"In","values":["ssd"]}]}]}}}`,
				MCPK8sSettingsTolerations:          `[{"key":"key1","operator":"Equal","value":"value1","effect":"NoSchedule"}]`,
				MCPK8sSettingsResources:            `{"limits":{"cpu":"2","memory":"4Gi"}}`,
				MCPK8sSettingsRuntimeClassName:     "gvisor",
				MCPK8sSettingsStorageClassName:     "fast-ssd",
				MCPK8sSettingsNanobotWorkspaceSize: "5Gi",
			},
			expectError: false,
			validateResult: func(t *testing.T, spec *v1.K8sSettingsSpec) {
				t.Helper()
				if spec.Affinity == nil {
					t.Error("expected affinity to be set")
				}
				if len(spec.Tolerations) != 1 {
					t.Error("expected tolerations to be set")
				}
				if spec.Resources == nil {
					t.Error("expected resources to be set")
				}
				if spec.RuntimeClassName == nil || *spec.RuntimeClassName != "gvisor" {
					t.Error("expected runtimeClassName to be 'gvisor'")
				}
				if spec.StorageClassName == nil || *spec.StorageClassName != "fast-ssd" {
					t.Error("expected storageClassName to be 'fast-ssd'")
				}
				if spec.NanobotWorkspaceSize != "5Gi" {
					t.Error("expected nanobotWorkspaceSize to be '5Gi'")
				}
			},
		},

		// Invalid cases - unknown fields (these should fail after implementing strict validation)
		{
			name: "affinity with unknown field",
			opts: mcp.Options{
				MCPK8sSettingsAffinity: `{"nodeAffinity":{"requiredDuringSchedulingIgnoredDuringExecution":{"nodeSelectorTerms":[{"matchExpressions":[{"key":"disktype","operator":"In","values":["ssd"]}]}]}},"unknownField":"invalid"}`,
			},
			expectError:   true,
			errorContains: "unknown field",
		},
		{
			name: "tolerations with unknown field",
			opts: mcp.Options{
				MCPK8sSettingsTolerations: `[{"key":"key1","operator":"Equal","value":"value1","effect":"NoSchedule","unknownField":"invalid"}]`,
			},
			expectError:   true,
			errorContains: "unknown field",
		},
		{
			name: "resources with unknown field",
			opts: mcp.Options{
				MCPK8sSettingsResources: `{"limits":{"cpu":"2"},"unknownField":"invalid"}`,
			},
			expectError:   true,
			errorContains: "unknown field",
		},

		// Invalid cases - malformed JSON
		{
			name: "affinity with malformed JSON",
			opts: mcp.Options{
				MCPK8sSettingsAffinity: `{invalid json`,
			},
			expectError:   true,
			errorContains: "failed to parse affinity from Helm",
		},
		{
			name: "tolerations with malformed JSON",
			opts: mcp.Options{
				MCPK8sSettingsTolerations: `[invalid json`,
			},
			expectError:   true,
			errorContains: "failed to parse tolerations from Helm",
		},
		{
			name: "resources with malformed JSON",
			opts: mcp.Options{
				MCPK8sSettingsResources: `{invalid json`,
			},
			expectError:   true,
			errorContains: "failed to parse resources from Helm",
		},

		// Invalid cases - wrong type
		{
			name: "affinity with wrong type (array instead of object)",
			opts: mcp.Options{
				MCPK8sSettingsAffinity: `[]`,
			},
			expectError:   true,
			errorContains: "failed to parse affinity from Helm",
		},
		{
			name: "tolerations with wrong type (object instead of array)",
			opts: mcp.Options{
				MCPK8sSettingsTolerations: `{}`,
			},
			expectError:   true,
			errorContains: "failed to parse tolerations from Helm",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parsePodSchedulingSettingsFromHelm(tt.opts)

			// Check error expectation
			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
					return
				}
				if tt.errorContains != "" && !strings.Contains(err.Error(), tt.errorContains) {
					t.Errorf("expected error to contain '%s', got: %v", tt.errorContains, err)
				}
				return
			}

			// Check for unexpected error
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			// Check nil expectation
			if tt.expectNil {
				if result != nil {
					t.Errorf("expected nil result, got: %+v", result)
				}
				return
			}

			// Validate result
			if result == nil {
				t.Error("expected non-nil result")
				return
			}

			if tt.validateResult != nil {
				tt.validateResult(t, result)
			}
		})
	}
}

func TestParsePSASettingsFromHelm(t *testing.T) {
	tests := []struct {
		name           string
		opts           mcp.Options
		expectError    bool
		errorContains  string
		expectNil      bool
		validateResult func(t *testing.T, psa *v1.PodSecurityAdmissionSettings)
	}{
		{
			name:      "no PSA settings",
			opts:      mcp.Options{},
			expectNil: true,
		},
		{
			name: "PSA enabled with defaults",
			opts: mcp.Options{
				MCPPodSecurityEnabled:        true,
				MCPPodSecurityEnforce:        "restricted",
				MCPPodSecurityEnforceVersion: "latest",
				MCPPodSecurityAudit:          "restricted",
				MCPPodSecurityAuditVersion:   "latest",
				MCPPodSecurityWarn:           "restricted",
				MCPPodSecurityWarnVersion:    "latest",
			},
			expectError: false,
			validateResult: func(t *testing.T, psa *v1.PodSecurityAdmissionSettings) {
				t.Helper()
				if !psa.Enabled {
					t.Error("expected PSA to be enabled")
				}
				if psa.Enforce != "restricted" {
					t.Errorf("expected enforce 'restricted', got '%s'", psa.Enforce)
				}
			},
		},
		{
			name: "PSA with baseline level",
			opts: mcp.Options{
				MCPPodSecurityEnabled:        true,
				MCPPodSecurityEnforce:        "baseline",
				MCPPodSecurityEnforceVersion: "v1.28",
				MCPPodSecurityAudit:          "baseline",
				MCPPodSecurityAuditVersion:   "v1.28",
				MCPPodSecurityWarn:           "baseline",
				MCPPodSecurityWarnVersion:    "v1.28",
			},
			expectError: false,
			validateResult: func(t *testing.T, psa *v1.PodSecurityAdmissionSettings) {
				t.Helper()
				if psa.Enforce != "baseline" {
					t.Errorf("expected enforce 'baseline', got '%s'", psa.Enforce)
				}
				if psa.EnforceVersion != "v1.28" {
					t.Errorf("expected enforce version 'v1.28', got '%s'", psa.EnforceVersion)
				}
			},
		},
		{
			name: "PSA with privileged level",
			opts: mcp.Options{
				MCPPodSecurityEnabled: true,
				MCPPodSecurityEnforce: "privileged",
			},
			expectError: false,
			validateResult: func(t *testing.T, psa *v1.PodSecurityAdmissionSettings) {
				t.Helper()
				if psa.Enforce != "privileged" {
					t.Errorf("expected enforce 'privileged', got '%s'", psa.Enforce)
				}
			},
		},
		{
			name: "invalid PSA enforce level",
			opts: mcp.Options{
				MCPPodSecurityEnabled: true,
				MCPPodSecurityEnforce: "invalid-level",
			},
			expectError:   true,
			errorContains: "invalid PSA enforce level",
		},
		{
			name: "invalid PSA audit level",
			opts: mcp.Options{
				MCPPodSecurityEnabled: true,
				MCPPodSecurityAudit:   "invalid-level",
			},
			expectError:   true,
			errorContains: "invalid PSA audit level",
		},
		{
			name: "invalid PSA warn level",
			opts: mcp.Options{
				MCPPodSecurityEnabled: true,
				MCPPodSecurityWarn:    "invalid-level",
			},
			expectError:   true,
			errorContains: "invalid PSA warn level",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parsePSASettingsFromHelm(tt.opts)

			// Check error expectation
			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
					return
				}
				if tt.errorContains != "" && !strings.Contains(err.Error(), tt.errorContains) {
					t.Errorf("expected error to contain '%s', got: %v", tt.errorContains, err)
				}
				return
			}

			// Check for unexpected error
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			// Check nil expectation
			if tt.expectNil {
				if result != nil {
					t.Errorf("expected nil result, got: %+v", result)
				}
				return
			}

			// Validate result
			if result == nil {
				t.Error("expected non-nil result")
				return
			}

			if tt.validateResult != nil {
				tt.validateResult(t, result)
			}
		})
	}
}

// TestPSASettingsIndependentOfPodScheduling verifies that PSA settings don't affect
// whether pod scheduling settings are considered "set via Helm"
func TestPSASettingsIndependentOfPodScheduling(t *testing.T) {
	// When only PSA settings are provided (with defaults), pod scheduling should be nil
	opts := mcp.Options{
		MCPPodSecurityEnabled:        true,
		MCPPodSecurityEnforce:        "restricted",
		MCPPodSecurityEnforceVersion: "latest",
		MCPPodSecurityAudit:          "restricted",
		MCPPodSecurityAuditVersion:   "latest",
		MCPPodSecurityWarn:           "restricted",
		MCPPodSecurityWarnVersion:    "latest",
	}

	podScheduling, err := parsePodSchedulingSettingsFromHelm(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if podScheduling != nil {
		t.Error("expected pod scheduling settings to be nil when only PSA is set")
	}

	psaSettings, err := parsePSASettingsFromHelm(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if psaSettings == nil {
		t.Error("expected PSA settings to be non-nil")
	}
}
