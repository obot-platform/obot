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
			list, total, err := c.ListEgressAuditLogs(t.Context(), EgressAuditLogOptions{Limit: 50, UserID: []string{"7"}})
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
			_, total, err = c.ListEgressAuditLogs(t.Context(), EgressAuditLogOptions{Limit: 50, EnvironmentID: []string{"different"}})
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

func TestGetEgressAuditLogFilterOptions(t *testing.T) {
	c := newTestClient(t)
	now := time.Date(2026, time.October, 21, 12, 0, 0, 0, time.UTC)
	for i := range 105 {
		require.NoError(t, c.SaveEgressAuditLog(t.Context(), types.EgressAuditLog{
			ID:            fmt.Sprintf("filter-%03d", i),
			CreatedAt:     now.Add(time.Duration(i) * time.Minute),
			UserID:        "7",
			Host:          fmt.Sprintf("host-%03d.example", i),
			ClaudeSession: fmt.Sprintf("session-%03d", i),
			ClaudeAccount: "account",
			EnvironmentID: "env",
			Outcome:       "complete",
		}))
	}
	for _, entry := range []types.EgressAuditLog{
		{
			ID:            "duplicate",
			CreatedAt:     now,
			UserID:        "8",
			Host:          "host-000.example",
			ClaudeSession: "session-000",
			ClaudeAccount: "account",
		},
		{
			ID:        "empty",
			CreatedAt: now,
			UserID:    "7",
		},
		{
			ID:            "unattributed",
			CreatedAt:     now,
			Host:          "hidden.example",
			ClaudeSession: "hidden-session",
			ClaudeAccount: "hidden-account",
			Finalized:     true,
		},
	} {
		require.NoError(t, c.SaveEgressAuditLog(t.Context(), entry))
	}

	for _, tc := range []struct {
		name   string
		filter string
		opts   EgressAuditLogOptions
		want   []string
	}{
		{
			name:   "hosts sorted before limit and offset ignored",
			filter: "host",
			opts: EgressAuditLogOptions{
				Limit:  2,
				Offset: 104,
			},
			want: []string{"host-000.example", "host-001.example"},
		},
		{
			name:   "sessions",
			filter: "claude_session",
			opts: EgressAuditLogOptions{
				Limit: 2,
			},
			want: []string{"session-000", "session-001"},
		},
		{
			name:   "accounts exclude empty duplicates and unattributed records",
			filter: "claude_account",
			want:   []string{"account"},
		},
		{
			name:   "all constraints including current field and inclusive time bounds",
			filter: "host",
			opts: EgressAuditLogOptions{
				UserID:        []string{"7"},
				ClaudeAccount: []string{"account"},
				EnvironmentID: []string{"env"},
				ClaudeSession: []string{"session-001"},
				Host:          []string{"host-001.example"},
				Outcome:       []string{"complete"},
				StartTime:     now.Add(time.Minute),
				EndTime:       now.Add(time.Minute),
			},
			want: []string{"host-001.example"},
		},
		{
			name:   "no matches",
			filter: "host",
			opts: EgressAuditLogOptions{
				UserID: []string{"missing"},
			},
			want: []string{},
		},
		{
			name:   "outside start time",
			filter: "host",
			opts: EgressAuditLogOptions{
				StartTime: now.Add(2 * time.Hour),
			},
			want: []string{},
		},
		{
			name:   "outside end time",
			filter: "host",
			opts: EgressAuditLogOptions{
				EndTime: now.Add(-time.Minute),
			},
			want: []string{},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := c.GetEgressAuditLogFilterOptions(t.Context(), tc.filter, tc.opts)
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
	for _, tc := range []struct {
		name string
		opts EgressAuditLogOptions
	}{
		{
			name: "user",
			opts: EgressAuditLogOptions{UserID: []string{"missing"}},
		},
		{
			name: "account",
			opts: EgressAuditLogOptions{ClaudeAccount: []string{"missing"}},
		},
		{
			name: "environment",
			opts: EgressAuditLogOptions{EnvironmentID: []string{"missing"}},
		},
		{
			name: "session",
			opts: EgressAuditLogOptions{ClaudeSession: []string{"missing"}},
		},
		{
			name: "host",
			opts: EgressAuditLogOptions{Host: []string{"missing"}},
		},
		{
			name: "outcome",
			opts: EgressAuditLogOptions{Outcome: []string{"missing"}},
		},
	} {
		t.Run(tc.name+" excludes nonmatching rows", func(t *testing.T) {
			got, err := c.GetEgressAuditLogFilterOptions(t.Context(), "host", tc.opts)
			require.NoError(t, err)
			require.Equal(t, []string{}, got)
		})
	}

	for _, limit := range []int{0, -1, 1000} {
		got, err := c.GetEgressAuditLogFilterOptions(t.Context(), "host", EgressAuditLogOptions{Limit: limit})
		require.NoError(t, err)
		require.Len(t, got, 105)
	}
	for _, filter := range []string{"", "user_id", "host; DROP TABLE egress_audit_logs"} {
		_, err := c.GetEgressAuditLogFilterOptions(t.Context(), filter, EgressAuditLogOptions{})
		require.ErrorContains(t, err, "invalid egress audit log filter")
	}
}

func TestEgressAuditLogMultiValueFilters(t *testing.T) {
	c := newTestClient(t)
	for _, value := range []string{"alpha", "beta", "gamma"} {
		require.NoError(t, c.SaveEgressAuditLog(t.Context(), types.EgressAuditLog{
			ID:            value,
			CreatedAt:     time.Now().UTC(),
			UserID:        value,
			ClaudeAccount: value,
			EnvironmentID: value,
			ClaudeSession: value,
			Host:          value,
			Outcome:       value,
		}))
	}
	for _, tc := range []struct {
		name string
		opts EgressAuditLogOptions
		want []string
	}{
		{
			name: "users OR",
			opts: EgressAuditLogOptions{UserID: []string{"alpha", "beta"}},
			want: []string{"alpha", "beta"},
		},
		{
			name: "accounts OR",
			opts: EgressAuditLogOptions{ClaudeAccount: []string{"alpha", "beta"}},
			want: []string{"alpha", "beta"},
		},
		{
			name: "environments OR",
			opts: EgressAuditLogOptions{EnvironmentID: []string{"alpha", "beta"}},
			want: []string{"alpha", "beta"},
		},
		{
			name: "sessions OR",
			opts: EgressAuditLogOptions{ClaudeSession: []string{"alpha", "beta"}},
			want: []string{"alpha", "beta"},
		},
		{
			name: "hosts OR",
			opts: EgressAuditLogOptions{Host: []string{"alpha", "beta"}},
			want: []string{"alpha", "beta"},
		},
		{
			name: "outcomes OR",
			opts: EgressAuditLogOptions{Outcome: []string{"alpha", "beta"}},
			want: []string{"alpha", "beta"},
		},
		{
			name: "AND across fields",
			opts: EgressAuditLogOptions{
				UserID:        []string{"alpha", "beta"},
				EnvironmentID: []string{"beta", "gamma"},
				Outcome:       []string{"alpha", "beta"},
			},
			want: []string{"beta"},
		},
		{
			name: "disjoint fields",
			opts: EgressAuditLogOptions{
				UserID: []string{"alpha", "beta"},
				Host:   []string{"gamma"},
			},
			want: []string{},
		},
		{
			name: "empty filters",
			opts: EgressAuditLogOptions{Host: []string{}},
			want: []string{"alpha", "beta", "gamma"},
		},
		{
			name: "duplicate selections",
			opts: EgressAuditLogOptions{Host: []string{"alpha", "alpha"}},
			want: []string{"alpha"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			opts := tc.opts
			opts.Limit = 100
			entries, total, err := c.ListEgressAuditLogs(t.Context(), opts)
			require.NoError(t, err)
			require.EqualValues(t, len(tc.want), total)
			ids := make([]string, 0, len(entries))
			for _, entry := range entries {
				ids = append(ids, entry.ID)
			}
			require.ElementsMatch(t, tc.want, ids)
			options, err := c.GetEgressAuditLogFilterOptions(t.Context(), "host", opts)
			require.NoError(t, err)
			require.Equal(t, tc.want, options)
		})
	}
}
