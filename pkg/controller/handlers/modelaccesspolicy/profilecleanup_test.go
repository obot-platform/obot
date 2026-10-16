package modelaccesspolicy

import (
	"testing"
	"time"

	"github.com/obot-platform/nah/pkg/router"
	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaydb "github.com/obot-platform/obot/pkg/gateway/db"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	storageservices "github.com/obot-platform/obot/pkg/storage/services"
	"github.com/stretchr/testify/require"
)

func TestDeleteProfileBudgetsAndGuardrailPolicies(t *testing.T) {
	services, err := storageservices.New(storageservices.Config{DSN: "sqlite://:memory:"})
	require.NoError(t, err)
	db, err := gatewaydb.New(services.DB.DB, services.DB.SQLDB, true)
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate())
	gateway := gatewayclient.New(t.Context(), db, nil, nil, nil, nil, nil, time.Hour, 10, 0, 0, 0, false)
	t.Cleanup(func() { require.NoError(t, gateway.Close()) })

	limits := []gatewaytypes.LLMGatewayBudgetLimit{{Metric: gatewaytypes.LLMGatewayBudgetMetricRequests, Amount: 1, Window: "1d"}}
	for _, profile := range []string{"deleted", "kept"} {
		require.NoError(t, gateway.CreateLLMGatewayBudget(t.Context(), &gatewaytypes.LLMGatewayBudget{Name: profile, ProfileID: profile, Limits: limits}))
		require.NoError(t, gateway.CreateGuardrailPolicy(t.Context(), &gatewaytypes.GuardrailPolicy{Name: profile, ProfileID: profile, Target: gatewaytypes.GuardrailTargetLLM}))
	}

	require.NoError(t, DeleteProfileBudgetsAndGuardrailPolicies(gateway)(router.Request{Ctx: t.Context(), Name: "deleted"}, nil))

	budgets, err := gateway.ListLLMGatewayBudgets(t.Context(), "")
	require.NoError(t, err)
	require.Len(t, budgets, 1)
	require.Equal(t, "kept", budgets[0].ProfileID)
	policies, err := gateway.ListGuardrailPolicies(t.Context())
	require.NoError(t, err)
	require.Len(t, policies, 1)
	require.Equal(t, "kept", policies[0].ProfileID)
}
