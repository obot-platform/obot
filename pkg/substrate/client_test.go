package substrate

import (
	"strings"
	"testing"

	"github.com/obot-platform/obot/pkg/clauderunner"
	gt "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/stretchr/testify/require"
)

func TestEnvironmentRuntimeValidation(t *testing.T) {
	runtime := gt.ClaudeRuntime{Image: "example/runner@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}
	clauderunner.DefaultRuntime(&runtime)
	runtime = clauderunner.WithDefaultImages(runtime, "example/worker:v0.3.0", "example/runner:main")
	require.NoError(t, clauderunner.ValidateRuntime(runtime))
	for _, image := range []string{"", "example/runner:main", "example/runner@sha256:abc", "runner image", strings.Repeat("a", 513)} {
		invalid := runtime
		invalid.Image = image
		require.ErrorContains(t, clauderunner.ValidateRuntime(invalid), "runner image")
	}
	for _, minutes := range []int{-1, 0, 10081} {
		invalid := runtime
		invalid.Resources.IdleTimeoutMinutes = minutes
		require.Error(t, clauderunner.ValidateRuntime(invalid))
	}
	for _, days := range []int{-2, 0, 36501} {
		invalid := runtime
		invalid.Resources.SessionRetentionDays = days
		require.Error(t, clauderunner.ValidateRuntime(invalid))
	}
	runtime.Resources.SessionRetentionDays = -1
	require.NoError(t, clauderunner.ValidateRuntime(runtime))
	runtime.Resources.ActorMemory = "5Gi"
	require.NoError(t, clauderunner.ValidateRuntime(runtime))
	runtime.Worker.MemoryLimit = "0"
	require.ErrorContains(t, clauderunner.ValidateRuntime(runtime), "positive")
}
