package llmengine

import (
	"fmt"
	"strings"

	"github.com/maximhq/bifrost/core/schemas"
)

var (
	// engineLogMessagesToDrop are lines Bifrost logs that don't apply inside Obot. Bifrost warns
	// on every start that it has no admin account and explains how to create one with its setup
	// token, and says its dashboard is reachable without a password from anyone who can route to
	// its port. Inside Obot it listens on no port, Obot alone holds the setup token, and creating
	// an admin account through Obot is refused, so the warning only alarms operators. Its
	// marketplace routes aren't reachable through Obot either, so the warning that they're off
	// without git is noise too.
	engineLogMessagesToDrop = map[string]bool{
		"No admin account is configured for this Bifrost instance yet.":          true,
		"Until one is created, the dashboard/API is reachable without a":         true,
		"password from anyone who can route to this port. To finish setup,":      true,
		"pass the configured setup token as auth_config.setup_token in the":      true,
		"PUT /api/config call that creates the admin account.":                   true,
		"The /api surface is locked until dashboard auth is enabled. Open":       true,
		"the dashboard and enter the setup token, or send it in the":             true,
		"X-Bifrost-Setup-Token header.":                                          true,
		"No admin account is configured for this Bifrost instance yet, and":      true,
		"no setup token is configured. Set setup_token in config.json (or":       true,
		"the BIFROST_SETUP_TOKEN environment variable) before creating the":      true,
		"first admin account — requests to create it will be rejected until":     true,
		"a setup token is configured.":                                           true,
		"git binary not found — Claude Code / Codex marketplace routes disabled": true,
	}
)

// engineLogger is the logger Obot gives Bifrost. It drops the lines that don't apply inside
// Obot (engineLogMessagesToDrop, and the rules of "=" that frame the admin account warning)
// and logs the rest with the engine's name replaced, as in its API errors.
type engineLogger struct {
	schemas.Logger
}

func newEngineLogger(inner schemas.Logger) schemas.Logger {
	return engineLogger{Logger: inner}
}

// message formats a log call the way Bifrost's own logger does, and reports whether to drop it.
func (engineLogger) message(msg string, args []any) (string, bool) {
	if len(args) > 0 {
		msg = fmt.Sprintf(msg, args...)
	}
	trimmed := strings.TrimSpace(msg)
	if engineLogMessagesToDrop[trimmed] || (len(trimmed) >= 10 && strings.Trim(trimmed, "=") == "") {
		return "", false
	}
	return ScrubEngineName(msg), true
}

func (l engineLogger) Debug(msg string, args ...any) {
	if m, ok := l.message(msg, args); ok {
		l.Logger.Debug("%s", m)
	}
}

func (l engineLogger) Info(msg string, args ...any) {
	if m, ok := l.message(msg, args); ok {
		l.Logger.Info("%s", m)
	}
}

func (l engineLogger) Warn(msg string, args ...any) {
	if m, ok := l.message(msg, args); ok {
		l.Logger.Warn("%s", m)
	}
}

func (l engineLogger) Error(msg string, args ...any) {
	if m, ok := l.message(msg, args); ok {
		l.Logger.Error("%s", m)
	}
}

// Fatal is never dropped: the process is about to exit, and the line says why.
func (l engineLogger) Fatal(msg string, args ...any) {
	if len(args) > 0 {
		msg = fmt.Sprintf(msg, args...)
	}
	l.Logger.Fatal("%s", ScrubEngineName(msg))
}
