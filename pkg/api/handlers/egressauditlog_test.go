package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	apitypes "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/stretchr/testify/require"
	"k8s.io/apiserver/pkg/authentication/user"
)

func TestEgressAuditEnvironmentDirectory(t *testing.T) {
	client := newEnforcementTestGatewayClient(t)
	environment := &types.ClaudeEnvironment{
		ID:             "restricted",
		DisplayName:    "Restricted environment",
		EnvironmentID:  "ccpool_private",
		OrganizationID: "private-org",
		Profiles:       []types.ClaudeEnvironmentProfile{},
		MCPServerIDs:   []string{"private-vmcp"},
	}
	require.NoError(t, client.SaveClaudeEnvironment(t.Context(), environment, map[string]string{"key": "private-key"}))
	request := httptest.NewRequest(http.MethodGet, "/api/egress-audit-logs/environments", nil)
	recorder := httptest.NewRecorder()
	handler := &EgressAuditLogHandler{}
	require.NoError(t, handler.Environments(api.Context{
		Request:        request,
		ResponseWriter: recorder,
		GatewayClient:  client,
		User:           &user.DefaultInfo{UID: "7", Groups: []string{apitypes.GroupAuditor}},
	}))
	require.JSONEq(t, `{"restricted":"Restricted environment"}`, recorder.Body.String())
}

func TestEgressAuditHandlerRequiresAuditorForPayloads(t *testing.T) {
	client := newEnforcementTestGatewayClient(t)
	require.NoError(t, client.SaveEgressAuditLog(t.Context(), types.EgressAuditLog{
		ID: "test", CreatedAt: time.Now().UTC(), UserID: "7", RequestBody: json.RawMessage(`"cmVxdWVzdA=="`), ResponseBody: json.RawMessage(`"cmVzcG9uc2U="`), RequestHeaders: json.RawMessage(`{"content-type":["text/plain"]}`),
	}))
	for _, tc := range []struct {
		name    string
		groups  []string
		payload bool
	}{
		{
			name:   "admin",
			groups: []string{apitypes.GroupAdmin},
		},
		{
			name:   "owner",
			groups: []string{apitypes.GroupOwner},
		},
		{
			name:    "auditor",
			groups:  []string{apitypes.GroupAdmin, apitypes.GroupAuditor},
			payload: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/api/egress-audit-logs/test?withSensitiveFields=true", nil)
			request.SetPathValue("audit_log_id", "test")
			recorder := httptest.NewRecorder()
			ctx := api.Context{Request: request, ResponseWriter: recorder, GatewayClient: client, User: &user.DefaultInfo{UID: "7", Groups: tc.groups}}
			handler := &EgressAuditLogHandler{}
			require.NoError(t, handler.Get(ctx))
			var data map[string]any
			require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &data))
			if tc.payload {
				require.Contains(t, data, "requestBody")
				require.Contains(t, data, "responseBody")
			} else {
				require.NotContains(t, data, "requestBody")
				require.NotContains(t, data, "responseBody")
				require.NotContains(t, data, "requestHeaders")
			}
			ctx.ResponseWriter = httptest.NewRecorder()
			require.NoError(t, handler.List(ctx))
			require.NotContains(t, ctx.ResponseWriter.(*httptest.ResponseRecorder).Body.String(), "requestBody", "list never returns payloads, even for auditors")
		})
	}
}
