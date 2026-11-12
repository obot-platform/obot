package mcpgateway

import (
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/mcp"
	"github.com/obot-platform/obot/pkg/mcp/auditlogs"
	"github.com/obot-platform/obot/pkg/principal"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/stretchr/testify/require"
	"k8s.io/apiserver/pkg/authentication/user"
)

// TestCompositeConfigIDIsPerPrincipal checks that mmmcp keeps the component sessions and catalogs of a user and an
// agent with the same numeric ID apart, even were they to name the same connection.
func TestCompositeConfigIDIsPerPrincipal(t *testing.T) {
	userConfig := mcp.ServerConfig{MCPServerName: "vmcpi1user", UserID: "5"}
	agentConfig := mcp.ServerConfig{MCPServerName: "vmcpi1agent", UserID: "agent:5"}
	require.NotEqual(t, compositeConfigID(userConfig), compositeConfigID(agentConfig))

	agentConfig.MCPServerName = userConfig.MCPServerName
	require.NotEqual(t, compositeConfigID(userConfig), compositeConfigID(agentConfig))
	require.Equal(t, compositeConfigID(agentConfig), compositeConfigID(mcp.ServerConfig{MCPServerName: "vmcpi1user", UserID: "agent:5"}))
}

// TestMCPClientSessionsArePerPrincipal checks that an MCP session opened by a user is never resumed by the agent with
// the user's numeric ID on the same vMCP, nor the other way around.
func TestMCPClientSessionsArePerPrincipal(t *testing.T) {
	storageClient := newMCPProxyTestStorage()
	userSpec := v1.MCPClientSessionSpec{MCPServerID: "vmcp1shared", UserID: "5", ConnectionID: "vmcpi1user"}
	agentSpec := v1.MCPClientSessionSpec{MCPServerID: "vmcp1shared", UserID: "agent:5", ConnectionID: "vmcpi1agent"}
	require.NotEqual(t, mcpClientSessionKey(userSpec, "session-1"), mcpClientSessionKey(agentSpec, "session-1"))

	for _, spec := range []v1.MCPClientSessionSpec{userSpec, agentSpec} {
		response := &http.Response{StatusCode: http.StatusOK, Header: http.Header{mcpSessionHeader: {"session-1"}}}
		require.NoError(t, saveMCPClientSession(t.Context(), storageClient, response, spec, true))
	}
	for _, tt := range []struct {
		name  string
		saved v1.MCPClientSessionSpec
		other v1.MCPClientSessionSpec
	}{
		{
			name:  "user",
			saved: userSpec,
			other: agentSpec,
		},
		{
			name:  "agent",
			saved: agentSpec,
			other: userSpec,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			session, err := loadMCPClientSession(t.Context(), storageClient, tt.saved, "session-1")
			require.NoError(t, err)
			require.Equal(t, tt.saved.UserID, session.Spec.UserID)

			// The other principal never resumes this principal's session, even naming this principal's connection.
			intruder := tt.other
			intruder.ConnectionID = tt.saved.ConnectionID
			_, err = loadMCPClientSession(t.Context(), storageClient, intruder, "session-1")
			require.True(t, errors.Is(err, errMCPSessionNotFound), "resumed another principal's session: %v", err)
		})
	}

	// A session saved by only one principal is not found for the other.
	only := v1.MCPClientSessionSpec{MCPServerID: "vmcp1shared", UserID: "5", ConnectionID: "vmcpi1user"}
	response := &http.Response{StatusCode: http.StatusOK, Header: http.Header{mcpSessionHeader: {"session-2"}}}
	require.NoError(t, saveMCPClientSession(t.Context(), storageClient, response, only, true))
	_, err := loadMCPClientSession(t.Context(), storageClient, agentSpec, "session-2")
	require.True(t, errors.Is(err, errMCPSessionNotFound), "the agent resumed the user's session: %v", err)
}

// TestProxyAuditStampsTheAuthenticatedPrincipal checks that the proxy attributes each entry to the principal it
// authenticated, rather than only to the subject in the server's audit metadata.
func TestProxyAuditStampsTheAuthenticatedPrincipal(t *testing.T) {
	agentExtra := map[string][]string{}
	principal.RecordKind(agentExtra, principal.KindAgent)
	userExtra := map[string][]string{}
	principal.RecordKind(userExtra, principal.KindUser)
	for _, tt := range []struct {
		name   string
		caller user.Info
		want   string
	}{
		{
			name:   "agent",
			caller: &user.DefaultInfo{UID: "agent:5", Extra: agentExtra},
			want:   "agent:5",
		},
		{
			name:   "user",
			caller: &user.DefaultInfo{UID: "5", Extra: userExtra},
			want:   "5",
		},
		{
			// A caller that is not a well-formed principal keeps the metadata's subject.
			name:   "caller without a recorded kind",
			caller: &user.DefaultInfo{UID: "system"},
			want:   "metadata-subject",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodPost, "http://obot.example/mcp-connect/vmcp1shared", strings.NewReader(`{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"search"}}`))
			require.NoError(t, err)
			collector := new(recordingProxyAuditCollector)
			audit, err := newProxyAudit(req, map[string]string{"mcpID": "vmcp1shared", "userID": "metadata-subject"}, collector)
			require.NoError(t, err)
			audit.setCaller(tt.caller)
			audit.recordRequest()
			require.Len(t, collector.entries, 1)
			require.Equal(t, tt.want, collector.entries[0].Subject)
		})
	}

	// A nil audit, which an ignored server has, accepts the caller.
	var audit *proxyAudit
	audit.setCaller(&user.DefaultInfo{UID: "agent:5", Extra: agentExtra})
}

