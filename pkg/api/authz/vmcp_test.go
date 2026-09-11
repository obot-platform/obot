package authz

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apiserver/pkg/authentication/user"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	clientfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestEmptyVMCPIsReadableButNotConnectable(t *testing.T) {
	u := &user.DefaultInfo{UID: "owner"}
	vmcp := &v1.VMCP{
		Name:      "vmcp1empty",
		Namespace: system.DefaultNamespace,
		Spec: v1.VMCPSpec{
			UserID:   u.UID,
			Manifest: types.VMCPManifest{DisplayName: "Empty draft"},
		},
	}
	vmcp.Spec.Manifest.Default(true, u.UID)
	if err := vmcp.Spec.Manifest.Validate(); err != nil {
		t.Fatalf("empty draft should be valid: %v", err)
	}
	if !UserCanReadVMCP(u, vmcp) || !UserCanManageVMCP(u, vmcp) {
		t.Fatal("owner must be able to read and edit the empty draft")
	}
	storage := clientfake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(vmcp).Build()
	if allowed, err := CheckMCPIDAccess(t.Context(), storage, nil, u, vmcp.Name); err != nil || allowed {
		t.Fatalf("empty draft connection: allowed=%v, err=%v", allowed, err)
	}
	vmcp.Spec.Manifest.Components = []types.VMCPComponent{{ID: "component"}}
	if !UserCanConnectVMCP(u, vmcp) {
		t.Fatal("owner should connect after adding a component")
	}
}

func TestValidateComponentWildcardSelection(t *testing.T) {
	u := &user.DefaultInfo{UID: "1"}
	vmcp := &v1.VMCP{Spec: v1.VMCPSpec{Manifest: types.VMCPManifest{
		Components: []types.VMCPComponent{
			{ID: "gmail", ToolOverrides: []types.ToolOverride{{Name: "read", Enabled: true}, {Name: "delete", Enabled: false}}},
			{ID: "everything"},
		},
		Profiles: []types.VMCPProfile{{
			Subjects:     []types.Subject{{Type: types.SubjectTypeUser, ID: "1"}},
			AllowedTools: types.VMCPToolSet{"gmail": {"*"}, "everything": {"echo"}},
		}},
	}}}
	for _, tt := range []struct {
		name      string
		selection types.VMCPToolSet
		valid     bool
	}{
		{
			name:      "concrete subset of wildcard",
			selection: types.VMCPToolSet{"gmail": {"read"}},
			valid:     true,
		},
		{
			name:      "wildcard subset of wildcard",
			selection: types.VMCPToolSet{"gmail": {"*"}},
			valid:     true,
		},
		{
			name:      "disabled tool rejected",
			selection: types.VMCPToolSet{"gmail": {"delete"}},
		},
		{
			name:      "wildcard cannot widen explicit grant",
			selection: types.VMCPToolSet{"everything": {"*"}},
		},
		{
			name:      "wildcard cannot cross components",
			selection: types.VMCPToolSet{"everything": {"read"}},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidateVMCPToolSelection(u, vmcp, tt.selection); (err == nil) != tt.valid {
				t.Fatalf("validation error = %v, valid = %v", err, tt.valid)
			}
		})
	}
}

