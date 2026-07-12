package mcpgateway

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaydb "github.com/obot-platform/obot/pkg/gateway/db"
	sservices "github.com/obot-platform/obot/pkg/storage/services"
	"gorm.io/gorm"
	"k8s.io/apiserver/pkg/authentication/user"
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
	observedAt := time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC)

	beforeSubmit := time.Now().UTC()
	handler := NewLocalAgentAuditLogHandler()
	err := handler.Submit(newLocalAgentAuditLogTestContext(t, gatewayClient, []types.LocalAgentToolCallAuditLogManifest{
		validLocalAgentAuditLogManifest(observedAt, "entry-1", types.LocalAgentAuditLogStatusSucceeded),
		validLocalAgentAuditLogManifest(observedAt.Add(time.Second), "entry-2", types.LocalAgentAuditLogStatusFailed),
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
	if local.IdentityStatus != string(types.LocalAgentIdentityStatusAuthenticatedUser) {
		t.Fatalf("identity status = %q, want authenticated_user", local.IdentityStatus)
	}
	if local.ReportedUserEmail != "reported@example.com" {
		t.Fatalf("reported email = %q, want untrusted reported metadata preserved", local.ReportedUserEmail)
	}
	if string(local.ToolInput) != `{"arg":true}` || string(local.ToolOutput) != `{"ok":true}` {
		t.Fatalf("unexpected payloads: input=%s output=%s", local.ToolInput, local.ToolOutput)
	}
}

func TestLocalAgentAuditLogSubmitInvalidBatchDoesNotPartiallyPersist(t *testing.T) {
	gatewayClient := newLocalAgentAuditLogTestGatewayClient(t)
	handler := NewLocalAgentAuditLogHandler()
	observedAt := time.Now().UTC()

	err := handler.Submit(newLocalAgentAuditLogTestContext(t, gatewayClient, []types.LocalAgentToolCallAuditLogManifest{
		validLocalAgentAuditLogManifest(observedAt, "entry-1", types.LocalAgentAuditLogStatusSucceeded),
		validLocalAgentAuditLogManifest(observedAt, "", types.LocalAgentAuditLogStatusSucceeded),
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

func TestLocalAgentAuditLogSubmitDuplicateIdempotencyKeyIsSuccessNoop(t *testing.T) {
	gatewayClient := newLocalAgentAuditLogTestGatewayClient(t)
	handler := NewLocalAgentAuditLogHandler()
	manifest := validLocalAgentAuditLogManifest(time.Now().UTC(), "same-entry", types.LocalAgentAuditLogStatusSucceeded)
	reqUser := &user.DefaultInfo{
		UID:    "42",
		Groups: []string{types.GroupAuthenticated, types.GroupDeviceScans},
	}

	if err := handler.Submit(newLocalAgentAuditLogTestContext(t, gatewayClient, []types.LocalAgentToolCallAuditLogManifest{manifest}, reqUser)); err != nil {
		t.Fatalf("first submit: %v", err)
	}
	manifest.ToolName = "different-tool"
	if err := handler.Submit(newLocalAgentAuditLogTestContext(t, gatewayClient, []types.LocalAgentToolCallAuditLogManifest{manifest}, reqUser)); err != nil {
		t.Fatalf("duplicate submit should succeed: %v", err)
	}

	got, err := gatewayClient.GetMCPAuditLog(t.Context(), 1, true)
	if err != nil {
		t.Fatalf("get original row: %v", err)
	}
	if got.LocalAgentToolCallFields.ToolName != "mcp__server__tool" {
		t.Fatalf("expected original row to remain unchanged, got tool %q", got.LocalAgentToolCallFields.ToolName)
	}
	_, err = gatewayClient.GetMCPAuditLog(t.Context(), 2, true)
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("expected duplicate to avoid second row, got err %v", err)
	}
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

	c := gatewayclient.New(t.Context(), db, nil, nil, nil, nil, time.Hour, 10, 90, 90, true)
	t.Cleanup(func() {
		_ = c.Close()
	})
	return c
}

func newLocalAgentAuditLogTestContext(t *testing.T, gatewayClient *gatewayclient.Client, logs []types.LocalAgentToolCallAuditLogManifest, u user.Info) api.Context {
	t.Helper()

	body, err := json.Marshal(types.LocalAgentToolCallAuditLogSubmitRequest{Logs: logs})
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

func validLocalAgentAuditLogManifest(observedAt time.Time, idempotencyKey string, status types.LocalAgentAuditLogStatus) types.LocalAgentToolCallAuditLogManifest {
	return types.LocalAgentToolCallAuditLogManifest{
		AgentProvider:     types.LocalAgentProviderCodex,
		CLIVersion:        "1.2.3",
		Status:            status,
		ObservedAt:        *types.NewTime(observedAt),
		IdempotencyKey:    idempotencyKey,
		ToolName:          "mcp__server__tool",
		ToolKind:          "mcp",
		ReportedUserEmail: "reported@example.com",
		ToolInput:         json.RawMessage(`{"arg":true}`),
		ToolOutput:        json.RawMessage(`{"ok":true}`),
		RawHookPayload:    json.RawMessage(`{"native":true}`),
	}
}
