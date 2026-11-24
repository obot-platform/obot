//go:build integration

package agentdelegation

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/obot-platform/obot/tests/integration/harness"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// tokenHeader is the value each user enters for the remote MCP server, which it receives as a header. Catalog
	// entries store header keys in upper case.
	tokenHeader = "X-TOKEN"
	// agentSessionCookie is the cookie of the fake agent's own sessions with its users' browsers.
	agentSessionCookie = "fake_agent_session"
)

// fakeAgent is an agent's web app: it keeps a session with each browser that uses it, asks Obot for a user's approval
// with a secret it keeps in that session, and redeems the code a browser brings back with the secret of that
// browser's session. It authenticates to Obot with its API key only.
type fakeAgent struct {
	server  *httptest.Server
	obotURL string
	key     string

	mu       sync.Mutex
	sessions map[string]*agentSession
	code     string
}

// agentSession is what the fake agent keeps for one browser.
type agentSession struct {
	secret       string
	requestID    string
	delegationID string
	token        string
	user         types.AgentDelegationUser
}

// browser is a user's browser: one cookie jar for Obot and the agent, and no automatic redirects. It calls the APIs
// that Obot's approval and "Agents acting for you" pages call, as those pages do.
type browser struct {
	*harness.Harness
	email  string
	userID string
}

// agentTransport sends every request as the agent's token, with a forged on-behalf-of header that Obot must drop.
type agentTransport struct {
	token, forged string
}

