package license

import (
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
			entitlements: types.Entitlements{Community: true},
			want:         types.ProductTelemetryDistributionRegistered,
		},
		{
			name:         "enterprise",
			entitlements: types.Entitlements{Community: true, Enterprise: true},
			want:         types.ProductTelemetryDistributionEnterprise,
		},
		{
			name:         "cloud",
			entitlements: types.Entitlements{Community: true, Enterprise: true, Cloud: true},
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
