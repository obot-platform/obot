package vmcpinstance

import (
	"context"
	"testing"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/principal"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	vmcpconfig "github.com/obot-platform/obot/pkg/vmcp"
	"github.com/stretchr/testify/require"
	kuser "k8s.io/apiserver/pkg/authentication/user"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

// TestPairInstances checks what the controller does with the instance of what a user set up through an agent, owned
// by the pair of agent 7 and user 9: it is never deleted for lack of a profile, it carries a server for each component
// that may need the user, whatever the profiles grant, owned by the pair, it reads the pair's values, and it never looks
// up the pair as a user or an agent.
func TestPairInstances(t *testing.T) {
	owner := principal.DelegatedOwner{AgentID: 7, UserID: 9}.String()
	component := func(id string, policy types.VMCPConfigurationPolicyType) types.VMCPComponent {
		return types.VMCPComponent{
			ID:              id,
			Name:            id,
			ForceSingleUser: true,
			Configuration:   []types.VMCPConfigurationPolicy{{Key: "TOKEN", Policy: policy}},
			CatalogEntry: types.MCPServerCatalogEntrySnapshot{Manifest: types.MCPServerCatalogEntryManifest{
				Runtime:   types.RuntimeUVX,
				UVXConfig: &types.UVXRuntimeConfig{Package: id},
				Config:    []types.MCPConfig{{Key: "TOKEN", Usage: types.Env, Required: true}},
			}},
		}
	}
	vmcp := &v1.VMCP{
		Name:      "vmcp1pair",
		Namespace: "default",
		Spec: v1.VMCPSpec{Manifest: types.VMCPManifest{
			Components: []types.VMCPComponent{
				component("fixed", types.VMCPConfigurationPolicyFixed),
				component("keyed", types.VMCPConfigurationPolicyUserAllowed),
			},
			// No profile names anyone: the pair's setup stays all the same.
		}},
	}
	instance := &v1.VMCPInstance{
		Name:       "vmcpi1pair",
		Namespace:  "default",
		Finalizers: []string{v1.VMCPInstanceFinalizer},
		Spec:       v1.VMCPInstanceSpec{UserID: owner, Manifest: types.VMCPInstanceManifest{VMCPID: vmcp.Name}},
	}
	client := fake.NewClientBuilder().WithScheme(scheme.Scheme).
		WithObjects(vmcp, instance).
		WithStatusSubresource(&v1.VMCPInstance{}).
		WithIndex(&v1.MCPServer{}, "spec.vmcpInstanceID", func(o kclient.Object) []string { return []string{o.(*v1.MCPServer).Spec.VMCPInstanceID} }).
		WithIndex(&v1.MCPServerInstance{}, "spec.vmcpInstanceID", func(o kclient.Object) []string {
			return []string{o.(*v1.MCPServerInstance).Spec.VMCPInstanceID}
		}).
		WithInterceptorFuncs(interceptor.Funcs{List: func(ctx context.Context, client kclient.WithWatch, list kclient.ObjectList, opts ...kclient.ListOption) error {
			switch list.(type) {
			case *v1.UserGroupChangeList, *v1.UserRoleChangeList, *v1.AgentIdentityChangeList, *v1.AgentIdentityDeleteList:
				t.Errorf("a pair's instance watched %T", list)
			}
			return client.List(ctx, list, opts...)
		}}).
		Build()
	var revealed []string
	handler := &Handler{
		userInfo: func(context.Context, principal.Principal) (kuser.Info, error) {
			t.Error("a pair's instance looked up its owner")
			return nil, nil
		},
		revealCredential: func(_ context.Context, contexts []string, _ string) (gatewaytypes.Credential, error) {
			revealed = append(revealed, contexts...)
			return gatewaytypes.Credential{Secrets: map[string]string{vmcpconfig.ConfigurationKey("keyed", "TOKEN"): "the user's"}}, nil
		},
	}
	request := func() router.Request {
		var current v1.VMCPInstance
		require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(instance), &current))
		return router.Request{Ctx: t.Context(), Client: client, Object: &current, Namespace: "default"}
	}

	require.NoError(t, handler.DeleteUnauthorized(request(), nil))
	require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(instance), &v1.VMCPInstance{}), "a pair's instance is kept without a profile")

	require.NoError(t, handler.EnsureMCPServers(request(), nil))
	var servers v1.MCPServerList
	require.NoError(t, client.List(t.Context(), &servers))
	require.Len(t, servers.Items, 1, "only the component that takes the user's value")
	require.Equal(t, "keyed", servers.Items[0].Spec.VMCPComponentID)
	require.Equal(t, owner, servers.Items[0].Spec.UserID)
	require.Equal(t, vmcpconfig.ComponentServerName(instance.Name, "keyed"), servers.Items[0].Name)

	require.NoError(t, handler.SyncUserConfigurationHash(request(), nil))
	require.Equal(t, []string{vmcpconfig.InstanceConfigurationCredentialContext(instance.Name)}, revealed)
	synced := request().Object.(*v1.VMCPInstance)
	require.True(t, synced.Status.Configured)
	require.Empty(t, synced.Status.MissingRequiredConfiguration)
	require.Equal(t, vmcpconfig.ConfigurationCheckHash(vmcpconfig.PairInstanceComponents(vmcpconfig.ComponentsForInstance(*vmcp, *synced)), ""), synced.Status.ConfigurationCheckHash)

	require.NoError(t, handler.ReconcileToolSelection(request(), nil))
}
