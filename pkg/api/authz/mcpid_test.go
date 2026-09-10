package authz

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/accesscontrolrule"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	"k8s.io/apiserver/pkg/authentication/user"
	gocache "k8s.io/client-go/tools/cache"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	clientfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestVMCPAPIKeyScopes(t *testing.T) {
	vmcp := &v1.VMCP{Name: "vmcp1test", Namespace: "default", Spec: v1.VMCPSpec{Manifest: types.VMCPManifest{
		Components: []types.VMCPComponent{{ID: "component"}},
		Profiles:   []types.VMCPProfile{{Subjects: []types.Subject{{Type: types.SubjectTypeUser, ID: "7"}}, AllowAllTools: true}},
	}}}
	instance := &v1.VMCPInstance{Name: "vmcpi1test", Namespace: "default", Spec: v1.VMCPInstanceSpec{UserID: "7", Manifest: types.VMCPInstanceManifest{VMCPID: vmcp.Name}}}
	shared := &v1.MCPServer{Name: "ms1shared", Namespace: "default", Spec: v1.MCPServerSpec{VMCPID: vmcp.Name}}
	dedicated := &v1.MCPServer{Name: "ms1dedicated", Namespace: "default", Spec: v1.MCPServerSpec{VMCPInstanceID: instance.Name, UserID: "7"}}
	storage := newMCPIDIsAuthorizedTestStorage(vmcp, instance, shared, dedicated)
	for _, id := range []string{vmcp.Name, shared.Name, dedicated.Name} {
		for _, scope := range []string{vmcp.Name, "*", "vmcp1other"} {
			u := &user.DefaultInfo{UID: "7", Extra: map[string][]string{"authorized_mcp_ids": {scope}}}
			authorizer := &Authorizer{cache: storage, uncached: storage}
			ok, err := authorizer.checkMCPID(httptest.NewRequest(http.MethodPost, "/mcp-connect/"+id, nil), &Resources{MCPID: id}, newUser(u))
			if err != nil || ok != (id == vmcp.Name && scope != "vmcp1other") {
				t.Fatalf("id=%s scope=%s: allowed=%v error=%v", id, scope, ok, err)
			}
		}
	}
	if ok, err := MCPIDIsAuthorized(t.Context(), storage, []string{vmcp.Name}, "other", dedicated.Name); err != nil || ok {
		t.Fatalf("scope crossed instance ownership: allowed=%v error=%v", ok, err)
	}
	vmcp.Spec.Manifest.Profiles = nil
	if err := storage.Update(t.Context(), vmcp); err != nil {
		t.Fatal(err)
	}
	authorizer := &Authorizer{cache: storage, uncached: storage}
	for _, id := range []string{vmcp.Name, shared.Name, dedicated.Name} {
		ok, err := authorizer.checkMCPID(httptest.NewRequest(http.MethodPost, "/mcp-connect/"+id, nil), &Resources{MCPID: id}, newUser(&user.DefaultInfo{UID: "7", Extra: map[string][]string{"authorized_mcp_ids": {vmcp.Name}}}))
		if err != nil || ok {
			t.Fatalf("revoked profile still authorized for %s: %v, %v", id, ok, err)
		}
	}
}

