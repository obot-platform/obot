package llmengine

import (
	"fmt"
	"slices"
	"testing"

	"github.com/maximhq/bifrost/core/schemas"
)

// recordingLogger records the lines it is asked to log.
type recordingLogger struct {
	schemas.Logger
	lines *[]string
}

func (r recordingLogger) record(level, msg string, args []any) {
	*r.lines = append(*r.lines, level+" "+fmt.Sprintf(msg, args...))
}

func (r recordingLogger) Debug(msg string, args ...any) { r.record("debug", msg, args) }
func (r recordingLogger) Info(msg string, args ...any)  { r.record("info", msg, args) }
func (r recordingLogger) Warn(msg string, args ...any)  { r.record("warn", msg, args) }
func (r recordingLogger) Error(msg string, args ...any) { r.record("error", msg, args) }

func TestEngineLoggerDropsBannerAndScrubsName(t *testing.T) {
	var lines []string
	l := newEngineLogger(recordingLogger{lines: &lines})

	// The admin account warning, exactly as the engine logs it at start, in both variants.
	for _, msg := range []string{
		"================================================================",
		"No admin account is configured for this Bifrost instance yet.",
		"Until one is created, the dashboard/API is reachable without a",
		"password from anyone who can route to this port. To finish setup,",
		"pass the configured setup token as auth_config.setup_token in the",
		"PUT /api/config call that creates the admin account.",
		"The /api surface is locked until dashboard auth is enabled. Open",
		"the dashboard and enter the setup token, or send it in the",
		"X-Bifrost-Setup-Token header.",
		"================================================================",
		"git binary not found — Claude Code / Codex marketplace routes disabled",
	} {
		l.Warn(msg)
	}
	l.Info("[Bifrost] client initialized")
	l.Warn("failed to list models with key %s: %s", "k1", "timeout")
	l.Error("= in a real error")

	want := []string{
		"info [LLM gateway] client initialized",
		"warn failed to list models with key k1: timeout",
		"error = in a real error",
	}
	if !slices.Equal(lines, want) {
		t.Fatalf("logged %q, want %q", lines, want)
	}
}
