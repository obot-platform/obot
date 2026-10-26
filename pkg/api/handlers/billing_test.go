package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/billing"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/stretchr/testify/require"
	"k8s.io/apiserver/pkg/authentication/user"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestBillingLicenseCountsUsersNow(t *testing.T) {
	gatewayClient, db := newHandlerTestGatewayWithDB(t)
	require.NoError(t, db.Create(&gatewaytypes.User{
		Username: "member",
		Email:    "member@example.com",
	}).Error)
	// The leader last counted no users and one hosted server.
	storage := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(&v1.BillingSubscription{
		Name:      billing.Key.Name,
		Namespace: billing.Key.Namespace,
		Status: v1.BillingSubscriptionStatus{
			Usage: types.EntitlementsUsage{
				MemoryMiB: 200,
			},
		},
	}).Build()

	rec := httptest.NewRecorder()
	require.NoError(t, NewBillingHandler(billing.NewSubscription(storage, nil), "").Get(api.Context{
		ResponseWriter: rec,
		Request:        httptest.NewRequest(http.MethodGet, "/api/license", nil),
		Storage:        storage,
		GatewayClient:  gatewayClient,
		User: &user.DefaultInfo{
			Groups: []string{types.GroupAdmin},
		},
	}))

	var status LicenseStatus
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&status))
	require.Equal(t, &types.EntitlementsUsage{
		Users:     1,
		MemoryMiB: 200,
	}, status.Usage)
}
