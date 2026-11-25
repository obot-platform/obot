package services

import (
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/obot-platform/obot/pkg/mcp"
	"github.com/obot-platform/obot/pkg/producttelemetry"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/rest"
)

func TestLeaderElectionRESTConfig(t *testing.T) {
	tests := []struct {
		name     string
		timeout  time.Duration
		expected time.Duration
	}{
		{
			name:     "unset",
			expected: leaderElectionRequestTimeout,
		},
		{
			name:     "longer",
			timeout:  time.Minute,
			expected: leaderElectionRequestTimeout,
		},
		{
			name:     "shorter",
			timeout:  time.Second,
			expected: time.Second,
		},
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
			name: "all valid fields combined",
			opts: mcp.Options{
				MCPK8sSettingsAffinity:         `{"nodeAffinity":{"requiredDuringSchedulingIgnoredDuringExecution":{"nodeSelectorTerms":[{"matchExpressions":[{"key":"disktype","operator":"In","values":["ssd"]}]}]}}}`,
				MCPK8sSettingsTolerations:      `[{"key":"key1","operator":"Equal","value":"value1","effect":"NoSchedule"}]`,
				MCPK8sSettingsResources:        `{"limits":{"cpu":"2","memory":"4Gi"}}`,
				MCPK8sSettingsRuntimeClassName: "gvisor",
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

func TestWithURLScheme(t *testing.T) {
	for _, tc := range []struct {
		hostname string
		expected string
	}{
		{
			hostname: "obot.example.ai",
			expected: "https://obot.example.ai",
		},
		{
			hostname: "httpdocs.example.com",
			expected: "https://httpdocs.example.com",
		},
		{
			hostname: "localhost:8080",
			expected: "http://localhost:8080",
		},
		{
			hostname: "127.0.0.1:5173",
			expected: "http://127.0.0.1:5173",
		},
		{
			hostname: "http://ui.internal:5173",
			expected: "http://ui.internal:5173",
		},
		{
			hostname: "https://obot.example.ai",
			expected: "https://obot.example.ai",
		},
	} {
		t.Run(tc.hostname, func(t *testing.T) {
			if got := withURLScheme(tc.hostname); got != tc.expected {
				t.Fatalf("withURLScheme(%q) = %q, want %q", tc.hostname, got, tc.expected)
			}
		})
	}
}

func TestApplyAirgap(t *testing.T) {
	const (
		mcpCatalog    = "https://github.com/obot-platform/mcp-catalog/v2-schema"
		systemCatalog = "https://github.com/obot-platform/system-mcp-catalog/v2-schema"
		modelProxyURL = "https://model-service.obot.ai"
	)

	tests := []struct {
		name              string
		config            Config
		wantAnalyticsMode producttelemetry.Mode
		wantModelProxyURL string
		wantMCPCatalog    string
		wantSystemCatalog string
	}{
		{
			name: "not air-gapped",
			config: Config{
				ProductAnalyticsMode:        producttelemetry.ModeOn,
				ModelProxyURL:               modelProxyURL,
				DefaultMCPCatalogPath:       mcpCatalog,
				DefaultSystemMCPCatalogPath: systemCatalog,
			},
			wantAnalyticsMode: producttelemetry.ModeOn,
			wantModelProxyURL: modelProxyURL,
			wantMCPCatalog:    mcpCatalog,
			wantSystemCatalog: systemCatalog,
		},
		{
			name: "air-gapped with the image's catalogs",
			config: Config{
				Airgap:                      true,
				ProductAnalyticsMode:        producttelemetry.ModeConsent,
				ModelProxyURL:               modelProxyURL,
				DefaultMCPCatalogPath:       mcpCatalog,
				DefaultSystemMCPCatalogPath: systemCatalog,
			},
			wantAnalyticsMode: producttelemetry.ModeOff,
			wantSystemCatalog: systemMCPCatalogImagePath,
		},
		{
			name: "air-gapped with repository catalog URLs",
			config: Config{
				Airgap:                      true,
				ProductAnalyticsMode:        producttelemetry.ModeOn,
				ModelProxyURL:               "https://proxy.example.com",
				DefaultMCPCatalogPath:       "https://github.com/obot-platform/mcp-catalog/",
				DefaultSystemMCPCatalogPath: "https://github.com/obot-platform/system-mcp-catalog",
			},
			wantAnalyticsMode: producttelemetry.ModeOff,
			wantSystemCatalog: systemMCPCatalogImagePath,
		},
		{
			name: "air-gapped without catalogs",
			config: Config{
				Airgap: true,
			},
			wantAnalyticsMode: producttelemetry.ModeOff,
			wantSystemCatalog: systemMCPCatalogImagePath,
		},
		{
			name: "air-gapped with mirrored catalogs",
			config: Config{
				Airgap:                      true,
				DefaultMCPCatalogPath:       "https://git.example.com/obot-platform/mcp-catalog-mirror",
				DefaultSystemMCPCatalogPath: "/catalogs/system",
			},
			wantAnalyticsMode: producttelemetry.ModeOff,
			wantMCPCatalog:    "https://git.example.com/obot-platform/mcp-catalog-mirror",
			wantSystemCatalog: "/catalogs/system",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := applyAirgap(tt.config)
			if got.ProductAnalyticsMode != tt.wantAnalyticsMode {
				t.Errorf("ProductAnalyticsMode = %q, want %q", got.ProductAnalyticsMode, tt.wantAnalyticsMode)
			}
			if got.ModelProxyURL != tt.wantModelProxyURL {
				t.Errorf("ModelProxyURL = %q, want %q", got.ModelProxyURL, tt.wantModelProxyURL)
			}
			if got.DisableUpdateCheck != tt.config.Airgap {
				t.Errorf("DisableUpdateCheck = %v, want %v", got.DisableUpdateCheck, tt.config.Airgap)
			}
			if got.DefaultMCPCatalogPath != tt.wantMCPCatalog {
				t.Errorf("DefaultMCPCatalogPath = %q, want %q", got.DefaultMCPCatalogPath, tt.wantMCPCatalog)
			}
			if got.DefaultSystemMCPCatalogPath != tt.wantSystemCatalog {
				t.Errorf("DefaultSystemMCPCatalogPath = %q, want %q", got.DefaultSystemMCPCatalogPath, tt.wantSystemCatalog)
			}
		})
	}
}

func TestApplyImageRegistry(t *testing.T) {
	const obotImage = "ghcr.io/obot-platform/example:v1"

	// Every image setting is rewritten, including the ones this test doesn't name.
	settings := ImageSettings()
	if !slices.ContainsFunc(settings, func(setting reflect.StructField) bool { return setting.Name == "MCPBaseImage" }) {
		t.Fatalf("ImageSettings() = %v, want it to include MCPBaseImage", settings)
	}
	config := Config{ImageRegistry: "registry.example.com/mirror"}
	for _, setting := range settings {
		reflect.ValueOf(&config).Elem().FieldByIndex(setting.Index).SetString(obotImage)
	}

	got := reflect.ValueOf(applyImageRegistry(config))
	for _, setting := range settings {
		if image, want := got.FieldByIndex(setting.Index).String(), "registry.example.com/mirror/obot-platform/example:v1"; image != want {
			t.Errorf("%s = %q, want %q", setting.Name, image, want)
		}
	}

	// Without a registry, and for images that aren't Obot's, nothing changes.
	config = Config{
		MCPBaseImage:    obotImage,
		MCPOpenAPIImage: "registry.example.com/openapi:v1",
		ImageRegistry:   "registry.example.com/mirror",
	}
	if image := applyImageRegistry(config).MCPOpenAPIImage; image != "registry.example.com/openapi:v1" {
		t.Errorf("MCPOpenAPIImage = %q, want it unchanged", image)
	}
	config.ImageRegistry = ""
	if image := applyImageRegistry(config).MCPBaseImage; image != obotImage {
		t.Errorf("MCPBaseImage = %q, want it unchanged without a registry", image)
	}
}
