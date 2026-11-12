package client

import (
	"context"
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestAuditQueueConfiguredBatching(t *testing.T) {
	for _, test := range []struct {
		name       string
		configured int
		batch      int
	}{
		{
			name:       "default",
			configured: 0,
			batch:      1000,
		},
		{
			name:       "negative",
			configured: -1,
			batch:      1000,
		},
		{
			name:       "small",
			configured: 2,
			batch:      2,
		},
		{
			name:       "above old cap",
			configured: 512,
			batch:      512,
		},
		{
			name:       "increased",
			configured: 2000,
			batch:      2000,
		},
		{
			name:       "overflow",
			configured: math.MaxInt,
			batch:      math.MaxInt / 2,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			batch, limit := auditBatchCapacity(test.configured)
			require.Equal(t, test.batch, batch)
			require.Equal(t, 2*test.batch, limit)

			if test.configured == math.MaxInt {
				base := newTestClient(t)
				ctx, cancel := context.WithCancel(t.Context())
				cancel()
				c := New(ctx, base.db, nil, nil, nil, time.Hour, test.configured, 0, 0, 0, false)
				require.LessOrEqual(t, cap(c.auditBuffer), 2000)
				require.LessOrEqual(t, cap(c.enforcementBuffer), 2000)
				require.Equal(t, 1, cap(c.kickAuditPersist))
				require.Equal(t, 1, cap(c.kickEnforcementPersist))
				require.NoError(t, c.Close())
				return
			}

			for _, kind := range []string{"mcp", "enforcement"} {
				c := &Client{
					auditLogBatchSize:      test.configured,
					kickAuditPersist:       make(chan struct{}, 1),
					kickEnforcementPersist: make(chan struct{}, 1),
				}
				q := auditQueueForTest(c, kind)
				kick := c.kickAuditPersist
				if kind == "enforcement" {
					kick = c.kickEnforcementPersist
				}

				for range batch - 1 {
					q.log("queued", 0)
				}
				require.Empty(t, kick)
				q.log("queued", 0)
				require.Len(t, kick, 1)
				for range limit - batch + 1 {
					q.log("queued", 0)
				}
				require.Len(t, q.state().ids, limit+1)
			}
		})
	}
}

func newRunningAuditClient(t *testing.T, batchSize int) *Client {
	t.Helper()

	// Cancellation can discard SQLite's last connection; keep the schema on disk for retries.
	c := newTestClientWithDSN(t, "sqlite://"+filepath.Join(t.TempDir(), "audit.db"))
	c.auditLogBatchSize = batchSize
	c.kickAuditPersist = make(chan struct{}, 1)
	c.kickEnforcementPersist = make(chan struct{}, 1)
	c.kickEgressAuditPersist = make(chan struct{}, 1)
	c.egressAuditBatchSize = batchSize
	c.startAuditLoops(t.Context(), time.Hour)
	t.Cleanup(func() { _ = c.Close() })

	return c
}

func TestAuditLoopRetainsWakeupDuringWrite(t *testing.T) {
	for _, kind := range []string{"mcp", "enforcement"} {
		for _, fail := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/failure=%t", kind, fail), func(t *testing.T) {
				c := newRunningAuditClient(t, 2)
				q := auditQueueForTest(c, kind)
				entered := make(chan int32, 4)
				release := make(chan struct{})
				defer close(release)
				var calls atomic.Int32
				outage := errors.New("database unavailable")
				require.NoError(t, c.db.WithContext(t.Context()).Callback().Create().Before("gorm:create").Register("test_wakeup", func(tx *gorm.DB) {
					if tx.Statement.Table != q.table {
						return
					}

					call := calls.Add(1)
					entered <- call
					if call == 1 {
						<-release
					}
					if fail {
						_ = tx.AddError(outage)
					}
				}))

				q.log("first-1", 0)
				q.log("first-2", 0)
				require.EqualValues(t, 1, awaitAuditQueue(t, entered))
				q.log("later-1", 0)
				q.log("later-2", 0)

				release <- struct{}{}
				require.EqualValues(t, 2, awaitAuditQueue(t, entered), "buffered wakeup must beat the hour-long timer")
				if fail {
					require.Eventually(t, func() bool { return len(q.state().ids) == 4 }, time.Second, time.Millisecond)
				} else {
					require.Eventually(t, func() bool { return len(q.state().ids) == 0 }, time.Second, time.Millisecond)
				}
			})
		}
	}
}

