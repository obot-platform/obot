package mcpgateway

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	mmmcpcomponent "github.com/obot-platform/mmmcp/component"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/agentprofile"
	"github.com/obot-platform/obot/pkg/api"
	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/mcp"
	"github.com/obot-platform/obot/pkg/mcp/auditlogs"
	"github.com/obot-platform/obot/pkg/principal"
	"github.com/obot-platform/obot/pkg/subjects"
	vmcpaccess "github.com/obot-platform/obot/pkg/vmcp"
	"github.com/stretchr/testify/require"
	"k8s.io/apiserver/pkg/authentication/user"
)

// agentUserInfo returns the principal of the active agent with id, as its authenticators build it.
func agentUserInfo(id uint) *user.DefaultInfo {
	return gatewayclient.AgentUserInfo(&gatewaytypes.Agent{ID: id}, []string{types.GroupMCP, types.GroupAuthenticated}, nil)
}

// onBehalfOfUserInfo returns the principal of the agent with agentID acting on behalf of the user with userID, as the
// authenticator of delegated tokens builds it.
func onBehalfOfUserInfo(t *testing.T, agentID, userID uint) *user.DefaultInfo {
	t.Helper()

	info := agentUserInfo(agentID)
	principal.RecordOnBehalfOf(info.Extra, userID)
	require.NoError(t, agentprofile.RecordDelegation(info.Extra, agentprofile.Delegation{
		ID:          "adl1test",
		User:        subjects.Caller{Principal: principal.User(userID)},
		Email:       "ann@example.com",
		DisplayName: "Ann",
	}))
	return info
}

