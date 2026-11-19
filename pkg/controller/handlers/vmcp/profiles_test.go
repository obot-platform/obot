package vmcp

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaydb "github.com/obot-platform/obot/pkg/gateway/db"
	"github.com/obot-platform/obot/pkg/principal"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	sservices "github.com/obot-platform/obot/pkg/storage/services"
	vmcpconfig "github.com/obot-platform/obot/pkg/vmcp"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
	"gorm.io/gorm"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// TestReconcileProfiles checks that a vMCP stored before profiles had IDs gets the IDs catalog sync would derive, once,
// and that nothing else about its profiles is recorded: profiles hold no data of their own.
func TestReconcileProfiles(t *testing.T) {
	everyone := types.Subject{Type: types.SubjectTypeSelector, ID: types.SubjectWildcardID}
	vmcp := &v1.VMCP{Name: "vmcp1test", Namespace: "default", Spec: v1.VMCPSpec{Manifest: types.VMCPManifest{Profiles: []types.VMCPProfile{
		{Name: "a", Subjects: []types.Subject{everyone}},
		{Name: "b", Subjects: []types.Subject{everyone}},
	}}}}
	client := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithStatusSubresource(vmcp).WithObjects(vmcp).Build()
	handler := &Handler{}
	reconcile := func() *v1.VMCP {
		t.Helper()
		current := &v1.VMCP{}
		require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(vmcp), current))
		require.NoError(t, handler.ReconcileProfiles(router.Request{Ctx: t.Context(), Client: client, Object: current}, nil))
		require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(vmcp), current))
		return current
	}

	current := reconcile()
	a, b := vmcpconfig.DerivedProfileID(vmcp.Name, "a"), vmcpconfig.DerivedProfileID(vmcp.Name, "b")
	require.Equal(t, a, current.Spec.Manifest.Profiles[0].ID)
	require.Equal(t, b, current.Spec.Manifest.Profiles[1].ID)

	// The IDs are kept, and nothing is written again.
	version := current.ResourceVersion
	current = reconcile()
	require.Equal(t, version, current.ResourceVersion)
	require.Equal(t, a, current.Spec.Manifest.Profiles[0].ID)
}

