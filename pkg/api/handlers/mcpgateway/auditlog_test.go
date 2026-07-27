package mcpgateway

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaydb "github.com/obot-platform/obot/pkg/gateway/db"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/storage"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	sservices "github.com/obot-platform/obot/pkg/storage/services"
	"github.com/obot-platform/obot/pkg/system"
	"gorm.io/gorm"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apiserver/pkg/authentication/user"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestAuditLogInputUnmarshalFlatMCPFields(t *testing.T) {
	var input auditLogInput
	if err := json.Unmarshal([]byte(`{
		"metadata":{"mcpID":"mcp-from-metadata"},
		"subject":"user-1",
		"mcpID":"mcp-1",
		"callType":"tools/call",
		"callIdentifier":"tool",
		"requestBody":{"name":"tool"}
	}`), &input); err != nil {
		t.Fatalf("unmarshal audit log input: %v", err)
	}

	if input.Subject != "user-1" {
		t.Fatalf("expected subject user-1, got %q", input.Subject)
	}
	if input.Metadata["mcpID"] != "mcp-from-metadata" {
		t.Fatalf("expected metadata to be preserved, got %#v", input.Metadata)
	}
	if input.MCP().MCPID != "mcp-1" || input.MCP().CallIdentifier != "tool" {
		t.Fatalf("expected flat MCP fields to be preserved, got %#v", input.MCP())
	}
}

func TestAuditLogInputUnmarshalIgnoresNestedMCPFields(t *testing.T) {
	var input auditLogInput
	if err := json.Unmarshal([]byte(`{
		"metadata":{"mcpID":"mcp-from-metadata"},
		"mcpFields":{
			"mcpID":"nested-mcp",
			"callIdentifier":"nested-tool"
		}
	}`), &input); err != nil {
		t.Fatalf("unmarshal audit log input: %v", err)
	}

	if input.MCP() == nil {
		t.Fatal("expected flat MCP field group to be initialized")
	}
	if input.MCP().MCPID != "" || input.MCP().CallIdentifier != "" {
		t.Fatalf("expected nested MCP fields to be ignored, got %#v", input.MCP())
	}
}

func TestLocalAgentAuditLogSubmitAuthenticatedBatchSucceeds(t *testing.T) {
	gatewayClient := newLocalAgentAuditLogTestGatewayClient(t)
	occurredAt := time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC)

	beforeSubmit := time.Now().UTC()
	handler := NewLocalAgentAuditLogHandler()
	err := handler.Submit(newLocalAgentAuditLogTestContext(t, gatewayClient, []types.LocalAgentToolCallAuditLogInput{
		validLocalAgentAuditLogInput(occurredAt, "entry-1", types.AuditLogOutcomeStatusSuccess),
		validLocalAgentAuditLogInput(occurredAt.Add(time.Second), "entry-2", types.AuditLogOutcomeStatusFailure),
	}, &user.DefaultInfo{
		UID:    "42",
		Name:   "alice@example.com",
		Groups: []string{types.GroupAuthenticated, types.GroupDeviceScans},
	}))
	if err != nil {
		t.Fatalf("submit local-agent audit logs: %v", err)
	}
	afterSubmit := time.Now().UTC()

	got, err := gatewayClient.GetMCPAuditLog(t.Context(), 1, true)
	if err != nil {
		t.Fatalf("get inserted local-agent audit log: %v", err)
	}
	if got.SourceType != types.AuditLogSourceTypeLocalAgentToolCall {
		t.Fatalf("source type = %q, want local-agent", got.SourceType)
	}
	if got.CreatedAt.Before(beforeSubmit) || got.CreatedAt.After(afterSubmit) {
		t.Fatalf("createdAt = %s, want between %s and %s", got.CreatedAt, beforeSubmit, afterSubmit)
	}
	if got.UserID != "42" {
		t.Fatalf("userID = %q, want authenticated user", got.UserID)
	}
	if got.ClientIP != "203.0.113.10" {
		t.Fatalf("clientIP = %q, want trusted forwarded IP", got.ClientIP)
	}
	local := got.LocalAgentToolCallFields
	if local == nil {
		t.Fatal("expected local-agent fields")
	}
	if local.ActorType != types.AuditLogActorTypeUser || local.ActorID != "42" {
		t.Fatalf("actor = %q/%q, want user/42", local.ActorType, local.ActorID)
	}
	if local.ReportedUserEmail != "reported@example.com" {
		t.Fatalf("reported email = %q, want untrusted reported metadata preserved", local.ReportedUserEmail)
	}
	if string(local.RequestBody) != `{"arg":true}` || string(local.ResponseBody) != `{"ok":true}` {
		t.Fatalf("unexpected payloads: request=%s response=%s", local.RequestBody, local.ResponseBody)
	}
}

