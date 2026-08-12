package client

import (
	"testing"

	"github.com/obot-platform/obot/pkg/gateway/types"
)

func TestAuditLogAPIKeySchema(t *testing.T) {
	c := newTestClient(t)
	migrator := c.db.WithContext(t.Context()).Migrator()

	for _, tt := range []struct {
		name      string
		model     any
		indexName string
	}{
		{name: "MCP", model: &types.MCPAuditLog{}, indexName: "idx_mcp_audit_api_key_created"},
		{name: "LLM", model: &types.LLMAuditLog{}, indexName: "idx_llm_audit_api_key_created"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if !migrator.HasColumn(tt.model, "api_key_id") {
				t.Fatal("expected api_key_id column")
			}
			if !migrator.HasColumn(tt.model, "api_key_name") {
				t.Fatal("expected api_key_name column")
			}
			if !migrator.HasIndex(tt.model, tt.indexName) {
				t.Fatalf("expected %s index", tt.indexName)
			}
		})
	}
}