// TestAgentActsForAUserEndToEnd runs the whole flow of an agent acting on behalf of users against the running server,
// with a fake agent web app that keeps a session for each browser, and browsers that are cookie jars calling the APIs
// the approval and "Agents acting for you" pages call:
//
//  1. An administrator creates the agent, a key that can't use the LLM gateway, an allowed return host, a vMCP profile
//     that names the agent with one tool, and an agent profile that lets the agent act for every user.
//  2. The agent asks for approval in Alice's session with it, keeping the secret in that session.
//  3. Alice signs in, reads the consent, enters the value the vMCP's component needs, and approves.
//  4. Her browser comes back to the agent, which redeems the code with the session's secret. It then connects for
//     her, sees only the tools of its own vMCP profile, never those of hers, calls a tool that receives her value, has
//     its own Skills and is refused the LLM gateway its key can't use, and the MCP audit records her as the user it
//     acted for.
//  5. Alice clears her value from "Agents acting for you": the agent is told to ask her again, and she approves again,
//     which keeps the delegation.
//  6. A link the agent made in another session is forwarded to Bob, who approves it. His browser comes back in his own
//     session with the agent, which lacks that session's secret, so the code can't be redeemed, and it's used up.
//  7. Alice revokes the agent: its token is refused within the delegation cache window, and the setup she did through
//     the agent is gone.
func TestAgentActsForAUserEndToEnd(t *testing.T) {
	owner := signIn(t, ownerEmail, ownerPassword)
	waitForDefaultCatalog(t, owner)
	upstream := startUpstream(t)
	fake := startFakeAgent(t, owner.BaseURL)
	// Each step is a subtest that builds on the ones before it, so a failed step ends the test. What must outlive a step
	// is cleaned up with the test.
	top := t

	var (
		agent       types.AgentIdentity
		vmcp        types.VMCP
		componentID string
		readOnlyID  string
		key         types.AgentAPIKeyCreateResponse
	)
	if !t.Run("an administrator sets up the agent", func(t *testing.T) {
		agent = owner.CreateAgent(t, owner.MCPServerName("assistant"))
		entry := owner.CreateMCPCatalogEntry(t, system.DefaultCatalog, types.MCPServerCatalogEntryManifest{
			Name:              owner.MCPServerName("notes"),
			ShortDescription:  "agent delegation integration test server",
			Description:       "A remote MCP server that reports what it receives",
			Runtime:           types.RuntimeRemote,
			RemoteConfig:      &types.RemoteCatalogConfig{FixedURL: upstream.URL},
			InjectObotHeaders: []types.ObotHeader{types.ObotHeaderUserID},
			Config: []types.MCPConfig{{
				Name:      "Token",
				Key:       tokenHeader,
				Required:  true,
				Sensitive: true,
				Usage:     types.Header,
			}},
		})
		agentSubject := []types.Subject{{Type: types.SubjectTypeAgent, ID: agent.ID}}
		vmcp = owner.CreateSharedVMCP(t, types.VMCPManifest{
			DisplayName: owner.MCPServerName("notes"),
			Components: []types.VMCPComponent{{
				Name:                    "notes",
				MCPCatalogID:            system.DefaultCatalog,
				MCPServerCatalogEntryID: entry.ID,
				Configuration: []types.VMCPConfigurationPolicy{{
					Key:    tokenHeader,
					Policy: types.VMCPConfigurationPolicyUserAllowed,
				}},
			}},
			// Every user has every tool, which the agent never has through them.
			Profiles: []types.VMCPProfile{{
				Name:        "users",
				Subjects:    []types.Subject{{Type: types.SubjectTypeSelector, ID: types.SubjectWildcardID}},
				Permissions: types.VMCPProfilePermissions{AllowAllComponents: true},
			}},
		})
		if len(vmcp.Components) != 1 || vmcp.Components[0].ID == "" {
			t.Fatalf("vMCP create returned %+v, want one component with an ID", vmcp)
		}
		componentID = vmcp.Components[0].ID

		// A second profile names the agent with one tool, which is all the agent has, for itself and for every user. A
		// controller may update the new vMCP meanwhile, so the update is retried on a conflict.
		eventually(t, "the vMCP's second profile", func() error {
			owner.Get(t, "/api/vmcps/"+vmcp.ID, &vmcp)
			manifest := vmcp.VMCPManifest
			manifest.Profiles = append(manifest.Profiles, types.VMCPProfile{
				Name:     "read-only",
				Subjects: agentSubject,
				Permissions: types.VMCPProfilePermissions{AllowedComponents: map[string]types.VMCPComponentSet{
					componentID: {AllowedTools: []string{"whoami"}},
				}},
			})
			resp := owner.RequestAs(t, http.MethodPut, "/api/vmcps/"+vmcp.ID, "", manifest)
			if resp.StatusCode == http.StatusConflict {
				return fmt.Errorf("%s", resp.Body)
			} else if resp.StatusCode != http.StatusOK {
				t.Fatalf("update the vMCP = %d %s", resp.StatusCode, resp.Body)
			}
			decode(t, resp, &vmcp)
			return nil
		})
		idx := slices.IndexFunc(vmcp.Profiles, func(p types.VMCPProfile) bool { return p.Name == "read-only" })
		if idx < 0 || vmcp.Profiles[idx].ID == "" {
			t.Fatalf("vMCP profiles = %+v, want a read-only profile with an ID", vmcp.Profiles)
		}
		readOnlyID = vmcp.Profiles[idx].ID

		// The key can reach the vMCP and Skills, but not the LLM gateway, which the agent can't use for a user either.
		key = owner.CreateAgentAPIKey(t, agent.ID, types.AgentAPIKeyCreateRequest{
			Name:            "assistant",
			MCPServerIDs:    []string{vmcp.ID},
			CanAccessSkills: true,
		})
		fake.key = key.Key

		owner.Put(t, "/api/agents/"+agent.ID, types.AgentIdentityManifest{
			DisplayName: agent.DisplayName,
			Profiles: []types.AgentProfile{{
				Name:     "everyone",
				Subjects: []types.Subject{{Type: types.SubjectTypeSelector, ID: types.SubjectWildcardID}},
			}},
			AllowedReturnHosts: []string{fake.host()},
		}, &agent)
		if len(agent.Profiles) != 1 || !strings.HasPrefix(agent.Profiles[0].ID, types.AgentProfileIDPrefix) {
			t.Fatalf("agent profiles = %+v, want one with an %s ID", agent.Profiles, types.AgentProfileIDPrefix)
		}

		// As itself, the agent can't use a vMCP that needs a value a user enters.
		resp := initialize(t, owner.BaseURL, vmcp.ID, key.Key)
		if resp.StatusCode != http.StatusConflict || !strings.Contains(string(resp.Body), "agent_configuration_required") {
			t.Fatalf("connect with the agent's key = %d %s, want 409 agent_configuration_required", resp.StatusCode, resp.Body)
		}
	}) {
		return
	}

	alice := newBrowser(t, owner, "alice")
	value := "alice-" + owner.RunID
	var (
		aliceRequestID string
		aliceSession   agentSession
	)
	if !t.Run("the agent asks for approval in Alice's session", func(t *testing.T) {
		approvalURL := alice.startWithAgent(t, fake)
		aliceRequestID = requestIDOf(t, owner.BaseURL, approvalURL)
		// The page needs a signed-in user: before Alice signs in, it can't read the request.
		if resp := alice.obot(t, http.MethodGet, "/api/agent-delegation-requests/"+aliceRequestID, nil, nil); resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("read the request signed out = %d %s, want 401", resp.StatusCode, resp.Body)
		}
	}) {
		return
	}

	if !t.Run("Alice signs in, reads the consent, sets her value and approves", func(t *testing.T) {
		alice.signIn(t)

		var request types.AgentDelegationApprovalRequest
		alice.obotJSON(t, http.MethodGet, "/api/agent-delegation-requests/"+aliceRequestID, nil, &request)
		wantAccess := []types.AgentDelegationVMCPAccess{{
			VMCPID:          vmcp.ID,
			VMCPDisplayName: vmcp.DisplayName,
			Profiles:        []types.AgentDelegationVMCPProfile{{ID: readOnlyID, Name: "read-only"}},
		}}
		if request.Agent.ID != agent.ID || request.DelegationID != "" || !jsonEqual(request.Access.VMCPs, wantAccess) {
			t.Fatalf("consent = %+v, want agent %s with only its own read-only profile of the vMCP", request, agent.ID)
		}

		setupPath := "/api/agent-delegation-requests/" + aliceRequestID + "/setup"
		var setup types.AgentDelegationSetup
		alice.obotJSON(t, http.MethodGet, setupPath, nil, &setup)
		if len(setup.VMCPs) != 1 || setup.VMCPs[0].Ready || len(setup.VMCPs[0].Components) != 1 ||
			len(setup.VMCPs[0].Components[0].Configuration) != 1 || setup.VMCPs[0].Components[0].Configuration[0].Set {
			t.Fatalf("setup before the value = %+v, want one vMCP that needs one value", setup)
		}

		resp := alice.obot(t, http.MethodPut, setupPath+"/vmcps/"+vmcp.ID+"/configuration", types.AgentDelegationSetupConfiguration{
			Components: map[string]map[string]*string{componentID: {tokenHeader: &value}},
		}, nil)
		if resp.StatusCode != http.StatusOK || strings.Contains(string(resp.Body), value) {
			t.Fatalf("set the value = %d, and the response must never hold it\nbody: %s", resp.StatusCode, resp.Body)
		}
		var configured types.AgentDelegationSetupVMCP
		decode(t, resp, &configured)
		if !configured.Ready || !configured.Components[0].Configuration[0].Set {
			t.Fatalf("setup after the value = %+v, want it ready with the value set", configured)
		}

		// Another site's page can't approve for her.
		crossSite := http.Header{"Sec-Fetch-Site": {"cross-site"}}
		if resp := alice.obot(t, http.MethodPost, "/api/agent-delegation-requests/"+aliceRequestID+"/approve", nil, crossSite); resp.StatusCode != http.StatusForbidden {
			t.Fatalf("approve from another site = %d %s, want 403", resp.StatusCode, resp.Body)
		}

		var redirect types.AgentDelegationRedirect
		alice.obotJSON(t, http.MethodPost, "/api/agent-delegation-requests/"+aliceRequestID+"/approve", nil, &redirect)
		if !strings.HasPrefix(redirect.RedirectURL, fake.server.URL+"/callback?") {
			t.Fatalf("approval redirect = %q, want the agent's return URL", redirect.RedirectURL)
		}

		// The browser follows the redirect back to the agent, which redeems the code in Alice's session.
		if resp := alice.visit(t, redirect.RedirectURL); resp.StatusCode != http.StatusOK {
			t.Fatalf("return to the agent = %d %s, want 200", resp.StatusCode, resp.Body)
		}
		aliceSession = fake.sessionOf(t, alice)
		if aliceSession.delegationID == "" || aliceSession.token == "" || aliceSession.user.ID != alice.userID || aliceSession.user.Email != alice.email {
			t.Fatalf("Alice's session with the agent = %+v, want a delegation and a token for her", aliceSession.redacted())
		}
	}) {
		return
	}

	pairOwner := "agent:" + agent.ID + "/user:" + alice.userID
	var mcpSession *gomcp.ClientSession
	if !t.Run("the agent acts for Alice", func(t *testing.T) {
		if pairInstances(t, pairOwner, vmcp.ID) == 0 {
			t.Fatalf("no vMCP instance holds what Alice set up through the agent (%s)", pairOwner)
		}

		// The controllers copy her value to her connection through the agent in the background, so the first calls
		// may come too early.
		var seen map[string]string
		eventually(t, "the agent's call for Alice", func() error {
			session, err := connect(t, owner.BaseURL+"/mcp-connect/"+vmcp.ID, aliceSession.token, alice.userID)
			if err != nil {
				return err
			}
			if seen, err = whoami(t.Context(), session); err != nil {
				_ = session.Close()
				return err
			}
			mcpSession = session
			return nil
		})
		top.Cleanup(func() { _ = mcpSession.Close() })
		if seen["token"] != value {
			t.Fatalf("the MCP server received %s = %q, want Alice's value", tokenHeader, seen["token"])
		}
		// The agent is the caller, and Alice is who it acts for. A client can't send that header: the fake agent sends a
		// forged one, which Obot replaces with what authentication found.
		if seen["user"] != "agent:"+agent.ID || seen["onBehalfOf"] != alice.userID {
			t.Fatalf("the MCP server received user %q on behalf of %q, want agent:%s on behalf of %s", seen["user"], seen["onBehalfOf"], agent.ID, alice.userID)
		}

		tools, err := mcpSession.ListTools(t.Context(), nil)
		if err != nil {
			t.Fatal(err)
		}
		var names []string
		for _, tool := range tools.Tools {
			names = append(names, tool.Name)
		}
		if !slices.Equal(names, []string{"whoami"}) {
			t.Fatalf("tools for Alice = %v, want only the whoami of the agent's own read-only profile", names)
		}
		if result, err := mcpSession.CallTool(t.Context(), &gomcp.CallToolParams{Name: "purge"}); err == nil && !result.IsError {
			t.Fatalf("the agent called purge for Alice, which only her own profile grants")
		}
	}) {
		return
	}

	if !t.Run("the agent has its own grants for Alice, within its key's scopes", func(t *testing.T) {
		skills := func(bearer string) []string {
			resp := send(t, http.DefaultClient, http.MethodGet, owner.BaseURL+"/api/skills", bearer, nil, nil)
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("list skills = %d %s, want 200", resp.StatusCode, resp.Body)
			}
			var list types.SkillList
			decode(t, resp, &list)
			ids := []string{}
			for _, skill := range list.Items {
				ids = append(ids, skill.ID)
			}
			return ids
		}
		if forAlice, forItself := skills(aliceSession.token), skills(key.Key); !slices.Equal(forAlice, forItself) {
			t.Fatalf("skills for Alice = %v, want the agent's own %v", forAlice, forItself)
		}
		// The key that minted the token can't use the LLM gateway, so the agent can't for Alice either.
		requireStatus(t, owner.BaseURL, http.MethodGet, "/api/llm-proxy/v1/models", aliceSession.token, http.StatusForbidden)
		// Nor does the token reach anything but the routes an agent acts for a user on.
		requireStatus(t, owner.BaseURL, http.MethodGet, "/api/me", aliceSession.token, http.StatusUnauthorized)
		requireStatus(t, owner.BaseURL, http.MethodPost, "/api/agent-delegations", aliceSession.token, http.StatusUnauthorized)
	}) {
		return
	}

	if !t.Run("the MCP audit records Alice as the user the agent acted for", func(t *testing.T) {
		query := "/api/mcp-audit-logs?user_id=" + alice.userID + "&limit=1000"
		eventually(t, "the agent's audit rows", func() error {
			var events types.AuditLogEventResponse
			owner.Get(t, query+"&include_on_behalf_of=true", &events)
			for _, event := range events.Items {
				if event.Action.Name == "whoami" && event.Actor.ActorType == types.AuditLogActorTypeAgent && event.Actor.ID == agent.ID &&
					event.Actor.OnBehalfOf == (types.PrincipalRef{Kind: types.PrincipalKindUser, ID: alice.userID}) {
					return nil
				}
			}
			return fmt.Errorf("no whoami call by agent %s on behalf of %s among %d rows", agent.ID, alice.userID, len(events.Items))
		})
		// The agent's rows count for Alice only when asked to.
		var events types.AuditLogEventResponse
		owner.Get(t, query, &events)
		if slices.ContainsFunc(events.Items, func(e types.AuditLogEvent) bool { return e.Actor.ActorType == types.AuditLogActorTypeAgent }) {
			t.Fatalf("Alice's own audit rows include the agent's: %+v", events.Items)
		}
	}) {
		return
	}

	if !t.Run("Alice clears her value, and approves the agent again", func(t *testing.T) {
		// She manages what she set up through the agent from "Agents acting for you".
		setupPath := "/api/me/agent-delegations/" + aliceSession.delegationID + "/setup"
		var cleared types.AgentDelegationSetupVMCP
		alice.obotJSON(t, http.MethodPut, setupPath+"/vmcps/"+vmcp.ID+"/configuration", types.AgentDelegationSetupConfiguration{
			Components: map[string]map[string]*string{componentID: {tokenHeader: nil}},
		}, &cleared)
		if cleared.Ready || cleared.Components[0].Configuration[0].Set {
			t.Fatalf("setup after clearing the value = %+v, want it not ready", cleared)
		}

		// The agent's next connection tells it to ask her again, and names the vMCP.
		eventually(t, "the agent to need Alice's approval", func() error {
			resp := initialize(t, owner.BaseURL, vmcp.ID, aliceSession.token)
			if resp.StatusCode != http.StatusConflict {
				return fmt.Errorf("connect = %d %s, want 409", resp.StatusCode, resp.Body)
			}
			var failure struct {
				Error  string `json:"error"`
				VMCPID string `json:"vmcpID"`
			}
			if err := json.Unmarshal(resp.Body, &failure); err != nil || failure.Error != "agent_user_approval_required" || failure.VMCPID != vmcp.ID {
				t.Fatalf("connect = 409 %s, want agent_user_approval_required for vMCP %s", resp.Body, vmcp.ID)
			}
			return nil
		})

		// The agent asks again for the delegation it holds. The consent says so, and Alice sets a new value.
		firstToken := aliceSession.token
		requestID := requestIDOf(t, owner.BaseURL, alice.startWithAgent(t, fake))
		var request types.AgentDelegationApprovalRequest
		alice.obotJSON(t, http.MethodGet, "/api/agent-delegation-requests/"+requestID, nil, &request)
		if request.DelegationID != aliceSession.delegationID {
			t.Fatalf("consent for approving again names delegation %q, want %q", request.DelegationID, aliceSession.delegationID)
		}
		value = "alice-again-" + owner.RunID
		alice.obotJSON(t, http.MethodPut, "/api/agent-delegation-requests/"+requestID+"/setup/vmcps/"+vmcp.ID+"/configuration", types.AgentDelegationSetupConfiguration{
			Components: map[string]map[string]*string{componentID: {tokenHeader: &value}},
		}, nil)
		var redirect types.AgentDelegationRedirect
		alice.obotJSON(t, http.MethodPost, "/api/agent-delegation-requests/"+requestID+"/approve", nil, &redirect)
		if resp := alice.visit(t, redirect.RedirectURL); resp.StatusCode != http.StatusOK {
			t.Fatalf("return to the agent = %d %s, want 200", resp.StatusCode, resp.Body)
		}
		again := fake.sessionOf(t, alice)
		if again.delegationID != aliceSession.delegationID || again.token == firstToken {
			t.Fatalf("Alice's session after approving again = %+v, want a new token from the same delegation %s", again.redacted(), aliceSession.delegationID)
		}
		aliceSession = again

		_ = mcpSession.Close()
		var seen map[string]string
		eventually(t, "the agent's call with Alice's new value", func() error {
			session, err := connect(t, owner.BaseURL+"/mcp-connect/"+vmcp.ID, aliceSession.token, alice.userID)
			if err != nil {
				return err
			}
			if seen, err = whoami(t.Context(), session); err == nil && seen["token"] != value {
				err = fmt.Errorf("the MCP server received %q, want Alice's new value", seen["token"])
			}
			if err != nil {
				_ = session.Close()
				return err
			}
			mcpSession = session
			return nil
		})
		// The token the agent got before still works: approving again keeps the delegation.
		if resp := initialize(t, owner.BaseURL, vmcp.ID, firstToken); resp.StatusCode != http.StatusOK {
			t.Fatalf("connect with the token from the first approval = %d %s, want 200", resp.StatusCode, resp.Body)
		}
	}) {
		return
	}

	bob := newBrowser(t, owner, "bob")
	if !t.Run("a forwarded link can't be redeemed in the approver's own session", func(t *testing.T) {
		// Someone using the agent, in session B, gets a link, and forwards it to Bob.
		forwarder := newBrowser(t, owner, "")
		forwardedURL := forwarder.startWithAgent(t, fake)
		forwardedID := requestIDOf(t, owner.BaseURL, forwardedURL)
		sessionB := fake.sessionOf(t, forwarder)

		// Bob uses the agent too, in his own session, which holds a secret of its own.
		bob.signIn(t)
		bob.startWithAgent(t, fake)

		var redirect types.AgentDelegationRedirect
		bob.obotJSON(t, http.MethodPost, "/api/agent-delegation-requests/"+forwardedID+"/approve", nil, &redirect)
		resp := bob.visit(t, redirect.RedirectURL)
		if resp.StatusCode != http.StatusBadRequest || !strings.Contains(string(resp.Body), "secret does not match") {
			t.Fatalf("redeem in Bob's session = %d %s, want 400 because the secret doesn't match", resp.StatusCode, resp.Body)
		}
		if session := fake.sessionOf(t, bob); session.token != "" || session.delegationID != "" {
			t.Fatalf("Bob's session with the agent holds a token for him: %+v", session.redacted())
		}
		// The wrong secret used the code up: even session B's secret can't redeem it now.
		if resp := fake.redeem(t, forwardedID, fake.lastCode(), sessionB.secret); resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("redeem the used code with the right secret = %d %s, want 400", resp.StatusCode, resp.Body)
		}

		// Bob approved, so he sees the agent and can revoke it, though it never got a token for him. Administrators see
		// both users' delegations.
		var mine types.AgentDelegationList
		bob.obotJSON(t, http.MethodGet, "/api/me/agent-delegations", nil, &mine)
		if len(mine.Items) != 1 || mine.Items[0].Agent.ID != agent.ID || mine.Items[0].LastUsedAt != nil {
			t.Fatalf("Bob's delegations = %+v, want the agent's, never used", mine.Items)
		}
		var all types.AgentDelegationList
		owner.Get(t, "/api/agents/"+agent.ID+"/delegations", &all)
		var users []string
		for _, delegation := range all.Items {
			users = append(users, delegation.User.ID)
		}
		slices.Sort(users)
		want := []string{alice.userID, bob.userID}
		slices.Sort(want)
		if !slices.Equal(users, want) {
			t.Fatalf("the agent's delegations are for users %v, want %v", users, want)
		}

		// An administrator revokes Bob's.
		owner.Delete(t, "/api/agents/"+agent.ID+"/delegations/"+mine.Items[0].ID)
		bob.obotJSON(t, http.MethodGet, "/api/me/agent-delegations", nil, &mine)
		if len(mine.Items) != 0 {
			t.Fatalf("Bob's delegations after an administrator revoked it = %+v, want none", mine.Items)
		}
	}) {
		return
	}

	if !t.Run("Alice revokes the agent", func(t *testing.T) {
		var mine types.AgentDelegationList
		alice.obotJSON(t, http.MethodGet, "/api/me/agent-delegations", nil, &mine)
		if len(mine.Items) != 1 || mine.Items[0].ID != aliceSession.delegationID || mine.Items[0].LastUsedAt == nil {
			t.Fatalf("Alice's delegations = %+v, want the agent's, used", mine.Items)
		}
		if resp := alice.obot(t, http.MethodDelete, "/api/me/agent-delegations/"+aliceSession.delegationID, nil, nil); resp.StatusCode != http.StatusNoContent {
			t.Fatalf("revoke = %d %s, want 204", resp.StatusCode, resp.Body)
		}
		revokedAt := time.Now()

		// Delegation checks are trusted for 15 seconds on other replicas. This one refuses the token at once, on the
		// session that's open too.
		cacheWindow := 15 * time.Second
		if _, err := whoami(t.Context(), mcpSession); err == nil {
			t.Fatalf("the agent's open MCP session still works for Alice after she revoked it")
		}
		resp := initialize(t, owner.BaseURL, vmcp.ID, aliceSession.token)
		if resp.StatusCode != http.StatusUnauthorized || !strings.Contains(string(resp.Body), "agent_delegation_invalid") {
			t.Fatalf("connect for Alice after the revocation = %d %s, want 401 agent_delegation_invalid", resp.StatusCode, resp.Body)
		}
		if elapsed := time.Since(revokedAt); elapsed > cacheWindow {
			t.Fatalf("the token was refused %s after the revocation, want within %s", elapsed, cacheWindow)
		}
		if resp := fake.mint(t, aliceSession.delegationID); resp.StatusCode != http.StatusNotFound {
			t.Fatalf("mint from the revoked delegation = %d %s, want 404", resp.StatusCode, resp.Body)
		}

		// What she set up through the agent is gone.
		eventually(t, "Alice's setup through the agent to be deleted", func() error {
			if n := pairInstances(t, pairOwner, vmcp.ID); n != 0 {
				return fmt.Errorf("%d vMCP instances of %s remain", n, pairOwner)
			}
			return nil
		})
		if resp := alice.obot(t, http.MethodGet, "/api/me/agent-delegations/"+aliceSession.delegationID+"/setup", nil, nil); resp.StatusCode != http.StatusForbidden {
			t.Fatalf("the revoked delegation's setup = %d %s, want 403", resp.StatusCode, resp.Body)
		}
	}) {
		return
	}
}