// TestAgentOAuthGrantsAreDeletedWithTheirComponentAndVMCP checks, with the gateway database, that a vMCP's agent OAuth
// grant is deleted, with its record in status, when its component leaves the vMCP, and every grant of the vMCP when
// the vMCP is deleted, each audited as the system's; that changing or removing the profiles that grant a component
// deletes no grant; and that no other vMCP's grant, and no user's or agent's own token, is ever deleted with them.
func TestAgentOAuthGrantsAreDeletedWithTheirComponentAndVMCP(t *testing.T) {
	remote := func(id string) types.VMCPComponent {
		return types.VMCPComponent{ID: id, Name: id, CatalogEntry: types.MCPServerCatalogEntrySnapshot{Manifest: types.MCPServerCatalogEntryManifest{
			Runtime:      types.RuntimeRemote,
			RemoteConfig: &types.RemoteCatalogConfig{FixedURL: "https://" + id + ".example.com/mcp"},
		}}}
	}
	newVMCP := func(name string) *v1.VMCP {
		return &v1.VMCP{Name: name, Namespace: "default", Spec: v1.VMCPSpec{Manifest: types.VMCPManifest{
			Components: []types.VMCPComponent{remote("static"), remote("dynamic")},
			Profiles: []types.VMCPProfile{
				{ID: "vp1builders", Name: "builders", Subjects: []types.Subject{{Type: types.SubjectTypeAgent, ID: "7"}}, Permissions: types.VMCPProfilePermissions{AllowAllComponents: true}},
				{ID: "vp1reviewers", Name: "reviewers", Subjects: []types.Subject{{Type: types.SubjectTypeAgent, ID: "8"}}, Permissions: types.VMCPProfilePermissions{AllowAllComponents: true}},
			},
		}}}
	}
	vmcp, other := newVMCP("vmcp1grants"), newVMCP("vmcp1other")
	client := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithStatusSubresource(vmcp, other).WithObjects(vmcp, other).Build()
	services, err := sservices.New(sservices.Config{DSN: "sqlite://:memory:"})
	require.NoError(t, err)
	database, err := gatewaydb.New(services.DB.DB, services.DB.SQLDB, true)
	require.NoError(t, err)
	require.NoError(t, database.AutoMigrate())
	gatewayClient := gateway.New(t.Context(), database, nil, nil, nil, time.Hour, 10, 0, 0, 0, false)
	t.Cleanup(func() { _ = gatewayClient.Close() })
	store := vmcpconfig.NewAgentOAuthGrantStore(gatewayClient)
	var removed []string
	store.SetGrantRemovalRecorder(func(_ context.Context, vmcpID, componentID, reason string) {
		removed = append(removed, vmcpID+"/"+componentID+": "+reason)
	})
	handler := &Handler{agentOAuthGrants: store}
	current := func(vmcp *v1.VMCP) *v1.VMCP {
		t.Helper()
		stored := &v1.VMCP{}
		require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(vmcp), stored))
		return stored
	}
	run := func(vmcp *v1.VMCP, handle func(router.Request, router.Response) error) *v1.VMCP {
		t.Helper()
		require.NoError(t, handle(router.Request{Ctx: t.Context(), Client: client, Object: current(vmcp)}, nil))
		return current(vmcp)
	}
	conf := &oauth2.Config{ClientID: "client", Endpoint: oauth2.Endpoint{TokenURL: "https://provider.example.com/token"}}
	token := &oauth2.Token{AccessToken: "access", RefreshToken: "refresh"}
	authorize := func(vmcp *v1.VMCP, componentID string) {
		t.Helper()
		_, err := store.Authorize(t.Context(), client, kclient.ObjectKeyFromObject(vmcp), componentID, func(*v1.VMCP, types.VMCPComponent) error { return nil },
			func(ctx context.Context, owner, mcpID, grantID string) error {
				return gatewayClient.ReplaceMCPOAuthGrant(ctx, owner, mcpID, "https://"+componentID+".example.com/mcp", grantID, "1", conf, token)
			})
		require.NoError(t, err)
	}
	hasGrant := func(vmcp *v1.VMCP, componentID string) bool {
		t.Helper()
		_, err := gatewayClient.GetMCPOAuthToken(t.Context(), principal.AgentOAuthGrantOwner(vmcp.Name), vmcpconfig.AgentOAuthGrantMCPID(vmcp.Name, componentID), "https://"+componentID+".example.com/mcp")
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false
		}
		require.NoError(t, err)
		return true
	}

	authorize(vmcp, "static")
	authorize(vmcp, "dynamic")
	authorize(other, "static")
	require.ElementsMatch(t, []string{"static", "dynamic"}, grantRecords(current(vmcp)))
	// Tokens of a user and of an agent, for connections of their own; neither is ever a vMCP's grant.
	for _, owner := range []string{"7", "agent:7"} {
		require.NoError(t, gatewayClient.ReplaceMCPOAuthToken(t.Context(), owner, "msi1connection", "https://static.example.com/mcp", "", conf, token))
	}
	requireOthersKept := func() {
		t.Helper()
		for _, owner := range []string{"7", "agent:7"} {
			_, err := gatewayClient.GetMCPOAuthToken(t.Context(), owner, "msi1connection", "https://static.example.com/mcp")
			require.NoError(t, err, "the token of %s was deleted with a vMCP's grants", owner)
		}
		require.True(t, hasGrant(other, "static"), "another vMCP's grant was deleted")
	}

	// Removing a profile, or the agent from one, deletes no grant: grants belong to the vMCP's components.
	stored := current(vmcp)
	stored.Spec.Manifest.Profiles = stored.Spec.Manifest.Profiles[:1]
	require.NoError(t, client.Update(t.Context(), stored))
	run(vmcp, handler.ReconcileProfiles)
	stored = run(vmcp, handler.CollectAgentOAuthGrants)
	require.True(t, hasGrant(vmcp, "static"))
	require.True(t, hasGrant(vmcp, "dynamic"))
	require.ElementsMatch(t, []string{"static", "dynamic"}, grantRecords(stored))
	require.Empty(t, removed)

	// Removing a component deletes its grant, and its record.
	stored.Spec.Manifest.Components = stored.Spec.Manifest.Components[:1]
	require.NoError(t, client.Update(t.Context(), stored))
	stored = run(vmcp, handler.CollectAgentOAuthGrants)
	require.False(t, hasGrant(vmcp, "dynamic"))
	require.True(t, hasGrant(vmcp, "static"))
	require.Equal(t, []string{"static"}, grantRecords(stored))
	require.Equal(t, []string{"vmcp1grants/dynamic: component removed"}, removed)
	requireOthersKept()

	// Deleting the vMCP deletes every grant it holds, and can run again.
	run(vmcp, handler.RemoveAgentOAuthGrants)
	require.False(t, hasGrant(vmcp, "static"))
	require.Equal(t, []string{"vmcp1grants/dynamic: component removed", "vmcp1grants/static: vMCP deleted"}, removed)
	run(vmcp, handler.RemoveAgentOAuthGrants)
	require.Len(t, removed, 2, "a grant already deleted was audited again")
	requireOthersKept()
}

func grantRecords(vmcp *v1.VMCP) []string {
	var records []string
	for _, record := range vmcp.Status.AgentOAuthGrants {
		records = append(records, record.ComponentID)
	}
	return records
}
