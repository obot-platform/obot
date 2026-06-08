package client

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/server/options/encryptionconfig"
	"k8s.io/apiserver/pkg/storage/value"
)

type prefixTestTransformer struct{}

func (prefixTestTransformer) TransformToStorage(_ context.Context, data []byte, _ value.Context) ([]byte, error) {
	return append([]byte("encrypted:"), data...), nil
}

func (prefixTestTransformer) TransformFromStorage(_ context.Context, data []byte, _ value.Context) ([]byte, bool, error) {
	return bytes.TrimPrefix(data, []byte("encrypted:")), false, nil
}

func TestCreateLocalAgentAuditLogsEncryptsPayloads(t *testing.T) {
	c := newTestClient(t)
	c.encryptionConfig = &encryptionconfig.EncryptionConfiguration{
		Transformers: map[schema.GroupResource]value.Transformer{
			localAgentAuditLogGroupResource: prefixTestTransformer{},
		},
	}

	rawInput := json.RawMessage(`{"path":"/Users/grant/project","command":"read"}`)
	inserted, count, err := c.CreateLocalAgentAuditLogs(t.Context(), []types.LocalAgentAuditLog{
		{
			EventID:            "event-1",
			UserID:             "42",
			ClientName:         "codex-cli",
			EventName:          "post-tool-use",
			ToolName:           "Read",
			RawClientHookEvent: json.RawMessage(`{"event":"post-tool-use"}`),
			RawToolInput:       rawInput,
			RawToolOutput:      json.RawMessage(`{"ok":true}`),
			RawError:           json.RawMessage(`{"message":"none"}`),
		},
	})
	if err != nil {
		t.Fatalf("unexpected error creating local agent audit log: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected one inserted row, got %d", count)
	}
	if len(inserted) != 1 {
		t.Fatalf("expected one stored log, got %d", len(inserted))
	}

	var stored types.LocalAgentAuditLog
	if err := c.db.WithContext(context.Background()).First(&stored, "event_id = ?", "event-1").Error; err != nil {
		t.Fatalf("failed to load stored log: %v", err)
	}
	if !stored.Encrypted {
		t.Fatal("expected stored log to be marked encrypted")
	}
	if bytes.Equal(stored.RawToolInput, rawInput) {
		t.Fatal("expected raw tool input to be encrypted at rest")
	}
	if err := c.decryptLocalAgentAuditLog(context.Background(), &stored); err != nil {
		t.Fatalf("failed to decrypt stored log: %v", err)
	}
	if !bytes.Equal(stored.RawToolInput, rawInput) {
		t.Fatalf("expected decrypted raw tool input %s, got %s", rawInput, stored.RawToolInput)
	}
}

func TestCreateLocalAgentAuditLogsDeduplicatesEventID(t *testing.T) {
	c := newTestClient(t)
	ctx := context.Background()

	log := types.LocalAgentAuditLog{
		EventID:   "duplicate-event",
		UserID:    "7",
		EventName: "post-tool-use",
	}
	if _, count, err := c.CreateLocalAgentAuditLogs(ctx, []types.LocalAgentAuditLog{log}); err != nil {
		t.Fatalf("unexpected first create error: %v", err)
	} else if count != 1 {
		t.Fatalf("expected first create to insert one row, got %d", count)
	}
	if _, count, err := c.CreateLocalAgentAuditLogs(ctx, []types.LocalAgentAuditLog{log}); err != nil {
		t.Fatalf("unexpected duplicate create error: %v", err)
	} else if count != 0 {
		t.Fatalf("expected duplicate create to insert zero rows, got %d", count)
	}

	var total int64
	if err := c.db.WithContext(ctx).Model(&types.LocalAgentAuditLog{}).Where("event_id = ?", log.EventID).Count(&total).Error; err != nil {
		t.Fatalf("failed to count duplicate rows: %v", err)
	}
	if total != 1 {
		t.Fatalf("expected one row after duplicate insert, got %d", total)
	}
}

