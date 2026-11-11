package subscription

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/billing"
	gclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaydb "github.com/obot-platform/obot/pkg/gateway/db"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	storageservices "github.com/obot-platform/obot/pkg/storage/services"
	"github.com/obot-platform/obot/pkg/system"
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type response struct {
	retryAfter time.Duration
}

func (r *response) Attributes() map[string]any {
	return map[string]any{}
}

func (r *response) RetryAfter(delay time.Duration) {
	r.retryAfter = delay
}

func TestNextSync(t *testing.T) {
	now := time.Now()
	for _, tt := range []struct {
		name         string
		subscription v1.Subscription
		want         time.Time
	}{
		{
			name: "never synced",
			want: time.Time{}.Add(syncInterval),
		},
		{
			name: "synced",
			subscription: v1.Subscription{
				Status: v1.SubscriptionStatus{LastSyncTime: metav1.NewTime(now)},
			},
			want: now.Add(syncInterval),
		},
		{
			name: "failed",
			subscription: v1.Subscription{
				Status: v1.SubscriptionStatus{
					LastSyncTime: metav1.NewTime(now),
					SyncError:    "billing is down",
				},
			},
			want: now.Add(retryInterval),
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := nextSync(&tt.subscription); !got.Equal(tt.want) {
				t.Fatalf("nextSync() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSync(t *testing.T) {
	answer := http.StatusOK
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(answer)
		if answer == http.StatusOK {
			_, _ = w.Write([]byte(`{"plan": "Team", "status": "active", "periodEnd": "2027-01-01T00:00:00Z", "selfService": true,
				"entitlements": {"users": 10}}`))
		}
	}))
	defer server.Close()
	handler := New(billing.NewClient(server.URL, "billing-key"), nil)
	subscription := &v1.Subscription{
		Namespace: system.DefaultNamespace,
		Name:      system.SubscriptionName,
	}
	storage := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(subscription).WithStatusSubresource(subscription).Build()

	resp := &response{}
	if err := handler.Sync(router.Request{Ctx: t.Context(), Client: storage, Object: subscription}, resp); err != nil {
		t.Fatal(err)
	}
	periodEnd := time.Date(2027, time.January, 1, 0, 0, 0, 0, time.UTC)
	answered := func(status v1.SubscriptionStatus) bool {
		return status.Plan == "Team" && status.Status == "active" && status.SelfService && !status.CancelAtPeriodEnd &&
			status.PeriodEnd != nil && status.PeriodEnd.Time.Equal(periodEnd) &&
			status.Entitlements.Users != nil && *status.Entitlements.Users == 10
	}
	status := subscription.Status
	if !answered(status) || status.SyncError != "" || status.LastSyncTime.IsZero() {
		t.Fatalf("status = %+v", status)
	}
	if resp.retryAfter <= syncInterval-time.Minute {
		t.Fatalf("retryAfter = %v, want about %v", resp.retryAfter, syncInterval)
	}

	// A requested sync runs before it's due. A failed one keeps the last answer and retries sooner. The answer is
	// stored and the request cleared.
	answer = http.StatusServiceUnavailable
	// The router stores the status its handlers leave.
	if err := storage.Status().Update(t.Context(), subscription); err != nil {
		t.Fatal(err)
	}
	subscription.Annotations = map[string]string{v1.SubscriptionSyncAnnotation: "true"}
	if err := storage.Update(t.Context(), subscription); err != nil {
		t.Fatal(err)
	}
	resp = &response{}
	if err := handler.Sync(router.Request{Ctx: t.Context(), Client: storage, Object: subscription}, resp); err != nil {
		t.Fatal(err)
	}
	status = subscription.Status
	if !answered(status) || status.SyncError == "" {
		t.Fatalf("status = %+v", status)
	}
	var stored v1.Subscription
	if err := storage.Get(t.Context(), kclient.ObjectKeyFromObject(subscription), &stored); err != nil {
		t.Fatal(err)
	}
	if !answered(stored.Status) || stored.Status.SyncError == "" || stored.Annotations[v1.SubscriptionSyncAnnotation] != "" {
		t.Fatalf("stored status = %+v, annotations = %v", stored.Status, stored.Annotations)
	}
	if resp.retryAfter > retryInterval {
		t.Fatalf("retryAfter = %v, want at most %v", resp.retryAfter, retryInterval)
	}

	// Until the next sync is due, billing isn't asked.
	answer = http.StatusOK
	subscription.Status.SyncError = ""
	resp = &response{}
	if err := handler.Sync(router.Request{Ctx: t.Context(), Object: subscription}, resp); err != nil {
		t.Fatal(err)
	}
	if subscription.Status.SyncError != "" || resp.retryAfter <= 0 {
		t.Fatalf("status = %+v, retryAfter = %v", subscription.Status, resp.retryAfter)
	}
}

func TestCountUsage(t *testing.T) {
	sized := types.VMCPComponent{
		ID:   "sized",
		Name: "sized",
		CatalogEntry: types.MCPServerCatalogEntrySnapshot{
			Manifest: types.MCPServerCatalogEntryManifest{
				Runtime: types.RuntimeContainerized,
				Resources: &types.MCPResourceRequirements{
					Requests: types.MCPResourceRequests{
						CPU:    "500m",
						Memory: "1Gi",
					},
				},
			},
		},
	}
	storage := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(&v1.VMCP{
		Name:      "vmcp1",
		Namespace: system.DefaultNamespace,
		Spec: v1.VMCPSpec{
			Manifest: types.VMCPManifest{
				Components: []types.VMCPComponent{sized},
			},
		},
	}).Build()
	usage := &v1.EntitlementUsage{}

	if err := New(nil, nil).CountUsage(router.Request{Ctx: t.Context(), Client: storage, Object: usage}, &response{}); err != nil {
		t.Fatal(err)
	}
	if usage.Status.MemoryMiB != 1024 || usage.Status.CPUMillicores != 500 {
		t.Fatalf("status = %+v", usage.Status)
	}
}

func TestEnsureOverLimitSince(t *testing.T) {
	recently := metav1.NewTime(time.Now().Add(-time.Hour))
	granted := func(memoryMiB int64) types.Entitlements {
		return types.Entitlements{
			MemoryMiB: new(memoryMiB),
		}
	}

	for _, tt := range []struct {
		name      string
		status    v1.SubscriptionStatus
		wantSince bool
		wantKept  bool
	}{
		{
			name: "no answer never starts the clock",
		},
		{
			name: "going over starts the clock",
			status: v1.SubscriptionStatus{
				Entitlements: granted(512),
			},
			wantSince: true,
		},
		{
			name: "staying over keeps the clock",
			status: v1.SubscriptionStatus{
				Entitlements:   granted(512),
				OverLimitSince: &recently,
			},
			wantSince: true,
			wantKept:  true,
		},
		{
			name: "getting back within the limits clears the clock",
			status: v1.SubscriptionStatus{
				Entitlements:   granted(1024),
				OverLimitSince: &recently,
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			storage := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects([]kclient.Object{
				&v1.EntitlementUsage{
					Name:      system.EntitlementUsageName,
					Namespace: system.DefaultNamespace,
					Status: v1.EntitlementUsageStatus{
						MemoryMiB: 1024,
					},
				},
			}...).Build()
			handler := New(nil, newGatewayClient(t))
			subscription := &v1.Subscription{Status: tt.status}

			if err := handler.EnsureOverLimitSince(router.Request{Ctx: t.Context(), Client: storage, Object: subscription}, &response{}); err != nil {
				t.Fatal(err)
			}
			since := subscription.Status.OverLimitSince
			if (since != nil) != tt.wantSince {
				t.Fatalf("overLimitSince = %v, want set %v", since, tt.wantSince)
			}
			if tt.wantKept && !since.Equal(&recently) {
				t.Fatalf("overLimitSince = %v, want %v", since, recently)
			}
		})
	}
}

func newGatewayClient(t *testing.T) *gclient.Client {
	t.Helper()
	services, err := storageservices.New(storageservices.Config{DSN: "sqlite://:memory:"})
	if err != nil {
		t.Fatal(err)
	}
	database, err := gatewaydb.New(services.DB.DB, services.DB.SQLDB, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.AutoMigrate(); err != nil {
		t.Fatal(err)
	}
	gateway := gclient.New(t.Context(), database, nil, nil, nil, time.Hour, 10, 0, 0, 0, false)
	t.Cleanup(func() { _ = gateway.Close() })
	return gateway
}

func TestStatusCanBeCompared(t *testing.T) {
	// The router compares a status before and after its handlers to decide whether to save it, and panics on a type it
	// can't compare.
	now := metav1.Now()
	status := v1.SubscriptionStatus{
		Plan:      "Team",
		PeriodEnd: &now,
		Entitlements: types.Entitlements{
			Users: new(int64(10)),
		},
		LastSyncTime:   now,
		OverLimitSince: &now,
	}
	if !equality.Semantic.DeepEqual(status, *status.DeepCopy()) {
		t.Fatal("a status doesn't equal its copy")
	}
	if !equality.Semantic.DeepEqual(v1.EntitlementUsageStatus{MemoryMiB: 1}, v1.EntitlementUsageStatus{MemoryMiB: 1}) {
		t.Fatal("a usage status doesn't equal its copy")
	}
}

func TestEnsureResources(t *testing.T) {
	storage := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
	exists := func(obj kclient.Object, name string) bool {
		return storage.Get(t.Context(), kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: name}, obj) == nil
	}

	// Creating them twice is fine, since every promotion does.
	for range 2 {
		if err := EnsureResources(t.Context(), storage); err != nil {
			t.Fatal(err)
		}
	}
	if !exists(&v1.Subscription{}, system.SubscriptionName) || !exists(&v1.EntitlementUsage{}, system.EntitlementUsageName) {
		t.Fatal("the Subscription and the EntitlementUsage weren't both created")
	}

	// A deleted one comes back when its handler sees it gone.
	for _, tt := range []struct {
		name    string
		obj     kclient.Object
		handler router.HandlerFunc
	}{
		{
			name:    system.SubscriptionName,
			obj:     &v1.Subscription{},
			handler: New(nil, nil).Sync,
		},
		{
			name:    system.EntitlementUsageName,
			obj:     &v1.EntitlementUsage{},
			handler: New(nil, nil).CountUsage,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if !exists(tt.obj, tt.name) {
				t.Fatalf("%s doesn't exist", tt.name)
			}
			if err := storage.Delete(t.Context(), tt.obj); err != nil {
				t.Fatal(err)
			}
			if err := tt.handler(router.Request{Ctx: t.Context(), Client: storage}, &response{}); err != nil {
				t.Fatal(err)
			}
			if !exists(tt.obj, tt.name) {
				t.Fatalf("%s wasn't recreated", tt.name)
			}
		})
	}
}

func TestEnsureOverLimitSinceWithoutStoredUsage(t *testing.T) {
	// Without the EntitlementUsage, compute counts as 0, so the clock clears instead of staying frozen.
	storage := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
	handler := New(nil, newGatewayClient(t))
	since := metav1.NewTime(time.Now().Add(-time.Hour))
	subscription := &v1.Subscription{
		Status: v1.SubscriptionStatus{
			Entitlements: types.Entitlements{
				MemoryMiB: new(int64(512)),
			},
			OverLimitSince: &since,
		},
	}

	if err := handler.EnsureOverLimitSince(router.Request{Ctx: t.Context(), Client: storage, Object: subscription}, &response{}); err != nil {
		t.Fatal(err)
	}
	if subscription.Status.OverLimitSince != nil {
		t.Fatalf("overLimitSince = %v, want it cleared", subscription.Status.OverLimitSince)
	}
}
