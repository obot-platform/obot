package v1

import (
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
)

func TestMCPServerSpec_IsOwnedBy(t *testing.T) {
	tests := []struct {
		name   string
		spec   MCPServerSpec
		userID string
		want   bool
	}{
		{
			name:   "single-user server owned by user",
			spec:   MCPServerSpec{ServerUserType: types.ServerUserTypeSingleUser, UserID: "user-1"},
			userID: "user-1",
			want:   true,
		},
		{
			name:   "single-user server owned by different user",
			spec:   MCPServerSpec{ServerUserType: types.ServerUserTypeSingleUser, UserID: "user-1"},
			userID: "user-2",
			want:   false,
		},
		{
			name:   "multi-user server — never directly owned",
			spec:   MCPServerSpec{ServerUserType: types.ServerUserTypeMultiUser, UserID: "user-1"},
			userID: "user-1",
			want:   false,
		},
		{
			name:   "legacy single-user: no catalog/workspace",
			spec:   MCPServerSpec{UserID: "user-1"},
			userID: "user-1",
			want:   true,
		},
		{
			name:   "legacy multi-user: catalog set — not directly owned even if UserID matches",
			spec:   MCPServerSpec{UserID: "user-1", MCPCatalogID: "default"},
			userID: "user-1",
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.spec.IsOwnedBy(tt.userID); got != tt.want {
				t.Errorf("MCPServerSpec.IsOwnedBy(%q) = %v, want %v", tt.userID, got, tt.want)
			}
		})
	}
}

func TestMCPServerSpec_IsSingleUser(t *testing.T) {
	tests := []struct {
		name   string
		spec   MCPServerSpec
		want   bool
	}{
		{
			name: "explicit singleUser type",
			spec: MCPServerSpec{ServerUserType: types.ServerUserTypeSingleUser},
			want: true,
		},
		{
			name: "explicit multiUser type",
			spec: MCPServerSpec{ServerUserType: types.ServerUserTypeMultiUser},
			want: false,
		},
		{
			name: "legacy: empty type, no catalog/workspace",
			spec: MCPServerSpec{MCPCatalogID: "", PowerUserWorkspaceID: ""},
			want: true,
		},
		{
			name: "legacy: empty type, catalog set",
			spec: MCPServerSpec{MCPCatalogID: "default"},
			want: false,
		},
		{
			name: "legacy: empty type, workspace set",
			spec: MCPServerSpec{PowerUserWorkspaceID: "ws-1"},
			want: false,
		},
		{
			name: "ServerUserType takes precedence over legacy MCPCatalogID",
			spec: MCPServerSpec{ServerUserType: types.ServerUserTypeSingleUser, MCPCatalogID: "default"},
			want: true,
		},
		{
			name: "ServerUserType takes precedence over legacy PowerUserWorkspaceID",
			spec: MCPServerSpec{ServerUserType: types.ServerUserTypeSingleUser, PowerUserWorkspaceID: "ws-1"},
			want: true,
		},
		{
			name: "multiUser type with empty ownership fields",
			spec: MCPServerSpec{ServerUserType: types.ServerUserTypeMultiUser},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.spec.IsSingleUser(); got != tt.want {
				t.Errorf("MCPServerSpec.IsSingleUser() = %v, want %v", got, tt.want)
			}
		})
	}
}
