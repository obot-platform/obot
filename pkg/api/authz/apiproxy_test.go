package authz

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"k8s.io/apiserver/pkg/authentication/user"
	clientfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestAuthorizeAPIProxyUsesConnectionAndKeyScopes(t *testing.T) {
	vmcp := &v1.VMCP{
		Name:      "vmcp1proxy",
		Namespace: "default",
		Spec: v1.VMCPSpec{Manifest: types.VMCPManifest{
			Components: []types.VMCPComponent{{ID: "component"}},
			Profiles: []types.VMCPProfile{{
				Subjects:    []types.Subject{{Type: types.SubjectTypeUser, ID: "7"}},
				Permissions: types.VMCPProfilePermissions{AllowAllComponents: true},
			}},
		}},
	}
	instance := &v1.VMCPInstance{
		Name:      "vmcpi1proxy",
		Namespace: "default",
		Spec: v1.VMCPInstanceSpec{
			UserID:   "7",
			Manifest: types.VMCPInstanceManifest{VMCPID: vmcp.Name},
		},
	}
	storage := clientfake.NewClientBuilder().
		WithScheme(storagescheme.Scheme).
		WithObjects(vmcp, instance).
		Build()
	authorizer := &Authorizer{uncached: storage}

	tests := []struct {
		name   string
		uid    string
		groups []string
		scopes []string
		want   bool
	}{
		{name: "browser user connection", uid: "7", groups: []string{types.GroupMCP}, want: true},
		{name: "scoped API key", uid: "7", groups: []string{types.GroupMCP}, scopes: []string{vmcp.Name}, want: true},
		{name: "wildcard API key", uid: "7", groups: []string{types.GroupMCP}, scopes: []string{"*"}, want: true},
		{name: "wrong API key scope", uid: "7", groups: []string{types.GroupMCP}, scopes: []string{"vmcp1other"}, want: false},
		{name: "another user instance", uid: "8", groups: []string{types.GroupMCP}, want: false},
		{name: "administrator without MCP group", uid: "7", groups: []string{types.GroupAdmin}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api-proxy/"+vmcp.Name+"/component/items/a%2Fb?keep=1", nil)
			req.Pattern = apiProxyPattern
			info := &user.DefaultInfo{UID: tt.uid, Groups: tt.groups}
			if tt.scopes != nil {
				info.Extra = map[string][]string{"authorized_mcp_ids": tt.scopes}
			}

			allowed := authorizer.authorizeAPIProxy(req, newUser(info))

			if allowed != tt.want {
				t.Fatalf("authorizeAPIProxy() = %v, want %v", allowed, tt.want)
			}
		})
	}
}

func TestAuthorizeAPIProxyRejectsMalformedRoutesBeforeStorage(t *testing.T) {
	authorizer := &Authorizer{}
	req := httptest.NewRequest(http.MethodGet, "/api-proxy/not-vmcp/component/items", nil)
	req.Pattern = apiProxyPattern

	if authorizer.authorizeAPIProxy(req, newUser(&user.DefaultInfo{UID: "7", Groups: []string{types.GroupMCP}})) {
		t.Fatal("non-canonical VMCP was authorized")
	}
}
