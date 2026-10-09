package license

import (
	"math"
	"strconv"
	"testing"

	keygen "github.com/keygen-sh/keygen-go/v3"
	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
)

func TestResourceLimitEnterpriseEntitlementsUnchanged(t *testing.T) {
	tests := []struct {
		name          string
		entitlements  []string
		suffix        string
		defaultMax    int64
		wantMaximum   int64
		wantUnlimited bool
	}{
		{
			name:        "no entitlements falls back to the default",
			suffix:      UsersEntitlementSuffix,
			defaultMax:  gatewayclient.DefaultUserLimit,
			wantMaximum: gatewayclient.DefaultUserLimit,
		},
		{
			name:          "enterprise edition alone is unlimited",
			entitlements:  []string{EnterpriseEntitlement},
			suffix:        UsersEntitlementSuffix,
			defaultMax:    gatewayclient.DefaultUserLimit,
			wantUnlimited: true,
		},
		{
			name:         "enterprise numeric entitlement",
			entitlements: []string{"OBOT_ENTERPRISE_500_USERS"},
			suffix:       UsersEntitlementSuffix,
			defaultMax:   gatewayclient.DefaultUserLimit,
			wantMaximum:  500,
		},
		{
			name: "enterprise numeric entitlements are additive",
			entitlements: []string{
				"OBOT_ENTERPRISE_250_USERS",
				"OBOT_ENTERPRISE_1000_USERS",
				"OBOT_ENTERPRISE_500_USERS",
			},
			suffix:      UsersEntitlementSuffix,
			defaultMax:  gatewayclient.DefaultUserLimit,
			wantMaximum: 1750,
		},
		{
			name: "malformed enterprise entitlements are ignored",
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
				"OBOT_ENTERPRISE_0_USERS",
				"OBOT_ENTERPRISE_CUSTOM_USER_LIMIT",
			},
			suffix:      UsersEntitlementSuffix,
			defaultMax:  gatewayclient.DefaultUserLimit,
			wantMaximum: gatewayclient.DefaultUserLimit,
		},
		{
			name:         "enterprise auth provider entitlement is not a limit",
			entitlements: []string{EnterpriseAuthProvidersEntitlement, EnterpriseModelProvidersEntitlement},
			suffix:       UsersEntitlementSuffix,
			defaultMax:   gatewayclient.DefaultUserLimit,
			wantMaximum:  gatewayclient.DefaultUserLimit,
		},
		{
			name:         "enterprise device entitlement",
			entitlements: []string{"OBOT_ENTERPRISE_25_DEVICES"},
			suffix:       DevicesEntitlementSuffix,
			defaultMax:   gatewayclient.DefaultDeviceLimit,
			wantMaximum:  25,
		},
		{
			name: "enterprise numeric entitlements saturate",
			entitlements: []string{
				"OBOT_ENTERPRISE_" + strconv.Itoa(math.MaxInt) + "_USERS",
				"OBOT_ENTERPRISE_1_USERS",
			},
			suffix:      UsersEntitlementSuffix,
			defaultMax:  gatewayclient.DefaultUserLimit,
			wantMaximum: math.MaxInt64,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			maximum, unlimited := ResourceLimit(tt.entitlements, tt.suffix, tt.defaultMax)
			if maximum != tt.wantMaximum || unlimited != tt.wantUnlimited {
				t.Fatalf("ResourceLimit() = (%d, %t), want (%d, %t)", maximum, unlimited, tt.wantMaximum, tt.wantUnlimited)
			}
		})
	}
}

