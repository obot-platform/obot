package principal

import (
	"context"
	"testing"

	types "github.com/obot-platform/obot/apiclient/types"
	kuser "k8s.io/apiserver/pkg/authentication/user"
)

func TestAdmittedActiveUser(t *testing.T) {
	principal := func(uid string, status types.UserStatus, owner string) kuser.Info {
		extra := map[string][]string{}
		if status != "" {
			RecordUserStatus(extra, status)
		}
		if owner != "" {
			extra[HostedAgentOwnerExtra] = []string{owner}
		}
		return &kuser.DefaultInfo{
			UID:   uid,
			Extra: extra,
		}
	}

	for _, tt := range []struct {
		name      string
		principal kuser.Info
		userID    string
		want      bool
	}{
		{
			name:      "an active user, for themselves",
			principal: principal("7", types.UserStatusActive, ""),
			userID:    "7",
			want:      true,
		},
		{
			name:      "an active user, for another user",
			principal: principal("7", types.UserStatusActive, ""),
			userID:    "8",
			want:      false,
		},
		{
			name:      "a disabled user",
			principal: principal("7", types.UserStatusDisabled, ""),
			userID:    "7",
			want:      false,
		},
		{
			name:      "a principal without a recorded status",
			principal: principal("7", "", ""),
			userID:    "7",
			want:      false,
		},
		{
			name:      "a hosted agent, for its active owner",
			principal: principal("agent-1", types.UserStatusActive, "7"),
			userID:    "7",
			want:      true,
		},
		{
			name:      "a hosted agent, for itself",
			principal: principal("agent-1", types.UserStatusActive, "7"),
			userID:    "agent-1",
			want:      false,
		},
		{
			name:   "no admitted principal",
			userID: "7",
			want:   false,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			if tt.principal != nil {
				ctx = WithAdmittedPrincipal(ctx, tt.principal)
			}
			if got := AdmittedActiveUser(ctx, tt.userID); got != tt.want {
				t.Fatalf("AdmittedActiveUser() = %v, want %v", got, tt.want)
			}
		})
	}
}
