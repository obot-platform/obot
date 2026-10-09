package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/server/options/encryptionconfig"
	"k8s.io/apiserver/pkg/storage/value"
)

func TestEgressAuditPayloadAccess(t *testing.T) {
	for _, encrypt := range []bool{false, true} {
		t.Run(map[bool]string{false: "plain", true: "encrypted"}[encrypt], func(t *testing.T) {
			c := newTestClient(t)
			if encrypt {
				c.encryptionConfig = &encryptionconfig.EncryptionConfiguration{Transformers: map[schema.GroupResource]value.Transformer{egressAuditResource: testTransformer{}}}
			}
			entry := types.EgressAuditLog{
				ID: "test", CreatedAt: time.Now().UTC(), UserID: "7", ClaudeAccount: "user_claude", EnvironmentID: "env", ClaudeSession: "session", Outcome: "running",
				RequestBody: json.RawMessage(`"c2VjcmV0"`), ResponseBody: json.RawMessage(`"cmVzcG9uc2U="`),
				RequestHeaders: json.RawMessage(`{"content-type":["text/plain"]}`), ResponseHeaders: json.RawMessage(`{"content-type":["text/plain"]}`),
			}
			require.NoError(t, c.SaveEgressAuditLog(t.Context(), entry))
			entry.ResponseStatus = 200
			entry.Outcome = "complete"
			require.NoError(t, c.SaveEgressAuditLog(t.Context(), entry))
			list, total, err := c.ListEgressAuditLogs(t.Context(), EgressAuditLogOptions{Limit: 50, UserID: "7"})
			require.NoError(t, err)
			require.EqualValues(t, 1, total)
			require.Len(t, list, 1)
			require.Empty(t, list[0].RequestBody)
			require.Empty(t, list[0].ResponseHeaders)
			for _, auditor := range []bool{false, true} {
				got, err := c.GetEgressAuditLog(t.Context(), entry.ID, auditor)
				require.NoError(t, err)
				if auditor {
					require.Equal(t, entry.RequestBody, got.RequestBody)
					require.Equal(t, entry.ResponseBody, got.ResponseBody)
				} else {
					require.Empty(t, got.RequestBody)
					require.Empty(t, got.ResponseBody)
					require.Empty(t, got.RequestHeaders)
					require.Empty(t, got.ResponseHeaders)
				}
			}
			_, total, err = c.ListEgressAuditLogs(t.Context(), EgressAuditLogOptions{Limit: 50, EnvironmentID: "different"})
			require.NoError(t, err)
			require.Zero(t, total)
			c.LogEgressAuditEntry(types.EgressAuditLog{
				ID:             entry.ID,
				ResponseStatus: 403,
				DurationMs:     2,
				Outcome:        "denied",
				Finalized:      true,
			})
			require.NoError(t, c.FlushEgressAuditLogs())
			got, err := c.GetEgressAuditLog(t.Context(), entry.ID, true)
			require.NoError(t, err)
			require.Equal(t, "denied", got.Outcome)
			require.Equal(t, entry.ResponseBody, got.ResponseBody)
			if encrypt {
				c.encryptionConfig = nil
				_, err = c.GetEgressAuditLog(t.Context(), entry.ID, true)
				require.Error(t, err)
				_, err = c.GetEgressAuditLog(t.Context(), entry.ID, false)
				require.NoError(t, err)
			}
		})
	}
}

func TestBufferedEgressAuditFinalStatusOrdering(t *testing.T) {
	for _, finalFirst := range []bool{false, true} {
		t.Run(map[bool]string{false: "capture-first", true: "status-first"}[finalFirst], func(t *testing.T) {
			c := newTestClient(t)
			c.encryptionConfig = &encryptionconfig.EncryptionConfiguration{Transformers: map[schema.GroupResource]value.Transformer{egressAuditResource: testTransformer{}}}
			capture := types.EgressAuditLog{
				ID:          "ordered",
				CreatedAt:   time.Now().UTC(),
				UserID:      "7",
				Outcome:     "interrupted",
				RequestBody: json.RawMessage(`"c2VjcmV0"`),
			}
			final := types.EgressAuditLog{
				ID:             capture.ID,
				CreatedAt:      time.Now().UTC(),
				ResponseStatus: 403,
				DurationMs:     123,
				Outcome:        "denied",
				Finalized:      true,
			}
			first, second := capture, final
			if finalFirst {
				first, second = final, capture
			}
			c.LogEgressAuditEntry(first)
			require.NoError(t, c.FlushEgressAuditLogs())
			if finalFirst {
				_, total, err := c.ListEgressAuditLogs(t.Context(), EgressAuditLogOptions{Limit: 10})
				require.NoError(t, err)
				require.Zero(t, total, "metadata-only status must not appear as an unattributed audit")
			}
			c.LogEgressAuditEntry(second)
			require.NoError(t, c.FlushEgressAuditLogs())
			entry, err := c.GetEgressAuditLog(t.Context(), capture.ID, true)
			require.NoError(t, err)
			require.Equal(t, "denied", entry.Outcome)
			require.Equal(t, 403, entry.ResponseStatus)
			require.EqualValues(t, 123, entry.DurationMs)
			require.Equal(t, capture.RequestBody, entry.RequestBody)
			require.Equal(t, capture.UserID, entry.UserID)
		})
	}
}

