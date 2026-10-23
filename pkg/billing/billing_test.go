package billing

import (
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestEntitlementsOfGrantsNothingBeforeTheFirstAnswer(t *testing.T) {
	entitlements := EntitlementsOf(v1.BillingSubscriptionStatus{})
	if !entitlements.Cloud || entitlements.EnterpriseAuthProviders || entitlements.EnterpriseModelProviders {
		t.Fatalf("features = %+v", entitlements)
	}
	for name, limit := range map[string]*int64{
		"users":            entitlements.Users,
		"devices":          entitlements.Devices,
		"hostedMcpServers": entitlements.HostedMCPServers,
		"memoryMiB":        entitlements.MemoryMiB,
		"cpuMillicores":    entitlements.CPUMillicores,
	} {
		if limit == nil || *limit != 0 {
			t.Fatalf("%s = %v, want 0", name, limit)
		}
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
			name: "before the first answer nothing is deleted",
		},
		{
			name:    "no plan retention keeps Obot's configuration",
			status:  v1.BillingSubscriptionStatus{Entitlements: &types.Entitlements{}},
			wantMCP: 90,
			wantLLM: 30,
		},
		{
			name:    "the plan's retention applies to both",
			status:  v1.BillingSubscriptionStatus{Entitlements: &types.Entitlements{AuditLogRetentionDays: days(7)}},
			wantMCP: 7,
			wantLLM: 7,
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

			mcpDays, llmDays, err := New(storage, nil).AuditLogRetention(90, 30)(t.Context())
			if err != nil || mcpDays != tt.wantMCP || llmDays != tt.wantLLM {
				t.Fatalf("AuditLogRetention() = %d, %d, %v, want %d, %d", mcpDays, llmDays, err, tt.wantMCP, tt.wantLLM)
			}
		})
	}
}