func TestResourceLimitBarePrefix(t *testing.T) {
	tests := []struct {
		name          string
		entitlements  []string
		suffix        string
		defaultMax    int64
		wantMaximum   int64
		wantUnlimited bool
	}{
		{
			name:         "bare user seat entitlement",
			entitlements: []string{"OBOT_20_USERS"},
			suffix:       UsersEntitlementSuffix,
			defaultMax:   gatewayclient.DefaultUserLimit,
			wantMaximum:  20,
		},
		{
			name:         "bare hosted MCP server entitlement",
			entitlements: []string{"OBOT_15_HOSTED_MCP_SERVERS"},
			suffix:       HostedMCPServersEntitlementSuffix,
			defaultMax:   gatewayclient.DefaultHostedMCPServerLimit,
			wantMaximum:  15,
		},
		{
			name:         "bare audit log retention entitlement",
			entitlements: []string{"OBOT_30_AUDIT_LOG_DAYS"},
			suffix:       AuditLogDaysEntitlementSuffix,
			defaultMax:   gatewayclient.DefaultAuditLogRetentionDays,
			wantMaximum:  30,
		},
		{
			name: "bare and enterprise entitlements are additive",
			entitlements: []string{
				"OBOT_20_USERS",
				"OBOT_ENTERPRISE_5_USERS",
			},
			suffix:      UsersEntitlementSuffix,
			defaultMax:  gatewayclient.DefaultUserLimit,
			wantMaximum: 25,
		},
		{
			name: "malformed bare entitlements are ignored",
			entitlements: []string{
				"OBOT_USERS",
				"OBOT_NOT_A_NUMBER_USERS",
				"OBOT_+20_USERS",
				"OBOT_-1_USERS",
				"OBOT_0_USERS",
				"OBOT_20_USERS_EXTRA",
				"OBOT_20_USER",
				"OBOT_CLOUD",
				"OBOT_COMMUNITY",
			},
			suffix:      UsersEntitlementSuffix,
			defaultMax:  gatewayclient.DefaultUserLimit,
			wantMaximum: gatewayclient.DefaultUserLimit,
		},
		{
			name:         "a seat entitlement is not a hosted server entitlement",
			entitlements: []string{"OBOT_20_USERS"},
			suffix:       HostedMCPServersEntitlementSuffix,
			defaultMax:   gatewayclient.DefaultHostedMCPServerLimit,
			wantMaximum:  gatewayclient.DefaultHostedMCPServerLimit,
		},
		{
			name:          "enterprise edition leaves hosted servers unlimited",
			entitlements:  []string{EnterpriseEntitlement},
			suffix:        HostedMCPServersEntitlementSuffix,
			defaultMax:    gatewayclient.DefaultHostedMCPServerLimit,
			wantUnlimited: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			maximum, unlimited := ResourceLimit(tt.entitlements, tt.suffix, tt.defaultMax)
			if maximum != tt.wantMaximum || unlimited != tt.wantUnlimited {
				t.Fatalf("ResourceLimit() = (%d, %t), want (%d, %t)", maximum, unlimited, tt.wantMaximum, tt.wantUnlimited)
			}
		})
	}
}

func TestProviderHostedMCPServerLimit(t *testing.T) {
	tests := []struct {
		name         string
		entitlements []string
		want         gatewayclient.HostedMCPServerLimit
	}{
		{
			name: "unlicensed installations are not capped",
			want: gatewayclient.HostedMCPServerLimit{},
		},
		{
			name:         "enterprise edition is unlimited",
			entitlements: []string{EnterpriseEntitlement},
			want:         gatewayclient.HostedMCPServerLimit{Unlimited: true},
		},
		{
			name:         "numeric entitlement caps hosted servers",
			entitlements: []string{"OBOT_15_HOSTED_MCP_SERVERS"},
			want:         gatewayclient.HostedMCPServerLimit{Maximum: 15},
		},
		{
			name: "numeric entitlement overrides enterprise edition",
			entitlements: []string{
				EnterpriseEntitlement,
				"OBOT_10_HOSTED_MCP_SERVERS",
			},
			want: gatewayclient.HostedMCPServerLimit{Maximum: 10},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := &Provider{entitlements: entitlementSet(tt.entitlements)}
			got, err := provider.HostedMCPServerLimit(t.Context())
			if err != nil {
				t.Fatalf("HostedMCPServerLimit() error = %v, want nil", err)
			}
			if got != tt.want {
				t.Fatalf("HostedMCPServerLimit() = %+v, want %+v", got, tt.want)
			}
			if got.Enforced() != (!tt.want.Unlimited && tt.want.Maximum > 0) {
				t.Fatalf("Enforced() = %t for %+v", got.Enforced(), got)
			}
		})
	}
}

func TestProviderAuditLogRetention(t *testing.T) {
	tests := []struct {
		name         string
		entitlements []string
		want         gatewayclient.AuditLogRetention
	}{
		{
			name: "unlicensed installations leave retention undefined",
			want: gatewayclient.AuditLogRetention{},
		},
		{
			name:         "enterprise edition is unlimited",
			entitlements: []string{EnterpriseEntitlement},
			want:         gatewayclient.AuditLogRetention{Unlimited: true},
		},
		{
			name:         "numeric entitlement sets retention",
			entitlements: []string{"OBOT_30_AUDIT_LOG_DAYS"},
			want:         gatewayclient.AuditLogRetention{Days: 30},
		},
		{
			name:         "enterprise numeric entitlement sets retention",
			entitlements: []string{"OBOT_ENTERPRISE_7_AUDIT_LOG_DAYS"},
			want:         gatewayclient.AuditLogRetention{Days: 7},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := &Provider{entitlements: entitlementSet(tt.entitlements)}
			got, err := provider.AuditLogRetention(t.Context())
			if err != nil {
				t.Fatalf("AuditLogRetention() error = %v, want nil", err)
			}
			if got != tt.want {
				t.Fatalf("AuditLogRetention() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func entitlementSet(entitlements []string) map[keygen.EntitlementCode]struct{} {
	set := make(map[keygen.EntitlementCode]struct{}, len(entitlements))
	for _, entitlement := range entitlements {
		set[keygen.EntitlementCode(entitlement)] = struct{}{}
	}
	return set
}
