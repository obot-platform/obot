package authz

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/stretchr/testify/require"
	"k8s.io/apiserver/pkg/authentication/user"
)

func TestVMCPComponentConnectionAuthorization(t *testing.T) {
	vmcp := &v1.VMCP{Name: "vmcp1shared", Namespace: "default", Spec: v1.VMCPSpec{Manifest: types.VMCPManifest{
		Components: []types.VMCPComponent{{ID: "one"}},
		Profiles:   []types.VMCPProfile{{Subjects: []types.Subject{{Type: types.SubjectTypeUser, ID: "7"}}, AllowAllTools: true}},
	}}}
	parent := &v1.VMCPInstance{Name: "vmcpi1one", Namespace: "default", Spec: v1.VMCPInstanceSpec{UserID: "7", Manifest: types.VMCPInstanceManifest{VMCPID: vmcp.Name}}}
	second := parent.DeepCopy()
	second.Name = "vmcpi1two"
	server := &v1.MCPServer{Name: "ms1shared", Namespace: "default", Spec: v1.MCPServerSpec{VMCPID: vmcp.Name, VMCPComponentID: "one"}}
	connection := &v1.MCPServerInstance{Name: "msi1one", Namespace: "default", Spec: v1.MCPServerInstanceSpec{
		UserID: "7", VMCPInstanceID: parent.Name, VMCPComponentID: "one", MCPServerName: server.Name,
	}}
	other := connection.DeepCopy()
	other.Name = "msi1two"
	other.Spec.VMCPInstanceID = second.Name
	storage := newMCPIDIsAuthorizedTestStorage(vmcp, parent, second, server, connection, other)
	authorizer := &Authorizer{cache: storage, uncached: storage}
	for _, tc := range []struct {
		name    string
		id      string
		userID  string
		groups  []string
		scope   string
		allowed bool
	}{
		{
			name:    "explicit vMCP instance URL",
			id:      parent.Name,
			userID:  "7",
			scope:   vmcp.Name,
			allowed: true,
		},
		{
			name:   "wrong vMCP instance owner",
			id:     parent.Name,
			userID: "8",
			scope:  vmcp.Name,
		},
		{
			name:    "signed component connection",
			id:      connection.Name,
			userID:  "7",
			groups:  []string{types.GroupCompositeMCP},
			scope:   connection.Name,
			allowed: true,
		},
		{
			name:   "same user's other connection",
			id:     other.Name,
			userID: "7",
			groups: []string{types.GroupCompositeMCP},
			scope:  connection.Name,
		},
		{
			name:   "other user",
			id:     connection.Name,
			userID: "8",
			groups: []string{types.GroupCompositeMCP},
			scope:  connection.Name,
		},
		{
			name:   "direct connection bypass",
			id:     connection.Name,
			userID: "7",
			scope:  connection.Name,
		},
		{
			name:   "shared server bypass",
			id:     server.Name,
			userID: "7",
			groups: []string{types.GroupCompositeMCP},
			scope:  server.Name,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			u := newUser(&user.DefaultInfo{UID: tc.userID, Groups: tc.groups, Extra: map[string][]string{"authorized_mcp_ids": {tc.scope}}})
			allowed, err := authorizer.checkMCPID(httptest.NewRequest(http.MethodPost, "/mcp-connect/"+tc.id, nil), &Resources{MCPID: tc.id}, u)
			if tc.allowed {
				require.NoError(t, err)
			}
			require.Equal(t, tc.allowed, allowed)
		})
	}
	u := &user.DefaultInfo{UID: "7", Groups: []string{types.GroupCompositeMCP}, Extra: map[string][]string{"authorized_mcp_ids": {connection.Name}}}
	vmcp.Spec.Manifest.Profiles = nil
	require.NoError(t, storage.Update(t.Context(), vmcp))
	allowed, err := CheckMCPIDAccess(t.Context(), storage, nil, u, connection.Name)
	require.NoError(t, err)
	require.False(t, allowed)
}
