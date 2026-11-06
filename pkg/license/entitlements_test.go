package license

import (
	"slices"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
)

func TestGetDistributionFromEntitlements(t *testing.T) {
	for _, test := range []struct {
		name         string
		entitlements types.Entitlements
		want         types.ProductTelemetryDistribution
	}{
		{
			name: "unregistered",
			want: types.ProductTelemetryDistributionUnregistered,
		},
		{
			name:         "registered",
			entitlements: Features([]string{CommunityEntitlement}),
			want:         types.ProductTelemetryDistributionRegistered,
		},
		{
			name:         "enterprise",
			entitlements: Features([]string{CommunityEntitlement, EnterpriseEntitlement}),
			want:         types.ProductTelemetryDistributionEnterprise,
		},
		{
			name:         "cloud",
			entitlements: Features([]string{CommunityEntitlement, EnterpriseEntitlement, CloudEntitlement}),
			want:         types.ProductTelemetryDistributionCloud,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := GetDistributionFromEntitlements(test.entitlements); got != test.want {
				t.Fatalf("GetDistributionFromEntitlements() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestFeaturesAndCodes(t *testing.T) {
	codes := []string{
		CloudEntitlement,
		CommunityEntitlement,
		EnterpriseEntitlement,
		EnterpriseAuthProvidersEntitlement,
		EnterpriseModelProvidersEntitlement,
	}
	entitlements := Features(append([]string{"OBOT_ENTERPRISE_50_USERS"}, codes...))
	if !entitlements.Cloud || !entitlements.Community || !entitlements.Enterprise ||
		!entitlements.EnterpriseAuthProviders || !entitlements.EnterpriseModelProviders {
		t.Fatalf("Features() = %+v", entitlements)
	}
	if got := Codes(entitlements); !slices.Equal(got, codes) {
		t.Fatalf("Codes() = %v, want %v", got, codes)
	}
	if got := Codes(types.Entitlements{}); got != nil {
		t.Fatalf("Codes() = %v, want none", got)
	}
}
