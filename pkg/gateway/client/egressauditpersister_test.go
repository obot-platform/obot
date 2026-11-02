package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestEgressAuditChunkFailureIsolation(t *testing.T) {
	for _, test := range []struct {
		name string
		err  error
	}{
		{
			name: "database",
			err:  errors.New("database unavailable"),
		},
		{
			name: "deadline",
			err:  context.DeadlineExceeded,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			c := newTestClient(t)
			c.egressAuditBatchSize = 2
			payload := json.RawMessage(`"c2VjcmV0"`)
			for _, entry := range []types.EgressAuditLog{
				{ID: "first", UserID: "7"},
				{ID: "ordered", UserID: "7", RequestBody: payload, Outcome: "running"},
				{ID: "rolled-back", UserID: "7", RequestBody: payload},
				{ID: "ordered", Finalized: true, Outcome: "complete", ResponseStatus: 200},
				{ID: "tail-1", UserID: "7"},
				{ID: "tail-2", UserID: "7"},
			} {
				c.LogEgressAuditEntry(entry)
			}
			snapshot := c.egressAuditBuffer

			creates := 0
			require.NoError(t, c.db.WithContext(t.Context()).Callback().Create().After("gorm:create").Register("test_chunk_failure", func(tx *gorm.DB) {
				creates++
				require.LessOrEqual(t, tx.Statement.ReflectValue.Len(), 2)
				if creates == 3 {
					// Fail after both writes in chunk two, proving its capture also rolls back.
					c.LogEgressAuditEntry(types.EgressAuditLog{ID: "arrival", UserID: "7"})
					_ = tx.AddError(test.err)
				}
			}))

			require.ErrorIs(t, c.FlushEgressAuditLogs(), test.err)
			require.Equal(t, 3, creates)
			require.Zero(t, snapshot[0])
			require.Zero(t, snapshot[1], "committed payload references must be released")
			var queuedIDs []string
			for _, entry := range c.egressAuditBuffer {
				queuedIDs = append(queuedIDs, entry.ID)
			}
			require.Equal(t, []string{"rolled-back", "ordered", "tail-1", "tail-2", "arrival"}, queuedIDs)

			var persisted []types.EgressAuditLog
			require.NoError(t, c.db.WithContext(t.Context()).Order("id").Find(&persisted).Error)
			require.Len(t, persisted, 2)
			require.Equal(t, "first", persisted[0].ID)
			require.Equal(t, "ordered", persisted[1].ID)
			require.Equal(t, "running", persisted[1].Outcome)

			require.NoError(t, c.db.WithContext(t.Context()).Callback().Create().Remove("test_chunk_failure"))
			require.NoError(t, c.FlushEgressAuditLogs())
			require.Nil(t, c.egressAuditBuffer)
			_, total, err := c.ListEgressAuditLogs(t.Context(), EgressAuditLogOptions{Limit: 10})
			require.NoError(t, err)
			require.EqualValues(t, 6, total)
			ordered, err := c.GetEgressAuditLog(t.Context(), "ordered", true)
			require.NoError(t, err)
			require.Equal(t, "complete", ordered.Outcome)
			require.Equal(t, 200, ordered.ResponseStatus)
			require.Equal(t, payload, ordered.RequestBody)
		})
	}
}

func TestEgressAuditBufferReleaseAndStableThreshold(t *testing.T) {
	c := newTestClient(t)
	c.egressAuditBatchSize = 2
	c.kickEgressAuditPersist = make(chan struct{}, 1)

	// Check the empty buffer path independently of shutdown context tracking.
	require.Zero(t, testing.AllocsPerRun(10, func() {
		require.NoError(t, c.flushEgressAuditLogs(t.Context()))
	}))
	require.Nil(t, c.egressAuditBuffer)

	for i := range 20 {
		c.LogEgressAuditEntry(types.EgressAuditLog{ID: fmt.Sprintf("backlog-%d", i), UserID: "7"})
	}
	require.NoError(t, c.FlushEgressAuditLogs())
	require.Nil(t, c.egressAuditBuffer, "draining must release the peak backing store")
	<-c.kickEgressAuditPersist

	c.LogEgressAuditEntry(types.EgressAuditLog{ID: "next-1", UserID: "7"})
	require.Empty(t, c.kickEgressAuditPersist)
	c.LogEgressAuditEntry(types.EgressAuditLog{ID: "next-2", UserID: "7"})
	require.Len(t, c.kickEgressAuditPersist, 1)
}