func TestProxyAuditRecordsTheResolvedComponent(t *testing.T) {
	cfg := mcp.ServerConfig{Components: []mcp.ComponentServer{
		{ComponentID: "search", URL: "http://localhost:8080/mcp-connect/ms1search"},
		{ComponentID: "files", URL: "http://localhost:8080/mcp-connect/msi1files"},
	}}
	req, err := http.NewRequest(http.MethodPost, "http://obot.example/mcp-connect/vmcp1shared", strings.NewReader(`{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"call_tool"}}`))
	require.NoError(t, err)
	collector := new(recordingProxyAuditCollector)
	audit, err := newProxyAudit(req, map[string]string{"mcpID": "vmcp1shared"}, collector)
	require.NoError(t, err)

	audit.setComponent(cfg, "http://localhost:8080/mcp-connect/unknown")
	require.Empty(t, audit.entry.Metadata[mcp.AuditLogComponentID])
	audit.setComponent(cfg, "http://localhost:8080/mcp-connect/msi1files")
	require.Equal(t, "files", audit.entry.Metadata[mcp.AuditLogComponentID])
}

// TestCollectMCPAuditEntryRecordsVMCPAttribution follows an agent's call through a vMCP through the collector and
// checks that the row records the vMCP, the component and the profiles that granted the call.
func TestCollectMCPAuditEntryRecordsVMCPAttribution(t *testing.T) {
	gatewayClient := newLocalAgentAuditLogTestGatewayClient(t)
	handler := NewAuditLogHandler(gatewayClient)
	now := time.Now().UTC()
	metadata := map[string]string{
		"mcpID":                      "vmcp1shared",
		"userID":                     "agent:7",
		mcp.AuditLogVMCPID:           "vmcp1shared",
		mcp.AuditLogComponentID:      "search",
		mcp.AuditLogProfiles:         "vp1explicit,vp1wildcard",
		mcp.AuditLogFallbackProfiles: "",
	}
	handler.CollectMCPProxyAuditEntry(auditlogs.MCPAuditLog{
		Metadata:       metadata,
		Subject:        "agent:7",
		CreatedAt:      now,
		CallType:       "tools/call",
		CallIdentifier: "search",
		RequestID:      "1",
		RequestBody:    json.RawMessage(`{}`),
	}, false, "exchange-1", "")
	handler.CollectMCPProxyAuditEntry(auditlogs.MCPAuditLog{
		Metadata:       metadata,
		Subject:        "agent:7",
		CreatedAt:      now.Add(time.Second),
		CallType:       "tools/call",
		CallIdentifier: "search",
		RequestID:      "1",
		ResponseBody:   json.RawMessage(`{}`),
		ResponseStatus: 200,
	}, true, "exchange-1", "tools/call")

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		logs, total, err := gatewayClient.GetMCPAuditLogs(t.Context(), gatewayclient.MCPAuditLogOptions{UserID: []string{"agent:7"}})
		require.NoError(t, err)
		if total == 1 && logs[0].MCP().ResponseStatus == 200 {
			fields := logs[0].MCP()
			require.Equal(t, "vmcp1shared", fields.VMCPID)
			require.Equal(t, "search", fields.VMCPComponentID)
			require.Equal(t, []string{"vp1explicit", "vp1wildcard"}, []string(fields.ProfileIDs))
			require.Empty(t, fields.FallbackProfileIDs)
			require.Equal(t, "agent", logs[0].PrincipalKind)
			require.Empty(t, logs[0].UserID)
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("the agent's call was not persisted")
}

func TestAuditProfileIDs(t *testing.T) {
	for _, tt := range []struct {
		value string
		want  []string
	}{
		{
			value: "",
		},
		{
			value: "vp1one",
			want:  []string{"vp1one"},
		},
		{
			value: "vp1one,,vp1two, ",
			want:  []string{"vp1one", "vp1two"},
		},
	} {
		if got := auditProfileIDs(tt.value); !slices.Equal(got, tt.want) {
			t.Errorf("auditProfileIDs(%q) = %v, want %v", tt.value, got, tt.want)
		}
	}
}

// TestCompositeConfigNeverDefersAuthorizationForAgents checks that an agent is never offered sign-in links, even with
// a group that only Claude environments have, which no authenticator of agents grants.
func TestCompositeConfigNeverDefersAuthorizationForAgents(t *testing.T) {
	extra := map[string][]string{}
	principal.RecordKind(extra, principal.KindAgent)
	caller := &user.DefaultInfo{UID: "agent:7", Groups: []string{types.GroupMCP, types.GroupClaudeEnvironment}, Extra: extra}
	cfg, err := compositeConfig(api.Context{User: caller}, mcp.ServerConfig{})
	require.NoError(t, err)
	require.False(t, cfg.DeferAuthorization)
}
