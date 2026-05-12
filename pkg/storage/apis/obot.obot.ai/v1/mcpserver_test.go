package v1

import (
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
)

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