func TestLocalAgentAuditLogSubmitDeviceJWTStampsDeviceAttribution(t *testing.T) {
	gatewayClient := newLocalAgentAuditLogTestGatewayClient(t)
	event := validLocalAgentAuditLogInput(time.Now().UTC(), "device-entry", types.AuditLogOutcomeStatusSuccess)
	handler := NewLocalAgentAuditLogHandler()

	err := handler.Submit(newLocalAgentAuditLogTestContext(t, gatewayClient, []types.LocalAgentToolCallAuditLogInput{event}, &user.DefaultInfo{
		UID:    "device:server-device",
		Name:   "device:server-device",
		Groups: []string{types.GroupAuthenticated, types.GroupDeviceScans},
		Extra: map[string][]string{
			"device_id":            {"server-device"},
			"mdm_configuration_id": {"123"},
		},
	}))
	if err != nil {
		t.Fatalf("submit local-agent audit logs as device: %v", err)
	}

	got, err := gatewayClient.GetMCPAuditLog(t.Context(), 1, true)
	if err != nil {
		t.Fatalf("get inserted local-agent audit log: %v", err)
	}
	if got.UserID != "" {
		t.Fatalf("userID = %q, want empty for device submitter", got.UserID)
	}
	local := got.LocalAgentToolCallFields
	if local == nil {
		t.Fatal("expected local-agent fields")
	}
	if local.ActorType != types.AuditLogActorTypeDevice || local.ActorID != "server-device" {
		t.Fatalf("actor = %q/%q, want device/server-device", local.ActorType, local.ActorID)
	}
	if local.DeviceID != "server-device" {
		t.Fatalf("deviceID = %q, want server-stamped device ID", local.DeviceID)
	}
	if local.DeviceDeploymentID != 123 {
		t.Fatalf("deviceDeploymentID = %d, want server-stamped deployment", local.DeviceDeploymentID)
	}
}

func TestLocalAgentAuditLogSubmitInvalidBatchDoesNotPartiallyPersist(t *testing.T) {
	gatewayClient := newLocalAgentAuditLogTestGatewayClient(t)
	handler := NewLocalAgentAuditLogHandler()
	occurredAt := time.Now().UTC()

	err := handler.Submit(newLocalAgentAuditLogTestContext(t, gatewayClient, []types.LocalAgentToolCallAuditLogInput{
		validLocalAgentAuditLogInput(occurredAt, "entry-1", types.AuditLogOutcomeStatusSuccess),
		validLocalAgentAuditLogInput(occurredAt, "", types.AuditLogOutcomeStatusSuccess),
	}, &user.DefaultInfo{
		UID:    "42",
		Groups: []string{types.GroupAuthenticated, types.GroupDeviceScans},
	}))
	if err == nil {
		t.Fatal("expected invalid batch to be rejected")
	}
	var errHTTP *types.ErrHTTP
	if !errors.As(err, &errHTTP) || errHTTP.Code != http.StatusBadRequest {
		t.Fatalf("expected bad request HTTP error, got %v", err)
	}

	_, err = gatewayClient.GetMCPAuditLog(t.Context(), 1, true)
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("expected no rows to be persisted, got err %v", err)
	}
}

