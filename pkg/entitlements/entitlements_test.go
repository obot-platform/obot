package entitlements

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/license"
)

type fakeSource struct {
	entitlements types.Entitlements
}

func (f fakeSource) Entitlements(context.Context) (types.Entitlements, error) {
	return f.entitlements, nil
}

func (f fakeSource) Usage(context.Context) (types.EntitlementsUsage, error) {
	return types.EntitlementsUsage{}, nil
}

func (f fakeSource) Restricted(context.Context) (bool, error) {
	return false, nil
}

func limit(n int64) *int64 {
	return new(n)
}

func TestMissing(t *testing.T) {
	granted := types.Entitlements{Cloud: true, EnterpriseAuthProviders: true}
	got := Missing(granted, []string{
		license.EnterpriseAuthProvidersEntitlement,
		license.EnterpriseModelProvidersEntitlement,
		"UNKNOWN",
	})
	if !slices.Equal(got, []string{license.EnterpriseModelProvidersEntitlement, "UNKNOWN"}) {
		t.Fatalf("Missing() = %v", got)
	}
}

func TestRequireEntitlements(t *testing.T) {
	source := fakeSource{entitlements: types.Entitlements{EnterpriseModelProviders: true}}
	if err := RequireEntitlements(t.Context(), source, []string{license.EnterpriseModelProvidersEntitlement}); err != nil {
		t.Fatalf("RequireEntitlements() error = %v", err)
	}

	err := RequireEntitlements(t.Context(), source, []string{license.EnterpriseAuthProvidersEntitlement})
	if httpErr, ok := errors.AsType[*types.ErrHTTP](err); !ok || httpErr.Code != http.StatusPaymentRequired {
		t.Fatalf("RequireEntitlements() error = %v, want 402", err)
	}
}

func TestLimitViolations(t *testing.T) {
	for _, tt := range []struct {
		name         string
		entitlements types.Entitlements
		usage        types.EntitlementsUsage
		want         []string
	}{
		{
			name:  "unlimited",
			usage: types.EntitlementsUsage{Users: 1000},
		},
		{
			name:         "at a zero limit",
			entitlements: types.Entitlements{Users: limit(0)},
		},
		{
			name: "over every limit",
			entitlements: types.Entitlements{
				Users:            limit(0),
				Devices:          limit(1),
				HostedMCPServers: limit(2),
				MemoryMiB:        limit(1024),
				CPUMillicores:    limit(1000),
			},
			usage: types.EntitlementsUsage{
				Users:            1,
				Devices:          2,
				HostedMCPServers: 3,
				MemoryMiB:        2048,
				CPUMillicores:    2000,
			},
			want: []string{"userLimit", "deviceLimit", "hostedMcpServerLimit", "memoryLimit", "cpuLimit"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var got []string
			for _, violation := range LimitViolations(tt.entitlements, tt.usage) {
				got = append(got, violation.Type)
			}
			if !slices.Equal(got, tt.want) {
				t.Fatalf("LimitViolations() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLimits(t *testing.T) {
	limits := Limits{Source: fakeSource{entitlements: types.Entitlements{Users: limit(0)}}}

	users, err := limits.UserLimit(t.Context())
	if err != nil || users != (gatewayclient.UserLimit{Maximum: 0}) {
		t.Fatalf("UserLimit() = %+v, %v", users, err)
	}
	devices, err := limits.DeviceLimit(t.Context())
	if err != nil || devices != (gatewayclient.DeviceLimit{Unlimited: true}) {
		t.Fatalf("DeviceLimit() = %+v, %v", devices, err)
	}
}
