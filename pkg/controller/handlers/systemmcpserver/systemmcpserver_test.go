package systemmcpserver

import (
	"errors"
	"fmt"
	"testing"

	"github.com/obot-platform/obot/pkg/mcp"
)

func TestLaunchErrorDoesNotRetryUnsupportedBackend(t *testing.T) {
	// what the none backend returns for a containerized system server (wrapped, as callers may do)
	unsupported := fmt.Errorf("launch: %w", &mcp.ErrNotSupportedByBackend{Feature: "hosted MCP servers", Backend: mcp.RuntimeBackendNone})
	if err := launchError("sms1obot-mcp-server", unsupported); err != nil {
		t.Fatalf("launchError(unsupported) = %v, want nil (no retry)", err)
	}
}

func TestLaunchErrorRetriesOtherErrors(t *testing.T) {
	cause := errors.New("image pull failed")
	err := launchError("sms1obot-mcp-server", cause)
	if err == nil || !errors.Is(err, cause) {
		t.Fatalf("launchError(other) = %v, want an error wrapping %v (retried)", err, cause)
	}
}