func TestAuditClosePreservesPartialAndQueuedLLM(t *testing.T) {
	c := newRunningAuditClient(t, 1000)
	var lock sync.Mutex
	counts := map[string]int64{}
	var queryErrors []error
	require.NoError(t, c.db.WithContext(t.Context()).Callback().Create().After("gorm:create").Register("test_shutdown_rows", func(tx *gorm.DB) {
		var count int64
		err := tx.Session(&gorm.Session{NewDB: true}).Table(tx.Statement.Table).Count(&count).Error
		lock.Lock()
		defer lock.Unlock()

		counts[tx.Statement.Table] = count
		if err != nil {
			queryErrors = append(queryErrors, err)
		}
	}))

	c.LogLLMAuditEntry(types.LLMAuditLog{ID: "partial"})
	require.Eventually(t, func() bool { return len(c.llmAuditEntries) == 0 }, time.Second, time.Millisecond)
	// Parent/loop cancellation must preserve the local batch for CloseContext, not write on a new deadline.
	c.auditLoopsCancel()
	c.auditLoops.Wait()
	require.Len(t, c.llmAuditPartial, 1)
	c.LogLLMAuditEntry(types.LLMAuditLog{ID: "queued-1"})
	c.LogLLMAuditEntry(types.LLMAuditLog{ID: "queued-2"})
	c.LogMCPAuditEntry(types.MCPAuditLog{UserID: "mcp"})
	c.LogEnforcementDecision(types.EnforcementDecisionLog{DeviceID: "device"})
	c.LogEgressAuditEntry(types.EgressAuditLog{ID: "egress"})

	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	require.NoError(t, c.CloseContext(ctx))
	lock.Lock()
	require.Empty(t, queryErrors)
	require.Equal(t, map[string]int64{
		"mcp_audit_logs":            1,
		"enforcement_decision_logs": 1,
		"egress_audit_logs":         1,
		"llm_audit_logs":            3,
	}, counts)
	lock.Unlock()
	require.Zero(t, len(c.auditBuffer))
	require.Zero(t, len(c.enforcementBuffer))
	require.Zero(t, len(c.egressAuditBuffer))
	require.Empty(t, c.llmAuditEntries)
	require.Empty(t, c.llmAuditPartial)
	require.Error(t, c.FlushEgressAuditLogs(), "explicit/deferred flush must not reopen the closed writer")
}

func TestAuditCloseWaitsForActiveLoopWrites(t *testing.T) {
	for _, kind := range []string{"mcp", "enforcement", "egress", "llm"} {
		t.Run(kind, func(t *testing.T) {
			c := newRunningAuditClient(t, 1)
			entered := make(chan context.Context, 1)
			release := make(chan struct{})
			defer close(release)
			loopsStopped := make(chan struct{})
			stopLoops := c.auditLoopsCancel
			c.auditLoopsCancel = func() {
				stopLoops()
				select {
				case <-loopsStopped:
				default:
					close(loopsStopped)
				}
			}

			var attempts atomic.Int32
			var written atomic.Int64
			require.NoError(t, c.db.WithContext(t.Context()).Callback().Create().Before("gorm:create").Register("test_cancel_loop", func(tx *gorm.DB) {
				if attempts.Add(1) == 1 {
					entered <- tx.Statement.Context
					select {
					case <-release:
					case <-tx.Statement.Context.Done():
						_ = tx.AddError(tx.Statement.Context.Err())
					}
				}
			}))
			require.NoError(t, c.db.WithContext(t.Context()).Callback().Create().After("gorm:create").Register("test_count_loop_rows", func(tx *gorm.DB) {
				if tx.Error == nil {
					written.Add(tx.RowsAffected)
				}
			}))

			want := int64(1)
			switch kind {
			case "mcp", "enforcement":
				auditQueueForTest(c, kind).log("accepted", 0)
			case "egress":
				c.LogEgressAuditEntry(types.EgressAuditLog{ID: "accepted"})
			case "llm":
				want = int64(c.llmAuditBatchSize)
				for i := range c.llmAuditBatchSize {
					c.LogLLMAuditEntry(types.LLMAuditLog{ID: fmt.Sprint(i)})
				}
			}
			writeCtx := awaitAuditQueue(t, entered)

			ctx, cancel := context.WithTimeout(t.Context(), time.Second)
			defer cancel()
			closed := make(chan error, 1)
			go func() { closed <- c.CloseContext(ctx) }()
			awaitAuditQueue(t, loopsStopped)

			select {
			case <-writeCtx.Done():
				t.Fatal("stopping audit loops canceled an active write")
			case err := <-closed:
				t.Fatalf("shutdown returned before the active write finished: %v", err)
			case <-time.After(30 * time.Millisecond):
			}

			release <- struct{}{}
			require.NoError(t, awaitAuditQueue(t, closed))
			require.EqualValues(t, 1, attempts.Load())
			require.Equal(t, want, written.Load())
			require.Zero(t, len(c.auditBuffer))
			require.Zero(t, len(c.enforcementBuffer))
			require.Zero(t, len(c.egressAuditBuffer))
			require.Empty(t, c.llmAuditPartial)
			require.Empty(t, c.llmAuditEntries)
		})
	}
}

