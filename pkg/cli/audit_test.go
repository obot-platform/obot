package cli

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/adrg/xdg"
	"github.com/obot-platform/obot/apiclient"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/spf13/cobra"
)

func TestNormalizeAuditEventFixtures(t *testing.T) {
	restore := useAuditTestEnv(t)
	defer restore()
	workspace := t.TempDir()

	fixtures := []struct {
		name     string
		format   string
		payload  string
		toolName string
		outcome  string
	}{
		{
			name:   "claude code success",
			format: auditClientClaudeCode,
			payload: fmt.Sprintf(`{
				"hook_event_name": "PostToolUse",
				"session_id": "claude-session",
				"cwd": %q,
				"tool_name": "Read",
				"tool_input": {"file_path": "README.md"},
				"tool_response": {"content": "ok"},
				"duration_ms": 12,
				"client": {"version": "1.2.3"}
			}`, workspace),
			toolName: "Read",
			outcome:  types.AuditLogOutcomeSuccess,
		},
		{
			name:   "codex success",
			format: auditClientCodex,
			payload: fmt.Sprintf(`{
				"event": "PostToolUse",
				"sessionID": "codex-session",
				"cwd": %q,
				"tool": {"name": "shell", "type": "command", "input": {"cmd": "go test"}, "response": {"output": "ok"}},
				"durationMs": 20
			}`, workspace),
			toolName: "shell",
			outcome:  types.AuditLogOutcomeSuccess,
		},
		{
			name:   "cursor failure",
			format: auditClientCursor,
			payload: fmt.Sprintf(`{
				"eventName": "postToolUseFailure",
				"workspace": {"path": %q},
				"toolName": "run_terminal_cmd",
				"args": {"command": "false"},
				"error": {"message": "exit status 1"}
			}`, workspace),
			toolName: "run_terminal_cmd",
			outcome:  types.AuditLogOutcomeError,
		},
		{
			name:   "vscode success",
			format: auditClientVSCode,
			payload: fmt.Sprintf(`{
				"event_name": "postToolUse",
				"project_path": %q,
				"tool": {"name": "codebase_search"},
				"request": {"query": "audit"},
				"response": {"results": []}
			}`, workspace),
			toolName: "codebase_search",
			outcome:  types.AuditLogOutcomeSuccess,
		},
	}

	for _, tt := range fixtures {
		t.Run(tt.name, func(t *testing.T) {
			event, err := normalizeAuditEvent(t.Context(), tt.format, []byte(tt.payload))
			if err != nil {
				t.Fatal(err)
			}
			if event.EventID == "" {
				t.Fatal("expected generated eventID")
			}
			if event.DeviceID == "" {
				t.Fatal("expected deviceID")
			}
			if event.SourceType != types.AuditLogSourceTypeLocalAgent || event.EventType != types.AuditLogEventTypeToolCall {
				t.Fatalf("source/event = %s/%s", event.SourceType, event.EventType)
			}
			if event.Client.Name != tt.format {
				t.Fatalf("client name = %q, want %q", event.Client.Name, tt.format)
			}
			if event.Tool.Name != tt.toolName {
				t.Fatalf("tool name = %q, want %q", event.Tool.Name, tt.toolName)
			}
			if event.Outcome != tt.outcome {
				t.Fatalf("outcome = %q, want %q", event.Outcome, tt.outcome)
			}
			if len(event.RawEvent) == 0 {
				t.Fatal("expected rawEvent")
			}
			if event.Context == nil || event.Context.Workspace == "" || event.Context.OS == "" || event.Context.Arch == "" {
				t.Fatalf("expected context to be populated: %+v", event.Context)
			}
		})
	}
}