func TestEgressAuditCloseDrainsAfterCancellation(t *testing.T) {
	c := newTestClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	c.egressAuditCancel = cancel
	c.egressAuditDone = make(chan struct{})
	go c.runEgressAuditPersistenceLoop(ctx, time.Hour)
	c.LogEgressAuditEntry(types.EgressAuditLog{
		ID:      "shutdown",
		UserID:  "7",
		Outcome: "interrupted",
	})
	var written bool
	require.NoError(t, c.db.WithContext(t.Context()).Callback().Create().After("gorm:create").Register("test-egress-close", func(tx *gorm.DB) {
		if tx.Statement.Table == "egress_audit_logs" {
			written = true
			require.NoError(t, tx.Statement.Context.Err())
		}
	}))
	cancel()
	require.NoError(t, c.Close())
	require.True(t, written)
	require.Empty(t, c.egressAuditBuffer)
}

func TestEgressAuditPersistenceTriggers(t *testing.T) {
	for _, trigger := range []string{"timer", "threshold"} {
		t.Run(trigger, func(t *testing.T) {
			c := newTestClient(t)
			ctx, cancel := context.WithCancel(t.Context())
			c.egressAuditDone = make(chan struct{})
			c.kickEgressAuditPersist = make(chan struct{}, 1)
			c.egressAuditBuffer = make([]types.EgressAuditLog, 0, 4)
			interval := time.Hour
			if trigger == "timer" {
				interval = time.Millisecond
			}
			go c.runEgressAuditPersistenceLoop(ctx, interval)
			defer func() { cancel(); <-c.egressAuditDone }()

			c.LogEgressAuditEntry(types.EgressAuditLog{ID: "first", UserID: "7", Outcome: "complete"})
			want := int64(1)
			if trigger == "threshold" {
				c.LogEgressAuditEntry(types.EgressAuditLog{ID: "second", UserID: "7", Outcome: "complete"})
				want = 2
			}
			require.Eventually(t, func() bool {
				_, total, err := c.ListEgressAuditLogs(t.Context(), EgressAuditLogOptions{Limit: 10})
				return err == nil && total == want
			}, time.Second, time.Millisecond)
		})
	}
}

func TestEgressAuditQueueBounds(t *testing.T) {
	for _, bodies := range []bool{false, true} {
		t.Run(fmt.Sprint(bodies), func(t *testing.T) {
			c := newTestClient(t)
			entry := types.EgressAuditLog{ID: "queued", UserID: "7"}
			if bodies {
				entry.RequestBody = make([]byte, 1<<20)
			}
			size := egressAuditEntryBytes(entry)
			limit := min(egressAuditMaxEntries, egressAuditMaxBytes/size)
			for range limit + 10 {
				c.LogEgressAuditEntry(entry)
			}
			require.Len(t, c.egressAuditBuffer, limit)
			require.Equal(t, limit, c.egressAuditPending)
			require.Equal(t, limit*size, c.egressAuditBytes)
			require.EqualValues(t, 10, c.egressAuditDropped)
		})
	}
}

func TestEgressAuditFailedFlushPreservesBoundDuringEnqueue(t *testing.T) {
	c := newTestClient(t)
	c.LogEgressAuditEntry(types.EgressAuditLog{ID: "original", UserID: "7"})
	outage := errors.New("database unavailable")
	require.NoError(t, c.db.WithContext(t.Context()).Callback().Create().Before("gorm:create").Register("test_outage", func(tx *gorm.DB) {
		// Producers continue submitting while the detached batch is in flight.
		for i := range egressAuditMaxEntries {
			c.LogEgressAuditEntry(types.EgressAuditLog{ID: fmt.Sprint(i), UserID: "7"})
		}
		_ = tx.AddError(outage)
	}))
	for range 3 {
		require.ErrorIs(t, c.FlushEgressAuditLogs(), outage)
		require.Len(t, c.egressAuditBuffer, egressAuditMaxEntries)
		require.Equal(t, egressAuditMaxEntries, c.egressAuditPending)
		require.LessOrEqual(t, c.egressAuditBytes, egressAuditMaxBytes)
		require.Equal(t, "original", c.egressAuditBuffer[0].ID)
	}
	require.Positive(t, c.egressAuditDropped)
	require.NoError(t, c.db.WithContext(t.Context()).Callback().Create().Remove("test_outage"))
	require.NoError(t, c.FlushEgressAuditLogs())
	require.Empty(t, c.egressAuditBuffer)
	require.Zero(t, c.egressAuditPending)
	require.Zero(t, c.egressAuditBytes)
	_, err := c.GetEgressAuditLog(t.Context(), "original", false)
	require.NoError(t, err)
	c.LogEgressAuditEntry(types.EgressAuditLog{ID: "recovered", UserID: "7"})
	require.Equal(t, 1, c.egressAuditPending)
}