func TestVMCPAuthorization(t *testing.T) {
	shared := &v1.VMCP{
		ObjectMeta: objectMetaForAuthzTest("vmcp-shared"),
		Spec: v1.VMCPSpec{Manifest: types.VMCPManifest{Profiles: []types.VMCPProfile{{
			Name:     "team",
			Subjects: []types.Subject{{Type: types.SubjectTypeGroup, ID: "team-a"}},
		}}}},
	}
	personal := &v1.VMCP{
		ObjectMeta: objectMetaForAuthzTest("vmcp-personal"),
		Spec: v1.VMCPSpec{
			UserID: "owner",
			Manifest: types.VMCPManifest{Profiles: []types.VMCPProfile{{
				Name:     "ignored-wildcard",
				Subjects: []types.Subject{{Type: types.SubjectTypeSelector, ID: "*"}},
			}}},
		},
	}
	authorizer := newVMCPTestAuthorizer(shared, personal)

	tests := []struct {
		name    string
		method  string
		path    string
		userID  string
		groups  []string
		allowed bool
	}{
		{
			name:    "administrator upgrades shared",
			method:  http.MethodPost,
			path:    "/api/vmcps/vmcp-shared/trigger-update",
			userID:  "admin",
			groups:  []string{types.GroupAdmin},
			allowed: true,
		},
		{
			name:   "profile cannot upgrade shared",
			method: http.MethodPost,
			path:   "/api/vmcps/vmcp-shared/trigger-update",
			userID: "member",
			groups: []string{"team-a", types.GroupPowerUserPlus},
		},
		{
			name:    "owner upgrades personal",
			method:  http.MethodPost,
			path:    "/api/vmcps/vmcp-personal/trigger-update",
			userID:  "owner",
			allowed: true,
		},
		{
			name:    "matching group reads shared",
			method:  http.MethodGet,
			path:    "/api/vmcps/vmcp-shared",
			userID:  "member",
			groups:  []string{"team-a"},
			allowed: true,
		},
		{
			name:    "profile does not grant shared update",
			method:  http.MethodPut,
			path:    "/api/vmcps/vmcp-shared",
			userID:  "member",
			groups:  []string{"team-a"},
			allowed: false,
		},
		{
			name:    "nonmatching user cannot read shared",
			method:  http.MethodGet,
			path:    "/api/vmcps/vmcp-shared",
			userID:  "outsider",
			allowed: false,
		},
		{
			name:    "owner manages personal",
			method:  http.MethodPut,
			path:    "/api/vmcps/vmcp-personal",
			userID:  "owner",
			allowed: true,
		},
		{
			name:    "personal wildcard cannot grant another user",
			method:  http.MethodGet,
			path:    "/api/vmcps/vmcp-personal",
			userID:  "outsider",
			allowed: false,
		},
		{
			name:    "administrator manages shared",
			method:  http.MethodDelete,
			path:    "/api/vmcps/vmcp-shared",
			userID:  "admin",
			groups:  []string{types.GroupAdmin},
			allowed: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			groups := append([]string{types.GroupAPI}, tt.groups...)
			req := httptest.NewRequest(tt.method, tt.path, nil)
			got := authorizer.Authorize(req, &user.DefaultInfo{Name: tt.userID, UID: tt.userID, Groups: groups})
			if got != tt.allowed {
				t.Fatalf("Authorize() = %v, want %v", got, tt.allowed)
			}
		})
	}
}

