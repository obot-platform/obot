package license

import (
	"context"
	"errors"
	"net/http"
	"testing"

	keygen "github.com/keygen-sh/keygen-go/v3"
	"github.com/obot-platform/obot/apiclient/types"
	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
)

func TestMissingAndRequire(t *testing.T) {
	provider := &Provider{
		entitlements: map[keygen.EntitlementCode]struct{}{
			"ENTITLED": {},
		},
	}

	missing, err := provider.MissingEntitlements(t.Context(), []string{"ENTITLED", "MISSING"})
	if err != nil {
		t.Fatalf("Missing() error = %v, want nil", err)
	}
	if len(missing) != 1 || missing[0] != "MISSING" {
		t.Fatalf("Missing() = %v, want [MISSING]", missing)
	}

	if err := provider.RequireEntitlements(t.Context(), []string{"ENTITLED"}); err != nil {
		t.Fatalf("Require() error = %v, want nil", err)
	}

	err = provider.RequireEntitlements(t.Context(), []string{"MISSING"})
	var httpErr *types.ErrHTTP
	if !errors.As(err, &httpErr) {
		t.Fatalf("Require() error = %T, want *types.ErrHTTP", err)
	}
	if httpErr.Code != http.StatusPaymentRequired {
		t.Fatalf("Require() status = %d, want %d", httpErr.Code, http.StatusPaymentRequired)
	}
}

func TestGetDistributionFromEntitlements(t *testing.T) {
	for _, test := range []struct {
		name         string
		entitlements []string
		want         types.ProductTelemetryDistribution
	}{
		{
			name: "unregistered",
			want: types.ProductTelemetryDistributionUnregistered,
		},
		{
			name:         "registered",
			entitlements: []string{CommunityEntitlement},
			want:         types.ProductTelemetryDistributionRegistered,
		},
		{
			name:         "enterprise",
			entitlements: []string{CommunityEntitlement, EnterpriseEntitlement},
			want:         types.ProductTelemetryDistributionEnterprise,
		},
		{
			name:         "cloud",
			entitlements: []string{CommunityEntitlement, EnterpriseEntitlement, CloudEntitlement},
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

func TestProviderLimits(t *testing.T) {
	for _, limit := range []struct {
		name         string
		suffix       string
		get          func(*Provider, context.Context) (gatewayclient.SystemLimit, error)
		defaultLimit gatewayclient.SystemLimit
	}{
		{
			name:         "UserLimit",
			suffix:       "_USERS",
			get:          (*Provider).UserLimit,
			defaultLimit: gatewayclient.SystemLimit{Maximum: gatewayclient.DefaultUserLimit},
		},
		{
			name:         "DeviceLimit",
			suffix:       "_DEVICES",
			get:          (*Provider).DeviceLimit,
			defaultLimit: gatewayclient.SystemLimit{Maximum: gatewayclient.DefaultDeviceLimit},
		},
		{
			name:   "AuditLogRetentionLimit",
			suffix: "_DAYS_AUDIT_LOG_RETENTION",
			get:    (*Provider).AuditLogRetentionLimit,
		},
	} {
		for _, test := range []struct {
			name         string
			entitlements []string
			want         gatewayclient.SystemLimit
		}{
			{
				name: "default",
				want: limit.defaultLimit,
			},
			{
				name: "both prefixes are additive",
				entitlements: []string{
					EnterpriseEntitlement,
					"OBOT_ENTERPRISE_10" + limit.suffix,
					"OBOT_20" + limit.suffix,
				},
				want: gatewayclient.SystemLimit{Maximum: 30},
			},
			{
				name: "malformed short prefix entitlements are ignored",
				entitlements: []string{
					"OBOT" + limit.suffix,
					"OBOT_TEN" + limit.suffix,
					"OBOT_ENTERPRISE_OBOT_10" + limit.suffix,
				},
				want: limit.defaultLimit,
			},
		} {
			t.Run(limit.name+"/"+test.name, func(t *testing.T) {
				entitlements := make(map[keygen.EntitlementCode]struct{}, len(test.entitlements))
				for _, entitlement := range test.entitlements {
					entitlements[keygen.EntitlementCode(entitlement)] = struct{}{}
				}

				got, err := limit.get(&Provider{entitlements: entitlements}, t.Context())
				if err != nil {
					t.Fatalf("%s() error = %v, want nil", limit.name, err)
				}
				if got != test.want {
					t.Fatalf("%s() = %+v, want %+v", limit.name, got, test.want)
				}
			})
		}
	}
}