func TestAuditCloseContextBudget(t *testing.T) {
	for _, kind := range []string{"egress", "mcp", "enforcement", "llm", "queued", "expired"} {
		t.Run(kind, func(t *testing.T) {
			c := newRunningAuditClient(t, 1)
			c.auditLoopsCancel()
			c.auditLoops.Wait()
			entered := make(chan context.Context, 8)
			var active atomic.Int32
			var calls atomic.Int32
			require.NoError(t, c.db.WithContext(t.Context()).Callback().Create().Before("gorm:create").Register("test_context_delay", func(tx *gorm.DB) {
				active.Add(1)
				defer active.Add(-1)
				calls.Add(1)
				entered <- tx.Statement.Context
				<-tx.Statement.Context.Done()
				_ = tx.AddError(tx.Statement.Context.Err())
			}))

			var flush func() error
			switch kind {
			case "egress", "expired":
				c.LogEgressAuditEntry(types.EgressAuditLog{ID: "egress"})
				flush = c.FlushEgressAuditLogs
			case "mcp", "enforcement":
				q := auditQueueForTest(c, kind)
				q.log("pending", 0)
				flush = q.persist
			case "llm":
				c.LogLLMAuditEntry(types.LLMAuditLog{ID: "llm"})
				// Restart only the owned LLM loop to exercise cancellation of a local in-flight batch.
				loopCtx, cancel := context.WithCancel(t.Context())
				c.auditLoopsCancel = cancel
				c.auditLoops.Go(func() {
					c.runLLMAuditPersistenceLoop(loopCtx, 1, time.Hour)
				})
			case "queued":
				c.LogEgressAuditEntry(types.EgressAuditLog{ID: "egress"})
				c.LogMCPAuditEntry(types.MCPAuditLog{UserID: "mcp"})
				c.LogEnforcementDecision(types.EnforcementDecisionLog{DeviceID: "device"})
				c.LogLLMAuditEntry(types.LLMAuditLog{ID: "llm"})
			}

			var flushed chan error
			if flush != nil {
				flushed = make(chan error, 1)
				go func() { flushed <- flush() }()
			}
			if kind != "queued" {
				awaitAuditQueue(t, entered)
			}

			deadline := time.Now().Add(100 * time.Millisecond)
			if kind == "expired" {
				deadline = time.Now().Add(-time.Second)
			}
			ctx, cancel := context.WithDeadline(t.Context(), deadline)
			defer cancel()
			started := time.Now()
			err := c.CloseContext(ctx)
			elapsed := time.Since(started)
			require.ErrorIs(t, err, context.DeadlineExceeded)
			if kind != "expired" {
				require.GreaterOrEqual(t, elapsed, 80*time.Millisecond)
			}
			require.Less(t, elapsed, 350*time.Millisecond, "all queues and ongoing writes share one deadline")
			if flushed != nil {
				require.Error(t, awaitAuditQueue(t, flushed))
			}
			require.Zero(t, active.Load(), "CloseContext must join every cooperative writer")
			require.GreaterOrEqual(t, len(c.auditBuffer), 0)
			require.GreaterOrEqual(t, len(c.enforcementBuffer), 0)
			require.GreaterOrEqual(t, len(c.egressAuditBuffer), 0)
			switch kind {
			case "mcp":
				require.Equal(t, 1, len(c.auditBuffer))
			case "enforcement":
				require.Equal(t, 1, len(c.enforcementBuffer))
			case "egress", "expired":
				require.Equal(t, 1, len(c.egressAuditBuffer))
			case "llm":
				require.Equal(t, 1, len(c.llmAuditPartial)+len(c.llmAuditEntries))
				require.EqualValues(t, 1, calls.Load(), "an expired shutdown budget must not retry the canceled write")
			case "queued":
				require.Equal(t, 1, len(c.auditBuffer))
				require.Equal(t, 1, len(c.enforcementBuffer))
				require.Equal(t, 1, len(c.egressAuditBuffer))
				require.Equal(t, 1, len(c.llmAuditEntries))
			}
			require.Error(t, c.db.Check(t.Context()), "database is closed after writer coordination")
			before := calls.Load()
			require.Error(t, c.FlushEgressAuditLogs())
			require.Equal(t, before, calls.Load(), "no writes may start after database closure")
		})
	}
}

func TestLogMCPAuditEntryNoopsWhenDisabled(t *testing.T) {
	c := newTestClient(t)
	require.True(t, c.MCPAuditLogEnabled())

	c.LogMCPAuditEntry(types.MCPAuditLog{UserID: "enabled"})
	require.Len(t, c.auditBuffer, 1)

	c.DisableMCPAuditLog()
	require.False(t, c.MCPAuditLogEnabled())

	c.LogMCPAuditEntry(types.MCPAuditLog{UserID: "disabled"})
	require.Len(t, c.auditBuffer, 1)
	require.Equal(t, "enabled", c.auditBuffer[0].UserID)
}