func TestUserCanReadVMCP(t *testing.T) {
	sharedWithoutProfiles := &v1.VMCP{}
	sharedWithDirectProfile := &v1.VMCP{
		Spec: v1.VMCPSpec{
			Manifest: types.VMCPManifest{
				Profiles: []types.VMCPProfile{{
					Name: "direct",
					Subjects: []types.Subject{{
						Type: types.SubjectTypeUser,
						ID:   "admin",
					}},
				}},
			},
		},
	}
	sharedWithGroupProfile := &v1.VMCP{
		Spec: v1.VMCPSpec{
			Manifest: types.VMCPManifest{
				Profiles: []types.VMCPProfile{{
					Name: "group",
					Subjects: []types.Subject{{
						Type: types.SubjectTypeGroup,
						ID:   "team-a",
					}},
				}},
			},
		},
	}
	sharedWithWildcardProfile := &v1.VMCP{
		Spec: v1.VMCPSpec{
			Manifest: types.VMCPManifest{
				Profiles: []types.VMCPProfile{{
					Name: "wildcard",
					Subjects: []types.Subject{{
						Type: types.SubjectTypeSelector,
						ID:   "*",
					}},
				}},
			},
		},
	}
	personal := &v1.VMCP{
		Spec: v1.VMCPSpec{
			UserID: "personal-owner",
			Manifest: types.VMCPManifest{
				Profiles: []types.VMCPProfile{{
					Name: "ignored-wildcard",
					Subjects: []types.Subject{{
						Type: types.SubjectTypeSelector,
						ID:   "*",
					}},
				}},
			},
		},
	}

	tests := []struct {
		name string
		user user.Info
		vmcp *v1.VMCP
		want bool
	}{
		{
			name: "administrator cannot read shared VMCP without profile",
			user: &user.DefaultInfo{
				UID:    "admin",
				Groups: []string{types.GroupAdmin},
			},
			vmcp: sharedWithoutProfiles,
			want: false,
		},
		{
			name: "owner cannot read shared VMCP without profile",
			user: &user.DefaultInfo{
				UID:    "owner-role",
				Groups: []string{types.GroupOwner},
			},
			vmcp: sharedWithoutProfiles,
			want: false,
		},
		{
			name: "direct profile grants shared VMCP",
			user: &user.DefaultInfo{
				UID:    "admin",
				Groups: []string{types.GroupAdmin},
			},
			vmcp: sharedWithDirectProfile,
			want: true,
		},
		{
			name: "group profile grants shared VMCP",
			user: &user.DefaultInfo{
				UID:    "owner-role",
				Groups: []string{types.GroupOwner, "team-a"},
			},
			vmcp: sharedWithGroupProfile,
			want: true,
		},
		{
			name: "Obot group extra grants shared VMCP",
			user: &user.DefaultInfo{
				UID: "obot-group-user",
				Extra: map[string][]string{
					"obot_groups": {"team-a"},
				},
			},
			vmcp: sharedWithGroupProfile,
			want: true,
		},
		{
			name: "auth provider group extra grants shared VMCP",
			user: &user.DefaultInfo{
				UID: "provider-group-user",
				Extra: map[string][]string{
					"auth_provider_groups": {"team-a"},
				},
			},
			vmcp: sharedWithGroupProfile,
			want: true,
		},
		{
			name: "wildcard profile grants shared VMCP",
			user: &user.DefaultInfo{
				UID: "wildcard-user",
			},
			vmcp: sharedWithWildcardProfile,
			want: true,
		},
		{
			name: "personal VMCP owner can read",
			user: &user.DefaultInfo{
				UID: "personal-owner",
			},
			vmcp: personal,
			want: true,
		},
		{
			name: "administrator cannot read another user's personal VMCP",
			user: &user.DefaultInfo{
				UID:    "admin",
				Groups: []string{types.GroupAdmin},
			},
			vmcp: personal,
			want: false,
		},
		{
			name: "owner cannot read another user's personal VMCP",
			user: &user.DefaultInfo{
				UID:    "owner-role",
				Groups: []string{types.GroupOwner},
			},
			vmcp: personal,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := UserCanReadVMCP(tt.user, tt.vmcp); got != tt.want {
				t.Fatalf("UserCanReadVMCP() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestVMCPInstanceAuthorizationRequiresCurrentVMCPAccess(t *testing.T) {
	shared := &v1.VMCP{
		ObjectMeta: objectMetaForAuthzTest("vmcp-shared"),
		Spec: v1.VMCPSpec{Manifest: types.VMCPManifest{Profiles: []types.VMCPProfile{{
			Name:     "allowed-user",
			Subjects: []types.Subject{{Type: types.SubjectTypeUser, ID: "allowed"}},
		}}}},
	}
	allowedInstance := &v1.VMCPInstance{
		ObjectMeta: objectMetaForAuthzTest("vmcpi-allowed"),
		Spec: v1.VMCPInstanceSpec{
			UserID:   "allowed",
			Manifest: types.VMCPInstanceManifest{VMCPID: shared.Name},
		},
	}
	revokedInstance := &v1.VMCPInstance{
		ObjectMeta: objectMetaForAuthzTest("vmcpi-revoked"),
		Spec: v1.VMCPInstanceSpec{
			UserID:   "revoked",
			Manifest: types.VMCPInstanceManifest{VMCPID: shared.Name},
		},
	}
	authorizer := newVMCPTestAuthorizer(shared, allowedInstance, revokedInstance)

	for _, tt := range []struct {
		name       string
		method     string
		suffix     string
		instanceID string
		userID     string
		allowed    bool
	}{
		{
			name:       "owner with current profile access",
			method:     http.MethodGet,
			instanceID: allowedInstance.Name,
			userID:     "allowed",
			allowed:    true,
		},
		{
			name:       "owner configures with current profile access",
			method:     http.MethodPost,
			suffix:     "/configure",
			instanceID: allowedInstance.Name,
			userID:     "allowed",
			allowed:    true,
		},
		{
			name:       "owner reveals with current profile access",
			method:     http.MethodPost,
			suffix:     "/reveal",
			instanceID: allowedInstance.Name,
			userID:     "allowed",
			allowed:    true,
		},
		{
			name:       "owner deconfigures with current profile access",
			method:     http.MethodPost,
			suffix:     "/deconfigure",
			instanceID: allowedInstance.Name,
			userID:     "allowed",
			allowed:    true,
		},
		{
			name:       "owner whose profile access was removed",
			method:     http.MethodGet,
			instanceID: revokedInstance.Name,
			userID:     "revoked",
			allowed:    false,
		},
		{
			name:       "different user",
			method:     http.MethodGet,
			instanceID: allowedInstance.Name,
			userID:     "other",
			allowed:    false,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			path := "/api/vmcp-instances/" + tt.instanceID + tt.suffix
			req := httptest.NewRequest(tt.method, path, nil)
			got := authorizer.Authorize(req, &user.DefaultInfo{Name: tt.userID, UID: tt.userID, Groups: []string{types.GroupAPI}})
			if got != tt.allowed {
				t.Fatalf("Authorize() = %v, want %v", got, tt.allowed)
			}
		})
	}
}

func TestVMCPActionRouteAuthorization(t *testing.T) {
	shared := &v1.VMCP{
		ObjectMeta: objectMetaForAuthzTest(system.VMCPPrefix + "shared-route"),
		Spec: v1.VMCPSpec{Manifest: types.VMCPManifest{Profiles: []types.VMCPProfile{{
			Name:     "allowed-user",
			Subjects: []types.Subject{{Type: types.SubjectTypeUser, ID: "allowed"}},
		}}}},
	}
	personal := &v1.VMCP{
		ObjectMeta: objectMetaForAuthzTest(system.VMCPPrefix + "personal-route"),
		Spec:       v1.VMCPSpec{UserID: "owner"},
	}
	authorizer := newVMCPTestAuthorizer(shared, personal)

	tests := []struct {
		name    string
		method  string
		path    string
		userID  string
		allowed bool
	}{
		{
			name:    "profile-matched shared VMCP is readable through VMCP route",
			method:  http.MethodGet,
			path:    "/api/vmcps/" + shared.Name,
			userID:  "allowed",
			allowed: true,
		},
		{
			name:   "profile-unmatched shared VMCP is denied through VMCP route",
			method: http.MethodGet,
			path:   "/api/vmcps/" + shared.Name,
			userID: "other",
		},
		{
			name:    "profile-matched shared VMCP can launch through VMCP route",
			method:  http.MethodPost,
			path:    "/api/vmcps/" + shared.Name + "/launch",
			userID:  "allowed",
			allowed: true,
		},
		{
			name:   "profile-unmatched shared VMCP cannot launch through VMCP route",
			method: http.MethodPost,
			path:   "/api/vmcps/" + shared.Name + "/launch",
			userID: "other",
		},
		{
			name:    "profile-matched shared VMCP can check OAuth through VMCP route",
			method:  http.MethodPost,
			path:    "/api/vmcps/" + shared.Name + "/check-oauth",
			userID:  "allowed",
			allowed: true,
		},
		{
			name:   "profile-unmatched shared VMCP cannot check OAuth through VMCP route",
			method: http.MethodPost,
			path:   "/api/vmcps/" + shared.Name + "/check-oauth",
			userID: "other",
		},
		{
			name:    "profile-matched shared VMCP can delete OAuth through VMCP route",
			method:  http.MethodDelete,
			path:    "/api/vmcps/" + shared.Name + "/oauth",
			userID:  "allowed",
			allowed: true,
		},
		{
			name:   "profile-unmatched shared VMCP cannot delete OAuth through VMCP route",
			method: http.MethodDelete,
			path:   "/api/vmcps/" + shared.Name + "/oauth",
			userID: "other",
		},
		{
			name:    "personal VMCP owner is readable through VMCP route",
			method:  http.MethodGet,
			path:    "/api/vmcps/" + personal.Name,
			userID:  "owner",
			allowed: true,
		},
		{
			name:    "personal VMCP owner can reveal configuration",
			method:  http.MethodPost,
			path:    "/api/vmcps/" + personal.Name + "/reveal",
			userID:  "owner",
			allowed: true,
		},
		{
			name:    "personal VMCP owner can deconfigure",
			method:  http.MethodPost,
			path:    "/api/vmcps/" + personal.Name + "/deconfigure",
			userID:  "owner",
			allowed: true,
		},
		{
			name:   "personal VMCP non-owner is denied through VMCP route",
			method: http.MethodGet,
			path:   "/api/vmcps/" + personal.Name,
			userID: "other",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			if got := authorizer.Authorize(req, &user.DefaultInfo{
				Name:   tt.userID,
				UID:    tt.userID,
				Groups: []string{types.GroupAPI},
			}); got != tt.allowed {
				t.Fatalf("Authorize() = %v, want %v", got, tt.allowed)
			}
		})
	}

	for _, suffix := range []string{"/reveal", "/deconfigure"} {
		req := httptest.NewRequest(http.MethodPost, "/api/vmcps/"+shared.Name+suffix, nil)
		if !authorizer.Authorize(req, &user.DefaultInfo{
			Name:   "admin",
			UID:    "admin",
			Groups: []string{types.GroupAPI, types.GroupAdmin},
		}) {
			t.Fatalf("administrator cannot access %s", suffix)
		}
	}
}

func TestVMCPComponentToolPreviewAuthorization(t *testing.T) {
	shared := &v1.VMCP{
		ObjectMeta: objectMetaForAuthzTest("vmcp-shared-tools"),
		Spec: v1.VMCPSpec{Manifest: types.VMCPManifest{Profiles: []types.VMCPProfile{{
			Name:     "allowed-user",
			Subjects: []types.Subject{{Type: types.SubjectTypeUser, ID: "consumer"}},
		}}}},
	}
	personal := &v1.VMCP{
		ObjectMeta: objectMetaForAuthzTest("vmcp-personal-tools"),
		Spec:       v1.VMCPSpec{UserID: "owner"},
	}
	authorizer := newVMCPTestAuthorizer(shared, personal)

	endpoints := []string{
		"/api/vmcps/" + shared.Name + "/components/component-a/generate-tool-previews",
		"/api/vmcps/" + shared.Name + "/components/component-a/generate-tool-previews/oauth-url",
	}
	for _, endpoint := range endpoints {
		t.Run(endpoint+" admin manages shared VMCP", func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, endpoint, nil)
			if got := authorizer.Authorize(req, &user.DefaultInfo{
				Name:   "admin",
				UID:    "admin",
				Groups: []string{types.GroupAPI, types.GroupAdmin},
			}); !got {
				t.Fatal("Authorize() denied an administrator for a shared VMCP")
			}
		})

		t.Run(endpoint+" profile consumer cannot manage shared VMCP", func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, endpoint, nil)
			if got := authorizer.Authorize(req, &user.DefaultInfo{
				Name:   "consumer",
				UID:    "consumer",
				Groups: []string{types.GroupAPI},
			}); got {
				t.Fatal("Authorize() allowed a profile consumer to manage a shared VMCP")
			}
		})

		t.Run(endpoint+" Power User Plus cannot manage shared VMCP", func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, endpoint, nil)
			if got := authorizer.Authorize(req, &user.DefaultInfo{
				Name:   "power-user-plus",
				UID:    "power-user-plus",
				Groups: []string{types.GroupAPI, types.GroupPowerUserPlus},
			}); got {
				t.Fatal("Authorize() allowed Power User Plus to manage a shared VMCP")
			}
		})

		personalEndpoint := "/api/vmcps/" + personal.Name + "/components/component-a" + endpoint[len("/api/vmcps/"+shared.Name+"/components/component-a"):]
		t.Run(personalEndpoint+" owner manages personal VMCP", func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, personalEndpoint, nil)
			if got := authorizer.Authorize(req, &user.DefaultInfo{
				Name:   "owner",
				UID:    "owner",
				Groups: []string{types.GroupAPI},
			}); !got {
				t.Fatal("Authorize() denied the owner of a personal VMCP")
			}
		})

		t.Run(personalEndpoint+" other user cannot manage personal VMCP", func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, personalEndpoint, nil)
			if got := authorizer.Authorize(req, &user.DefaultInfo{
				Name:   "other",
				UID:    "other",
				Groups: []string{types.GroupAPI},
			}); got {
				t.Fatal("Authorize() allowed another user to manage a personal VMCP")
			}
		})
	}
}

