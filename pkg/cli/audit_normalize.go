package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"runtime"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/obot-platform/obot/apiclient/types"
)

func normalizeAuditEvent(ctx context.Context, format string, raw []byte) (types.AuditEvent, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	hook, err := decodeAuditHookPayload(format, raw)
	if err != nil {
		return types.AuditEvent{}, err
	}

	now := time.Now().UTC()
	eventID := strings.TrimSpace(hook.EventID)
	if eventID == "" {
		eventID = uuid.NewString()
	}
	createdAt := hook.CreatedAt
	if createdAt.IsZero() {
		createdAt = now
	}

	deviceID, err := ensureDeviceID("")
	if err != nil {
		return types.AuditEvent{}, err
	}

	cwd := strings.TrimSpace(hook.CWD)
	if cwd == "" {
		cwd, _ = os.Getwd()
	}
	workspace := strings.TrimSpace(hook.Workspace)
	if workspace == "" {
		workspace = gitRoot(ctx, cwd)
	}
	if workspace == "" {
		workspace = cwd
	}

	hostname, _ := os.Hostname()
	username := ""
	if u, err := user.Current(); err == nil {
		username = u.Username
	}

	request, requestMeta := limitedRawMessage(hook.Request, auditPayloadRequestLimit)
	response, responseMeta := limitedRawMessage(hook.Response, auditPayloadResponseLimit)
	rawEvent, rawMeta := limitedRawJSON(raw, auditPayloadRawLimit)
	errorText, errorMeta := limitedText(hook.Error, auditPayloadErrorLimit)

	payloadMeta := map[string]types.PayloadFieldMeta{}
	addPayloadMeta(payloadMeta, "request", requestMeta)
	addPayloadMeta(payloadMeta, "response", responseMeta)
	addPayloadMeta(payloadMeta, "error", errorMeta)
	addPayloadMeta(payloadMeta, "rawEvent", rawMeta)

	event := types.AuditEvent{
		EventID:    eventID,
		SourceType: auditSourceLocalAgent,
		EventType:  auditEventToolCall,
		CreatedAt:  types.Time{Time: createdAt.UTC()},
		DeviceID:   deviceID,
		Client: types.ClientInfo{
			Name:    format,
			Version: firstNonEmpty(hook.ClientVersion, "unknown"),
		},
		Tool: types.ToolInfo{
			Name: firstNonEmpty(hook.ToolName, "unknown"),
			Type: firstNonEmpty(hook.ToolType, "tool"),
		},
		Outcome:     firstNonEmpty(hook.Outcome, types.AuditLogOutcomeSuccess),
		DurationMs:  hook.DurationMs,
		SessionID:   hook.SessionID,
		Request:     request,
		Response:    response,
		Error:       errorText,
		RawEvent:    rawEvent,
		PayloadMeta: payloadMeta,
		Context: &types.AuditLogContext{
			ConversationID:  hook.ConversationID,
			CWD:             cwd,
			Workspace:       workspace,
			GitRemote:       gitOutput(ctx, workspace, "config", "--get", "remote.origin.url"),
			GitBranch:       gitOutput(ctx, workspace, "branch", "--show-current"),
			SourceHookEvent: hook.SourceHookEvent,
			ClientEventID:   hook.ClientEventID,
			Hostname:        hostname,
			OS:              runtime.GOOS,
			Arch:            runtime.GOARCH,
			Username:        username,
		},
	}
	if len(event.PayloadMeta) == 0 {
		event.PayloadMeta = nil
	}
	return event, nil
}