func TestVMCPComponentsRequireInternalForwarding(t *testing.T) {
	vmcp := &v1.VMCP{Name: "vmcp1restricted", Namespace: "default", Spec: v1.VMCPSpec{Manifest: types.VMCPManifest{
		Components: []types.VMCPComponent{{ID: "component"}},
		Profiles:   []types.VMCPProfile{{Subjects: []types.Subject{{Type: types.SubjectTypeUser, ID: "7"}}, AllowedTools: types.VMCPToolSet{"component": {"echo"}}}},
	}}}
	instance := &v1.VMCPInstance{Name: "vmcpi1restricted", Namespace: "default", Spec: v1.VMCPInstanceSpec{UserID: "7", Manifest: types.VMCPInstanceManifest{VMCPID: vmcp.Name}}}
	shared := &v1.MCPServer{Name: "ms1shared", Namespace: "default", Spec: v1.MCPServerSpec{VMCPID: vmcp.Name, VMCPComponentID: "component"}}
	dedicated := &v1.MCPServer{Name: "ms1dedicated", Namespace: "default", Spec: v1.MCPServerSpec{VMCPInstanceID: instance.Name, UserID: "7", VMCPComponentID: "component"}}
	storage := newMCPIDIsAuthorizedTestStorage(vmcp, instance, shared, dedicated)
	authorizer := &Authorizer{cache: storage, uncached: storage}
	for _, server := range []*v1.MCPServer{shared, dedicated} {
		alias := &v1.MCPServerInstance{Name: "msi1" + server.Name, Namespace: "default", Spec: v1.MCPServerInstanceSpec{MCPServerName: server.Name, UserID: "7"}}
		if err := storage.Create(t.Context(), alias); err != nil {
			t.Fatal(err)
		}
		for _, caller := range []User{newUser(&user.DefaultInfo{UID: "7"}), agentUser(alias.Name)} {
			allowed, err := authorizer.checkMCPID(httptest.NewRequest(http.MethodPost, "/mcp-connect/"+alias.Name, nil), &Resources{MCPID: alias.Name}, caller)
			if err != nil || allowed {
				t.Fatalf("legacy instance bypassed aggregate: allowed=%v error=%v", allowed, err)
			}
		}
		for _, scope := range []string{server.Name, vmcp.Name, "*"} {
			allowed, err := authorizer.checkMCPID(httptest.NewRequest(http.MethodPost, "/mcp-connect/"+server.Name, nil), &Resources{MCPID: server.Name}, agentUser(scope))
			if err != nil || allowed {
				t.Fatalf("hosted agent bypassed aggregate: allowed=%v error=%v", allowed, err)
			}
		}
		for _, tc := range []struct {
			name    string
			groups  []string
			scopes  []string
			allowed bool
		}{
			{
				name: "ordinary user",
			},
			{
				name:   "vmcp key",
				scopes: []string{vmcp.Name},
			},
			{
				name:   "component key",
				scopes: []string{server.Name},
			},
			{
				name:   "wildcard key",
				scopes: []string{"*"},
			},
			{
				name:   "internal wrong scope",
				groups: []string{types.GroupCompositeMCP},
				scopes: []string{vmcp.Name},
			},
			{
				name:    "internal forwarding",
				groups:  []string{types.GroupCompositeMCP},
				scopes:  []string{server.Name},
				allowed: server.Spec.VMCPID == "",
			},
		} {
			t.Run(server.Name+"/"+tc.name, func(t *testing.T) {
				u := &user.DefaultInfo{UID: "7", Groups: tc.groups, Extra: map[string][]string{"authorized_mcp_ids": tc.scopes}}
				allowed, err := authorizer.checkMCPID(httptest.NewRequest(http.MethodPost, "/mcp-connect/"+server.Name, nil), &Resources{MCPID: server.Name}, newUser(u))
				if err != nil || allowed != tc.allowed {
					t.Fatalf("allowed=%v error=%v", allowed, err)
				}
			})
		}
	}
}

func TestCheckMCPIDAllowsAnonymousMCPConnect(t *testing.T) {
	authorizer := &Authorizer{}
	req := httptest.NewRequest(http.MethodGet, "/mcp-connect/ms1test", nil)

	ok, err := authorizer.checkMCPID(req, &Resources{MCPID: "ms1test"}, newUser(&user.DefaultInfo{Name: "anonymous"}))
	if err != nil {
		t.Fatalf("checkMCPID() error = %v", err)
	}
	if !ok {
		t.Fatal("checkMCPID() = false, want true")
	}
}

func TestCheckMCPIDDoesNotBypassNonMCPConnectForAnonymous(t *testing.T) {
	storage := clientfake.NewClientBuilder().WithScheme(storagescheme.Scheme).Build()
	authorizer := &Authorizer{cache: storage, uncached: storage}
	req := httptest.NewRequest(http.MethodGet, "/oauth/authorize/ms1test", nil)

	ok, err := authorizer.checkMCPID(req, &Resources{MCPID: "ms1test"}, newUser(&user.DefaultInfo{Name: "anonymous"}))
	if err == nil {
		t.Fatal("checkMCPID() error = nil, want error")
	}
	if ok {
		t.Fatal("checkMCPID() = true, want false")
	}
}

func TestCheckMCPIDChecksMCPServerInstanceOwner(t *testing.T) {
	storage := clientfake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(&v1.MCPServerInstance{
		Name:      "msi1test",
		Namespace: system.DefaultNamespace,
		Spec: v1.MCPServerInstanceSpec{
			UserID:        "user-uid",
			MCPServerName: "ms1test",
		},
	}, &v1.MCPServer{Name: "ms1test", Namespace: system.DefaultNamespace}).Build()
	authorizer := &Authorizer{cache: storage, uncached: storage}
	req := httptest.NewRequest(http.MethodGet, "/mcp-connect/msi1test", nil)

	ok, err := authorizer.checkMCPID(req, &Resources{MCPID: "msi1test"}, newUser(&user.DefaultInfo{
		Name: "user",
		UID:  "user-uid",
	}))
	if err != nil {
		t.Fatalf("checkMCPID() error = %v", err)
	}
	if !ok {
		t.Fatal("checkMCPID() = false, want true")
	}
}

