package mcpgateway

import (
	"encoding/json"
	"testing"
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
