package billing

import (
	"context"
	"testing"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// notStarted is a cache that hasn't started.
type notStarted struct {
	kclient.Reader
}

func TestEntitlements(t *testing.T) {
	for _, tt := range []struct {
		name         string
		subscription *v1.Subscription
		want         types.Entitlements
	}{
		{
			name: "billing's answer, always Obot Cloud",
			subscription: testSubscription(types.Entitlements{
				EnterpriseAuthProviders: true,
				Users:                   new(int64(10)),
			}),
			want: types.Entitlements{
				Cloud:                   true,
				EnterpriseAuthProviders: true,
				Users:                   new(int64(10)),
			},
		},
		{
			name: "nothing before billing first answers",
			subscription: &v1.Subscription{
				Namespace: system.DefaultNamespace,
				Name:      system.SubscriptionName,
			},
			want: types.Entitlements{
				Cloud: true,
			},
		},
		{
			name: "nothing before the subscription exists",
			want: types.Entitlements{
				Cloud: true,
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			builder := fake.NewClientBuilder().WithScheme(scheme.Scheme)
			if tt.subscription != nil {
				builder = builder.WithObjects(tt.subscription)
			}
			got, err := testProvider(builder.Build()).Entitlements(t.Context())
			if err != nil {
				t.Fatalf("Entitlements() error = %v", err)
			}
			if got.Cloud != tt.want.Cloud || got.EnterpriseAuthProviders != tt.want.EnterpriseAuthProviders ||
				(got.Users == nil) != (tt.want.Users == nil) || (got.Users != nil && *got.Users != *tt.want.Users) {
				t.Fatalf("Entitlements() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func (notStarted) Get(context.Context, kclient.ObjectKey, kclient.Object, ...kclient.GetOption) error {
	return &cache.ErrCacheNotStarted{}
}

func TestEntitlementsBeforeTheCacheStarts(t *testing.T) {
	storage := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(testSubscription(types.Entitlements{
		Users: new(int64(10)),
	})).Build()

	got, err := NewProvider(nil, notStarted{}, storage).Entitlements(t.Context())
	if err != nil {
		t.Fatalf("Entitlements() error = %v", err)
	}
	if got.Users == nil || *got.Users != 10 {
		t.Fatalf("Entitlements() = %+v, want the stored subscription's", got)
	}
}

func TestRestricted(t *testing.T) {
	overSince := func(ago time.Duration) *v1.Subscription {
		subscription := testSubscription(types.Entitlements{})
		subscription.Status.OverLimitSince = &metav1.Time{Time: time.Now().Add(-ago)}
		return subscription
	}
	periodEnded := func(ago time.Duration, syncError string) *v1.Subscription {
		subscription := testSubscription(types.Entitlements{})
		subscription.Status.PeriodEnd = &metav1.Time{Time: time.Now().Add(-ago)}
		subscription.Status.SyncError = syncError
		return subscription
	}
	unreachable := testSubscription(types.Entitlements{})
	unreachable.Status.SyncError = "billing is unreachable"

	for _, tt := range []struct {
		name         string
		subscription *v1.Subscription
		want         bool
	}{
		{
			name:         "within the limits",
			subscription: testSubscription(types.Entitlements{}),
		},
		{
			name:         "over within the grace period",
			subscription: overSince(time.Hour),
		},
		{
			name:         "over for the grace period",
			subscription: overSince(gracePeriod),
			want:         true,
		},
		{
			name:         "billing is unreachable within the period",
			subscription: periodEnded(-time.Hour, "billing is unreachable"),
		},
		{
			name:         "billing is unreachable within the grace period past the period",
			subscription: periodEnded(time.Hour, "billing is unreachable"),
		},
		{
			name:         "billing is unreachable for the grace period past the period",
			subscription: periodEnded(gracePeriod, "billing is unreachable"),
			want:         true,
		},
		{
			name:         "billing answers past the period",
			subscription: periodEnded(gracePeriod, ""),
		},
		{
			name:         "billing is unreachable for a subscription without a period",
			subscription: unreachable,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			storage := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(tt.subscription).Build()
			got, err := testProvider(storage).Restricted(t.Context())
			if err != nil {
				t.Fatalf("Restricted() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("Restricted() = %v, want %v", got, tt.want)
			}
		})
	}
}

func testSubscription(entitlements types.Entitlements) *v1.Subscription {
	return &v1.Subscription{
		Namespace: system.DefaultNamespace,
		Name:      system.SubscriptionName,
		Status: v1.SubscriptionStatus{
			Entitlements: entitlements,
		},
	}
}

func testProvider(storage kclient.Client) *Provider {
	return NewProvider(nil, storage, storage)
}