// startUpstream starts the remote MCP server of the vMCP's component. Its whoami tool reports the user's value and
// the caller attribution it received, and purge is a tool only the users' profile grants, never the agent's.
func startUpstream(t *testing.T) *httptest.Server {
	t.Helper()
	server := gomcp.NewServer(&gomcp.Implementation{Name: "notes", Version: "test"}, nil)
	server.AddTool(&gomcp.Tool{Name: "whoami", InputSchema: map[string]any{"type": "object"}}, func(_ context.Context, req *gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
		header := req.GetExtra().Header
		body, err := json.Marshal(map[string]string{
			"token":      header.Get(tokenHeader),
			"user":       header.Get(string(types.ObotHeaderUserID)),
			"onBehalfOf": header.Get(string(types.ObotHeaderOnBehalfOfUserID)),
		})
		if err != nil {
			return nil, err
		}
		return &gomcp.CallToolResult{Content: []gomcp.Content{&gomcp.TextContent{Text: string(body)}}}, nil
	})
	server.AddTool(&gomcp.Tool{Name: "purge", InputSchema: map[string]any{"type": "object"}}, func(context.Context, *gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
		return &gomcp.CallToolResult{Content: []gomcp.Content{&gomcp.TextContent{Text: "purged"}}}, nil
	})
	upstream := httptest.NewServer(gomcp.NewStreamableHTTPHandler(func(*http.Request) *gomcp.Server { return server }, nil))
	t.Cleanup(upstream.Close)
	return upstream
}

