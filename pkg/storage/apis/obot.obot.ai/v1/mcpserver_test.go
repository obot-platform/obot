package v1

import (
	"testing"
)

func TestMCPServerDeleteRefsProtectsComponentsFromCatalogDeletion(t *testing.T) {
	for _, tc := range []struct {
		name        string
		spec        MCPServerSpec
		wantCatalog bool
	}{
		{
			name:        "standalone",
			wantCatalog: true,
		},
		{
			name: "composite component",
			spec: MCPServerSpec{CompositeName: "ms1composite"},
		},
		{
			name: "shared vMCP component",
			spec: MCPServerSpec{
				VMCPID:          "vmcp1shared",
				VMCPComponentID: "one",
			},
		},
		{
			name: "dedicated vMCP component",
			spec: MCPServerSpec{
				VMCPInstanceID:  "vmcpi1dedicated",
				VMCPComponentID: "one",
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.spec.MCPServerCatalogEntryName = "mcpce1source"
			server := MCPServer{Spec: tc.spec}
			var catalog, composite, vmcp, instance string
			for _, ref := range server.DeleteRefs() {
				switch ref.ObjType.(type) {
				case *MCPServerCatalogEntry:
					catalog = ref.Name
				case *MCPServer:
					composite = ref.Name
				case *VMCP:
					vmcp = ref.Name
				case *VMCPInstance:
					instance = ref.Name
				}
			}
			wantCatalog := ""
			if tc.wantCatalog {
				wantCatalog = tc.spec.MCPServerCatalogEntryName
			}
			if catalog != wantCatalog {
				t.Errorf("catalog deletion reference = %q, want %q", catalog, wantCatalog)
			}
			if composite != tc.spec.CompositeName || vmcp != tc.spec.VMCPID || instance != tc.spec.VMCPInstanceID {
				t.Errorf("component owner references were lost: composite=%q vmcp=%q instance=%q", composite, vmcp, instance)
			}
		})
	}
}

func TestMCPServerCredentialContext(t *testing.T) {
	for _, tc := range []struct {
		name   string
		spec   MCPServerSpec
		userID string
		want   string
	}{
		{
			name:   "personal uses supplied user",
			spec:   MCPServerSpec{UserID: "owner"},
			userID: "requester",
			want:   "requester-server",
		},
		{
			name: "catalog",
			spec: MCPServerSpec{
				UserID:       "owner",
				MCPCatalogID: "catalog",
			},
			userID: "requester",
			want:   "catalog-server",
		},
		{
			name: "workspace",
			spec: MCPServerSpec{
				UserID:               "owner",
				PowerUserWorkspaceID: "workspace",
			},
			userID: "requester",
			want:   "workspace-server",
		},
		{
			name: "vMCP takes precedence over catalog",
			spec: MCPServerSpec{
				UserID:       "owner",
				MCPCatalogID: "catalog",
				VMCPID:       "vmcp",
			},
			userID: "requester",
			want:   "vmcp-server",
		},
		{
			name: "vMCP instance takes precedence over workspace",
			spec: MCPServerSpec{
				UserID:               "owner",
				PowerUserWorkspaceID: "workspace",
				VMCPInstanceID:       "instance",
			},
			userID: "requester",
			want:   "instance-server",
		},
		{
			name: "vMCP instance takes precedence over catalog",
			spec: MCPServerSpec{
				MCPCatalogID:   "catalog",
				VMCPInstanceID: "instance",
			},
			userID: "requester",
			want:   "instance-server",
		},
		{
			name: "vMCP does not require a user",
			spec: MCPServerSpec{
				UserID:       "owner",
				MCPCatalogID: "catalog",
				VMCPID:       "vmcp",
			},
			want: "vmcp-server",
		},
		{
			name: "personal empty user remains empty",
			spec: MCPServerSpec{UserID: "owner"},
			want: "-server",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := MCPServer{Name: "server", Spec: tc.spec}
			if got := server.CredentialContext(tc.userID); got != tc.want {
				t.Fatalf("CredentialContext(%q) = %q, want %q", tc.userID, got, tc.want)
			}
		})
	}
}

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
			name:   "matching UserID",
			spec:   MCPServerSpec{UserID: "user-1"},
			userID: "user-1",
			want:   true,
		},
		{
			name:   "non-matching UserID",
			spec:   MCPServerSpec{UserID: "user-1"},
			userID: "user-2",
			want:   false,
		},
		{
			name:   "admin-deployed catalog server — not owned even if UserID matches",
			spec:   MCPServerSpec{UserID: "user-1", MCPCatalogID: "default"},
			userID: "user-1",
			want:   false,
		},
		{
			name:   "workspace server with matching UserID — owned",
			spec:   MCPServerSpec{UserID: "user-1", PowerUserWorkspaceID: "ws-1"},
			userID: "user-1",
			want:   true,
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
