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
	profileGrantURL = "https://provider.example.com/mcp"
)

// storeProfileGrant stores a grant of a vMCP profile, authorized by the user authorizedBy.
func storeProfileGrant(t *testing.T, gateway *gclient.Client, profileID, authorizedBy string) (string, string) {
	t.Helper()
	owner, mcpID := principal.ProfileGrant("vmcp1shared", profileID), vmcpconfig.ProfileOAuthGrantMCPID("vmcp1shared", "component")
	require.NoError(t, gateway.ReplaceMCPOAuthGrant(t.Context(), owner, mcpID, profileGrantURL, "grant-"+profileID, authorizedBy,
		&oauth2.Config{ClientID: "client"}, &oauth2.Token{AccessToken: "token", RefreshToken: "refresh"}))
	return owner, mcpID
}

func requireProfileGrant(t *testing.T, gateway *gclient.Client, owner, mcpID string) {
	t.Helper()
	grant, err := gateway.GetMCPOAuthToken(t.Context(), owner, mcpID, profileGrantURL)
	require.NoError(t, err, "the grant of %s was deleted", owner)
	require.Nil(t, grant.InvalidatedAt)
}

// TestUserCleanupKeepsProfileGrantsTheUserAuthorized checks that deleting the administrator who authorized a profile's
// grant deletes nothing of it: the grant is the profile's, keyed by the profile, and records the administrator only as
// who authorized it.
func TestUserCleanupKeepsProfileGrantsTheUserAuthorized(t *testing.T) {
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
	owner, mcpID := storeProfileGrant(t, gateway, "vp1agents", "1")
	indexer := gocache.NewIndexer(gocache.MetaNamespaceKeyFunc, gocache.Indexers{
		"user-ids": func(any) ([]string, error) { return nil, nil },
	})

	require.NoError(t, NewUserCleanup(gateway, accesscontrolrule.NewAccessControlRuleHelper(indexer, client)).Cleanup(router.Request{
		Ctx:       t.Context(),
		Client:    client,
		Object:    userDelete,
		Namespace: system.DefaultNamespace,
	}, nil))
	requireProfileGrant(t, gateway, owner, mcpID)
	grant, err := gateway.GetMCPOAuthToken(t.Context(), owner, mcpID, profileGrantURL)
	require.NoError(t, err)
	require.Equal(t, "1", grant.AuthorizedBy)
}

// TestAgentCleanupKeepsProfileGrants checks that deleting an agent deletes no grant of the profiles it resolved to,
// even one whose profile named only that agent: grants belong to profiles.
func TestAgentCleanupKeepsProfileGrants(t *testing.T) {
	f := newAgentCleanupFixture(t, nil)
	explicitOwner, mcpID := storeProfileGrant(t, f.gateway, "vp1agent", "3")
	wildcardOwner, _ := storeProfileGrant(t, f.gateway, "vp1agents", "3")

	require.NoError(t, DeletedAgent(f.request(t), nil))
	f.requireCleanedUp(t)
	requireProfileGrant(t, f.gateway, explicitOwner, mcpID)
	requireProfileGrant(t, f.gateway, wildcardOwner, mcpID)
}