// redacted is the session without its secret and token, for failure messages.
func (s agentSession) redacted() map[string]any {
	return map[string]any{"requestID": s.requestID, "delegationID": s.delegationID, "hasToken": s.token != "", "user": s.user}
}

// startFakeAgent starts the agent's web app. Its key is set once an administrator has created it.
func startFakeAgent(t *testing.T, obotURL string) *fakeAgent {
	t.Helper()
	a := &fakeAgent{obotURL: obotURL, sessions: map[string]*agentSession{}}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /connect", a.connect)
	mux.HandleFunc("GET /callback", a.callback)
	a.server = httptest.NewServer(mux)
	t.Cleanup(a.server.Close)
	return a
}

// host is the fake agent's host and port, its allowed return host. A return URL may use plain http to a loopback
// address.
func (a *fakeAgent) host() string {
	return strings.TrimPrefix(a.server.URL, "http://")
}

// session returns the session of the browser that made req, and starts one if it has none.
func (a *fakeAgent) session(rw http.ResponseWriter, req *http.Request) *agentSession {
	a.mu.Lock()
	defer a.mu.Unlock()
	if cookie, err := req.Cookie(agentSessionCookie); err == nil {
		if session, ok := a.sessions[cookie.Value]; ok {
			return session
		}
	}
	id := rand.Text()
	session := &agentSession{}
	a.sessions[id] = session
	http.SetCookie(rw, &http.Cookie{Name: agentSessionCookie, Value: id, Path: "/", HttpOnly: true})
	return session
}