func TestCreateLocalAgentAuditLogsDefaultsCreatedAtToUTC(t *testing.T) {
	c := newTestClient(t)
	createdAt := time.Date(2026, 6, 5, 12, 0, 0, 0, time.FixedZone("offset", -7*60*60))

	inserted, _, err := c.CreateLocalAgentAuditLogs(context.Background(), []types.LocalAgentAuditLog{
		{
			EventID:   "time-event",
			UserID:    "9",
			CreatedAt: createdAt,
			EventName: "post-tool-use",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error creating local agent audit log: %v", err)
	}
	if got := inserted[0].CreatedAt.Location(); got != time.UTC {
		t.Fatalf("expected CreatedAt to be normalized to UTC, got %v", got)
	}
}

func TestGetLocalAgentAuditLogPayloadAccess(t *testing.T) {
	c := newTestClient(t)
	c.encryptionConfig = &encryptionconfig.EncryptionConfiguration{
		Transformers: map[schema.GroupResource]value.Transformer{
			localAgentAuditLogGroupResource: prefixTestTransformer{},
		},
	}

	rawHook := json.RawMessage(`{"hook":"post-tool-use"}`)
	rawInput := json.RawMessage(`{"command":"read"}`)
	rawOutput := json.RawMessage(`{"ok":true}`)
	rawError := json.RawMessage(`{"message":"boom"}`)
	stored, _, err := c.CreateLocalAgentAuditLogs(t.Context(), []types.LocalAgentAuditLog{
		{
			EventID:            "payload-event",
			UserID:             "42",
			ClientName:         "codex-cli",
			EventName:          "post-tool-use",
			RawClientHookEvent: rawHook,
			RawToolInput:       rawInput,
			RawToolOutput:      rawOutput,
			RawError:           rawError,
		},
	})
	if err != nil {
		t.Fatalf("unexpected error creating local agent audit log: %v", err)
	}

	metadataOnly, err := c.GetLocalAgentAuditLog(t.Context(), stored[0].ID, false)
	if err != nil {
		t.Fatalf("failed to get metadata-only detail: %v", err)
	}
	if len(metadataOnly.RawClientHookEvent) != 0 || len(metadataOnly.RawToolInput) != 0 || len(metadataOnly.RawToolOutput) != 0 || len(metadataOnly.RawError) != 0 {
		t.Fatal("expected metadata-only detail to blank encrypted payload fields")
	}

	withPayloads, err := c.GetLocalAgentAuditLog(t.Context(), stored[0].ID, true)
	if err != nil {
		t.Fatalf("failed to get detail with payloads: %v", err)
	}
	if !bytes.Equal(withPayloads.RawClientHookEvent, rawHook) {
		t.Fatalf("expected decrypted raw hook %s, got %s", rawHook, withPayloads.RawClientHookEvent)
	}
	if !bytes.Equal(withPayloads.RawToolInput, rawInput) {
		t.Fatalf("expected decrypted raw input %s, got %s", rawInput, withPayloads.RawToolInput)
	}
	if !bytes.Equal(withPayloads.RawToolOutput, rawOutput) {
		t.Fatalf("expected decrypted raw output %s, got %s", rawOutput, withPayloads.RawToolOutput)
	}
	if !bytes.Equal(withPayloads.RawError, rawError) {
		t.Fatalf("expected decrypted raw error %s, got %s", rawError, withPayloads.RawError)
	}
}

func TestGetLocalAgentAuditLogsFiltersAndSortsMetadataOnly(t *testing.T) {
	c := newTestClient(t)
	ctx := t.Context()
	success := true
	failure := false
	exitOne := 1
	duration10 := int64(10)
	duration20 := int64(20)

	_, _, err := c.CreateLocalAgentAuditLogs(ctx, []types.LocalAgentAuditLog{
		{
			EventID:           "event-1",
			UserID:            "7",
			CreatedAt:         time.Date(2026, 6, 5, 10, 0, 0, 0, time.UTC),
			ClientName:        "codex-cli",
			ClientVersion:     "1.0.0",
			ToolName:          "Read",
			ToolType:          "filesystem",
			EventName:         "post-tool-use",
			Success:           &success,
			Status:            "ok",
			DurationMs:        &duration10,
			SessionID:         "session-a",
			RequestID:         "request-a",
			WorkspaceHash:     "hash-a",
			WorkspaceBasename: "repo-a",
			RawToolInput:      json.RawMessage(`{"secret":"hidden"}`),
		},
		{
			EventID:       "event-2",
			UserID:        "8",
			CreatedAt:     time.Date(2026, 6, 5, 11, 0, 0, 0, time.UTC),
			ClientName:    "claude-code",
			ToolName:      "Bash",
			ToolType:      "shell",
			EventName:     "post-tool-use",
			Success:       &failure,
			Status:        "failed",
			ExitCode:      &exitOne,
			DurationMs:    &duration20,
			SessionID:     "session-b",
			RequestID:     "request-b",
			WorkspaceHash: "hash-b",
			RawToolInput:  json.RawMessage(`{"secret":"also-hidden"}`),
		},
	})
	if err != nil {
		t.Fatalf("unexpected error creating local agent audit logs: %v", err)
	}

	logs, total, err := c.GetLocalAgentAuditLogs(ctx, LocalAgentAuditLogOptions{
		ClientName:    []string{"codex-cli"},
		ToolName:      []string{"Read"},
		Success:       []bool{true},
		DurationMsMin: 5,
		DurationMsMax: 15,
		SortBy:        "created_at",
		SortOrder:     "asc",
	})
	if err != nil {
		t.Fatalf("failed to query local agent audit logs: %v", err)
	}
	if total != 1 {
		t.Fatalf("expected total 1, got %d", total)
	}
	if len(logs) != 1 {
		t.Fatalf("expected one log, got %d", len(logs))
	}
	if logs[0].EventID != "event-1" {
		t.Fatalf("expected event-1, got %s", logs[0].EventID)
	}
	if len(logs[0].RawToolInput) != 0 {
		t.Fatal("expected list query to blank encrypted payload fields")
	}
}

func TestGetLocalAgentAuditLogsQueryMatchesUsersAndMetadata(t *testing.T) {
	c := newTestClient(t)
	ctx := context.Background()

	if err := c.db.WithContext(ctx).Create(&types.User{ID: 77, DisplayName: "Ada Auditor"}).Error; err != nil {
		t.Fatalf("failed to create user: %v", err)
	}
	if _, _, err := c.CreateLocalAgentAuditLogs(ctx, []types.LocalAgentAuditLog{
		{
			EventID:   "user-match",
			UserID:    "77",
			EventName: "post-tool-use",
			ToolName:  "Read",
		},
		{
			EventID:   "metadata-match",
			UserID:    "78",
			EventName: "post-tool-use",
			ToolName:  "Bash",
			Error:     "permission denied",
		},
	}); err != nil {
		t.Fatalf("unexpected error creating local agent audit logs: %v", err)
	}

	logs, total, err := c.GetLocalAgentAuditLogs(ctx, LocalAgentAuditLogOptions{Query: "auditor"})
	if err != nil {
		t.Fatalf("failed to query by user display name: %v", err)
	}
	if total != 1 || len(logs) != 1 || logs[0].EventID != "user-match" {
		t.Fatalf("expected user-match for display-name query, got total=%d logs=%v", total, logs)
	}

	logs, total, err = c.GetLocalAgentAuditLogs(ctx, LocalAgentAuditLogOptions{Query: "permission"})
	if err != nil {
		t.Fatalf("failed to query by metadata: %v", err)
	}
	if total != 1 || len(logs) != 1 || logs[0].EventID != "metadata-match" {
		t.Fatalf("expected metadata-match for metadata query, got total=%d logs=%v", total, logs)
	}
}

func TestGetLocalAgentAuditLogFilterOptionsDistinctWithoutDecrypt(t *testing.T) {
	c := newTestClient(t)
	ctx := context.Background()

	if err := c.db.WithContext(ctx).Create(&[]types.LocalAgentAuditLog{
		{
			EventID:      "filter-1",
			ClientName:   "codex-cli",
			EventName:    "post-tool-use",
			RawToolInput: json.RawMessage(`not-valid-encrypted-payload`),
			Encrypted:    true,
		},
		{
			EventID:    "filter-2",
			ClientName: "claude-code",
			EventName:  "post-tool-use",
		},
		{
			EventID:    "filter-3",
			ClientName: "codex-cli",
			EventName:  "post-tool-use",
		},
	}).Error; err != nil {
		t.Fatalf("failed to insert local agent audit logs: %v", err)
	}

	options, err := c.GetLocalAgentAuditLogFilterOptions(ctx, "client_name", LocalAgentAuditLogOptions{Limit: 10})
	if err != nil {
		t.Fatalf("failed to query filter options: %v", err)
	}
	want := []string{"claude-code", "codex-cli"}
	if !reflect.DeepEqual(options, want) {
		t.Fatalf("expected filter options %v, got %v", want, options)
	}
}
