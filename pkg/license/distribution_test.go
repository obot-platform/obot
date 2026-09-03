package license

import (
	"testing"

	clienttypes "github.com/obot-platform/obot/apiclient/types"
)

func TestGetDistributionFromEntitlements(t *testing.T) {
	for _, test := range []struct {
		name         string
		entitlements []string
		want         clienttypes.ProductTelemetryDistribution
	}{
		{name: "unregistered", want: clienttypes.ProductTelemetryDistributionUnregistered},
		{name: "registered", entitlements: []string{CommunityEntitlement}, want: clienttypes.ProductTelemetryDistributionRegistered},
		{name: "enterprise", entitlements: []string{CommunityEntitlement, EnterpriseEntitlement}, want: clienttypes.ProductTelemetryDistributionEnterprise},
		{name: "cloud", entitlements: []string{CommunityEntitlement, EnterpriseEntitlement, CloudEntitlement}, want: clienttypes.ProductTelemetryDistributionCloud},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := GetDistributionFromEntitlements(test.entitlements); got != test.want {
				t.Fatalf("GetDistributionFromEntitlements() = %q, want %q", got, test.want)
			}
		})
	}
}
