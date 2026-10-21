package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
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

func TestEgressAuditFilterOptions(t *testing.T) {
	client := newEnforcementTestGatewayClient(t)
	now := time.Date(2026, time.October, 21, 12, 0, 0, 0, time.UTC)
	for i := range 105 {
		require.NoError(t, client.SaveEgressAuditLog(t.Context(), types.EgressAuditLog{
			ID:            fmt.Sprintf("options-%03d", i),
			CreatedAt:     now,
			UserID:        "7",
			Host:          fmt.Sprintf("host-%03d.example", i),
			ClaudeSession: "session",
			ClaudeAccount: "account",
		}))
	}
	for _, tc := range []struct {
		name   string
		filter string
		query  string
		want   string
	}{
		{
			name:   "host options",
			filter: "host",
			query:  "?limit=2&offset=100",
			want:   `{"options":["host-000.example","host-001.example"]}`,
		},
		{
			name:   "session options",
			filter: "claude_session",
			want:   `{"options":["session"]}`,
		},
		{
			name:   "account options",
			filter: "claude_account",
			want:   `{"options":["account"]}`,
		},
		{
			name:   "filters applied",
			filter: "host",
			query:  "?host=host-001.example&claude_session=session&claude_account=account&user_id=7&start_time=2026-10-21T12:00:00Z&end_time=2026-10-21T12:00:00Z",
			want:   `{"options":["host-001.example"]}`,
		},
		{
			name:   "empty options array",
			filter: "host",
			query:  "?environment_id=missing",
			want:   `{"options":[]}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/api/egress-audit-logs/filter-options/"+tc.filter+tc.query, nil)
			request.SetPathValue("filter", tc.filter)
			recorder := httptest.NewRecorder()
			require.NoError(t, (&EgressAuditLogHandler{}).ListFilterOptions(api.Context{
				Request:        request,
				ResponseWriter: recorder,
				GatewayClient:  client,
			}))
			require.JSONEq(t, tc.want, recorder.Body.String())
		})
	}
	t.Run("limit above list cap", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodGet, "/api/egress-audit-logs/filter-options/host?limit=1000", nil)
		request.SetPathValue("filter", "host")
		recorder := httptest.NewRecorder()
		require.NoError(t, (&EgressAuditLogHandler{}).ListFilterOptions(api.Context{
			Request:        request,
			ResponseWriter: recorder,
			GatewayClient:  client,
		}))
		var response struct {
			Options []string `json:"options"`
		}
		require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
		require.Len(t, response.Options, 105)
	})
}

func TestEgressAuditFilterOptionsBadRequest(t *testing.T) {
	for _, tc := range []struct {
		name   string
		filter string
		query  string
	}{
		{
			name: "missing filter",
		},
		{
			name:   "unsupported filter",
			filter: "user_id",
		},
		{
			name:   "invalid start time",
			filter: "host",
			query:  "?start_time=invalid",
		},
		{
			name:   "invalid end time",
			filter: "host",
			query:  "?end_time=invalid",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/api/egress-audit-logs/filter-options/"+tc.filter+tc.query, nil)
			request.SetPathValue("filter", tc.filter)
			err := (&EgressAuditLogHandler{}).ListFilterOptions(api.Context{Request: request})
			var httpErr *apitypes.ErrHTTP
			require.ErrorAs(t, err, &httpErr)
			require.Equal(t, http.StatusBadRequest, httpErr.Code)
		})
	}
}

func TestParseEgressAuditLogMultiValueFilters(t *testing.T) {
	keys := []string{"user_id", "environment_id", "outcome", "claude_account", "claude_session", "host"}
	for _, tc := range []struct {
		name   string
		values []string
		want   []string
	}{
		{
			name:   "single value compatibility",
			values: []string{"alpha"},
			want:   []string{"alpha"},
		},
		{
			name:   "comma separated and repeated parameters",
			values: []string{"alpha, beta", "gamma", " , "},
			want:   []string{"alpha", "beta", "gamma"},
		},
		{
			name:   "empty selections",
			values: []string{"", " , "},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			query := url.Values{}
			for _, key := range keys {
				query[key] = tc.values
			}
			opts, err := parseEgressAuditLogOptions(query)
			require.NoError(t, err)
			for _, got := range [][]string{
				opts.UserID, opts.EnvironmentID, opts.Outcome, opts.ClaudeAccount, opts.ClaudeSession, opts.Host,
			} {
				require.Equal(t, tc.want, got)
			}
		})
	}
}
