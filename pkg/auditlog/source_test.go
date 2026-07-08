package auditlog

import (
	"slices"
	"testing"

	api "github.com/obot-platform/obot/apiclient/types"
)

func TestNormalizeSourceTypes(t *testing.T) {
	tests := []struct {
		name string
		in   []api.AuditLogSourceType
		want []api.AuditLogSourceType
	}{
		{
			name: "empty defaults to MCP",
			want: []api.AuditLogSourceType{api.AuditLogSourceTypeMCP},
		},
		{
			name: "deduplicates while preserving order",
			in: []api.AuditLogSourceType{
				api.AuditLogSourceTypeLocalAgentToolCall,
				api.AuditLogSourceTypeMCP,
				api.AuditLogSourceTypeLocalAgentToolCall,
			},
			want: []api.AuditLogSourceType{
				api.AuditLogSourceTypeLocalAgentToolCall,
				api.AuditLogSourceTypeMCP,
			},
		},
		{
			name: "preserves unknown values for validation",
			in:   []api.AuditLogSourceType{"future", "future"},
			want: []api.AuditLogSourceType{"future"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := NormalizeSourceTypes(test.in); !slices.Equal(got, test.want) {
				t.Fatalf("NormalizeSourceTypes(%v) = %v, want %v", test.in, got, test.want)
			}
		})
	}
}