// connect is where the agent sends a user who must approve it: it keeps a new secret in the browser's session, asks
// Obot for an approval link with the secret's hash, and redirects the browser to it.
func (a *fakeAgent) connect(rw http.ResponseWriter, req *http.Request) {
	session := a.session(rw, req)
	secret := rand.Text() + rand.Text()
	digest := sha256.Sum256([]byte(secret))
	a.mu.Lock()
	// A session that holds a delegation asks to have it approved again, so that only its user can.
	delegationID := session.delegationID
	a.mu.Unlock()
	resp := a.call(req.Context(), "/api/agent-delegations", types.AgentDelegationRequestCreate{
		ReturnURL:    a.server.URL + "/callback",
		SecretHash:   base64.RawURLEncoding.EncodeToString(digest[:]),
		DelegationID: delegationID,
	})
	if resp.StatusCode != http.StatusCreated {
		http.Error(rw, string(resp.Body), resp.StatusCode)
		return
	}
	var created types.AgentDelegationRequestCreated
	if err := json.Unmarshal(resp.Body, &created); err != nil {
		http.Error(rw, err.Error(), http.StatusBadGateway)
		return
	}
	a.mu.Lock()
	session.secret, session.requestID = secret, created.RequestID
	a.mu.Unlock()
	http.Redirect(rw, req, created.ApprovalURL, http.StatusFound)
}

