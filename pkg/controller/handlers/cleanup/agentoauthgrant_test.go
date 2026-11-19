package cleanup

import (
	"testing"

	"github.com/obot-platform/nah/pkg/fields"
	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/pkg/accesscontrolrule"
	gclient "github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/principal"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	vmcpconfig "github.com/obot-platform/obot/pkg/vmcp"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
	gocache "k8s.io/client-go/tools/cache"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	agentGrantURL = "https://provider.example.com/mcp"
)

// storeAgentGrant stores a vMCP's agent OAuth grant for a component, authorized by the user authorizedBy.
func storeAgentGrant(t *testing.T, gateway *gclient.Client, vmcpID, componentID, authorizedBy string) (string, string) {
	t.Helper()
	owner, mcpID := principal.AgentOAuthGrantOwner(vmcpID), vmcpconfig.AgentOAuthGrantMCPID(vmcpID, componentID)
	require.NoError(t, gateway.ReplaceMCPOAuthGrant(t.Context(), owner, mcpID, agentGrantURL, "grant-"+vmcpID+"-"+componentID, authorizedBy,
		&oauth2.Config{ClientID: "client"}, &oauth2.Token{AccessToken: "token", RefreshToken: "refresh"}))
	return owner, mcpID
}

func requireAgentGrant(t *testing.T, gateway *gclient.Client, owner, mcpID string) {
	t.Helper()
	grant, err := gateway.GetMCPOAuthToken(t.Context(), owner, mcpID, agentGrantURL)
	require.NoError(t, err, "the grant of %s for %s was deleted", owner, mcpID)
	require.Nil(t, grant.InvalidatedAt)
}

// TestUserCleanupKeepsAgentGrantsTheUserAuthorized checks that deleting the administrator who authorized a vMCP's agent
// OAuth grant deletes nothing of it: the grant is the vMCP's, keyed by the vMCP and component, and records the
// administrator only as who authorized it.
func TestUserCleanupKeepsAgentGrantsTheUserAuthorized(t *testing.T) {
	userDelete := &v1.UserDelete{Name: "delete-user", Namespace: system.DefaultNamespace, Spec: v1.UserDeleteSpec{UserID: 1}}
	builder := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(userDelete)
	for _, obj := range []kclient.Object{
		&v1.VMCP{}, &v1.VMCPInstance{}, &v1.MCPServer{}, &v1.MCPServerInstance{}, &v1.PowerUserWorkspace{}, &v1.AgentInstance{},
	} {
		builder.WithIndex(obj, "spec.userID", func(obj kclient.Object) []string {
			return []string{obj.(fields.Fields).Get("spec.userID")}
		})
	}
	client := builder.Build()
	gateway := newCredentialsCleanupGatewayClient(t)
	owner, mcpID := storeAgentGrant(t, gateway, "vmcp1shared", "component", "1")
	indexer := gocache.NewIndexer(gocache.MetaNamespaceKeyFunc, gocache.Indexers{
		"user-ids": func(any) ([]string, error) { return nil, nil },
	})

	require.NoError(t, NewUserCleanup(gateway, accesscontrolrule.NewAccessControlRuleHelper(indexer, client)).Cleanup(router.Request{
		Ctx:       t.Context(),
		Client:    client,
		Object:    userDelete,
		Namespace: system.DefaultNamespace,
	}, nil))
	requireAgentGrant(t, gateway, owner, mcpID)
	grant, err := gateway.GetMCPOAuthToken(t.Context(), owner, mcpID, agentGrantURL)
	require.NoError(t, err)
	require.Equal(t, "1", grant.AuthorizedBy)
}

// TestAgentCleanupKeepsAgentGrants checks that deleting an agent deletes no agent OAuth grant of any vMCP, even of a
// vMCP whose profiles named only that agent: grants belong to the vMCP's components, which other agents may be granted.
func TestAgentCleanupKeepsAgentGrants(t *testing.T) {
	f := newAgentCleanupFixture(t, nil)
	owner, mcpID := storeAgentGrant(t, f.gateway, "vmcp1shared", "component", "3")
	otherOwner, otherMCPID := storeAgentGrant(t, f.gateway, "vmcp1other", "component", "3")

	require.NoError(t, DeletedAgent(f.request(t), nil))
	f.requireCleanedUp(t)
	requireAgentGrant(t, f.gateway, owner, mcpID)
	requireAgentGrant(t, f.gateway, otherOwner, otherMCPID)
}
