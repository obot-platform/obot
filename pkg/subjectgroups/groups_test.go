package subjectgroups

import (
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	kuser "k8s.io/apiserver/pkg/authentication/user"
)

func TestIsAdmin(t *testing.T) {
	tests := []struct {
		name string
		user kuser.Info
		want bool
	}{
		{
			name: "no user",
		},
		{
			name: "admin session",
			user: &kuser.DefaultInfo{Groups: types.RoleAdmin.Groups()},
			want: true,
		},
		{
			name: "owner session",
			user: &kuser.DefaultInfo{Groups: types.RoleOwner.Groups()},
			want: true,
		},
		{
			// Tokens carry only their scopes' groups; the user's current role is in obot_groups.
			name: "token of an admin",
			user: &kuser.DefaultInfo{
				Groups: []string{types.GroupMCP, types.GroupAuthenticated},
				Extra:  map[string][]string{"obot_groups": types.RoleAdmin.Groups()},
			},
			want: true,
		},
		{
			name: "basic user",
			user: &kuser.DefaultInfo{Groups: types.RoleBasic.Groups()},
		},
		{
			name: "auditor",
			user: &kuser.DefaultInfo{Groups: (types.RoleBasic | types.RoleAuditor).Groups()},
		},
		{
			name: "token of a basic user",
			user: &kuser.DefaultInfo{
				Groups: []string{types.GroupMCP, types.GroupAuthenticated},
				Extra:  map[string][]string{"obot_groups": types.RoleBasic.Groups()},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsAdmin(tt.user); got != tt.want {
				t.Fatalf("IsAdmin() = %v, want %v", got, tt.want)
			}
		})
	}
}
