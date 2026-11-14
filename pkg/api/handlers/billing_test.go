package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/billing"
	gclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/license"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apiserver/pkg/authentication/user"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestBillingLicenseShowsUsage(t *testing.T) {
	gatewayClient, db := newHandlerTestGatewayWithDB(t)
	require.NoError(t, db.Create(&gatewaytypes.User{
		Username: "member",
		Email:    "member@example.com",
	}).Error)
	periodEnd := metav1.NewTime(time.Date(2027, time.January, 1, 0, 0, 0, 0, time.UTC))
	// Users are counted now. The leader last counted one hosted server.
	storage := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(
		&v1.Subscription{
			Name:      system.SubscriptionName,
			Namespace: system.DefaultNamespace,
			Annotations: map[string]string{
				v1.SubscriptionSyncAnnotation: "true",
			},
			Status: v1.SubscriptionStatus{
				Plan:      "Team",
				Status:    "active",
				PeriodEnd: &periodEnd,
				Entitlements: types.Entitlements{
					Users: new(int64(10)),
				},
			},
		},
		&v1.EntitlementUsage{
			Name:      system.EntitlementUsageName,
			Namespace: system.DefaultNamespace,
			Status: v1.EntitlementUsageStatus{
				MemoryMiB: 200,
			},
		},
	).Build()

	rec := httptest.NewRecorder()
	require.NoError(t, NewBillingHandler(nil, "").Get(api.Context{
		ResponseWriter: rec,
		Request:        httptest.NewRequest(http.MethodGet, "/api/license", nil),
		Storage:        storage,
		GatewayClient:  gatewayClient,
		User: &user.DefaultInfo{
			Groups: []string{types.GroupAdmin},
		},
	}))

	// The entitlements are shown once, not again in the subscription.
	var shown struct {
		Subscription map[string]any `json:"subscription"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &shown))
	require.Equal(t, "Team", shown.Subscription["plan"])
	require.NotContains(t, shown.Subscription, "entitlements")

	var status BillingStatus
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&status))
	require.Equal(t, int64(10), *status.Entitlements.Users)
	require.Equal(t, &types.EntitlementsUsage{
		Users:     1,
		MemoryMiB: 200,
	}, status.Usage)
	require.True(t, status.Syncing)
	require.True(t, status.Entitlements.Cloud)
	require.Equal(t, "active", status.Subscription.Status)
	require.True(t, periodEnd.Time.Equal(status.Subscription.PeriodEnd.GetTime()))
}

func TestVersionReportsABilledEnvironment(t *testing.T) {
	gatewayClient, _ := newHandlerTestGatewayWithDB(t)
	licenseProvider, err := license.NewProvider(t.Context(), gatewayClient, license.Config{})
	require.NoError(t, err)
	// The subscription limits memory below what hosted MCP servers request.
	storage := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(
		&v1.Subscription{
			Name:      system.SubscriptionName,
			Namespace: system.DefaultNamespace,
			Status: v1.SubscriptionStatus{
				Entitlements: types.Entitlements{
					Users:     new(int64(5)),
					MemoryMiB: new(int64(100)),
				},
			},
		},
		&v1.EntitlementUsage{
			Name:      system.EntitlementUsageName,
			Namespace: system.DefaultNamespace,
			Status: v1.EntitlementUsageStatus{
				MemoryMiB: 200,
			},
		},
	).Build()

	for _, tt := range []struct {
		name           string
		billing        *billing.Provider
		wantCloud      bool
		wantBilled     bool
		wantUserLimit  float64
		wantViolations []string
	}{
		{
			name:           "billed",
			billing:        billing.NewProvider(nil, storage, storage),
			wantCloud:      true,
			wantBilled:     true,
			wantUserLimit:  5,
			wantViolations: []string{"memoryLimit"},
		},
		{
			name:          "licensed",
			wantUserLimit: 100,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var entitlementProvider gclient.EntitlementProvider = licenseProvider
			if tt.billing != nil {
				entitlementProvider = tt.billing
			}
			handler := NewVersionHandler(VersionHandlerOptions{
				GatewayClient:       gatewayClient,
				StorageClient:       storage,
				LicenseProvider:     licenseProvider,
				BillingProvider:     tt.billing,
				EntitlementProvider: entitlementProvider,
				ModelProxySettings:  gatewayClient,
			})
			rec := httptest.NewRecorder()
			require.NoError(t, handler.GetVersion(api.Context{
				ResponseWriter: rec,
				Request:        httptest.NewRequest(http.MethodGet, "/api/version", nil),
			}))

			var version struct {
				Cloud          bool    `json:"cloud"`
				BillingEnabled bool    `json:"billingEnabled"`
				UserLimit      float64 `json:"userLimit"`
				Violations     []struct {
					Type string `json:"type"`
				} `json:"licenseEntitlementViolations"`
			}
			require.NoError(t, json.NewDecoder(rec.Body).Decode(&version))
			var violations []string
			for _, violation := range version.Violations {
				violations = append(violations, violation.Type)
			}
			require.Equal(t, tt.wantCloud, version.Cloud)
			require.Equal(t, tt.wantBilled, version.BillingEnabled)
			require.Equal(t, tt.wantUserLimit, version.UserLimit)
			require.Equal(t, tt.wantViolations, violations)
		})
	}
}

func TestBillingCheckRequestsASync(t *testing.T) {
	gatewayClient, _ := newHandlerTestGatewayWithDB(t)
	storage := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(&v1.Subscription{
		Name:      system.SubscriptionName,
		Namespace: system.DefaultNamespace,
	}).Build()

	rec := httptest.NewRecorder()
	require.NoError(t, NewBillingHandler(nil, "").Check(api.Context{
		ResponseWriter: rec,
		Request:        httptest.NewRequest(http.MethodPost, "/api/license", nil),
		Storage:        storage,
		GatewayClient:  gatewayClient,
		User:           &user.DefaultInfo{},
	}))

	// A member sees that a sync is running, but not the usage or the subscription.
	var status BillingStatus
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&status))
	require.True(t, status.Syncing)
	require.Nil(t, status.Usage)
	require.Nil(t, status.Subscription)

	var subscription v1.Subscription
	require.NoError(t, storage.Get(t.Context(), kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: system.SubscriptionName}, &subscription))
	require.Equal(t, "true", subscription.Annotations[v1.SubscriptionSyncAnnotation])
}

func TestBillingManage(t *testing.T) {
	for _, tt := range []struct {
		name       string
		status     int
		body       string
		wantStatus int
		wantURL    string
	}{
		{
			name:    "opens the billing pages",
			status:  http.StatusCreated,
			body:    `{"url": "https://billing.example.com/manage/one-time"}`,
			wantURL: "https://billing.example.com/manage/one-time",
		},
		{
			name:       "billing's conflict is shown as it is",
			status:     http.StatusConflict,
			body:       `{"error": {"message": "This plan has no billing pages."}}`,
			wantStatus: http.StatusConflict,
		},
		{
			name:       "any other refusal is a bad gateway",
			status:     http.StatusInternalServerError,
			wantStatus: http.StatusBadGateway,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var returnURL string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body map[string]string
				_ = json.NewDecoder(r.Body).Decode(&body)
				returnURL = body["returnUrl"]
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer server.Close()

			rec := httptest.NewRecorder()
			err := NewBillingHandler(billing.NewClient(server.URL, "billing-key"), "https://acme.on-obot.ai").Manage(api.Context{
				ResponseWriter: rec,
				Request:        httptest.NewRequest(http.MethodPost, "/api/license/manage", nil),
			})
			require.Equal(t, "https://acme.on-obot.ai/admin/platform?billing=returned", returnURL)
			if tt.wantStatus != 0 {
				var httpErr *types.ErrHTTP
				require.ErrorAs(t, err, &httpErr)
				require.Equal(t, tt.wantStatus, httpErr.Code)
				return
			}
			require.NoError(t, err)
			var session types.BillingSession
			require.NoError(t, json.NewDecoder(rec.Body).Decode(&session))
			require.Equal(t, tt.wantURL, session.URL)
		})
	}
}