func TestVMCPActionsDoNotUseMCPServerRoutes(t *testing.T) {
	vmcp := &v1.VMCP{
		ObjectMeta: objectMetaForAuthzTest("vmcp1actions"),
		Spec: v1.VMCPSpec{Manifest: types.VMCPManifest{Profiles: []types.VMCPProfile{{
			Subjects:      []types.Subject{{Type: types.SubjectTypeUser, ID: "consumer"}},
			AllowAllTools: true,
		}}}},
	}
	authorizer := newVMCPTestAuthorizer(vmcp)
	u := &user.DefaultInfo{UID: "consumer", Groups: []string{types.GroupAPI}}
	for _, tc := range []struct {
		method string
		suffix string
	}{
		{
			method: http.MethodGet,
			suffix: "/tools",
		},
		{
			method: http.MethodGet,
			suffix: "/resources",
		},
		{
			method: http.MethodGet,
			suffix: "/prompts",
		},
		{
			method: http.MethodGet,
			suffix: "/oauth-url",
		},
		{
			method: http.MethodPost,
			suffix: "/launch",
		},
		{
			method: http.MethodPost,
			suffix: "/check-oauth",
		},
		{
			method: http.MethodDelete,
			suffix: "/oauth",
		},
	} {
		t.Run(tc.method+tc.suffix, func(t *testing.T) {
			if !authorizer.Authorize(httptest.NewRequest(tc.method, "/api/vmcps/"+vmcp.Name+tc.suffix, nil), u) {
				t.Fatal("profile member cannot use dedicated vMCP action")
			}
			if authorizer.Authorize(httptest.NewRequest(tc.method, "/api/mcp-servers/"+vmcp.Name+tc.suffix, nil), u) {
				t.Fatal("vMCP unexpectedly accepted by MCPServer route")
			}
		})
	}
	if authorizer.Authorize(httptest.NewRequest(http.MethodPost, "/api/vmcps/"+vmcp.Name+"/trigger-update", nil), u) {
		t.Fatal("consumption permissions allowed vMCP management")
	}
}