func TestNormalizeAuditEventPayloadLimits(t *testing.T) {
	restore := useAuditTestEnv(t)
	defer restore()

	payload := fmt.Sprintf(`{
		"event": "PostToolUse",
		"tool": {
			"name": "big",
			"input": {"value": %q},
			"response": {"value": %q}
		},
		"error": %q
	}`, strings.Repeat("a", auditPayloadRequestLimit+1024), strings.Repeat("b", auditPayloadResponseLimit+1024), strings.Repeat("c", auditPayloadErrorLimit+1024))

	event, err := normalizeAuditEvent(t.Context(), auditClientCodex, []byte(payload))
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"request", "response", "error", "rawEvent"} {
		meta, ok := event.PayloadMeta[field]
		if !ok || !meta.Truncated {
			t.Fatalf("expected %s truncation metadata, got %#v", field, event.PayloadMeta[field])
		}
	}
	if len(event.Request) > auditPayloadRequestLimit {
		t.Fatalf("request stored bytes = %d, want <= %d", len(event.Request), auditPayloadRequestLimit)
	}
	if len(event.Response) > auditPayloadResponseLimit {
		t.Fatalf("response stored bytes = %d, want <= %d", len(event.Response), auditPayloadResponseLimit)
	}
	if len([]byte(event.Error)) > auditPayloadErrorLimit {
		t.Fatalf("error stored bytes = %d, want <= %d", len([]byte(event.Error)), auditPayloadErrorLimit)
	}
	if len(event.RawEvent) > auditPayloadRawLimit {
		t.Fatalf("rawEvent stored bytes = %d, want <= %d", len(event.RawEvent), auditPayloadRawLimit)
	}
}

func TestFileAuditSpoolEncryptRoundTripAndDelete(t *testing.T) {
	key := bytes.Repeat([]byte{7}, 32)
	spool := fileAuditSpool{
		dir: t.TempDir(),
		key: staticAuditSpoolKey{key: key},
	}
	event := types.AuditEvent{
		EventID:    "event-secret",
		SourceType: types.AuditLogSourceTypeLocalAgent,
		EventType:  types.AuditLogEventTypeToolCall,
		CreatedAt:  types.Time{Time: time.Date(2026, 6, 12, 1, 2, 3, 0, time.UTC)},
		RawEvent:   json.RawMessage(`{"secret":"do-not-store-plain"}`),
	}

	if err := spool.Write(event); err != nil {
		t.Fatal(err)
	}
	files, err := os.ReadDir(spool.dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Fatalf("spool files = %d, want 1", len(files))
	}
	data, err := os.ReadFile(filepath.Join(spool.dir, files[0].Name()))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte("do-not-store-plain")) || bytes.Contains(data, []byte("event-secret")) {
		t.Fatalf("spool file contains plaintext: %s", data)
	}

	records, err := spool.List(0)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].Event.EventID != event.EventID || !records[0].Event.CreatedAt.Time.Equal(event.CreatedAt.Time) {
		t.Fatalf("unexpected records: %+v", records)
	}
	if err := spool.Delete(records[0].Path); err != nil {
		t.Fatal(err)
	}
	records, err = spool.List(0)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 0 {
		t.Fatalf("records after delete = %d, want 0", len(records))
	}
}

func TestFileAuditSpoolKeyUnavailableDoesNotWritePlaintext(t *testing.T) {
	dir := t.TempDir()
	spool := fileAuditSpool{dir: dir, key: errAuditSpoolKey{err: errors.New("keychain unavailable")}}
	err := spool.Write(types.AuditEvent{EventID: "drop-me", RawEvent: json.RawMessage(`{"secret":"plain"}`)})
	if err == nil {
		t.Fatal("expected key error")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("spool wrote files without a key: %v", entries)
	}
}