func TestEgressAuditFlushLeavesLaterArrivalsQueued(t *testing.T) {
	c := newTestClient(t)
	c.LogEgressAuditEntry(types.EgressAuditLog{ID: "snapshot", UserID: "7"})
	require.NoError(t, c.db.WithContext(t.Context()).Callback().Create().Before("gorm:create").Register("test_later_arrival", func(*gorm.DB) {
		c.LogEgressAuditEntry(types.EgressAuditLog{ID: "later", UserID: "7"})
	}))

	require.NoError(t, c.FlushEgressAuditLogs())
	require.Len(t, c.egressAuditBuffer, 1)
	require.Equal(t, "later", c.egressAuditBuffer[0].ID)
	_, total, err := c.ListEgressAuditLogs(t.Context(), EgressAuditLogOptions{Limit: 10})
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
}

func TestEgressAuditFailureCooldown(t *testing.T) {
	c := newTestClient(t)
	c.egressAuditBatchSize = 2
	c.kickEgressAuditPersist = make(chan struct{}, 1)
	done := make(chan struct{})
	ctx, cancel := context.WithCancel(t.Context())
	defer func() {
		cancel()
		<-done
	}()

	var failing atomic.Bool
	failing.Store(true)
	attempts := make(chan time.Time, 100)
	entered := make(chan struct{})
	release := make(chan struct{})
	defer func() {
		// Also unblock the writer if an assertion fails before the explicit release.
		select {
		case <-release:
		default:
			close(release)
		}
	}()
	first := true
	require.NoError(t, c.db.WithContext(t.Context()).Callback().Create().Before("gorm:create").Register("test_cooldown", func(tx *gorm.DB) {
		if first {
			first = false
			close(entered)
			<-release
		}

		if failing.Load() {
			_ = tx.AddError(errors.New("database unavailable"))
		}
		attempts <- time.Now()
	}))

	const interval = time.Second
	go func() {
		defer close(done)
		c.runEgressAuditPersistenceLoop(ctx, interval)
	}()
	c.LogEgressAuditEntry(types.EgressAuditLog{ID: "first", UserID: "7"})
	c.LogEgressAuditEntry(types.EgressAuditLog{ID: "second", UserID: "7"})
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("threshold flush did not start")
	}

	// Queue a pending kick while the failed write is still in flight.
	c.LogEgressAuditEntry(types.EgressAuditLog{ID: "arrival-1", UserID: "7"})
	c.LogEgressAuditEntry(types.EgressAuditLog{ID: "arrival-2", UserID: "7"})
	close(release)
	failedAt := <-attempts

	producerDone := make(chan struct{})
	producerCtx, stopProducer := context.WithCancel(t.Context())
	defer func() {
		stopProducer()
		<-producerDone
	}()
	go func() {
		defer close(producerDone)
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-producerCtx.Done():
				return
			case <-ticker.C:
				c.LogEgressAuditEntry(types.EgressAuditLog{ID: "sustained", UserID: "7"})
			}
		}
	}()

	select {
	case <-attempts:
		t.Fatal("pending kicks or sustained arrivals bypassed cooldown")
	case <-time.After(interval / 2):
	}
	stopProducer()

	// Keep the next retry failing to verify fixed pacing across repeated failures.
	select {
	case retriedAt := <-attempts:
		require.GreaterOrEqual(t, retriedAt.Sub(failedAt), interval)
	case <-time.After(3 * interval):
		t.Fatal("interval retry did not resume")
	}
	failing.Store(false)

	// Explicit flushes bypass background cooldown and recovery restores threshold kicks.
	require.NoError(t, c.FlushEgressAuditLogs())
	for len(attempts) > 0 {
		<-attempts
	}
	require.Eventually(t, func() bool {
		c.egressAuditLock.Lock()
		defer c.egressAuditLock.Unlock()

		return len(c.kickEgressAuditPersist) == 0
	}, 3*interval, 10*time.Millisecond)
	c.LogEgressAuditEntry(types.EgressAuditLog{ID: "recovered-1", UserID: "7"})
	c.LogEgressAuditEntry(types.EgressAuditLog{ID: "recovered-2", UserID: "7"})
	select {
	case <-attempts:
	case <-time.After(interval / 2):
		t.Fatal("threshold flush did not resume after recovery")
	}

	failing.Store(true)
	c.LogEgressAuditEntry(types.EgressAuditLog{ID: "cancel-1", UserID: "7"})
	c.LogEgressAuditEntry(types.EgressAuditLog{ID: "cancel-2", UserID: "7"})
	select {
	case <-attempts:
	case <-time.After(3 * interval):
		t.Fatal("failure before cancellation did not start")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(interval / 2):
		t.Fatal("cancellation did not interrupt cooldown")
	}
}