// callback is the agent's return URL. It redeems the code with the secret of the session of the browser that came
// back, whichever session that is, and relays Obot's refusal.
func (a *fakeAgent) callback(rw http.ResponseWriter, req *http.Request) {
	session := a.session(rw, req)
	query := req.URL.Query()
	if query.Get("error") != "" {
		http.Error(rw, query.Get("error"), http.StatusForbidden)
		return
	}
	a.mu.Lock()
	a.code = query.Get("code")
	secret := session.secret
	a.mu.Unlock()

	resp := a.call(req.Context(), "/api/agent-delegations/token", types.AgentDelegationTokenRequest{
		GrantType: types.AgentDelegationGrantCode,
		RequestID: query.Get("requestID"),
		Code:      query.Get("code"),
		Secret:    secret,
	})
	if resp.StatusCode != http.StatusOK {
		http.Error(rw, string(resp.Body), resp.StatusCode)
		return
	}
	var token types.AgentDelegationToken
	if err := json.Unmarshal(resp.Body, &token); err != nil {
		http.Error(rw, err.Error(), http.StatusBadGateway)
		return
	}
	a.mu.Lock()
	session.delegationID, session.token, session.user = token.DelegationID, token.AccessToken, token.User
	a.mu.Unlock()
	_, _ = rw.Write([]byte("connected"))
}

