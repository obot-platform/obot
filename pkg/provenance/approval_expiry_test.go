package provenance

import (
	"testing"
	"time"
)

func TestApprovalEffectiveStatus(t *testing.T) {
	now := time.Now().UTC()
	for _, tt := range []struct {
		name      string
		status    string
		expiresAt time.Time
		want      string
	}{
		{
			name:      "still pending",
			status:    ApprovalPending,
			expiresAt: now.Add(time.Minute),
			want:      ApprovalPending,
		},
		{
			name:      "expires at deadline",
			status:    ApprovalPending,
			expiresAt: now,
			want:      ApprovalExpired,
		},
		{
			name:      "past deadline",
			status:    ApprovalPending,
			expiresAt: now.Add(-time.Minute),
			want:      ApprovalExpired,
		},
		{
			name:      "approved stays approved",
			status:    ApprovalApproved,
			expiresAt: now.Add(-time.Minute),
			want:      ApprovalApproved,
		},
		{
			name:      "denied stays denied",
			status:    ApprovalDenied,
			expiresAt: now.Add(-time.Minute),
			want:      ApprovalDenied,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			a := Approval{Status: tt.status, ExpiresAt: tt.expiresAt}
			if got := a.EffectiveStatus(now); got != tt.want {
				t.Fatalf("status = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestMemStoreApprovalExpiry(t *testing.T) {
	store := NewMemStore()
	now := time.Now().UTC()
	for _, a := range []Approval{
		{
			ID:        "expired",
			Status:    ApprovalPending,
			CreatedAt: now,
			ExpiresAt: now.Add(-time.Hour),
		},
		{
			ID:        "pending",
			Status:    ApprovalPending,
			CreatedAt: now.Add(-time.Minute),
			ExpiresAt: now.Add(time.Hour),
		},
	} {
		if err := store.CreateApproval(t.Context(), &a); err != nil {
			t.Fatal(err)
		}
	}
	for _, status := range []string{ApprovalPending, ApprovalExpired} {
		items, total, err := store.ListApprovals(t.Context(), ApprovalQuery{Status: status, Limit: 1})
		if err != nil || total != 1 || len(items) != 1 || items[0].ID != status || items[0].Status != status {
			t.Fatalf("%s: items=%+v total=%d err=%v", status, items, total, err)
		}
	}
	a, err := store.GetApproval(t.Context(), "expired")
	if err != nil || a.Status != ApprovalExpired {
		t.Fatalf("get expired: %+v (%v)", a, err)
	}
	if store.approvals["expired"].Status != ApprovalPending {
		t.Fatal("reading an expired approval should not rewrite it")
	}
}
