package authz

import (
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/principal/principaltest"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/stretchr/testify/require"
	"k8s.io/apiserver/pkg/authentication/user"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func TestVMCPComponentConnectionAuthorization(t *testing.T) {
	vmcp := &v1.VMCP{Name: "vmcp1shared", Namespace: "default", Spec: v1.VMCPSpec{Manifest: types.VMCPManifest{
		Components: []types.VMCPComponent{{ID: "one"}},
		Profiles:   []types.VMCPProfile{{Subjects: []types.Subject{{Type: types.SubjectTypeUser, ID: "7"}}, Permissions: types.VMCPProfilePermissions{AllowAllComponents: true}}},
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
			u := newUser(principaltest.AsUser(&user.DefaultInfo{UID: tc.userID, Groups: tc.groups, Extra: map[string][]string{"authorized_mcp_ids": {tc.scope}}}))
			allowed, err := authorizer.checkMCPID(httptest.NewRequest(http.MethodPost, "/mcp-connect/"+tc.id, nil), &Resources{MCPID: tc.id}, u)
			if tc.allowed {
				require.NoError(t, err)
			}
			require.Equal(t, tc.allowed, allowed)
		})
	}
	u := principaltest.AsUser(&user.DefaultInfo{UID: "7", Groups: []string{types.GroupCompositeMCP}, Extra: map[string][]string{"authorized_mcp_ids": {connection.Name}}})
	vmcp.Spec.Manifest.Profiles = nil
	require.NoError(t, storage.Update(t.Context(), vmcp))
	allowed, err := CheckMCPIDAccess(t.Context(), storage, nil, u, connection.Name)
	require.NoError(t, err)
	require.False(t, allowed)
}

// TestPairComponentAuthorization checks the component hops of an agent acting on behalf of a user: the composite
// loopback of agent 7 acting for user 9 reaches the pair's own connection and component server, and the agent's own
// connection, but never another user's pair, and neither the agent acting for itself nor the user directly reaches
// the pair's.
func TestPairComponentAuthorization(t *testing.T) {
	vmcp := &v1.VMCP{Name: "vmcp1pair", Namespace: "default", Spec: v1.VMCPSpec{Manifest: types.VMCPManifest{
		Components: []types.VMCPComponent{{ID: "shared"}, {ID: "dedicated", ForceSingleUser: true}},
		Profiles: []types.VMCPProfile{
			{ID: "vp1agent", Subjects: []types.Subject{{Type: types.SubjectTypeAgent, ID: "7"}}, Permissions: types.VMCPProfilePermissions{AllowAllComponents: true}},
			{ID: "vp1users", Subjects: []types.Subject{{Type: types.SubjectTypeSelector, ID: types.SubjectWildcardID}}, Permissions: types.VMCPProfilePermissions{AllowAllComponents: true}},
		},
	}}}
	shared := &v1.MCPServer{Name: "ms1shared", Namespace: "default", Spec: v1.MCPServerSpec{VMCPID: vmcp.Name, VMCPComponentID: "shared"}}
	objects := []kclient.Object{vmcp, shared}
	connections, servers := map[string]string{}, map[string]string{}
	for _, owner := range []string{"agent:7", "agent:7/user:9", "agent:7/user:10", "9"} {
		instance := &v1.VMCPInstance{Name: "vmcpi1" + strings.NewReplacer(":", "", "/", "").Replace(owner), Namespace: "default", Spec: v1.VMCPInstanceSpec{UserID: owner, Manifest: types.VMCPInstanceManifest{VMCPID: vmcp.Name}}}
		connection := &v1.MCPServerInstance{Name: "msi1" + instance.Name, Namespace: "default", Spec: v1.MCPServerInstanceSpec{
			UserID: owner, VMCPInstanceID: instance.Name, VMCPComponentID: "shared", MCPServerName: shared.Name,
		}}
		server := &v1.MCPServer{Name: "ms1" + instance.Name, Namespace: "default", Spec: v1.MCPServerSpec{
			UserID: owner, VMCPInstanceID: instance.Name, VMCPComponentID: "dedicated",
		}}
		objects = append(objects, instance, connection, server)
		connections[owner], servers[owner] = connection.Name, server.Name
	}
	storage := newMCPIDIsAuthorizedTestStorage(objects...)
	authorizer := &Authorizer{cache: storage, uncached: storage}

	loopback := func(info user.Info, id string) user.Info {
		extra := maps.Clone(info.GetExtra())
		extra["authorized_mcp_ids"] = []string{id}
		return &user.DefaultInfo{UID: info.GetUID(), Name: info.GetName(), Groups: []string{types.GroupMCP, types.GroupCompositeMCP, types.GroupAuthenticated}, Extra: extra}
	}
	forNine := onBehalfOfPrincipal(t, allAgentScopes)
	itself := agentKeyPrincipal(allAgentScopes)
	userNine := principaltest.AsUser(&user.DefaultInfo{UID: "9"})
	for _, tc := range []struct {
		name    string
		caller  user.Info
		owner   string
		allowed bool
	}{
		{name: "the agent for user 9, to the pair's", caller: forNine, owner: "agent:7/user:9", allowed: true},
		{name: "the agent for user 9, to the agent's own", caller: forNine, owner: "agent:7", allowed: true},
		{name: "the agent for user 9, to another user's pair", caller: forNine, owner: "agent:7/user:10"},
		{name: "the agent for user 9, to the user's own", caller: forNine, owner: "9"},
		{name: "the agent for itself, to the pair's", caller: itself, owner: "agent:7/user:9"},
		{name: "the user, to the pair's", caller: userNine, owner: "agent:7/user:9"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, id := range []string{connections[tc.owner], servers[tc.owner]} {
				u := newUser(loopback(tc.caller, id))
				allowed, err := authorizer.checkMCPID(httptest.NewRequest(http.MethodPost, "/mcp-connect-composite/"+id, nil), &Resources{MCPID: id}, u)
				if tc.allowed {
					require.NoError(t, err)
				}
				require.Equal(t, tc.allowed, allowed, id)
			}
		})
	}
}
