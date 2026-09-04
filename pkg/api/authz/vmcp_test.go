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
		{name: "matching group reads shared", method: http.MethodGet, path: "/api/vmcps/vmcp-shared", userID: "member", groups: []string{"team-a"}, allowed: true},
		{name: "profile does not grant shared update", method: http.MethodPut, path: "/api/vmcps/vmcp-shared", userID: "member", groups: []string{"team-a"}, allowed: false},
		{name: "nonmatching user cannot read shared", method: http.MethodGet, path: "/api/vmcps/vmcp-shared", userID: "outsider", allowed: false},
		{name: "owner manages personal", method: http.MethodPut, path: "/api/vmcps/vmcp-personal", userID: "owner", allowed: true},
		{name: "personal wildcard cannot grant another user", method: http.MethodGet, path: "/api/vmcps/vmcp-personal", userID: "outsider", allowed: false},
		{name: "administrator manages shared", method: http.MethodDelete, path: "/api/vmcps/vmcp-shared", userID: "admin", groups: []string{types.GroupAdmin}, allowed: true},
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
		name, method, instanceID, userID string
		allowed                          bool
	}{
		{name: "owner with current profile access", method: http.MethodGet, instanceID: allowedInstance.Name, userID: "allowed", allowed: true},
		{name: "owner configures with current profile access", method: http.MethodPost, instanceID: allowedInstance.Name, userID: "allowed", allowed: true},
		{name: "owner whose profile access was removed", method: http.MethodGet, instanceID: revokedInstance.Name, userID: "revoked", allowed: false},
		{name: "different user", method: http.MethodGet, instanceID: allowedInstance.Name, userID: "other", allowed: false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			path := "/api/vmcp-instances/" + tt.instanceID
			if tt.method == http.MethodPost {
				path += "/configure"
			}
			req := httptest.NewRequest(tt.method, path, nil)
			got := authorizer.Authorize(req, &user.DefaultInfo{Name: tt.userID, UID: tt.userID, Groups: []string{types.GroupAPI}})
			if got != tt.allowed {
				t.Fatalf("Authorize() = %v, want %v", got, tt.allowed)
			}
		})
	}
}

func newVMCPTestAuthorizer(objects ...kclient.Object) *Authorizer {
	storage := clientfake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(objects...).Build()
	return NewAuthorizer(nil, storage, storage, false, nil, nil, nil, false)
}

func objectMetaForAuthzTest(name string) metav1.ObjectMeta {
	return metav1.ObjectMeta{Name: name, Namespace: system.DefaultNamespace}
}
