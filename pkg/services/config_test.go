package services

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/obot-platform/obot/pkg/agentbackend"
	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/rest"
)

func TestNewAgentBackend(t *testing.T) {
	tests := []struct {
		name       string
		kind       string
		mcpBackend string
		devMode    bool
		wantKind   string
		wantErr    bool
		wantActive bool
	}{
		// Unset follows the MCP runtime rather than defaulting on its own, so a
		// deployment names its backend once. There is no docker agent backend,
		// so a docker MCP runtime lands on fake.
		{name: "unset follows a docker MCP runtime", mcpBackend: "docker", wantKind: "fake", wantActive: true},
		{name: "unset with no MCP runtime configured", wantKind: "fake", wantActive: true},
		{name: "unset follows a kubernetes MCP runtime", mcpBackend: "kubernetes", wantErr: true},
		{name: "unset follows the k8s alias", mcpBackend: "k8s", wantErr: true},
		{name: "development defaults fake", devMode: true, wantKind: "fake", wantActive: true},
		// An explicit value always wins over the MCP runtime, in both directions.
		{name: "explicit disabled under a kubernetes MCP runtime", kind: "disabled", mcpBackend: "kubernetes", wantKind: "disabled"},
		{name: "explicit disabled in development", kind: "disabled", devMode: true, wantKind: "disabled"},
		{name: "explicit fake", kind: "FAKE", wantKind: "fake", wantActive: true},
		// The Kubernetes backend needs a cluster, so selecting it without one
		// has to fail at startup rather than at the first reconcile.
		{name: "kubernetes without a cluster", kind: "kubernetes", wantErr: true},
		{name: "explicit kubernetes under a docker MCP runtime", kind: "kubernetes", mcpBackend: "docker", wantErr: true},
		{name: "discobox removed", kind: "discobox", wantErr: true},
		{name: "unknown", kind: "other", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := Config{
				HostedAgentsBackend: tt.kind,
				DevMode:             tt.devMode,

				MCPRuntimeBackend: tt.mcpBackend}
			kind, backend, err := newHostedAgentsBackend(config, nil, nil, nil)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected an error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if kind != tt.wantKind {
				t.Fatalf("expected kind %q, got %q", tt.wantKind, kind)
			}
			_, err = backend.ObserveInstance(t.Context(), agentbackend.InstanceRef{ID: "test"})
			if tt.wantActive && errors.Is(err, agentbackend.ErrDisabled) {
				t.Fatal("expected an active backend")
			}
			if !tt.wantActive && !errors.Is(err, agentbackend.ErrDisabled) {
				t.Fatalf("expected disabled backend, got %v", err)
			}
		})
	}
}

