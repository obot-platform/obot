package types

import (
	"testing"
	"time"

	types2 "github.com/obot-platform/obot/apiclient/types"
)

func TestConvertUserReportsLifecycle(t *testing.T) {
	at := time.Date(2026, time.September, 23, 12, 0, 0, 0, time.UTC)

	for _, tt := range []struct {
		name                 string
		user                 User
		wantStatus           types2.UserStatus
		wantDisabledAt       bool
		wantReason           string
		wantManagementSource types2.UserManagementSource
	}{
		{
			name: "active",
			user: User{
				ID: 1,
			},
			wantStatus:           types2.UserStatusActive,
			wantManagementSource: types2.UserManagementSourceObot,
		},
		{
			name: "deprovisioned by SCIM",
			user: User{
				ID:             2,
				DisabledAt:     &at,
				DisabledReason: UserDisabledReasonSCIMInactive,
			},
			wantStatus:           types2.UserStatusDisabled,
			wantDisabledAt:       true,
			wantReason:           string(UserDisabledReasonSCIMInactive),
			wantManagementSource: types2.UserManagementSourceSCIM,
		},
		{
			name: "deleted after being disabled",
			user: User{
				ID:             4,
				DeletedAt:      &at,
				DisabledAt:     &at,
				DisabledReason: UserDisabledReasonSCIMInactive,
			},
			wantStatus:           types2.UserStatusDeleted,
			wantDisabledAt:       true,
			wantReason:           string(UserDisabledReasonSCIMInactive),
			wantManagementSource: types2.UserManagementSourceSCIM,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := ConvertUser(&tt.user, false, "")
			if got.Status != tt.wantStatus {
				t.Errorf("status = %q, want %q", got.Status, tt.wantStatus)
			}
			if (got.DisabledAt != nil) != tt.wantDisabledAt {
				t.Errorf("disabled at = %v, want set %v", got.DisabledAt, tt.wantDisabledAt)
			}
			if got.DisabledReason != tt.wantReason {
				t.Errorf("disable reason = %q, want %q", got.DisabledReason, tt.wantReason)
			}
			if got.ManagementSource != tt.wantManagementSource {
				t.Errorf("management source = %q, want %q", got.ManagementSource, tt.wantManagementSource)
			}
		})
	}
}