func TestCheckMCPIDChecksSystemMCPServerEnabled(t *testing.T) {
	tests := []struct {
		name    string
		enabled *bool
		allowed bool
	}{
		{
			name:    "nil enabled defaults to allowed",
			allowed: true,
		},
		{
			name:    "explicitly enabled is allowed",
			enabled: new(true),
			allowed: true,
		},
		{
			name:    "explicitly disabled is denied",
			enabled: new(false),
			allowed: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			storage := clientfake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(&v1.SystemMCPServer{
				Name:      "sms1test",
				Namespace: system.DefaultNamespace,
				Spec: v1.SystemMCPServerSpec{
					Manifest: types.SystemMCPServerManifest{
						Enabled: tt.enabled,
					},
				},
			}).Build()

			authorizer := &Authorizer{cache: storage, uncached: storage}
			req := httptest.NewRequest(http.MethodGet, "/mcp-connect/sms1test", nil)

			ok, err := authorizer.checkMCPID(req, &Resources{MCPID: "sms1test"}, newUser(&user.DefaultInfo{
				Name: "user",
				UID:  "user-uid",
			}))
			if err != nil {
				t.Fatalf("checkMCPID() error = %v", err)
			}
			if ok != tt.allowed {
				t.Fatalf("checkMCPID() = %v, want %v", ok, tt.allowed)
			}
		})
	}
}

func TestCheckMCPIDChecksVMCPAccess(t *testing.T) {
	shared := &v1.VMCP{
		Name:      "vmcp1shared",
		Namespace: system.DefaultNamespace,
		Spec: v1.VMCPSpec{
			Manifest: types.VMCPManifest{
				Components: []types.VMCPComponent{{ID: "component"}},
				Profiles: []types.VMCPProfile{
					{
						Name: "allowed-users",
						Subjects: []types.Subject{
							{
								Type: types.SubjectTypeUser,
								ID:   "allowed-user",
							},
						},
					},
				},
			},
		},
	}
	personal := &v1.VMCP{
		Name:      "vmcp1personal",
		Namespace: system.DefaultNamespace,
		Spec: v1.VMCPSpec{
			UserID:   "owner-user",
			Manifest: types.VMCPManifest{Components: []types.VMCPComponent{{ID: "component"}}},
		},
	}
	storage := clientfake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(shared, personal).Build()
	authorizer := &Authorizer{
		cache:    storage,
		uncached: storage,
	}

	tests := []struct {
		name    string
		mcpID   string
		userID  string
		allowed bool
	}{
		{
			name:    "shared profile user is allowed",
			mcpID:   shared.Name,
			userID:  "allowed-user",
			allowed: true,
		},
		{
			name:   "shared unrelated user is denied",
			mcpID:  shared.Name,
			userID: "other-user",
		},
		{
			name:    "personal owner is allowed",
			mcpID:   personal.Name,
			userID:  "owner-user",
			allowed: true,
		},
		{
			name:   "personal non-owner is denied",
			mcpID:  personal.Name,
			userID: "other-user",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/mcp-connect/"+tt.mcpID, nil)
			ok, err := authorizer.checkMCPID(req, &Resources{MCPID: tt.mcpID}, newUser(&user.DefaultInfo{
				Name: tt.userID,
				UID:  tt.userID,
			}))
			if err != nil {
				t.Fatalf("checkMCPID() error = %v", err)
			}
			if ok != tt.allowed {
				t.Fatalf("checkMCPID() = %v, want %v", ok, tt.allowed)
			}
		})
	}
}