func TestVMCPComponentOAuthAuthorizationChecksParentConnection(t *testing.T) {
	newAuthorizer := func(objects ...kclient.Object) *Authorizer {
		storage := clientfake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(objects...).
			WithIndex(&v1.VMCPInstance{}, "spec.userID", func(obj kclient.Object) []string {
				return []string{obj.(*v1.VMCPInstance).Spec.UserID}
			}).
			WithIndex(&v1.VMCPInstance{}, "spec.manifest.vmcpID", func(obj kclient.Object) []string {
				return []string{obj.(*v1.VMCPInstance).Spec.Manifest.VMCPID}
			}).Build()
		return NewAuthorizer(nil, storage, storage, false, nil, nil, nil, false)
	}
	newObjects := func(singleUser bool) (*v1.VMCP, *v1.VMCPInstance, *v1.MCPServer) {
		vmcp := &v1.VMCP{
			ObjectMeta: objectMetaForAuthzTest("vmcp1oauth"),
			Spec: v1.VMCPSpec{Manifest: types.VMCPManifest{
				Components: []types.VMCPComponent{{ID: "component", ForceSingleUser: singleUser}},
				Profiles: []types.VMCPProfile{{
					Subjects:      []types.Subject{{Type: types.SubjectTypeUser, ID: "consumer"}},
					AllowAllTools: true,
				}},
			}},
		}
		instance := &v1.VMCPInstance{
			ObjectMeta: objectMetaForAuthzTest("vmcpi1oauth"),
			Spec: v1.VMCPInstanceSpec{
				UserID: "consumer",
				Manifest: types.VMCPInstanceManifest{
					VMCPID: vmcp.Name,
				},
			},
		}
		component := &v1.MCPServer{
			ObjectMeta: objectMetaForAuthzTest("ms1component"),
			Spec: v1.MCPServerSpec{
				VMCPID:          vmcp.Name,
				VMCPComponentID: "component",
			},
		}
		if singleUser {
			component.Spec.UserID = "consumer"
			component.Spec.VMCPID = ""
			component.Spec.VMCPInstanceID = instance.Name
		}
		return vmcp, instance, component
	}
	authorized := func(authorizer *Authorizer, parentID, componentID string) bool {
		path := "/api/oauth/vmcp/" + parentID + "/components/" + componentID
		return authorizer.Authorize(httptest.NewRequest(http.MethodGet, path, nil), &user.DefaultInfo{
			Name:   "consumer",
			UID:    "consumer",
			Groups: []string{types.GroupAPI},
		})
	}

	for _, singleUser := range []bool{false, true} {
		vmcp, instance, component := newObjects(singleUser)
		vmcp.Spec.Manifest.Components = append(vmcp.Spec.Manifest.Components, types.VMCPComponent{
			ID:              "other",
			ForceSingleUser: !singleUser,
		})
		parentID := vmcp.Name
		if singleUser {
			parentID = instance.Name
		}
		if !authorized(newAuthorizer(vmcp, instance, component), parentID, component.Name) {
			t.Fatalf("valid component denied for singleUser=%v", singleUser)
		}
	}

	vmcp, instance, component := newObjects(true)
	component.Spec.VMCPInstanceID = "vmcpi1other"
	if authorized(newAuthorizer(vmcp, instance, component), instance.Name, component.Name) {
		t.Fatal("component from another vMCP connection was authorized")
	}

	vmcp, instance, component = newObjects(false)
	component.Spec.VMCPID = "vmcp1other"
	if authorized(newAuthorizer(vmcp, instance, component), vmcp.Name, component.Name) {
		t.Fatal("component from another shared vMCP was authorized")
	}

	vmcp, instance, component = newObjects(true)
	instance.Spec.LegacyDisabledComponents = []string{"component"}
	if authorized(newAuthorizer(vmcp, instance, component), instance.Name, component.Name) {
		t.Fatal("disabled component was authorized")
	}
}

func newVMCPTestAuthorizer(objects ...kclient.Object) *Authorizer {
	storage := clientfake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(objects...).Build()
	return NewAuthorizer(nil, storage, storage, false, nil, nil, nil, false)
}

func objectMetaForAuthzTest(name string) metav1.ObjectMeta {
	return metav1.ObjectMeta{Name: name, Namespace: system.DefaultNamespace}
}