// call POSTs body to Obot's path with the agent's key.
func (a *fakeAgent) call(ctx context.Context, path string, body any) harness.Response {
	data, err := json.Marshal(body)
	if err != nil {
		return harness.Response{StatusCode: http.StatusInternalServerError, Body: []byte(err.Error())}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.obotURL+path, bytes.NewReader(data))
	if err != nil {
		return harness.Response{StatusCode: http.StatusInternalServerError, Body: []byte(err.Error())}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.key)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return harness.Response{StatusCode: http.StatusBadGateway, Body: []byte(err.Error())}
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	return harness.Response{StatusCode: resp.StatusCode, Header: resp.Header, Body: respBody}
}

// redeem redeems a code as the agent's backend does, with secret.
func (a *fakeAgent) redeem(t *testing.T, requestID, code, secret string) harness.Response {
	t.Helper()
	return a.call(t.Context(), "/api/agent-delegations/token", types.AgentDelegationTokenRequest{
		GrantType: types.AgentDelegationGrantCode,
		RequestID: requestID,
		Code:      code,
		Secret:    secret,
	})
}

// mint gets a new token from a delegation, as the agent does when its token expires.
func (a *fakeAgent) mint(t *testing.T, delegationID string) harness.Response {
	t.Helper()
	return a.call(t.Context(), "/api/agent-delegations/token", types.AgentDelegationTokenRequest{
		GrantType:    types.AgentDelegationGrantDelegation,
		DelegationID: delegationID,
	})
}

// lastCode is the code the last browser brought back.
func (a *fakeAgent) lastCode() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.code
}

// sessionOf returns a copy of the agent's session with b.
func (a *fakeAgent) sessionOf(t *testing.T, b *browser) agentSession {
	t.Helper()
	agentURL, err := url.Parse(a.server.URL)
	if err != nil {
		t.Fatal(err)
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, cookie := range b.HTTP.Jar.Cookies(agentURL) {
		if session, ok := a.sessions[cookie.Value]; ok && cookie.Name == agentSessionCookie {
			return *session
		}
	}
	t.Fatalf("the browser has no session with the agent")
	return agentSession{}
}

// newBrowser returns a signed-out browser, for a new local user named name, or for nobody in particular when name is
// empty.
func newBrowser(t *testing.T, owner *harness.Harness, name string) *browser {
	t.Helper()
	b := &browser{Harness: harness.NewSession(t)}
	if name != "" {
		b.email = fmt.Sprintf("%s-%s@integration.example.com", name, owner.RunID)
		owner.Post(t, "/api/local-auth/users", map[string]any{
			"email":                 b.email,
			"password":              memberPassword,
			"requirePasswordChange": false,
		}, nil)
	}
	return b
}

func (b *browser) signIn(t *testing.T) {
	t.Helper()
	status, body := b.SignIn(t, b.email, memberPassword)
	if status != http.StatusOK {
		t.Fatalf("%s sign-in = %d %s, want 200", b.email, status, body)
	}
	var me struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(body), &me); err != nil || me.ID == "" {
		t.Fatalf("%s's /api/me = %s, want an ID (%v)", b.email, body, err)
	}
	b.userID = me.ID
}

// startWithAgent opens the agent's page that asks for approval, and returns the approval URL it redirects to.
func (b *browser) startWithAgent(t *testing.T, a *fakeAgent) string {
	t.Helper()
	resp := b.visit(t, a.server.URL+"/connect")
	location := resp.Header.Get("Location")
	if resp.StatusCode != http.StatusFound || location == "" {
		t.Fatalf("the agent's connect page = %d %s, want a redirect to Obot", resp.StatusCode, resp.Body)
	}
	return location
}

// visit navigates to rawURL.
func (b *browser) visit(t *testing.T, rawURL string) harness.Response {
	t.Helper()
	return send(t, b.HTTP, http.MethodGet, rawURL, "", nil, nil)
}

// obot calls Obot's API from Obot's own pages, which a browser marks as same-origin unless header says otherwise.
func (b *browser) obot(t *testing.T, method, path string, body any, header http.Header) harness.Response {
	t.Helper()
	if header == nil {
		header = http.Header{"Sec-Fetch-Site": {"same-origin"}}
	}
	return send(t, b.HTTP, method, b.BaseURL+path, "", body, header)
}

// obotJSON is obot, which must succeed, decoding the response into out.
func (b *browser) obotJSON(t *testing.T, method, path string, body, out any) {
	t.Helper()
	resp := b.obot(t, method, path, body, nil)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		t.Fatalf("%s %s = %d %s", method, path, resp.StatusCode, resp.Body)
	}
	decode(t, resp, out)
}

// send makes a request with client, as bearer when it is set, and returns the response read in full.
func send(t *testing.T, client *http.Client, method, rawURL, bearer string, body any, header http.Header) harness.Response {
	t.Helper()
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(data)
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, rawURL, reader)
	if err != nil {
		t.Fatal(err)
	}
	maps.Copy(req.Header, header)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, rawURL, err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return harness.Response{StatusCode: resp.StatusCode, Header: resp.Header, Body: respBody}
}

func requireStatus(t *testing.T, baseURL, method, path, bearer string, want int) {
	t.Helper()
	if resp := send(t, http.DefaultClient, method, baseURL+path, bearer, nil, nil); resp.StatusCode != want {
		t.Fatalf("%s %s = %d %s, want %d", method, path, resp.StatusCode, resp.Body, want)
	}
}

