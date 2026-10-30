package billing

import (
	"reflect"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestEntitlementsOf(t *testing.T) {
	for _, tt := range []struct {
		name   string
		status v1.BillingSubscriptionStatus
		want   types.Entitlements
	}{
		{
			name: "before the first answer no plan applies",
			want: types.Entitlements{
				Cloud:                    true,
				EnterpriseAuthProviders:  true,
				EnterpriseModelProviders: true,
			},
		},
		{
			name: "billing's answer, restricted",
			status: v1.BillingSubscriptionStatus{
				Entitlements: &types.Entitlements{
					Users: new(int64(10)),
				},
				Restricted: true,
			},
			want: types.Entitlements{
				Cloud:      true,
				Restricted: true,
				Users:      new(int64(10)),
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := EntitlementsOf(tt.status); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("EntitlementsOf() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestAuditLogRetention(t *testing.T) {
	days := func(n int64) *int64 { return &n }

	for _, tt := range []struct {
		name             string
		status           v1.BillingSubscriptionStatus
		wantMCP, wantLLM int
	}{
		{
			name:    "before the first answer Obot's configuration applies",
			wantMCP: 90,
			wantLLM: 30,
		},
		{
			name:    "no plan retention keeps Obot's configuration",
			status:  v1.BillingSubscriptionStatus{Entitlements: &types.Entitlements{}},
			wantMCP: 90,
			wantLLM: 30,
		},
		{
			name:    "a plan retention of 0 keeps Obot's configuration",
			status:  v1.BillingSubscriptionStatus{Entitlements: &types.Entitlements{AuditLogRetentionDays: days(0)}},
			wantMCP: 90,
			wantLLM: 30,
		},
		{
			name:    "the plan's retention applies to both",
			status:  v1.BillingSubscriptionStatus{Entitlements: &types.Entitlements{AuditLogRetentionDays: days(7)}},
			wantMCP: 7,
			wantLLM: 7,
		},
		{
			name:    "the plan's retention is capped",
			status:  v1.BillingSubscriptionStatus{Entitlements: &types.Entitlements{AuditLogRetentionDays: days(1_000_000)}},
			wantMCP: maxAuditLogRetentionDays,
			wantLLM: maxAuditLogRetentionDays,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			storage := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
			subscription := &v1.BillingSubscription{}
			subscription.Namespace, subscription.Name = Key.Namespace, Key.Name
			subscription.Status = tt.status
			if err := storage.Create(t.Context(), subscription); err != nil {
				t.Fatal(err)
			}

			mcpDays, llmDays, err := NewSubscription(storage, nil).AuditLogRetention(90, 30)(t.Context())
			if err != nil || mcpDays != tt.wantMCP || llmDays != tt.wantLLM {
				t.Fatalf("AuditLogRetention() = %d, %d, %v, want %d, %d", mcpDays, llmDays, err, tt.wantMCP, tt.wantLLM)
			}
		})
	}
}