func TestLocalAgentAuditLogSubmitRejectsRemovedLogsEnvelope(t *testing.T) {
	gatewayClient := newLocalAgentAuditLogTestGatewayClient(t)
	req := httptest.NewRequest(http.MethodPost, "/api/local-agent-audit-logs", strings.NewReader(`{"logs":[]}`))
	err := NewLocalAgentAuditLogHandler().Submit(api.Context{
		ResponseWriter: httptest.NewRecorder(),
		Request:        req,
		GatewayClient:  gatewayClient,
		User: &user.DefaultInfo{
			UID:    "42",
			Groups: []string{types.GroupAuthenticated, types.GroupDeviceScans},
		},
	})
	var errHTTP *types.ErrHTTP
	if !errors.As(err, &errHTTP) || errHTTP.Code != http.StatusBadRequest {
		t.Fatalf("expected removed logs envelope to be rejected, got %v", err)
	}
}

func TestLocalAgentAuditLogSubmitDuplicateIdempotencyKeyIsSuccessNoop(t *testing.T) {
	gatewayClient := newLocalAgentAuditLogTestGatewayClient(t)
	handler := NewLocalAgentAuditLogHandler()
	event := validLocalAgentAuditLogInput(time.Now().UTC(), "same-entry", types.AuditLogOutcomeStatusSuccess)
	reqUser := &user.DefaultInfo{
		UID:    "42",
		Groups: []string{types.GroupAuthenticated, types.GroupDeviceScans},
	}

	if err := handler.Submit(newLocalAgentAuditLogTestContext(t, gatewayClient, []types.LocalAgentToolCallAuditLogInput{event}, reqUser)); err != nil {
		t.Fatalf("first submit: %v", err)
	}
	event.Action.Name = "different-tool"
	if err := handler.Submit(newLocalAgentAuditLogTestContext(t, gatewayClient, []types.LocalAgentToolCallAuditLogInput{event}, reqUser)); err != nil {
		t.Fatalf("duplicate submit should succeed: %v", err)
	}

	got, err := gatewayClient.GetMCPAuditLog(t.Context(), 1, true)
	if err != nil {
		t.Fatalf("get original row: %v", err)
	}
	if got.LocalAgentToolCallFields.ActionName != "mcp__server__tool" {
		t.Fatalf("expected original row to remain unchanged, got tool %q", got.LocalAgentToolCallFields.ActionName)
	}
	_, err = gatewayClient.GetMCPAuditLog(t.Context(), 2, true)
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("expected duplicate to avoid second row, got err %v", err)
	}
}

func TestListAuditLogsLocalAgentRequiresPrivilege(t *testing.T) {
	gatewayClient := newLocalAgentAuditLogTestGatewayClient(t)
	seedLocalAgentAuditLog(t, gatewayClient, "entry-1")

	handler := NewAuditLogHandler(gatewayClient)

	// A normal (non-admin, non-auditor) user cannot list local-agent logs.
	normalCtx := newAuditLogListContext(t, gatewayClient, "event_type=local_agent_tool_call", &user.DefaultInfo{
		UID:    "42",
		Groups: []string{types.GroupAuthenticated, types.GroupAPI},
	})
	err := handler.ListAuditLogs(normalCtx)
	var errHTTP *types.ErrHTTP
	if !errors.As(err, &errHTTP) || errHTTP.Code != http.StatusForbidden {
		t.Fatalf("expected forbidden for normal user, got %v", err)
	}
}

func TestListAuditLogsRejectsSourceTypeQuery(t *testing.T) {
	gatewayClient := newLocalAgentAuditLogTestGatewayClient(t)
	handler := NewAuditLogHandler(gatewayClient)
	ctx := newAuditLogListContext(t, gatewayClient, "source_type=mcp", &user.DefaultInfo{
		UID:    "1",
		Groups: []string{types.GroupAuthenticated, types.GroupAdmin},
	})
	err := handler.ListAuditLogs(ctx)
	var errHTTP *types.ErrHTTP
	if !errors.As(err, &errHTTP) || errHTTP.Code != http.StatusBadRequest {
		t.Fatalf("expected bad request for source_type, got %v", err)
	}
}