func decodeAuditHookPayload(format string, raw []byte) (auditHookPayload, error) {
	switch format {
	case auditClientClaudeCode:
		var payload claudeCodeHookPayload
		if err := json.Unmarshal(raw, &payload); err != nil {
			return auditHookPayload{}, err
		}
		return payload.auditHookPayload(), nil
	case auditClientCodex:
		var payload codexHookPayload
		if err := json.Unmarshal(raw, &payload); err != nil {
			return auditHookPayload{}, err
		}
		return payload.auditHookPayload(), nil
	case auditClientCursor:
		var payload cursorHookPayload
		if err := json.Unmarshal(raw, &payload); err != nil {
			return auditHookPayload{}, err
		}
		return payload.auditHookPayload(), nil
	case auditClientVSCode:
		var payload vscodeHookPayload
		if err := json.Unmarshal(raw, &payload); err != nil {
			return auditHookPayload{}, err
		}
		return payload.auditHookPayload(), nil
	default:
		return auditHookPayload{}, fmt.Errorf("unsupported audit format %q", format)
	}
}

func auditOutcome(hookEvent, errorText, status string) string {
	if errorText != "" || strings.Contains(strings.ToLower(hookEvent), "failure") {
		return types.AuditLogOutcomeError
	}
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "error", "failed", "failure":
		return types.AuditLogOutcomeError
	default:
		return types.AuditLogOutcomeSuccess
	}
}

func parseAuditTime(v any) (time.Time, bool) {
	switch t := v.(type) {
	case string:
		t = strings.TrimSpace(t)
		if t == "" {
			return time.Time{}, false
		}
		for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
			if parsed, err := time.Parse(layout, t); err == nil {
				return parsed, true
			}
		}
	case json.Number:
		if n, err := t.Int64(); err == nil {
			return unixTime(n), true
		}
	case float64:
		return unixTime(int64(t)), true
	}
	return time.Time{}, false
}

func unixTime(n int64) time.Time {
	if n > 1_000_000_000_000 {
		return time.UnixMilli(n)
	}
	return time.Unix(n, 0)
}

func limitedRawMessage(raw json.RawMessage, limit int64) (json.RawMessage, types.PayloadFieldMeta) {
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return nil, types.PayloadFieldMeta{}
	}
	return limitedRawJSON(raw, limit)
}

func limitedRawJSON(b []byte, limit int64) (json.RawMessage, types.PayloadFieldMeta) {
	if int64(len(b)) <= limit {
		return json.RawMessage(b), types.PayloadFieldMeta{}
	}
	stored := jsonStringWithinLimit(b, int(limit))
	return stored, types.PayloadFieldMeta{
		Truncated:     true,
		OriginalBytes: int64(len(b)),
		StoredBytes:   int64(len(stored)),
	}
}

func limitedText(s string, limit int64) (string, types.PayloadFieldMeta) {
	if int64(len([]byte(s))) <= limit {
		return s, types.PayloadFieldMeta{}
	}
	truncated := truncateUTF8([]byte(s), int(limit))
	return truncated, types.PayloadFieldMeta{
		Truncated:     true,
		OriginalBytes: int64(len([]byte(s))),
		StoredBytes:   int64(len([]byte(truncated))),
	}
}

func jsonStringWithinLimit(b []byte, limit int) json.RawMessage {
	if limit <= 2 {
		return json.RawMessage(`""`)
	}
	n := limit - 2
	for n > 0 {
		s := truncateUTF8(b, n)
		quoted, _ := json.Marshal(s)
		if len(quoted) <= limit {
			return quoted
		}
		n = n / 2
	}
	return json.RawMessage(`""`)
}

func truncateUTF8(b []byte, limit int) string {
	if len(b) <= limit {
		return string(b)
	}
	b = b[:limit]
	for !utf8.Valid(b) && len(b) > 0 {
		b = b[:len(b)-1]
	}
	return string(b)
}

func addPayloadMeta(meta map[string]types.PayloadFieldMeta, key string, value types.PayloadFieldMeta) {
	if value.Truncated || value.OriginalBytes != 0 || value.StoredBytes != 0 {
		meta[key] = value
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func gitRoot(ctx context.Context, dir string) string {
	return gitOutput(ctx, dir, "rev-parse", "--show-toplevel")
}

func gitOutput(ctx context.Context, dir string, args ...string) string {
	if strings.TrimSpace(dir) == "" {
		return ""
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
	defer cancel()
	// TODO(g-linville): harden this against command injection. dir and args need to be safe.
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
