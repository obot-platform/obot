package authz

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/stretchr/testify/require"
	"k8s.io/apiserver/pkg/authentication/user"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type mcpConnectionCountingClient struct {
	kclient.Client
	vmcpLookups int
}

func (c *mcpConnectionCountingClient) Get(ctx context.Context, key kclient.ObjectKey, obj kclient.Object, opts ...kclient.GetOption) error {
	if _, ok := obj.(*v1.VMCP); ok {
		c.vmcpLookups++
	}
	return c.Client.Get(ctx, key, obj, opts...)
}

func (c *mcpConnectionCountingClient) List(ctx context.Context, obj kclient.ObjectList, opts ...kclient.ListOption) error {
	if _, ok := obj.(*v1.VMCPList); ok {
		c.vmcpLookups++
	}
	return c.Client.List(ctx, obj, opts...)
}

func TestMCPConnectResolvesVMCPOnce(t *testing.T) {
	vmcp := &v1.VMCP{
		Name:      "vmcp1personal",
		Namespace: "default",
		Spec: v1.VMCPSpec{
			UserID:     "7",
			LegacySlug: "default-entry",
			Manifest: types.VMCPManifest{
				Components: []types.VMCPComponent{
					{
						ID: "component",
					},
				},
			},
		},
	}
	instance := &v1.VMCPInstance{
		Name:      "vmcpi1personal",
		Namespace: "default",
		Spec: v1.VMCPInstanceSpec{
			UserID:     "7",
			LegacySlug: "ms1personal",
			Manifest: types.VMCPInstanceManifest{
				VMCPID: vmcp.Name,
			},
		},
	}
	for _, id := range []string{vmcp.Name, vmcp.Spec.LegacySlug, instance.Name, instance.Spec.LegacySlug} {
		t.Run(id, func(t *testing.T) {
			client := &mcpConnectionCountingClient{Client: newMCPIDIsAuthorizedTestStorage(vmcp, instance)}
			authorizer := &Authorizer{cache: client, uncached: client}
			caller := newUser(&user.DefaultInfo{
				UID:   "7",
				Extra: map[string][]string{"authorized_mcp_ids": {vmcp.Name}},
			})
			allowed, err := authorizer.checkMCPID(httptest.NewRequest(http.MethodPost, "/mcp-connect/"+id, nil), &Resources{MCPID: id}, caller)
			require.NoError(t, err)
			require.True(t, allowed)
			require.Equal(t, 1, client.vmcpLookups, "access and scope checks must share the resolved vMCP")
		})
	}
}

func TestMCPConnectRejectsStandaloneResources(t *testing.T) {
	server := &v1.MCPServer{Name: "ms1standalone", Namespace: "default", Spec: v1.MCPServerSpec{UserID: "7"}}
	instance := &v1.MCPServerInstance{Name: "msi1standalone", Namespace: "default", Spec: v1.MCPServerInstanceSpec{UserID: "7", MCPServerName: server.Name}}
	entry := &v1.MCPServerCatalogEntry{Name: "mcp1template", Namespace: "default"}
	storage := newMCPIDIsAuthorizedTestStorage(server, instance, entry)
	authorizer := &Authorizer{cache: storage, uncached: storage}
	for _, id := range []string{server.Name, instance.Name, entry.Name} {
		for _, groups := range [][]string{{types.GroupAuthenticated}, {types.GroupAdmin}, {types.GroupCompositeMCP}} {
			for _, scope := range []string{id, "*"} {
				for _, prefix := range []string{"/mcp-connect/", "/mcp-connect-composite/"} {
					t.Run(prefix+id+scope+groups[0], func(t *testing.T) {
						u := newUser(&user.DefaultInfo{UID: "7", Groups: groups, Extra: map[string][]string{"authorized_mcp_ids": {scope}}})
						allowed, err := authorizer.checkMCPID(httptest.NewRequest(http.MethodPost, prefix+id, nil), &Resources{MCPID: id}, u)
						require.NoError(t, err)
						require.False(t, allowed)
					})
				}
			}
		}
	}
}

func TestMCPConnectVMCPSubpaths(t *testing.T) {
	vmcp := &v1.VMCP{
		Name:      "vmcp1personal",
		Namespace: "default",
		Spec: v1.VMCPSpec{
			UserID:   "7",
			Manifest: types.VMCPManifest{Components: []types.VMCPComponent{{ID: "component"}}},
		},
	}
	instance := &v1.VMCPInstance{
		Name:      "vmcpi1personal",
		Namespace: "default",
		Spec: v1.VMCPInstanceSpec{
			UserID:   "7",
			Manifest: types.VMCPInstanceManifest{VMCPID: vmcp.Name},
		},
	}
	storage := newMCPIDIsAuthorizedTestStorage(vmcp, instance)
	authorizer := NewAuthorizer(nil, storage, storage, false, nil, nil, nil, false)
	for _, id := range []string{vmcp.Name, instance.Name} {
		for _, suffix := range []string{"", "/", "/messages/123"} {
			for _, uid := range []string{"7", "8"} {
				t.Run(id+suffix+uid, func(t *testing.T) {
					req := httptest.NewRequest(http.MethodPost, "/mcp-connect/"+id+suffix, nil)
					caller := &user.DefaultInfo{UID: uid, Groups: []string{types.GroupMCP, types.GroupAuthenticated}}
					require.Equal(t, uid == "7", authorizer.Authorize(req, caller))
				})
			}
		}
	}
}