func TestListAuditLogsLocalAgentAdminSeesMetadataNoPayload(t *testing.T) {
	gatewayClient := newLocalAgentAuditLogTestGatewayClient(t)
	seedLocalAgentAuditLog(t, gatewayClient, "entry-1")

	handler := NewAuditLogHandler(gatewayClient)
	rec := httptest.NewRecorder()
	adminCtx := newAuditLogListContextWithRecorder(t, gatewayClient, "event_type=local_agent_tool_call", &user.DefaultInfo{
		UID:    "1",
		Groups: []string{types.GroupAuthenticated, types.GroupAdmin},
	}, rec)

	if err := handler.ListAuditLogs(adminCtx); err != nil {
		t.Fatalf("admin list local-agent logs: %v", err)
	}

	var resp types.AuditLogEventResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Total != 1 || len(resp.Items) != 1 {
		t.Fatalf("expected one local-agent row, got total=%d len=%d", resp.Total, len(resp.Items))
	}
	event := resp.Items[0]
	if event.EventType != types.AuditLogEventTypeLocalAgentToolCall || event.Details != nil {
		t.Fatalf("expected normalized local-agent summary without details, got %#v", event)
	}
}

func TestListAuditLogsAdminGetsOneMixedPage(t *testing.T) {
	gatewayClient := newLocalAgentAuditLogTestGatewayClient(t)
	seedLocalAgentAuditLog(t, gatewayClient, "entry-1")
	gatewayClient.LogMCPAuditEntry(gatewaytypes.MCPAuditLog{
		CreatedAt:  time.Now().UTC().Add(time.Second),
		SourceType: types.AuditLogSourceTypeMCP,
		MCPFields: &gatewaytypes.MCPAuditLogFields{
			MCPID: "mcp-1", CallType: "tools/call", CallIdentifier: "search",
			RequestBody: json.RawMessage(`{}`), ResponseReceived: true, ResponseStatus: 200,
		},
	})
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		_, total, err := gatewayClient.GetMCPAuditLogs(t.Context(), gatewayclient.MCPAuditLogOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if total == 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	rec := httptest.NewRecorder()
	ctx := newAuditLogListContextWithRecorder(t, gatewayClient,
		"event_type=mcp_call,local_agent_tool_call", &user.DefaultInfo{
			UID: "1", Groups: []string{types.GroupAuthenticated, types.GroupAdmin},
		}, rec)
	if err := NewAuditLogHandler(gatewayClient).ListAuditLogs(ctx); err != nil {
		t.Fatal(err)
	}
	var response types.AuditLogEventResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Total != 2 || len(response.Items) != 2 {
		t.Fatalf("expected one mixed page containing two events, got total=%d items=%#v", response.Total, response.Items)
	}
	if response.Items[0].EventType != types.AuditLogEventTypeMCPCall || response.Items[1].EventType != types.AuditLogEventTypeLocalAgentToolCall {
		t.Fatalf("unexpected mixed event order: %#v", response.Items)
	}
}

// TestListAuditLogsDefaultsToAllAuthorizedSources verifies that when no event_type is specified an
// admin sees every source they are authorized for (both MCP and local-agent).
func TestListAuditLogsDefaultsToAllAuthorizedSources(t *testing.T) {
	gatewayClient := newLocalAgentAuditLogTestGatewayClient(t)
	seedLocalAgentAuditLog(t, gatewayClient, "entry-1")
	gatewayClient.LogMCPAuditEntry(gatewaytypes.MCPAuditLog{
		CreatedAt:  time.Now().UTC().Add(time.Second),
		SourceType: types.AuditLogSourceTypeMCP,
		MCPFields: &gatewaytypes.MCPAuditLogFields{
			MCPID: "mcp-1", CallType: "tools/call", CallIdentifier: "search",
			RequestBody: json.RawMessage(`{}`), ResponseReceived: true, ResponseStatus: 200,
		},
	})
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		_, total, err := gatewayClient.GetMCPAuditLogs(t.Context(), gatewayclient.MCPAuditLogOptions{
			SourceTypes: []types.AuditLogSourceType{
				types.AuditLogSourceTypeMCP, types.AuditLogSourceTypeLocalAgentToolCall,
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		if total == 2 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	rec := httptest.NewRecorder()
	// No event_type in the query string: the handler must default to all authorized sources.
	ctx := newAuditLogListContextWithRecorder(t, gatewayClient, "", &user.DefaultInfo{
		UID: "1", Groups: []string{types.GroupAuthenticated, types.GroupAdmin},
	}, rec)
	if err := NewAuditLogHandler(gatewayClient).ListAuditLogs(ctx); err != nil {
		t.Fatal(err)
	}
	var response types.AuditLogEventResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Total != 2 || len(response.Items) != 2 {
		t.Fatalf("expected both sources by default, got total=%d items=%#v", response.Total, response.Items)
	}
}

func TestListAuditLogsScopesBasicAndPowerUserPlus(t *testing.T) {
	tests := []struct {
		name      string
		groups    []string
		wantCalls []string
	}{
		{
			name:      "basic user sees only owned server",
			groups:    types.RoleBasic.Groups(),
			wantCalls: []string{"owned-call"},
		},
		{
			name:      "power user plus sees owned and workspace servers",
			groups:    types.RolePowerUserPlus.Groups(),
			wantCalls: []string{"workspace-call", "owned-call"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gatewayClient := newLocalAgentAuditLogTestGatewayClient(t)
			storageClient, logs := newAuditLogScopeTestFixture(t, "42")
			seedMCPAuditLogs(t, gatewayClient, logs...)

			recorder := httptest.NewRecorder()
			ctx := newScopedAuditLogTestContext(t, gatewayClient, storageClient, recorder, "", "", &user.DefaultInfo{
				UID:    "42",
				Groups: test.groups,
			})
			if err := NewAuditLogHandler(gatewayClient).ListAuditLogs(ctx); err != nil {
				t.Fatalf("list audit logs: %v", err)
			}

			var response types.AuditLogEventResponse
			if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if response.Total != int64(len(test.wantCalls)) || len(response.Items) != len(test.wantCalls) {
				t.Fatalf("got total=%d items=%d, want %d; response=%s", response.Total, len(response.Items), len(test.wantCalls), recorder.Body.String())
			}
			for i, wantCall := range test.wantCalls {
				if got := response.Items[i].Action.Name; got != wantCall {
					t.Fatalf("item %d action name = %q, want %q", i, got, wantCall)
				}
			}
		})
	}
}

func TestAuditLogFilterOptionsScopeBasicAndPowerUserPlus(t *testing.T) {
	tests := []struct {
		name        string
		groups      []string
		wantOptions []string
	}{
		{
			name:        "basic user gets options only from owned server",
			groups:      types.RoleBasic.Groups(),
			wantOptions: []string{"Owned Server"},
		},
		{
			name:        "power user plus gets options from owned and workspace servers",
			groups:      types.RolePowerUserPlus.Groups(),
			wantOptions: []string{"Owned Server", "Workspace Server"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gatewayClient := newLocalAgentAuditLogTestGatewayClient(t)
			storageClient, logs := newAuditLogScopeTestFixture(t, "42")
			seedMCPAuditLogs(t, gatewayClient, logs...)

			recorder := httptest.NewRecorder()
			ctx := newScopedAuditLogTestContext(t, gatewayClient, storageClient, recorder, "mcp_server_display_name", "", &user.DefaultInfo{
				UID:    "42",
				Groups: test.groups,
			})
			if err := NewAuditLogHandler(gatewayClient).ListAuditLogFilterOptions(ctx); err != nil {
				t.Fatalf("list filter options: %v", err)
			}

			var response struct {
				Options []string `json:"options"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if !slices.Equal(response.Options, test.wantOptions) {
				t.Fatalf("options = %v, want %v", response.Options, test.wantOptions)
			}
		})
	}
}

func TestGetAuditLogLocalAgentAccessByRole(t *testing.T) {
	gatewayClient := newLocalAgentAuditLogTestGatewayClient(t)
	seedLocalAgentAuditLog(t, gatewayClient, "entry-1")

	handler := NewAuditLogHandler(gatewayClient)

	// Auditor gets the decrypted payload.
	auditorRec := httptest.NewRecorder()
	auditorCtx := newAuditLogDetailContext(t, gatewayClient, "1", &user.DefaultInfo{
		UID:    "2",
		Groups: []string{types.GroupAuthenticated, types.GroupAuditor},
	}, auditorRec)
	if err := handler.GetAuditLog(auditorCtx); err != nil {
		t.Fatalf("auditor get local-agent log: %v", err)
	}
	var auditorLog types.AuditLogEvent
	if err := json.Unmarshal(auditorRec.Body.Bytes(), &auditorLog); err != nil {
		t.Fatalf("decode auditor response: %v", err)
	}
	if auditorLog.Details == nil || auditorLog.Details.Request == nil || string(auditorLog.Details.Request.Body) != `{"arg":true}` || auditorLog.Details.PayloadRedacted {
		t.Fatalf("expected auditor to see decrypted normalized payload, got %#v", auditorLog.Details)
	}

	// Admin (non-auditor) sees metadata but not the encrypted payload.
	adminRec := httptest.NewRecorder()
	adminCtx := newAuditLogDetailContext(t, gatewayClient, "1", &user.DefaultInfo{
		UID:    "1",
		Groups: []string{types.GroupAuthenticated, types.GroupAdmin},
	}, adminRec)
	if err := handler.GetAuditLog(adminCtx); err != nil {
		t.Fatalf("admin get local-agent log: %v", err)
	}
	var adminLog types.AuditLogEvent
	if err := json.Unmarshal(adminRec.Body.Bytes(), &adminLog); err != nil {
		t.Fatalf("decode admin response: %v", err)
	}
	if adminLog.Details == nil || !adminLog.Details.PayloadRedacted {
		t.Fatal("expected admin detail to explicitly report payload redaction")
	}
	if adminLog.Details.Request != nil && !isBlankJSON(adminLog.Details.Request.Body) {
		t.Fatalf("expected admin to be denied encrypted payload, got %#v", adminLog.Details)
	}

	// Normal user is denied entirely.
	normalCtx := newAuditLogDetailContext(t, gatewayClient, "1", &user.DefaultInfo{
		UID:    "42",
		Groups: []string{types.GroupAuthenticated, types.GroupAPI},
	}, httptest.NewRecorder())
	err := handler.GetAuditLog(normalCtx)
	var errHTTP *types.ErrHTTP
	if !errors.As(err, &errHTTP) || errHTTP.Code != http.StatusForbidden {
		t.Fatalf("expected forbidden for normal user, got %v", err)
	}
}

// isBlankJSON reports whether a JSON payload is empty or the literal null, i.e. a blanked field.
func isBlankJSON(m json.RawMessage) bool {
	s := strings.TrimSpace(string(m))
	return s == "" || s == "null"
}

func seedLocalAgentAuditLog(t *testing.T, gatewayClient *gatewayclient.Client, idempotencyKey string) {
	t.Helper()
	event := validLocalAgentAuditLogInput(time.Now().UTC(), idempotencyKey, types.AuditLogOutcomeStatusSuccess)
	event.Details.Environment.CWD = "/Users/alice/project"
	handler := NewLocalAgentAuditLogHandler()
	if err := handler.Submit(newLocalAgentAuditLogTestContext(t, gatewayClient, []types.LocalAgentToolCallAuditLogInput{event}, &user.DefaultInfo{
		UID:    "42",
		Groups: []string{types.GroupAuthenticated, types.GroupDeviceScans},
	})); err != nil {
		t.Fatalf("seed local-agent audit log: %v", err)
	}
}

func newAuditLogListContext(t *testing.T, gatewayClient *gatewayclient.Client, rawQuery string, u user.Info) api.Context {
	t.Helper()
	return newAuditLogListContextWithRecorder(t, gatewayClient, rawQuery, u, httptest.NewRecorder())
}

func newAuditLogListContextWithRecorder(t *testing.T, gatewayClient *gatewayclient.Client, rawQuery string, u user.Info, rec *httptest.ResponseRecorder) api.Context {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/mcp-audit-logs?"+rawQuery, nil)
	return api.Context{
		ResponseWriter: rec,
		Request:        req,
		GatewayClient:  gatewayClient,
		User:           u,
	}
}

func newAuditLogDetailContext(t *testing.T, gatewayClient *gatewayclient.Client, auditLogID string, u user.Info, rec *httptest.ResponseRecorder) api.Context {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/mcp-audit-logs/detail/"+auditLogID, nil)
	req.SetPathValue("audit_log_id", auditLogID)
	return api.Context{
		ResponseWriter: rec,
		Request:        req,
		GatewayClient:  gatewayClient,
		User:           u,
	}
}

func newScopedAuditLogTestContext(t *testing.T, gatewayClient *gatewayclient.Client, storageClient storage.Client, recorder *httptest.ResponseRecorder, filter, rawQuery string, u user.Info) api.Context {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, "/api/mcp-audit-logs?"+rawQuery, nil)
	if filter != "" {
		request.SetPathValue("filter", filter)
	}
	return api.Context{
		ResponseWriter: recorder,
		Request:        request,
		Storage:        storageClient,
		GatewayClient:  gatewayClient,
		User:           u,
	}
}

func newAuditLogScopeTestFixture(t *testing.T, userID string) (storage.Client, []gatewaytypes.MCPAuditLog) {
	t.Helper()
	workspaceID := system.GetPowerUserWorkspaceID(userID)
	objects := []kclient.Object{
		&v1.MCPServer{
			ObjectMeta: metav1.ObjectMeta{Name: "mcp-owned", Namespace: system.DefaultNamespace},
			Spec:       v1.MCPServerSpec{UserID: userID},
		},
		&v1.MCPServer{
			ObjectMeta: metav1.ObjectMeta{Name: "mcp-workspace", Namespace: system.DefaultNamespace},
			Spec: v1.MCPServerSpec{
				UserID:               userID,
				PowerUserWorkspaceID: workspaceID,
			},
		},
		&v1.MCPServer{
			ObjectMeta: metav1.ObjectMeta{Name: "mcp-unrelated", Namespace: system.DefaultNamespace},
			Spec:       v1.MCPServerSpec{UserID: "another-user"},
		},
	}
	storageClient := storage.Client(fake.NewClientBuilder().
		WithScheme(storagescheme.Scheme).
		WithIndex(&v1.MCPServer{}, "spec.userID", func(object kclient.Object) []string {
			server := object.(*v1.MCPServer)
			if server.Spec.UserID == "" {
				return nil
			}
			return []string{server.Spec.UserID}
		}).
		WithObjects(objects...).
		Build())

	base := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	logs := []gatewaytypes.MCPAuditLog{
		newAuditLogScopeTestRow(base, "mcp-owned", "Owned Server", "", "owned-call"),
		newAuditLogScopeTestRow(base.Add(time.Second), "mcp-workspace", "Workspace Server", workspaceID, "workspace-call"),
		newAuditLogScopeTestRow(base.Add(2*time.Second), "mcp-unrelated", "Unrelated Server", "", "unrelated-call"),
	}
	return storageClient, logs
}

func newAuditLogScopeTestRow(createdAt time.Time, mcpID, displayName, workspaceID, callName string) gatewaytypes.MCPAuditLog {
	return gatewaytypes.MCPAuditLog{
		CreatedAt:  createdAt,
		SourceType: types.AuditLogSourceTypeMCP,
		UserID:     "42",
		MCPFields: &gatewaytypes.MCPAuditLogFields{
			MCPID:                mcpID,
			MCPServerDisplayName: displayName,
			PowerUserWorkspaceID: workspaceID,
			CallType:             "tools/call",
			CallIdentifier:       callName,
			RequestBody:          json.RawMessage(`{}`),
			ResponseReceived:     true,
			ResponseStatus:       http.StatusOK,
		},
	}
}

func seedMCPAuditLogs(t *testing.T, gatewayClient *gatewayclient.Client, logs ...gatewaytypes.MCPAuditLog) {
	t.Helper()
	for _, log := range logs {
		gatewayClient.LogMCPAuditEntry(log)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		_, total, err := gatewayClient.GetMCPAuditLogs(t.Context(), gatewayclient.MCPAuditLogOptions{})
		if err != nil {
			t.Fatalf("wait for MCP audit logs: %v", err)
		}
		if total == int64(len(logs)) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %d MCP audit logs", len(logs))
}

func newLocalAgentAuditLogTestGatewayClient(t *testing.T) *gatewayclient.Client {
	t.Helper()

	storageServices, err := sservices.New(sservices.Config{
		DSN: "sqlite://:memory:",
	})
	if err != nil {
		t.Fatalf("failed to create storage services: %v", err)
	}

	db, err := gatewaydb.New(storageServices.DB.DB, storageServices.DB.SQLDB, true)
	if err != nil {
		t.Fatalf("failed to create gateway db: %v", err)
	}
	if err := db.AutoMigrate(); err != nil {
		t.Fatalf("failed to migrate gateway db: %v", err)
	}

	c := gatewayclient.New(t.Context(), db, nil, nil, nil, nil, nil, 10*time.Millisecond, 10, 90, 90, true)
	t.Cleanup(func() {
		_ = c.Close()
	})
	return c
}

func newLocalAgentAuditLogTestContext(t *testing.T, gatewayClient *gatewayclient.Client, events []types.LocalAgentToolCallAuditLogInput, u user.Info) api.Context {
	t.Helper()

	body, err := json.Marshal(types.LocalAgentToolCallAuditLogSubmitRequest{Events: events})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/local-agent-audit-logs", bytes.NewReader(body))
	req.RemoteAddr = "192.0.2.10:1234"
	req.Header.Set("X-Forwarded-For", "198.51.100.10, 203.0.113.10")

	return api.Context{
		ResponseWriter: httptest.NewRecorder(),
		Request:        req,
		GatewayClient:  gatewayClient,
		User:           u,
	}
}

func validLocalAgentAuditLogInput(occurredAt time.Time, idempotencyKey string, status types.AuditLogOutcomeStatus) types.LocalAgentToolCallAuditLogInput {
	return types.LocalAgentToolCallAuditLogInput{
		OccurredAt: *types.NewTime(occurredAt),
		Action: types.LocalAgentToolCallAuditLogAction{
			Name: "mcp__server__tool",
			Kind: "mcp",
		},
		Target: types.LocalAgentToolCallAuditLogTarget{
			TargetType: types.AuditLogTargetTypeMCPTool,
			Name:       "tool",
			Parent: &types.LocalAgentToolCallAuditLogTargetRef{
				TargetType: types.AuditLogTargetTypeMCPServer,
				Name:       "server",
			},
		},
		Outcome: types.LocalAgentToolCallAuditLogOutcome{Status: status},
		Details: types.LocalAgentToolCallAuditLogReportedDetails{
			Trace: types.LocalAgentToolCallAuditLogTrace{IdempotencyKey: idempotencyKey},
			Agent: types.LocalAgentToolCallAuditLogAgent{
				Provider:   types.LocalAgentProviderCodex,
				CLIVersion: "1.2.3",
			},
			Environment: types.LocalAgentToolCallAuditLogEnvironment{ReportedUserEmail: "reported@example.com"},
			Request:     types.LocalAgentToolCallAuditLogPayload{Body: json.RawMessage(`{"arg":true}`)},
			Response:    types.LocalAgentToolCallAuditLogPayload{Body: json.RawMessage(`{"ok":true}`)},
			RawEvent:    json.RawMessage(`{"native":true}`),
		},
	}
}