func TestCheckMCPIDChecksMCPServerCatalogAccess(t *testing.T) {
	storage := clientfake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(&v1.MCPServer{
		Name:      "ms1catalog",
		Namespace: system.DefaultNamespace,
		Spec: v1.MCPServerSpec{
			MCPCatalogID: "catalog-a",
			UserID:       "owner-uid",
		},
	}).Build()
	authorizer := newMCPIDTestAuthorizer(t, storage, &v1.AccessControlRule{
		Name:      "acr1server",
		Namespace: system.DefaultNamespace,
		Spec: v1.AccessControlRuleSpec{
			MCPCatalogID: "catalog-a",
			Manifest: types.AccessControlRuleManifest{
				Subjects:  []types.Subject{{Type: types.SubjectTypeUser, ID: "user-uid"}},
				Resources: []types.Resource{{Type: types.ResourceTypeMCPServer, ID: "ms1catalog"}},
			},
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/mcp-connect/ms1catalog", nil)

	ok, err := authorizer.checkMCPID(req, &Resources{MCPID: "ms1catalog"}, newUser(&user.DefaultInfo{Name: "user", UID: "user-uid"}))
	if err != nil {
		t.Fatalf("checkMCPID() error = %v", err)
	}
	if !ok {
		t.Fatal("checkMCPID() = false, want true")
	}

	ok, err = authorizer.checkMCPID(req, &Resources{MCPID: "ms1catalog"}, newUser(&user.DefaultInfo{Name: "other", UID: "other-uid"}))
	if err != nil {
		t.Fatalf("checkMCPID() error = %v", err)
	}
	if ok {
		t.Fatal("checkMCPID() = true, want false")
	}
}

func TestCheckMCPIDChecksCatalogEntryAccess(t *testing.T) {
	storage := clientfake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(&v1.MCPServerCatalogEntry{
		Name:      "entry-test",
		Namespace: system.DefaultNamespace,
		Spec: v1.MCPServerCatalogEntrySpec{
			MCPCatalogName: "catalog-a",
		},
	}).Build()
	authorizer := newMCPIDTestAuthorizer(t, storage, &v1.AccessControlRule{
		Name:      "acr1entry",
		Namespace: system.DefaultNamespace,
		Spec: v1.AccessControlRuleSpec{
			MCPCatalogID: "catalog-a",
			Manifest: types.AccessControlRuleManifest{
				Subjects:  []types.Subject{{Type: types.SubjectTypeUser, ID: "user-uid"}},
				Resources: []types.Resource{{Type: types.ResourceTypeMCPServerCatalogEntry, ID: "entry-test"}},
			},
		},
	})
	req := httptest.NewRequest(http.MethodGet, "/mcp-connect/entry-test", nil)

	ok, err := authorizer.checkMCPID(req, &Resources{MCPID: "entry-test"}, newUser(&user.DefaultInfo{Name: "user", UID: "user-uid"}))
	if err != nil {
		t.Fatalf("checkMCPID() error = %v", err)
	}
	if !ok {
		t.Fatal("checkMCPID() = false, want true")
	}

	ok, err = authorizer.checkMCPID(req, &Resources{MCPID: "entry-test"}, newUser(&user.DefaultInfo{Name: "other", UID: "other-uid"}))
	if err != nil {
		t.Fatalf("checkMCPID() error = %v", err)
	}
	if ok {
		t.Fatal("checkMCPID() = true, want false")
	}
}

func TestCheckMCPIDChecksWorkspaceAccess(t *testing.T) {
	storage := clientfake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(
		&v1.MCPServer{
			Name:      "ms1workspace",
			Namespace: system.DefaultNamespace,
			Spec: v1.MCPServerSpec{
				PowerUserWorkspaceID: "puw1test",
				UserID:               "owner-uid",
			},
		},
		&v1.MCPServerCatalogEntry{
			Name:      "workspace-entry",
			Namespace: system.DefaultNamespace,
			Spec: v1.MCPServerCatalogEntrySpec{
				PowerUserWorkspaceID: "puw1test",
			},
		},
		&v1.PowerUserWorkspace{
			Name:      "puw1test",
			Namespace: system.DefaultNamespace,
			Spec: v1.PowerUserWorkspaceSpec{
				UserID: "owner-uid",
			},
		},
	).Build()
	authorizer := newMCPIDTestAuthorizer(t, storage, &v1.AccessControlRule{
		Name:      "acr1workspace",
		Namespace: system.DefaultNamespace,
		Spec: v1.AccessControlRuleSpec{
			PowerUserWorkspaceID: "puw1test",
			Manifest: types.AccessControlRuleManifest{
				Subjects:  []types.Subject{{Type: types.SubjectTypeUser, ID: "shared-user-uid"}},
				Resources: []types.Resource{{Type: types.ResourceTypeSelector, ID: "*"}},
			},
		},
	})

	tests := []struct {
		name    string
		mcpID   string
		userID  string
		allowed bool
	}{
		{name: "server owner is allowed", mcpID: "ms1workspace", userID: "owner-uid", allowed: true},
		{name: "server shared user is allowed", mcpID: "ms1workspace", userID: "shared-user-uid", allowed: true},
		{name: "server unrelated user is denied", mcpID: "ms1workspace", userID: "other-uid", allowed: false},
		{name: "entry workspace owner is allowed", mcpID: "workspace-entry", userID: "owner-uid", allowed: true},
		{name: "entry shared user is allowed", mcpID: "workspace-entry", userID: "shared-user-uid", allowed: true},
		{name: "entry unrelated user is denied", mcpID: "workspace-entry", userID: "other-uid", allowed: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/mcp-connect/"+tt.mcpID, nil)

			ok, err := authorizer.checkMCPID(req, &Resources{MCPID: tt.mcpID}, newUser(&user.DefaultInfo{Name: "user", UID: tt.userID}))
			if err != nil {
				t.Fatalf("checkMCPID() error = %v", err)
			}
			if ok != tt.allowed {
				t.Fatalf("checkMCPID() = %v, want %v", ok, tt.allowed)
			}
		})
	}
}

func TestMCPIDIsAuthorized(t *testing.T) {
	tests := []struct {
		name       string
		objects    []kclient.Object
		authorized []string
		userID     string
		mcpID      string
		want       bool
		wantErr    bool
	}{
		{
			name:       "wildcard allows missing server",
			authorized: []string{"*"},
			userID:     "user-uid",
			mcpID:      "ms1missing",
			want:       true,
		},
		{
			name:       "direct server ID allows without storage lookup",
			authorized: []string{"ms1missing"},
			userID:     "user-uid",
			mcpID:      "ms1missing",
			want:       true,
		},
		{
			name: "server composite parent allows component server",
			objects: []kclient.Object{&v1.MCPServer{
				Name:      "ms1component",
				Namespace: system.DefaultNamespace,
				Spec: v1.MCPServerSpec{
					CompositeName: "ms1composite",
				},
			}},
			authorized: []string{"ms1composite"},
			userID:     "user-uid",
			mcpID:      "ms1component",
			want:       true,
		},
		{
			name: "server without authorized composite is denied",
			objects: []kclient.Object{&v1.MCPServer{
				Name:      "ms1component",
				Namespace: system.DefaultNamespace,
				Spec: v1.MCPServerSpec{
					CompositeName: "ms1composite",
				},
			}},
			authorized: []string{"ms1other"},
			userID:     "user-uid",
			mcpID:      "ms1component",
			want:       false,
		},
		{
			name:       "missing server returns error",
			authorized: []string{"ms1other"},
			userID:     "user-uid",
			mcpID:      "ms1missing",
			wantErr:    true,
		},
		{
			name: "instance direct ID allows without storage lookup",
			objects: []kclient.Object{&v1.MCPServerInstance{
				Name:      "msi1instance",
				Namespace: system.DefaultNamespace,
			}},
			authorized: []string{"msi1instance"},
			userID:     "user-uid",
			mcpID:      "msi1instance",
			want:       true,
		},
		{
			name: "instance composite parent allows component instance",
			objects: []kclient.Object{&v1.MCPServerInstance{
				Name:      "msi1component",
				Namespace: system.DefaultNamespace,
				Spec: v1.MCPServerInstanceSpec{
					CompositeName: "ms1composite",
				},
			}},
			authorized: []string{"ms1composite"},
			userID:     "user-uid",
			mcpID:      "msi1component",
			want:       true,
		},
		{
			name: "instance checks associated server composite parent",
			objects: []kclient.Object{
				&v1.MCPServerInstance{
					Name:      "msi1instance",
					Namespace: system.DefaultNamespace,
					Spec: v1.MCPServerInstanceSpec{
						MCPServerName: "ms1component",
					},
				},
				&v1.MCPServer{
					Name:      "ms1component",
					Namespace: system.DefaultNamespace,
					Spec: v1.MCPServerSpec{
						CompositeName: "ms1composite",
					},
				},
			},
			authorized: []string{"ms1composite"},
			userID:     "user-uid",
			mcpID:      "msi1instance",
			want:       true,
		},
		{
			name: "catalog entry allows matching user server",
			objects: []kclient.Object{
				&v1.MCPServerCatalogEntry{
					Name:      "entry-test",
					Namespace: system.DefaultNamespace,
				},
				&v1.MCPServer{
					Name:      "ms1fromentry",
					Namespace: system.DefaultNamespace,
					Spec: v1.MCPServerSpec{
						MCPServerCatalogEntryName: "entry-test",
						UserID:                    "user-uid",
					},
				},
			},
			authorized: []string{"ms1fromentry"},
			userID:     "user-uid",
			mcpID:      "entry-test",
			want:       true,
		},
		{
			name: "catalog entry allows matching user server composite parent",
			objects: []kclient.Object{
				&v1.MCPServerCatalogEntry{
					Name:      "entry-test",
					Namespace: system.DefaultNamespace,
				},
				&v1.MCPServer{
					Name:      "ms1fromentry",
					Namespace: system.DefaultNamespace,
					Spec: v1.MCPServerSpec{
						MCPServerCatalogEntryName: "entry-test",
						UserID:                    "user-uid",
						CompositeName:             "ms1composite",
					},
				},
			},
			authorized: []string{"ms1composite"},
			userID:     "user-uid",
			mcpID:      "entry-test",
			want:       true,
		},
		{
			name: "catalog entry ignores matching server for different user",
			objects: []kclient.Object{
				&v1.MCPServerCatalogEntry{
					Name:      "entry-test",
					Namespace: system.DefaultNamespace,
				},
				&v1.MCPServer{
					Name:      "ms1fromentry",
					Namespace: system.DefaultNamespace,
					Spec: v1.MCPServerSpec{
						MCPServerCatalogEntryName: "entry-test",
						UserID:                    "other-uid",
					},
				},
			},
			authorized: []string{"ms1fromentry"},
			userID:     "user-uid",
			mcpID:      "entry-test",
			want:       false,
		},
		{
			name: "catalog entry ignores vMCP component server scope",
			objects: []kclient.Object{
				&v1.MCPServerCatalogEntry{
					Name:      "entry-test",
					Namespace: system.DefaultNamespace,
				},
				&v1.MCPServer{
					Name:      "ms1-vmcp-component",
					Namespace: system.DefaultNamespace,
					Spec: v1.MCPServerSpec{
						MCPServerCatalogEntryName: "entry-test",
						UserID:                    "user-uid",
						VMCPID:                    "vmcp1-test",
					},
				},
			},
			authorized: []string{"ms1-vmcp-component"},
			userID:     "user-uid",
			mcpID:      "entry-test",
			want:       false,
		},
		{
			name: "vMCP component server does not inherit catalog entry scope",
			objects: []kclient.Object{&v1.MCPServer{
				Name:      "ms1-vmcp-component",
				Namespace: system.DefaultNamespace,
				Spec: v1.MCPServerSpec{
					MCPServerCatalogEntryName: "entry-test",
					UserID:                    "user-uid",
					VMCPID:                    "vmcp1-test",
				},
			}},
			authorized: []string{"entry-test"},
			userID:     "user-uid",
			mcpID:      "ms1-vmcp-component",
			want:       false,
		},
		{
			name: "catalog entry ignores vMCP instance component server scope",
			objects: []kclient.Object{
				&v1.MCPServerCatalogEntry{
					Name:      "entry-test",
					Namespace: system.DefaultNamespace,
				},
				&v1.MCPServer{
					Name:      "ms1-vmcp-instance-component",
					Namespace: system.DefaultNamespace,
					Spec: v1.MCPServerSpec{
						MCPServerCatalogEntryName: "entry-test",
						UserID:                    "user-uid",
						VMCPInstanceID:            "vmcpi1-test",
					},
				},
			},
			authorized: []string{"ms1-vmcp-instance-component"},
			userID:     "user-uid",
			mcpID:      "entry-test",
			want:       false,
		},
		{
			name:       "missing catalog entry denies without error",
			authorized: []string{"ms1fromentry"},
			userID:     "user-uid",
			mcpID:      "entry-test",
			want:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			storage := newMCPIDIsAuthorizedTestStorage(tt.objects...)

			got, err := MCPIDIsAuthorized(t.Context(), storage, tt.authorized, tt.userID, tt.mcpID)
			if (err != nil) != tt.wantErr {
				t.Fatalf("MCPIDIsAuthorized() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("MCPIDIsAuthorized() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMCPConnectSubtreeAuthorization(t *testing.T) {
	storage := clientfake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(
		&v1.MCPServer{
			Name:      "ms1test",
			Namespace: system.DefaultNamespace,
			Spec: v1.MCPServerSpec{
				UserID: "user-uid",
			},
		},
		&v1.MCPServer{
			Name:      "ms1keytest",
			Namespace: system.DefaultNamespace,
			Spec: v1.MCPServerSpec{
				UserID: "key-user-uid",
			},
		},
		&v1.MCPServerInstance{
			Name:      "msi1test",
			Namespace: system.DefaultNamespace,
			Spec: v1.MCPServerInstanceSpec{
				UserID:        "user-uid",
				MCPServerName: "ms1test",
			},
		},
		&v1.MCPServerInstance{
			Name:      "msi1keytest",
			Namespace: system.DefaultNamespace,
			Spec: v1.MCPServerInstanceSpec{
				UserID:        "key-user-uid",
				MCPServerName: "ms1keytest",
			},
		},
	).Build()
	authorizer := NewAuthorizer(nil, storage, storage, false, nil, nil, nil, false)

	tests := []struct {
		name    string
		method  string
		path    string
		user    user.Info
		allowed bool
	}{
		{
			name:   "MCP-scoped user can access exact connect path",
			method: http.MethodGet,
			path:   "/mcp-connect/msi1test",
			user: &user.DefaultInfo{
				Name:   "user",
				UID:    "user-uid",
				Groups: []string{types.GroupMCP, types.GroupAuthenticated},
			},
			allowed: true,
		},
		{
			name:   "MCP-scoped user can access exact server connect path",
			method: http.MethodGet,
			path:   "/mcp-connect/ms1test",
			user: &user.DefaultInfo{
				Name:   "user",
				UID:    "user-uid",
				Groups: []string{types.GroupMCP, types.GroupAuthenticated},
			},
			allowed: true,
		},
		{
			name:   "MCP-scoped user can access trailing slash",
			method: http.MethodPost,
			path:   "/mcp-connect/msi1test/",
			user: &user.DefaultInfo{
				Name:   "user",
				UID:    "user-uid",
				Groups: []string{types.GroupMCP, types.GroupAuthenticated},
			},
			allowed: true,
		},
		{
			name:   "MCP-scoped user can access server trailing slash",
			method: http.MethodPost,
			path:   "/mcp-connect/ms1test/",
			user: &user.DefaultInfo{
				Name:   "user",
				UID:    "user-uid",
				Groups: []string{types.GroupMCP, types.GroupAuthenticated},
			},
			allowed: true,
		},
		{
			name:   "MCP-scoped user can access subpath",
			method: http.MethodDelete,
			path:   "/mcp-connect/msi1test/messages/123",
			user: &user.DefaultInfo{
				Name:   "user",
				UID:    "user-uid",
				Groups: []string{types.GroupMCP, types.GroupAuthenticated},
			},
			allowed: true,
		},
		{
			name:   "MCP-scoped user can access server subpath",
			method: http.MethodDelete,
			path:   "/mcp-connect/ms1test/messages/123",
			user: &user.DefaultInfo{
				Name:   "user",
				UID:    "user-uid",
				Groups: []string{types.GroupMCP, types.GroupAuthenticated},
			},
			allowed: true,
		},
		{
			name:   "api key cannot access subpath for a server they don't own",
			method: http.MethodGet,
			path:   "/mcp-connect/msi1test/messages/123",
			user: &user.DefaultInfo{
				Name:   "key-user",
				UID:    "key-user-uid",
				Groups: []string{types.GroupMCP},
			},
			allowed: false,
		},
		{
			name:   "api key cannot access subpath for an MCP server they don't own",
			method: http.MethodGet,
			path:   "/mcp-connect/ms1test/messages/123",
			user: &user.DefaultInfo{
				Name:   "key-user",
				UID:    "key-user-uid",
				Groups: []string{types.GroupMCP},
			},
			allowed: false,
		},
		{
			name:   "api key can access subpath for a server they own",
			method: http.MethodGet,
			path:   "/mcp-connect/msi1keytest/messages/123",
			user: &user.DefaultInfo{
				Name:   "key-user",
				UID:    "key-user-uid",
				Groups: []string{types.GroupMCP},
			},
			allowed: true,
		},
		{
			name:   "api key can access subpath for an MCP server they own",
			method: http.MethodGet,
			path:   "/mcp-connect/ms1keytest/messages/123",
			user: &user.DefaultInfo{
				Name:   "key-user",
				UID:    "key-user-uid",
				Groups: []string{types.GroupMCP},
			},
			allowed: true,
		},
		{
			name:   "authenticated user without basic group cannot access subpath",
			method: http.MethodGet,
			path:   "/mcp-connect/msi1test/messages/123",
			user: &user.DefaultInfo{
				Name:   "user",
				UID:    "user-uid",
				Groups: []string{types.GroupAuthenticated},
			},
			allowed: false,
		},
		{
			name:   "authenticated user without basic group cannot access server subpath",
			method: http.MethodGet,
			path:   "/mcp-connect/ms1test/messages/123",
			user: &user.DefaultInfo{
				Name:   "user",
				UID:    "user-uid",
				Groups: []string{types.GroupAuthenticated},
			},
			allowed: false,
		},
		{
			name:   "basic user cannot access another user's instance subpath",
			method: http.MethodGet,
			path:   "/mcp-connect/msi1test/messages/123",
			user: &user.DefaultInfo{
				Name:   "other-user",
				UID:    "other-user-uid",
				Groups: []string{types.GroupBasic, types.GroupAuthenticated},
			},
			allowed: false,
		},
		{
			name:   "basic user cannot access another user's server subpath",
			method: http.MethodGet,
			path:   "/mcp-connect/ms1test/messages/123",
			user: &user.DefaultInfo{
				Name:   "other-user",
				UID:    "other-user-uid",
				Groups: []string{types.GroupBasic, types.GroupAuthenticated},
			},
			allowed: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			if got := authorizer.Authorize(req, tt.user); got != tt.allowed {
				t.Fatalf("Authorize() = %v, want %v", got, tt.allowed)
			}
		})
	}
}

func TestMCPTesterChatAuthorizationUsesConnectionPermission(t *testing.T) {
	storage := clientfake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(
		&v1.MCPServer{
			Name:      "ms1tester",
			Namespace: system.DefaultNamespace,
			Spec: v1.MCPServerSpec{
				UserID: "user-uid",
			},
		},
	).Build()
	authorizer := NewAuthorizer(nil, storage, storage, false, nil, nil, nil, false)

	allowedRequest := httptest.NewRequest(http.MethodPost, "/api/mcp-servers/ms1tester/tester/chat", nil)
	allowed := authorizer.Authorize(allowedRequest, &user.DefaultInfo{
		Name:   "user",
		UID:    "user-uid",
		Groups: []string{types.GroupAPI},
	})
	if !allowed {
		t.Fatal("authorized MCP user cannot access tester Chat")
	}

	deniedRequest := httptest.NewRequest(http.MethodPost, "/api/mcp-servers/ms1tester/tester/chat", nil)
	denied := authorizer.Authorize(deniedRequest, &user.DefaultInfo{
		Name:   "management-only-user",
		UID:    "management-only-uid",
		Groups: []string{types.GroupAPI},
	})
	if denied {
		t.Fatal("management-only user can access tester Chat")
	}
}

func newMCPIDIsAuthorizedTestStorage(objects ...kclient.Object) kclient.Client {
	return clientfake.NewClientBuilder().
		WithScheme(storagescheme.Scheme).
		WithIndex(&v1.VMCP{}, "spec.legacySlug", func(obj kclient.Object) []string { return []string{obj.(*v1.VMCP).Spec.LegacySlug} }).
		WithIndex(&v1.VMCPInstance{}, "spec.legacySlug", func(obj kclient.Object) []string { return []string{obj.(*v1.VMCPInstance).Spec.LegacySlug} }).
		WithIndex(&v1.VMCPInstance{}, "spec.userID", func(obj kclient.Object) []string { return []string{obj.(*v1.VMCPInstance).Spec.UserID} }).
		WithIndex(&v1.VMCPInstance{}, "spec.manifest.vmcpID", func(obj kclient.Object) []string { return []string{obj.(*v1.VMCPInstance).Spec.Manifest.VMCPID} }).
		WithIndex(&v1.MCPServer{}, "spec.mcpServerCatalogEntryName", func(obj kclient.Object) []string {
			server := obj.(*v1.MCPServer)
			if server.Spec.MCPServerCatalogEntryName == "" {
				return nil
			}
			return []string{server.Spec.MCPServerCatalogEntryName}
		}).
		WithIndex(&v1.MCPServer{}, "spec.userID", func(obj kclient.Object) []string {
			server := obj.(*v1.MCPServer)
			if server.Spec.UserID == "" {
				return nil
			}
			return []string{server.Spec.UserID}
		}).
		WithObjects(objects...).
		Build()
}

func newMCPIDTestAuthorizer(t *testing.T, storage kclient.Client, acrs ...*v1.AccessControlRule) *Authorizer {
	t.Helper()

	indexer := gocache.NewIndexer(gocache.MetaNamespaceKeyFunc, gocache.Indexers{
		"user-ids": func(obj any) ([]string, error) {
			acr := obj.(*v1.AccessControlRule)
			var results []string
			for _, subject := range acr.Spec.Manifest.Subjects {
				if subject.Type == types.SubjectTypeUser {
					results = append(results, subject.ID)
				}
			}
			return results, nil
		},
		"catalog-entry-names": func(obj any) ([]string, error) {
			acr := obj.(*v1.AccessControlRule)
			var results []string
			for _, resource := range acr.Spec.Manifest.Resources {
				if resource.Type == types.ResourceTypeMCPServerCatalogEntry {
					results = append(results, resource.ID)
				}
			}
			return results, nil
		},
		"server-names": func(obj any) ([]string, error) {
			acr := obj.(*v1.AccessControlRule)
			var results []string
			for _, resource := range acr.Spec.Manifest.Resources {
				if resource.Type == types.ResourceTypeMCPServer {
					results = append(results, resource.ID)
				}
			}
			return results, nil
		},
		"selectors": func(obj any) ([]string, error) {
			acr := obj.(*v1.AccessControlRule)
			var results []string
			for _, resource := range acr.Spec.Manifest.Resources {
				if resource.Type == types.ResourceTypeSelector {
					results = append(results, resource.ID)
				}
			}
			return results, nil
		},
	})

	for _, acr := range acrs {
		if err := indexer.Add(acr); err != nil {
			t.Fatalf("add access control rule to indexer: %v", err)
		}
	}

	return &Authorizer{
		cache:     storage,
		uncached:  storage,
		acrHelper: accesscontrolrule.NewAccessControlRuleHelper(indexer, storage),
	}
}
