package authz

import (
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/system"
	kuser "k8s.io/apiserver/pkg/authentication/user"
)

func TestUserCanUseBuiltinMCP(t *testing.T) {
	tests := []struct {
		name  string
		user  kuser.Info
		mcpID string
		want  bool
	}{
		{
			name:  "basic user, user server",
			user:  &kuser.DefaultInfo{Name: "u", UID: "1", Groups: []string{types.GroupMCP}, Extra: map[string][]string{"obot_groups": {types.GroupBasic}}},
			mcpID: system.BuiltinUserMCPID,
			want:  true,
		},
		{
			name:  "basic user, admin server",
			user:  &kuser.DefaultInfo{Name: "u", UID: "1", Groups: []string{types.GroupMCP}, Extra: map[string][]string{"obot_groups": {types.GroupBasic}}},
			mcpID: system.BuiltinAdminMCPID,
		},
		{
			name:  "admin role from an MCP token",
			user:  &kuser.DefaultInfo{Name: "a", UID: "2", Groups: []string{types.GroupMCP}, Extra: map[string][]string{"obot_groups": {types.GroupAdmin}}},
			mcpID: system.BuiltinAdminMCPID,
			want:  true,
		},
		{
			name:  "owner in groups",
			user:  &kuser.DefaultInfo{Name: "o", UID: "3", Groups: []string{types.GroupOwner}},
			mcpID: system.BuiltinAdminMCPID,
			want:  true,
		},
		{
			name:  "anonymous",
			user:  &kuser.DefaultInfo{Name: "anonymous"},
			mcpID: system.BuiltinUserMCPID,
		},
		{
			name:  "not built in",
			user:  &kuser.DefaultInfo{Name: "o", UID: "3", Groups: []string{types.GroupOwner}},
			mcpID: "vmcp1abc",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := UserCanUseBuiltinMCP(tt.user, tt.mcpID); got != tt.want {
				t.Fatalf("UserCanUseBuiltinMCP() = %v, want %v", got, tt.want)
			}
		})
	}
}
