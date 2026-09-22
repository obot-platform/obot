package services

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRejectNegativeMCPAuditBodyLimit(t *testing.T) {
	config := Config{
		MCPAuditLogMaxBodyBytes: new(-1),
	}

	_, err := New(t.Context(), config)
	require.EqualError(t, err, "mcpaudit-log-max-body-bytes must be non-negative")
}

func TestRejectNegativeLLMAuditBodyLimit(t *testing.T) {
	config := Config{
		LLMAuditLogMaxBodyBytes: new(-1),
	}

	_, err := New(t.Context(), config)
	require.EqualError(t, err, "llmaudit-log-max-body-bytes must be non-negative")
}

func TestRejectNegativeLocalAgentAuditBodyLimit(t *testing.T) {
	config := Config{
		LocalAgentAuditLogMaxBodyBytes: new(-1),
	}

	_, err := New(t.Context(), config)
	require.EqualError(t, err, "local-agent-audit-log-max-body-bytes must be non-negative")
}
