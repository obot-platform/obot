package license

import (
	"math"
	"strconv"
	"testing"

	keygen "github.com/keygen-sh/keygen-go/v3"
	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
)

func TestProviderUserLimit(t *testing.T) {
	tests := []struct {
		name         string
		entitlements []string
		want         gatewayclient.UserLimit
	}{
		{
			name: "default",
			want: gatewayclient.UserLimit{Maximum: gatewayclient.DefaultUserLimit},
		},
		{
			name:         "unrelated entitlement",
			entitlements: []string{EnterpriseModelProvidersEntitlement},
			want:         gatewayclient.UserLimit{Maximum: gatewayclient.DefaultUserLimit},
		},
		{
			name:         "enterprise edition grants unlimited users",
			entitlements: []string{EnterpriseEntitlement},
			want:         gatewayclient.UserLimit{Unlimited: true},
		},
		{
			name: "malformed user limit entitlements",
			entitlements: []string{
				"OBOT_ENTERPRISE_USERS",
				"OBOT_ENTERPRISE_NOT_A_NUMBER_USERS",
				"OBOT_ENTERPRISE_USER_LIMIT",
				"OBOT_ENTERPRISE_NOT_A_NUMBER_USER_LIMIT",
				"OBOT_ENTERPRISE_500_USER_LIMIT",
				"OBOT_ENTERPRISE_+500_USERS",
				"OBOT_ENTERPRISE_-1_USERS",
				"OBOT_ENTERPRISE_500_USER",
				"OBOT_ENTERPRISE_500_USERS_EXTRA",
				"OBOT_ENTERPRISE_500_USER_LIMIT_EXTRA",
				"OBOT_ENTERPRISE_0_USERS",
			},
			want: gatewayclient.UserLimit{Maximum: gatewayclient.DefaultUserLimit},
		},
		{
			name:         "numeric users entitlement",
			entitlements: []string{"OBOT_ENTERPRISE_500_USERS"},
			want:         gatewayclient.UserLimit{Maximum: 500},
		},
		{
			name: "numeric user entitlements are additive",
			entitlements: []string{
				"OBOT_ENTERPRISE_100_USERS",
				"OBOT_ENTERPRISE_50_USERS",
			},
			want: gatewayclient.UserLimit{Maximum: 150},
		},
		{
			name: "all numeric user entitlements are additive",
			entitlements: []string{
				"OBOT_ENTERPRISE_250_USERS",
				"OBOT_ENTERPRISE_1000_USERS",
				"OBOT_ENTERPRISE_500_USERS",
			},
			want: gatewayclient.UserLimit{Maximum: 1750},
		},
		{
			name: "numeric user entitlements override enterprise edition unlimited users",
			entitlements: []string{
				EnterpriseEntitlement,
				"OBOT_ENTERPRISE_100_USERS",
				"OBOT_ENTERPRISE_50_USERS",
			},
			want: gatewayclient.UserLimit{Maximum: 150},
		},
		{
			name: "malformed numeric entitlements do not override enterprise edition unlimited users",
			entitlements: []string{
				EnterpriseEntitlement,
				"OBOT_ENTERPRISE_0_USERS",
				"OBOT_ENTERPRISE_NOT_A_NUMBER_USERS",
			},
			want: gatewayclient.UserLimit{Unlimited: true},
		},
		{
			name:         "removed custom user limit entitlement is ignored",
			entitlements: []string{"OBOT_ENTERPRISE_CUSTOM_USER_LIMIT"},
			want:         gatewayclient.UserLimit{Maximum: gatewayclient.DefaultUserLimit},
		},
		{
			name:         "numeric entitlement replaces default",
			entitlements: []string{"OBOT_ENTERPRISE_50_USERS"},
			want:         gatewayclient.UserLimit{Maximum: 50},
		},
		{
			name: "numeric entitlement sum saturates at maximum integer",
			entitlements: []string{
				"OBOT_ENTERPRISE_" + strconv.Itoa(math.MaxInt) + "_USERS",
				"OBOT_ENTERPRISE_1_USERS",
			},
			want: gatewayclient.UserLimit{Maximum: math.MaxInt},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entitlements := make(map[keygen.EntitlementCode]struct{}, len(tt.entitlements))
			for _, entitlement := range tt.entitlements {
				entitlements[keygen.EntitlementCode(entitlement)] = struct{}{}
			}

			provider := &Provider{entitlements: entitlements}
			got, err := provider.UserLimit(t.Context())
			if err != nil {
				t.Fatalf("UserLimit() error = %v, want nil", err)
			}
			if got != tt.want {
				t.Fatalf("UserLimit() = %+v, want %+v", got, tt.want)
			}
		})
	}
}
