package license

import (
	"fmt"
	"math"
	"strconv"
	"testing"

	keygen "github.com/keygen-sh/keygen-go/v3"
	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	kfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestProviderDeviceLimit(t *testing.T) {
	tests := []struct {
		name         string
		entitlements []string
		want         gatewayclient.DeviceLimit
	}{
		{
			name: "default",
			want: gatewayclient.DeviceLimit{Maximum: gatewayclient.DefaultDeviceLimit},
		},
		{
			name:         "unrelated entitlement",
			entitlements: []string{EnterpriseModelProvidersEntitlement, "OBOT_ENTERPRISE_500_USERS"},
			want:         gatewayclient.DeviceLimit{Maximum: gatewayclient.DefaultDeviceLimit},
		},
		{
			name:         "enterprise edition grants unlimited devices",
			entitlements: []string{EnterpriseEntitlement},
			want:         gatewayclient.DeviceLimit{Unlimited: true},
		},
		{
			name: "malformed device limit entitlements",
			entitlements: []string{
				"OBOT_ENTERPRISE_DEVICES",
				"OBOT_ENTERPRISE_NOT_A_NUMBER_DEVICES",
				"OBOT_ENTERPRISE_DEVICE_LIMIT",
				"OBOT_ENTERPRISE_NOT_A_NUMBER_DEVICE_LIMIT",
				"OBOT_ENTERPRISE_500_DEVICE_LIMIT",
				"OBOT_ENTERPRISE_+500_DEVICES",
				"OBOT_ENTERPRISE_-1_DEVICES",
				"OBOT_ENTERPRISE_500_DEVICE",
				"OBOT_ENTERPRISE_500_DEVICES_EXTRA",
				"OBOT_ENTERPRISE_500_DEVICE_LIMIT_EXTRA",
				"OBOT_ENTERPRISE_0_DEVICES",
			},
			want: gatewayclient.DeviceLimit{Maximum: gatewayclient.DefaultDeviceLimit},
		},
		{
			name:         "numeric devices entitlement",
			entitlements: []string{"OBOT_ENTERPRISE_500_DEVICES"},
			want:         gatewayclient.DeviceLimit{Maximum: 500},
		},
		{
			name: "numeric device entitlements are additive",
			entitlements: []string{
				"OBOT_ENTERPRISE_100_DEVICES",
				"OBOT_ENTERPRISE_50_DEVICES",
			},
			want: gatewayclient.DeviceLimit{Maximum: 150},
		},
		{
			name: "all numeric device entitlements are additive",
			entitlements: []string{
				"OBOT_ENTERPRISE_250_DEVICES",
				"OBOT_ENTERPRISE_1000_DEVICES",
				"OBOT_ENTERPRISE_500_DEVICES",
			},
			want: gatewayclient.DeviceLimit{Maximum: 1750},
		},
		{
			name: "numeric device entitlements override enterprise edition unlimited devices",
			entitlements: []string{
				EnterpriseEntitlement,
				"OBOT_ENTERPRISE_100_DEVICES",
				"OBOT_ENTERPRISE_50_DEVICES",
			},
			want: gatewayclient.DeviceLimit{Maximum: 150},
		},
		{
			name: "malformed numeric entitlements do not override enterprise edition unlimited devices",
			entitlements: []string{
				EnterpriseEntitlement,
				"OBOT_ENTERPRISE_0_DEVICES",
				"OBOT_ENTERPRISE_NOT_A_NUMBER_DEVICES",
			},
			want: gatewayclient.DeviceLimit{Unlimited: true},
		},
		{
			name:         "removed custom device limit entitlement is ignored",
			entitlements: []string{"OBOT_ENTERPRISE_CUSTOM_DEVICE_LIMIT"},
			want:         gatewayclient.DeviceLimit{Maximum: gatewayclient.DefaultDeviceLimit},
		},
		{
			name:         "numeric entitlement replaces default",
			entitlements: []string{"OBOT_ENTERPRISE_50_DEVICES"},
			want:         gatewayclient.DeviceLimit{Maximum: 50},
		},
		{
			name: "numeric entitlement sum saturates at maximum integer",
			entitlements: []string{
				"OBOT_ENTERPRISE_" + strconv.FormatInt(math.MaxInt64, 10) + "_DEVICES",
				"OBOT_ENTERPRISE_1_DEVICES",
			},
			want: gatewayclient.DeviceLimit{Maximum: math.MaxInt64},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entitlements := make(map[keygen.EntitlementCode]struct{}, len(tt.entitlements))
			for _, entitlement := range tt.entitlements {
				entitlements[keygen.EntitlementCode(entitlement)] = struct{}{}
			}

			provider := &Provider{entitlements: entitlements}
			got, err := provider.DeviceLimit(t.Context())
			if err != nil {
				t.Fatalf("DeviceLimit() error = %v, want nil", err)
			}
			if got != tt.want {
				t.Fatalf("DeviceLimit() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestGetLicenseViolationsDeviceLimit(t *testing.T) {
	tests := []struct {
		name         string
		entitlements []string
		deviceCount  int
		want         *Violation
	}{
		{
			name:         "at limit",
			entitlements: []string{"OBOT_ENTERPRISE_2_DEVICES"},
			deviceCount:  2,
		},
		{
			name:         "over limit",
			entitlements: []string{"OBOT_ENTERPRISE_1_DEVICES"},
			deviceCount:  2,
			want: &Violation{
				Type:    "deviceLimit",
				Message: "device count (2) exceeds maximum limit (1)",
			},
		},
		{
			name:         "unlimited",
			entitlements: []string{EnterpriseEntitlement},
			deviceCount:  2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gatewayClient := newTestLicenseGatewayClient(t)
			configuration, err := gatewayClient.CreateMDMConfiguration(t.Context(), 1, &gatewaytypes.MDMConfiguration{})
			if err != nil {
				t.Fatalf("creating MDM configuration: %v", err)
			}
			for i := range tt.deviceCount {
				if _, err := gatewayClient.EnrollDevice(t.Context(), gatewayclient.DeviceEnrollment{
					DeviceID:           fmt.Sprintf("device-%d", i),
					MDMConfigurationID: configuration.ID,
					PublicKey:          []byte{byte(i)},
				}, gatewayclient.DeviceLimit{Unlimited: true}); err != nil {
					t.Fatalf("enrolling device %d: %v", i, err)
				}
			}

			entitlements := make(map[keygen.EntitlementCode]struct{}, len(tt.entitlements))
			for _, entitlement := range tt.entitlements {
				entitlements[keygen.EntitlementCode(entitlement)] = struct{}{}
			}
			provider := &Provider{
				gatewayClient: gatewayClient,
				entitlements:  entitlements,
			}
			storageClient := kfake.NewClientBuilder().WithScheme(storagescheme.Scheme).Build()

			violations, err := provider.GetLicenseViolations(t.Context(), storageClient)
			if err != nil {
				t.Fatalf("GetLicenseViolations() error = %v", err)
			}

			var got *Violation
			for _, violation := range violations {
				if violation.Type == "deviceLimit" {
					violation := violation
					got = &violation
					break
				}
			}
			if tt.want == nil {
				if got != nil {
					t.Fatalf("device limit violation = %+v, want nil", got)
				}
				return
			}
			if got == nil {
				t.Fatalf("device limit violation = nil, want %+v", *tt.want)
			}
			if got.Type != tt.want.Type || got.Message != tt.want.Message {
				t.Fatalf("device limit violation = %+v, want %+v", *got, *tt.want)
			}
		})
	}
}
