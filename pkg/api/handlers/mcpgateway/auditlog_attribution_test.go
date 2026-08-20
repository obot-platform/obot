package mcpgateway

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/obot-platform/nanobot/pkg/mcp/auditlogs"
	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
)

func TestCollectMCPAuditEntryPersistsWhenAPIKeyAttributionFails(t *testing.T) {
	gatewayClient := newLocalAgentAuditLogTestGatewayClient(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	NewAuditLogHandler(gatewayClient).collectMCPAuditEntry(ctx, auditlogs.MCPAuditLog{
		Metadata:       map[string]string{"mcpID": "mcp-1"},
		CreatedAt:      time.Now().UTC(),
		APIKey:         auditlogs.RedactAPIKey("ok1-7-42-abcdefghijklmnopqrstuvwxyz"),
		CallType:       "tools/call",
		CallIdentifier: "search",
		RequestBody:    json.RawMessage(`{}`),
		ResponseBody:   json.RawMessage(`{}`),
		ResponseStatus: 200,
	})

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		logs, total, err := gatewayClient.GetMCPAuditLogs(t.Context(), gatewayclient.MCPAuditLogOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if total == 1 {
			if logs[0].APIKeyID != nil || logs[0].APIKeyName != "" {
				t.Fatalf("persisted event gained attribution: ID %v, name %q", logs[0].APIKeyID, logs[0].APIKeyName)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("audit event was dropped after API-key attribution failure")
}

func TestAttributeMCPAuditLogAPIKeyFallsBackOnlyWhenKeyIsMissing(t *testing.T) {
	gatewayClient := newLocalAgentAuditLogTestGatewayClient(t)
	input := auditLogInput{MCPFields: &gatewaytypes.MCPAuditLogFields{
		APIKey: auditlogs.RedactAPIKey("ok1-7-42-abcdefghijklmnopqrstuvwxyz"),
	}}

	if err := NewAuditLogHandler(gatewayClient).attributeMCPAuditLogAPIKey(t.Context(), &input); err != nil {
		t.Fatal(err)
	}

	if input.APIKeyID == nil || *input.APIKeyID != 42 || input.APIKeyName != "ok1-7-42-*****" {
		t.Fatalf("missing-key attribution = ID %v, name %q", input.APIKeyID, input.APIKeyName)
	}
}

func TestAttributeMCPAuditLogAPIKeyReturnsTransientLookupError(t *testing.T) {
	gatewayClient := newLocalAgentAuditLogTestGatewayClient(t)
	input := auditLogInput{MCPFields: &gatewaytypes.MCPAuditLogFields{
		APIKey: auditlogs.RedactAPIKey("ok1-7-42-abcdefghijklmnopqrstuvwxyz"),
	}}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	err := NewAuditLogHandler(gatewayClient).attributeMCPAuditLogAPIKey(ctx, &input)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("lookup error = %v, want context canceled", err)
	}
	if input.APIKeyID != nil || input.APIKeyName != "" {
		t.Fatalf("transient lookup error produced attribution: ID %v, name %q", input.APIKeyID, input.APIKeyName)
	}
}
