package clauderunner

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOrchestratorUnsupportedOnWindows(t *testing.T) {
	stop, err := StartOrchestrator(t.Context(), "key", "token", "environment", "http://localhost")
	require.ErrorContains(t, err, "not supported on Windows")
	require.Nil(t, stop)

	// Server shutdown can call the cleanup function more than once.
	stop = ManageOrchestrators(t.Context(), nil, "http://localhost")
	stop()
	stop()
}
