package billingsubscription

import (
	"errors"
	"testing"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/billing"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	now = time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
)

func limit(n int64) *int64 {
	return new(n)
}

func TestSyncDue(t *testing.T) {
	for _, tt := range []struct {
		name   string
		status v1.BillingSubscriptionStatus
		want   bool
	}{
		{
			name: "never synced",
			want: true,
		},
		{
			name:   "synced recently",
			status: v1.BillingSubscriptionStatus{LastSyncTime: metav1.NewTime(now.Add(-time.Minute))},
		},
		{
			name:   "synced an interval ago",
			status: v1.BillingSubscriptionStatus{LastSyncTime: metav1.NewTime(now.Add(-syncInterval))},
			want:   true,
		},
		{
			name: "failed a retry interval ago",
			status: v1.BillingSubscriptionStatus{
				LastSyncTime: metav1.NewTime(now.Add(-retryInterval)),
				SyncError:    "billing is down",
			},
			want: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := syncDue(tt.status, now); got != tt.want {
				t.Fatalf("syncDue() = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestSyncedKeepsTheLastAnswerOnFailure(t *testing.T) {
	answered := synced(v1.BillingSubscriptionStatus{}, types.Subscription{Plan: "Team"}, types.Entitlements{Users: limit(10)}, nil, now)
	if answered.Subscription.Plan != "Team" || *answered.Entitlements.Users != 10 || answered.SyncError != "" {
		t.Fatalf("answered = %+v", answered)
	}

	failed := synced(answered, types.Subscription{}, types.Entitlements{}, errors.New("billing is down"), now.Add(time.Hour))
	if failed.Subscription.Plan != "Team" || *failed.Entitlements.Users != 10 || failed.SyncError != "billing is down" ||
		!failed.LastSyncTime.Equal(&metav1.Time{Time: now.Add(time.Hour)}) {
		t.Fatalf("failed = %+v", failed)
	}
}

func TestTracked(t *testing.T) {
	recently := metav1.NewTime(now.Add(-time.Hour))
	plan := &types.Entitlements{MemoryMiB: limit(1024)}
	over := types.EntitlementsUsage{MemoryMiB: 1025}
	within := types.EntitlementsUsage{MemoryMiB: 1024}

	for _, tt := range []struct {
		name      string
		status    v1.BillingSubscriptionStatus
		usage     types.EntitlementsUsage
		wantSince *metav1.Time
	}{
		{
			name:  "no answer never starts the clock",
			usage: over,
		},
		{
			name: "getting back within the plan clears the clock",
			status: v1.BillingSubscriptionStatus{
				Entitlements:   plan,
				OverLimitSince: &recently,
			},
			usage: within,
		},
		{
			name:      "going over starts the clock",
			status:    v1.BillingSubscriptionStatus{Entitlements: plan},
			usage:     over,
			wantSince: &metav1.Time{Time: now},
		},
		{
			name: "staying over keeps the clock",
			status: v1.BillingSubscriptionStatus{
				Entitlements:   plan,
				OverLimitSince: &recently,
			},
			usage:     over,
			wantSince: &recently,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := tracked(tt.status, tt.usage, now)
			if got.Usage != tt.usage {
				t.Fatalf("usage = %+v, want %+v", got.Usage, tt.usage)
			}
			if (got.OverLimitSince == nil) != (tt.wantSince == nil) || (got.OverLimitSince != nil && !got.OverLimitSince.Equal(tt.wantSince)) {
				t.Fatalf("overLimitSince = %v, want %v", got.OverLimitSince, tt.wantSince)
			}
		})
	}
}

func TestRestricted(t *testing.T) {
	graceAgo := metav1.NewTime(now.Add(-billing.GracePeriod))
	recently := metav1.NewTime(now.Add(-time.Hour))
	periodEnded := func(at metav1.Time) *v1.BillingSubscriptionPlan {
		return &v1.BillingSubscriptionPlan{
			PeriodEnd: &at,
		}
	}

	for _, tt := range []struct {
		name   string
		status v1.BillingSubscriptionStatus
		want   bool
	}{
		{
			name: "no answer is never restricted",
		},
		{
			name: "over within the grace period",
			status: v1.BillingSubscriptionStatus{
				OverLimitSince: &recently,
			},
		},
		{
			name: "over for the grace period",
			status: v1.BillingSubscriptionStatus{
				OverLimitSince: &graceAgo,
			},
			want: true,
		},
		{
			name: "billing unreachable before the period end",
			status: v1.BillingSubscriptionStatus{
				Subscription: periodEnded(metav1.NewTime(now.Add(time.Hour))),
				SyncError:    "billing is down",
			},
		},
		{
			name: "billing unreachable since a recent period end",
			status: v1.BillingSubscriptionStatus{
				Subscription: periodEnded(recently),
				SyncError:    "billing is down",
			},
		},
		{
			name: "billing unreachable for the grace period past the period end",
			status: v1.BillingSubscriptionStatus{
				Subscription: periodEnded(graceAgo),
				SyncError:    "billing is down",
			},
			want: true,
		},
		{
			name: "billing reachable past the period end",
			status: v1.BillingSubscriptionStatus{
				Subscription: periodEnded(graceAgo),
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := restricted(tt.status, now); got != tt.want {
				t.Fatalf("restricted() = %t, want %t", got, tt.want)
			}
		})
	}
}
