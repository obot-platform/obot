package client

import (
	"bytes"
	"context"
	"encoding/json"
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