func TestParseHostedAgentPodSchedulingSettings(t *testing.T) {
	tests := []struct {
		name          string
		config        Config
		errorContains string
		validate      func(*testing.T, hostedAgentPodSchedulingSettings)
	}{
		{
			name: "empty settings",
			config: Config{
				HostedAgentsAffinity:     "{}",
				HostedAgentsTolerations:  "[]",
				HostedAgentsNodeSelector: "{}",
			},
			validate: func(t *testing.T, settings hostedAgentPodSchedulingSettings) {
				t.Helper()
				if settings.Affinity != nil || len(settings.Tolerations) != 0 || len(settings.NodeSelector) != 0 {
					t.Fatalf("expected empty scheduling settings, got %+v", settings)
				}
			},
		},
		{
			name: "valid combined settings",
			config: Config{
				HostedAgentsAffinity:     `{"nodeAffinity":{"requiredDuringSchedulingIgnoredDuringExecution":{"nodeSelectorTerms":[{"matchExpressions":[{"key":"workload","operator":"In","values":["hosted-agent"]}]}]}}}`,
				HostedAgentsTolerations:  `[{"key":"workload","operator":"Equal","value":"hosted-agent","effect":"NoSchedule"}]`,
				HostedAgentsNodeSelector: `{"node-pool":"agents"}`,
			},
			validate: func(t *testing.T, settings hostedAgentPodSchedulingSettings) {
				t.Helper()
				if settings.Affinity == nil || settings.Affinity.NodeAffinity == nil {
					t.Fatal("expected node affinity")
				}
				if len(settings.Tolerations) != 1 || settings.Tolerations[0].Key != "workload" {
					t.Fatalf("unexpected tolerations: %+v", settings.Tolerations)
				}
				if settings.NodeSelector["node-pool"] != "agents" {
					t.Fatalf("unexpected node selector: %+v", settings.NodeSelector)
				}
			},
		},
		{
			name:          "malformed affinity",
			config:        Config{HostedAgentsAffinity: `{invalid`},
			errorContains: "failed to parse hosted agent affinity",
		},
		{
			name:          "unknown affinity field",
			config:        Config{HostedAgentsAffinity: `{"unknownField":true}`},
			errorContains: "unknown field",
		},
		{
			name:          "tolerations have wrong type",
			config:        Config{HostedAgentsTolerations: `{}`},
			errorContains: "failed to parse hosted agent tolerations",
		},
		{
			name:          "node selector has wrong value type",
			config:        Config{HostedAgentsNodeSelector: `{"node-pool":1}`},
			errorContains: "failed to parse hosted agent node selector",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			settings, err := parseHostedAgentPodSchedulingSettings(tt.config)
			if tt.errorContains != "" {
				if err == nil || !strings.Contains(err.Error(), tt.errorContains) {
					t.Fatalf("expected error containing %q, got %v", tt.errorContains, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.validate != nil {
				tt.validate(t, settings)
			}
		})
	}
}

func TestLeaderElectionRESTConfig(t *testing.T) {
	tests := []struct {
		name     string
		timeout  time.Duration
		expected time.Duration
	}{
		{name: "unset", expected: leaderElectionRequestTimeout},
		{name: "longer", timeout: time.Minute, expected: leaderElectionRequestTimeout},
		{name: "shorter", timeout: time.Second, expected: time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			original := &rest.Config{Timeout: tt.timeout}
			result := leaderElectionRESTConfig(original)

			if result == original {
				t.Fatal("expected the REST config to be copied")
			}
			if result.Timeout != tt.expected {
				t.Fatalf("expected timeout %s, got %s", tt.expected, result.Timeout)
			}
			if original.Timeout != tt.timeout {
				t.Fatalf("original timeout changed from %s to %s", tt.timeout, original.Timeout)
			}
		})
	}
}

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
			name: "resource maximums are independently Helm managed",
			opts: mcp.Options{
				MCPK8sMaxCPURequest:    "500m",
				MCPK8sMaxCPULimit:      "2",
				MCPK8sMaxMemoryRequest: "512Mi",
				MCPK8sMaxMemoryLimit:   "2Gi",
			},
			validateResult: func(t *testing.T, spec *v1.K8sSettingsSpec) {
				t.Helper()
				if spec.SetViaHelm {
					t.Error("expected scheduling settings to remain UI managed")
				}
				if !spec.MaximumsSetViaHelm {
					t.Error("expected resource maximums to be Helm managed")
				}
				if spec.MaxCPURequest == nil || spec.MaxCPURequest.String() != "500m" {
					t.Errorf("expected max CPU request 500m, got %v", spec.MaxCPURequest)
				}
				if spec.MaxMemoryLimit == nil || spec.MaxMemoryLimit.String() != "2Gi" {
					t.Errorf("expected max memory limit 2Gi, got %v", spec.MaxMemoryLimit)
				}
			},
		},
		{
			name: "valid nanobot agent resources only",
			opts: mcp.Options{
				MCPK8sSettingsNanobotAgentResources: `{"limits":{"memory":"1Gi"},"requests":{"memory":"512Mi"}}`,
			},
			expectError: false,
			validateResult: func(t *testing.T, spec *v1.K8sSettingsSpec) {
				t.Helper()
				if spec.NanobotAgentResources == nil {
					t.Error("expected nanobot agent resources to be set")
					return
				}
				memoryRequest := spec.NanobotAgentResources.Requests[corev1.ResourceMemory]
				if memoryRequest.String() != "512Mi" {
					t.Errorf("expected memory request '512Mi', got '%s'", memoryRequest.String())
				}
			},
		},
		{
			name: "all valid fields combined",
			opts: mcp.Options{
				MCPK8sSettingsAffinity:              `{"nodeAffinity":{"requiredDuringSchedulingIgnoredDuringExecution":{"nodeSelectorTerms":[{"matchExpressions":[{"key":"disktype","operator":"In","values":["ssd"]}]}]}}}`,
				MCPK8sSettingsTolerations:           `[{"key":"key1","operator":"Equal","value":"value1","effect":"NoSchedule"}]`,
				MCPK8sSettingsResources:             `{"limits":{"cpu":"2","memory":"4Gi"}}`,
				MCPK8sSettingsNanobotAgentResources: `{"requests":{"memory":"512Mi"}}`,
				MCPK8sSettingsRuntimeClassName:      "gvisor",
				MCPK8sSettingsStorageClassName:      "fast-ssd",
				MCPK8sSettingsNanobotWorkspaceSize:  "5Gi",
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
				if spec.NanobotAgentResources == nil {
					t.Error("expected nanobot agent resources to be set")
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
