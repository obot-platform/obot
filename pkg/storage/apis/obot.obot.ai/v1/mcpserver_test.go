package v1

import "testing"

func TestMCPServerSpec_IsCatalogServer(t *testing.T) {
	if (MCPServerSpec{MCPCatalogID: "default"}).IsCatalogServer() != true {
		t.Error("expected true for catalog server")
	}
	if (MCPServerSpec{PowerUserWorkspaceID: "ws-1"}).IsCatalogServer() != false {
		t.Error("expected false for workspace server")
	}
	if (MCPServerSpec{}).IsCatalogServer() != false {
		t.Error("expected false for single-user server")
	}
}

func TestMCPServerSpec_IsPowerUserWorkspaceServer(t *testing.T) {
	if (MCPServerSpec{PowerUserWorkspaceID: "ws-1"}).IsPowerUserWorkspaceServer() != true {
		t.Error("expected true for workspace server")
	}
	if (MCPServerSpec{MCPCatalogID: "default"}).IsPowerUserWorkspaceServer() != false {
		t.Error("expected false for catalog server")
	}
	if (MCPServerSpec{}).IsPowerUserWorkspaceServer() != false {
		t.Error("expected false for single-user server")
	}
}

func TestMCPServerSpec_IsOwnedBy(t *testing.T) {
	tests := []struct {
		name   string
		spec   MCPServerSpec
		userID string
		want   bool
	}{
		{
			name:   "single-user server owned by user",
			spec:   MCPServerSpec{UserID: "user-1"},
			userID: "user-1",
			want:   true,
		},
		{
			name:   "single-user server owned by different user",
			spec:   MCPServerSpec{UserID: "user-1"},
			userID: "user-2",
			want:   false,
		},
		{
			name:   "catalog server — never directly owned even if UserID matches",
			spec:   MCPServerSpec{UserID: "user-1", MCPCatalogID: "default"},
			userID: "user-1",
			want:   false,
		},
		{
			name:   "workspace server — never directly owned even if UserID matches",
			spec:   MCPServerSpec{UserID: "user-1", PowerUserWorkspaceID: "ws-1"},
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
		name string
		spec MCPServerSpec
		want bool
	}{
		{
			name: "no catalog/workspace: single-user",
			spec: MCPServerSpec{MCPCatalogID: "", PowerUserWorkspaceID: ""},
			want: true,
		},
		{
			name: "catalog set: multi-user",
			spec: MCPServerSpec{MCPCatalogID: "default"},
			want: false,
		},
		{
			name: "workspace set: multi-user",
			spec: MCPServerSpec{PowerUserWorkspaceID: "ws-1"},
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