func TestFlushAuditSpoolDeletesTerminalStatuses(t *testing.T) {
	key := bytes.Repeat([]byte{8}, 32)
	spool := fileAuditSpool{dir: t.TempDir(), key: staticAuditSpoolKey{key: key}}
	for _, id := range []string{"accepted", "duplicate", "rejected"} {
		if err := spool.Write(types.AuditEvent{EventID: id, SourceType: types.AuditLogSourceTypeLocalAgent, EventType: types.AuditLogEventTypeToolCall}); err != nil {
			t.Fatal(err)
		}
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/audit-events" {
			t.Fatalf("path = %s, want /api/audit-events", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Fatalf("authorization = %q", r.Header.Get("Authorization"))
		}
		_ = json.NewEncoder(w).Encode(types.AuditEventSubmitResponse{
			Items: []types.AuditEventSubmitStatus{
				{EventID: "accepted", Status: types.AuditEventSubmitStatusAccepted},
				{EventID: "duplicate", Status: types.AuditEventSubmitStatusDuplicate},
				{EventID: "rejected", Status: types.AuditEventSubmitStatusError, Error: "bad"},
			},
		})
	}))
	defer srv.Close()

	flushed, err := flushAuditSpool(t.Context(), &apiclient.Client{BaseURL: srv.URL + "/api", Token: "test-token"}, spool, 0)
	if err != nil {
		t.Fatal(err)
	}
	if flushed != 3 {
		t.Fatalf("flushed = %d, want 3", flushed)
	}
	records, err := spool.List(0)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 0 {
		t.Fatalf("remaining records = %+v, want none", records)
	}
}

func TestAuditSubmitHookModeExitsZeroOnNetworkAndSpoolFailure(t *testing.T) {
	restore := useAuditTestEnv(t)
	defer restore()
	oldDefaultSpool := defaultAuditSpool
	defaultAuditSpool = func() auditSpool {
		return failingAuditSpool{err: errors.New("spool unavailable")}
	}
	defer func() {
		defaultAuditSpool = oldDefaultSpool
	}()

	submit := &AuditSubmit{
		Format: auditClientCodex,
		Input:  "-",
		root: &Obot{Client: &apiclient.Client{
			BaseURL: "http://127.0.0.1:1/api",
			Token:   "test-token",
		}},
	}
	cmd := &cobra.Command{}
	cmd.SetIn(strings.NewReader(`{"event":"PostToolUse","tool":{"name":"shell","input":{"cmd":"false"}}}`))
	if err := submit.Run(cmd, nil); err != nil {
		t.Fatalf("hook submit returned error: %v", err)
	}
}

type staticAuditSpoolKey struct {
	key []byte
}

func (s staticAuditSpoolKey) LoadOrCreate() ([]byte, error) {
	return s.key, nil
}

type errAuditSpoolKey struct {
	err error
}

func (s errAuditSpoolKey) LoadOrCreate() ([]byte, error) {
	return nil, s.err
}

type failingAuditSpool struct {
	err error
}

func (s failingAuditSpool) Write(types.AuditEvent) error {
	return s.err
}

func (s failingAuditSpool) List(int) ([]auditSpoolRecord, error) {
	return nil, s.err
}

func (s failingAuditSpool) Delete(string) error {
	return s.err
}

func (s failingAuditSpool) Status() (string, int, bool, error) {
	return "", 0, false, s.err
}

func useAuditTestEnv(t *testing.T) func() {
	t.Helper()
	rootRestore := useRootTestEnv(t)
	dataHome := filepath.Join(t.TempDir(), "data")
	home := t.TempDir()
	oldDataHome, hadDataHome := os.LookupEnv("XDG_DATA_HOME")
	oldHome, hadHome := os.LookupEnv("HOME")
	if err := os.Setenv("XDG_DATA_HOME", dataHome); err != nil {
		t.Fatal(err)
	}
	if err := os.Setenv("HOME", home); err != nil {
		t.Fatal(err)
	}
	xdg.Reload()
	return func() {
		rootRestore()
		if hadDataHome {
			_ = os.Setenv("XDG_DATA_HOME", oldDataHome)
		} else {
			_ = os.Unsetenv("XDG_DATA_HOME")
		}
		if hadHome {
			_ = os.Setenv("HOME", oldHome)
		} else {
			_ = os.Unsetenv("HOME")
		}
		xdg.Reload()
	}
}

func TestDecodeAuditSpoolKey(t *testing.T) {
	key := bytes.Repeat([]byte{9}, 32)
	got, err := decodeAuditSpoolKey(base64.StdEncoding.EncodeToString(key))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, key) {
		t.Fatalf("decoded key mismatch")
	}
}

var _ auditSpool = failingAuditSpool{}
var _ auditSpoolKeyStore = staticAuditSpoolKey{}
var _ auditSpoolKeyStore = errAuditSpoolKey{}