// initialize sends an MCP initialize to the vMCP as bearer, and returns the response unread by any MCP client.
func initialize(t *testing.T, baseURL, vmcpID, bearer string) harness.Response {
	t.Helper()
	return send(t, http.DefaultClient, http.MethodPost, baseURL+"/mcp-connect/"+vmcpID, bearer, map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]any{
			"protocolVersion": "2025-06-18",
			"capabilities":    map[string]any{},
			"clientInfo":      map[string]any{"name": "agent-delegation-integration-test", "version": "0"},
		},
	}, http.Header{"Accept": {"application/json, text/event-stream"}})
}

func (a agentTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.Header.Set("Authorization", "Bearer "+a.token)
	req.Header.Set(string(types.ObotHeaderOnBehalfOfUserID), a.forged)
	return http.DefaultTransport.RoundTrip(req)
}

// connect opens an MCP session to the vMCP at endpoint with the agent's token for a user.
func connect(t *testing.T, endpoint, token, forged string) (*gomcp.ClientSession, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	client := gomcp.NewClient(&gomcp.Implementation{Name: "fake-agent", Version: "0"}, nil)
	return client.Connect(ctx, &gomcp.StreamableClientTransport{
		Endpoint:             endpoint,
		HTTPClient:           &http.Client{Transport: agentTransport{token: token, forged: "forged-" + forged}},
		MaxRetries:           -1,
		DisableStandaloneSSE: true,
	}, nil)
}

// whoami calls the upstream's whoami tool through session, and returns what the upstream received.
func whoami(ctx context.Context, session *gomcp.ClientSession) (map[string]string, error) {
	result, err := session.CallTool(ctx, &gomcp.CallToolParams{Name: "whoami"})
	if err != nil {
		return nil, err
	}
	if result.IsError || len(result.Content) != 1 {
		return nil, fmt.Errorf("whoami failed: %s", contentText(result))
	}
	var seen map[string]string
	return seen, json.Unmarshal([]byte(contentText(result)), &seen)
}

func contentText(result *gomcp.CallToolResult) string {
	var texts []string
	for _, content := range result.Content {
		if text, ok := content.(*gomcp.TextContent); ok {
			texts = append(texts, text.Text)
		}
	}
	return strings.Join(texts, "\n")
}

// pairInstances counts the vMCP instances of vmcpID owned by owner, which no API shows.
func pairInstances(t *testing.T, owner, vmcpID string) int {
	t.Helper()
	var instances v1.VMCPInstanceList
	if err := storage.List(t.Context(), &instances, kclient.InNamespace(system.DefaultNamespace)); err != nil {
		t.Fatalf("list vMCP instances: %v", err)
	}
	var n int
	for _, instance := range instances.Items {
		if instance.Spec.UserID == owner && instance.Spec.Manifest.VMCPID == vmcpID && instance.DeletionTimestamp.IsZero() {
			n++
		}
	}
	return n
}

// requestIDOf returns the request ID of approvalURL, which must be Obot's approval page. Obot's server URL names it
// localhost, and the test reaches it at 127.0.0.1, so only the port and the path are compared.
func requestIDOf(t *testing.T, obotURL, approvalURL string) string {
	t.Helper()
	page, err := url.Parse(approvalURL)
	if err != nil {
		t.Fatal(err)
	}
	obot, err := url.Parse(obotURL)
	if err != nil {
		t.Fatal(err)
	}
	id, ok := strings.CutPrefix(page.Path, "/agents/approve/")
	if !ok || page.Port() != obot.Port() || !strings.HasPrefix(id, types.AgentDelegationRequestIDPrefix) || strings.Contains(id, "/") {
		t.Fatalf("approval URL %q isn't Obot's approval page for one request", approvalURL)
	}
	return id
}

func signIn(t *testing.T, email, password string) *harness.Harness {
	t.Helper()
	session := harness.NewSession(t)
	if status, body := session.SignIn(t, email, password); status != http.StatusOK {
		t.Fatalf("%s sign-in = %d %s, want 200", email, status, body)
	}
	return session
}

// waitForDefaultCatalog waits for the controller to create the default catalog, which it does after the server is
// healthy.
func waitForDefaultCatalog(t *testing.T, owner *harness.Harness) {
	t.Helper()
	eventually(t, "the default catalog", func() error {
		if status, body := owner.Request(t, http.MethodGet, "/api/mcp-catalogs/"+system.DefaultCatalog, nil); status != http.StatusOK {
			return fmt.Errorf("%d %s", status, body)
		}
		return nil
	})
}

func decode(t *testing.T, resp harness.Response, out any) {
	t.Helper()
	if out == nil || len(resp.Body) == 0 {
		return
	}
	if err := json.Unmarshal(resp.Body, out); err != nil {
		t.Fatalf("decode %s: %v", resp.Body, err)
	}
}

func jsonEqual(a, b any) bool {
	x, errX := json.Marshal(a)
	y, errY := json.Marshal(b)
	return errX == nil && errY == nil && bytes.Equal(x, y)
}

// eventually retries check until it succeeds or a minute passes.
func eventually(t *testing.T, what string, check func() error) {
	t.Helper()
	deadline := time.Now().Add(time.Minute)
	for {
		err := check()
		if err == nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("waiting for %s: %v", what, err)
		}
		time.Sleep(500 * time.Millisecond)
	}
}
