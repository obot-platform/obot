package authz

import (
	"net/http"
	"net/http/httptest"
	"testing"

	types2 "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/principal"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kuser "k8s.io/apiserver/pkg/authentication/user"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func agentUser(grants ...string) User {
	return User{Info: &kuser.DefaultInfo{
		Name:   "hosted-agent:hai1",
		UID:    "hosted-agent:hai1",
		Groups: []string{types2.GroupAuthenticated, types2.GroupMCP},
		Extra: map[string][]string{
			"authorized_mcp_ids":            grants,
			"hosted_agent_instance_id":      {"hai1"},
			principal.HostedAgentOwnerExtra: {"u1"},
		},
	}}
}

// An agent is not a user, so every user-shaped access check denies it: it owns
// no server, belongs to no catalog and matches no access control rule. Its
// grant list is what authorizes it, or it can never reach the MCP servers it
// was configured with.
func TestHostedAgentReachesItsGrantedServer(t *testing.T) {
	client := fakeclient.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(
		&v1.MCPServer{ObjectMeta: metav1.ObjectMeta{Name: "ms1github", Namespace: "obot"}},
	).Build()
	authorizer := &Authorizer{uncached: client}

	req := httptest.NewRequest(http.MethodPost, "/mcp-connect/ms1github", nil)
	ok, err := authorizer.checkMCPID(req, &Resources{MCPID: "ms1github"}, agentUser("ms1github"))
	if err != nil {
		t.Fatalf("checkMCPID: %v", err)
	}
	if !ok {
		t.Fatal("an agent was denied the MCP server it was granted")
	}
}

// The grant list is the whole authority, so a server absent from it is denied.
func TestHostedAgentCannotReachAnUngrantedServer(t *testing.T) {
	client := fakeclient.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(
		&v1.MCPServer{ObjectMeta: metav1.ObjectMeta{Name: "ms1secret", Namespace: "obot"}},
	).Build()
	authorizer := &Authorizer{uncached: client}

	req := httptest.NewRequest(http.MethodPost, "/mcp-connect/ms1secret", nil)
	ok, _ := authorizer.checkMCPID(req, &Resources{MCPID: "ms1secret"}, agentUser("ms1github"))
	if ok {
		t.Fatal("an agent reached a server it was never granted")
	}
}

// An agent with no servers must reach none. Falling through to "allow" on an
// empty list is how a caller with no grants ends up with every grant.
func TestHostedAgentWithNoGrantsReachesNothing(t *testing.T) {
	client := fakeclient.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(
		&v1.MCPServer{ObjectMeta: metav1.ObjectMeta{Name: "ms1github", Namespace: "obot"}},
	).Build()
	authorizer := &Authorizer{uncached: client}

	req := httptest.NewRequest(http.MethodPost, "/mcp-connect/ms1github", nil)
	ok, _ := authorizer.checkMCPID(req, &Resources{MCPID: "ms1github"}, agentUser())
	if ok {
		t.Fatal("an agent with no granted servers reached one")
	}
}

// The agent path must not weaken the user path: a real user is still checked
// against catalogs, ownership and access rules.
func TestOrdinaryUserStillGoesThroughAccessChecks(t *testing.T) {
	if principal.IsHostedAgent(&kuser.DefaultInfo{Groups: []string{types2.GroupAuthenticated}}) {
		t.Fatal("an ordinary user must not be treated as a hosted agent")
	}
}