// collectAndRead records entry through collect and returns the row persisted for it.
func collectAndRead(t *testing.T, gatewayClient *gatewayclient.Client, collect func(auditlogs.MCPAuditLog), entry auditlogs.MCPAuditLog) gatewaytypes.MCPAuditLog {
	t.Helper()

	collect(entry)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		logs, total, err := gatewayClient.GetMCPAuditLogs(t.Context(), gatewayclient.MCPAuditLogOptions{})
		require.NoError(t, err)
		if total == 1 {
			return logs[0]
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("the audit entry was not persisted")
	return gatewaytypes.MCPAuditLog{}
}

// TestMCPAuditRecordsTheUserFromTheCaller checks that a call an agent makes on behalf of a user is attributed to the
// user from the caller the gateway authenticated, and that nothing an MCP server submits can say that an agent acted
// for a user.
func TestMCPAuditRecordsTheUserFromTheCaller(t *testing.T) {
	newEntry := func() auditlogs.MCPAuditLog {
		return auditlogs.MCPAuditLog{
			Metadata:       map[string]string{"mcpID": "vmcp1test", mcp.AuditLogOnBehalfOf: "8"},
			CreatedAt:      time.Now().UTC(),
			CallType:       "tools/call",
			CallIdentifier: "search",
			RequestBody:    json.RawMessage(`{}`),
			ResponseBody:   json.RawMessage(`{}`),
			ResponseStatus: 200,
		}
	}

	t.Run("the gateway's entry for an agent acting on behalf of a user", func(t *testing.T) {
		gatewayClient := newLocalAgentAuditLogTestGatewayClient(t)
		audit := &proxyAudit{entry: newEntry()}
		audit.setCaller(onBehalfOfUserInfo(t, 5, 7))
		require.Equal(t, "agent:5", audit.entry.Subject)
		require.Equal(t, "7", audit.entry.Metadata[mcp.AuditLogOnBehalfOf], "the caller's user replaces any other value")

		handler := NewAuditLogHandler(gatewayClient)
		row := collectAndRead(t, gatewayClient, func(entry auditlogs.MCPAuditLog) {
			handler.CollectMCPProxyAuditEntry(entry, true, "", "")
		}, audit.entry)
		require.Equal(t, "agent", row.PrincipalKind)
		require.Equal(t, "5", row.PrincipalID)
		require.Empty(t, row.UserID, "an agent's row never counts against the user")
		require.Equal(t, "7", row.OnBehalfOf)
	})

	t.Run("the gateway's entry for an agent acting for itself", func(t *testing.T) {
		gatewayClient := newLocalAgentAuditLogTestGatewayClient(t)
		audit := &proxyAudit{entry: newEntry()}
		audit.setCaller(agentUserInfo(5))
		require.NotContains(t, audit.entry.Metadata, mcp.AuditLogOnBehalfOf)

		handler := NewAuditLogHandler(gatewayClient)
		row := collectAndRead(t, gatewayClient, func(entry auditlogs.MCPAuditLog) {
			handler.CollectMCPProxyAuditEntry(entry, true, "", "")
		}, audit.entry)
		require.Equal(t, "5", row.PrincipalID)
		require.Empty(t, row.OnBehalfOf)
	})

	t.Run("an MCP server's entry", func(t *testing.T) {
		gatewayClient := newLocalAgentAuditLogTestGatewayClient(t)
		entry := newEntry()
		entry.Subject = "agent:5"
		handler := NewAuditLogHandler(gatewayClient)
		row := collectAndRead(t, gatewayClient, handler.CollectMCPAuditEntry, entry)
		require.Equal(t, "5", row.PrincipalID)
		require.Empty(t, row.OnBehalfOf)
		require.Equal(t, "8", entry.Metadata[mcp.AuditLogOnBehalfOf], "the submitted entry is not changed")
	})
}

// TestHeadlessFailuresOnBehalfOfAUser checks that a component asking an agent acting on behalf of a user for OAuth that
// the vMCP's preconfigured OAuth does not cover, or for a value the user enters, is reported as needing the user's
// approval, naming the vMCP, while preconfigured OAuth that no longer works is still an administrator's to fix.
func TestHeadlessFailuresOnBehalfOfAUser(t *testing.T) {
	cfg := mcp.ServerConfig{
		Runtime: types.RuntimeVMCP,
		Components: []mcp.ComponentServer{
			{ComponentID: "granted", Name: "ms1granted", URL: "http://localhost/mcp-connect/msi1granted", OAuthGrant: &mcp.OAuthGrantRef{ComponentID: "granted", Owner: principal.PreconfiguredOAuthOwner("vmcp1agent"), MCPID: "vmcp1agent/granted"}},
			{ComponentID: "ungranted", Name: "ms1ungranted", URL: "http://localhost/mcp-connect/msi1ungranted"},
		},
		AuditLogMetadata: map[string]string{mcp.AuditLogVMCPID: "vmcp1agent"},
	}
	req := api.Context{Request: httptest.NewRequest(http.MethodPost, "/mcp-connect/vmcp1agent", nil), User: onBehalfOfUserInfo(t, 7, 9)}
	failure := func(response *mcpResponse) vmcpaccess.AgentReadinessFailure {
		t.Helper()
		require.Equal(t, http.StatusConflict, response.status)
		require.Empty(t, response.headers.Values("WWW-Authenticate"))
		var failure vmcpaccess.AgentReadinessFailure
		require.NoError(t, json.Unmarshal(response.body, &failure))
		return failure
	}

	response, err := agentOAuthRequiredResponse(req, cfg, []*mmmcpcomponent.AuthorizationError{{URL: "http://localhost/mcp-connect/msi1ungranted", StatusCode: http.StatusUnauthorized}})
	require.NoError(t, err)
	require.Equal(t, vmcpaccess.AgentReadinessFailure{
		Error:   vmcpaccess.AgentUserApprovalRequiredError,
		VMCPID:  "vmcp1agent",
		Reasons: []vmcpaccess.ReadinessReason{{ComponentID: "ungranted", Code: types.AgentReadinessOAuthRequired}},
	}, failure(response))

	response, err = agentOAuthRequiredResponse(req, cfg, []*mmmcpcomponent.AuthorizationError{
		{URL: "http://localhost/mcp-connect/msi1granted", StatusCode: http.StatusUnauthorized},
		{URL: "http://localhost/mcp-connect/msi1ungranted", StatusCode: http.StatusUnauthorized},
	})
	require.NoError(t, err)
	require.Equal(t, vmcpaccess.AgentReadinessFailure{
		Error:   vmcpaccess.AgentOAuthRequiredError,
		VMCPID:  "vmcp1agent",
		Reasons: []vmcpaccess.ReadinessReason{{ComponentID: "granted", Code: types.AgentReadinessOAuthInvalid}},
	}, failure(response))

	require.Equal(t, vmcpaccess.AgentReadinessFailure{
		Error:   vmcpaccess.AgentUserApprovalRequiredError,
		Reasons: []vmcpaccess.ReadinessReason{{ComponentID: "ungranted", Code: types.AgentReadinessConfigurationRequired, Field: "TOKEN"}},
	}, failure(agentConfigurationRequiredResponse(req, "ungranted", []string{"TOKEN"})))

	// The agent acting for itself is told as before.
	req.User = agentUserInfo(7)
	response, err = agentOAuthRequiredResponse(req, cfg, []*mmmcpcomponent.AuthorizationError{{URL: "http://localhost/mcp-connect/msi1ungranted", StatusCode: http.StatusUnauthorized}})
	require.NoError(t, err)
	require.Equal(t, vmcpaccess.AgentReadinessFailure{
		Error:   vmcpaccess.AgentOAuthRequiredError,
		Reasons: []vmcpaccess.ReadinessReason{{ComponentID: "ungranted", Code: types.AgentReadinessOAuthRequired}},
	}, failure(response))
}
