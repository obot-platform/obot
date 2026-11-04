package client

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type auditQueueState struct {
	ids []string
}

type auditQueueTest struct {
	log      func(string, int)
	persist  func() error
	state    func() auditQueueState
	table    string
	idColumn string
}

// Exercise ordering and persistence for both queues without a production queue abstraction.
func auditQueueForTest(c *Client, kind string) auditQueueTest {
	if kind == "mcp" {
		return auditQueueTest{
			log: func(id string, payloadBytes int) {
				headers := make([]byte, min(1, payloadBytes), payloadBytes)
				if len(headers) > 0 {
					headers[0] = '0'
				}

				c.LogMCPAuditEntry(types.MCPAuditLog{
					UserID: id,
					MCPFields: &types.MCPAuditLogFields{
						RequestHeaders: headers,
					},
				})
			},
			persist: c.persistMCPAuditLogs,
			state: func() auditQueueState {
				c.auditLock.Lock()
				defer c.auditLock.Unlock()

				state := auditQueueState{}
				for _, entry := range c.auditBuffer {
					state.ids = append(state.ids, entry.UserID)
				}

				return state
			},
			table:    "mcp_audit_logs",
			idColumn: "user_id",
		}
	}

	return auditQueueTest{
		log: func(id string, payloadBytes int) {
			c.LogEnforcementDecision(types.EnforcementDecisionLog{
				DeviceID: id,
				Reason:   strings.Repeat("x", payloadBytes),
			})
		},
		persist: c.persistEnforcementDecisions,
		state: func() auditQueueState {
			c.enforcementLock.Lock()
			defer c.enforcementLock.Unlock()

			state := auditQueueState{}
			for _, entry := range c.enforcementBuffer {
				state.ids = append(state.ids, entry.DeviceID)
			}

			return state
		},
		table:    "enforcement_decision_logs",
		idColumn: "device_id",
	}
}

func TestAuditQueueGrowsBeyondBatchCapacity(t *testing.T) {
	for _, kind := range []string{"mcp", "enforcement"} {
		t.Run(kind, func(t *testing.T) {
			q := auditQueueForTest(&Client{
				auditLogBatchSize: 2,
			}, kind)

			for range 6 {
				q.log("queued", 0)
			}

			q.log("large", 65<<20)
			require.Len(t, q.state().ids, 7)
		})
	}
}

func TestAuditQueueFailuresAndRecovery(t *testing.T) {
	for _, kind := range []string{"mcp", "enforcement"} {
		for _, payloadBytes := range []int{0, 8 << 10} {
			t.Run(fmt.Sprintf("%s/%d", kind, payloadBytes), func(t *testing.T) {
				c := newTestClient(t)
				c.auditLogBatchSize = 8
				q := auditQueueForTest(c, kind)
				q.log("original-00", payloadBytes)
				limit := 20
				outage := errors.New("database unavailable")
				g := c.db.WithContext(t.Context())
				require.NoError(t, g.Callback().Create().Before("gorm:create").Register("test_audit_outage", func(tx *gorm.DB) {
					if tx.Statement.Table != q.table {
						return
					}

					// A producer must finish while the detached batch is still in the DB callback.
					produced := make(chan struct{})
					go func() {
						defer close(produced)

						for i := range limit {
							q.log(fmt.Sprintf("arrival-%03d", i), payloadBytes)
						}
					}()
					select {
					case <-produced:
					case <-time.After(5 * time.Second):
						t.Error("admission blocked on database work")
					}

					_ = tx.AddError(outage)
				}))

				originalOrder := []string{"original-00"}
				for attempt := range 3 {
					require.ErrorIs(t, q.persist(), outage)
					state := q.state()
					require.Len(t, state.ids, 1+(attempt+1)*limit)
					require.Equal(t, "original-00", state.ids[0])
					require.Equal(t, originalOrder, state.ids[:len(originalOrder)])
					originalOrder = state.ids
				}

				require.NoError(t, g.Callback().Create().Remove("test_audit_outage"))
				require.NoError(t, q.persist())
				state := q.state()
				require.Empty(t, state.ids)

				var stored []string
				require.NoError(t, g.Table(q.table).Order("id").Pluck(q.idColumn, &stored).Error)
				require.Equal(t, originalOrder, stored)
				q.log("recovered", 0)
				require.NoError(t, q.persist())
			})
		}
	}
}

func TestAuditQueueConcurrentPersistence(t *testing.T) {
	for _, kind := range []string{"mcp", "enforcement"} {
		t.Run(kind, func(t *testing.T) {
			c := newTestClient(t)
			q := auditQueueForTest(c, kind)
			entered := make(chan int32, 3)
			releaseFirst := make(chan struct{})
			releaseSecond := make(chan struct{})
			defer close(releaseSecond)
			defer close(releaseFirst)
			var calls atomic.Int32
			require.NoError(t, c.db.WithContext(t.Context()).Callback().Create().Before("gorm:create").Register("test_audit_delay", func(tx *gorm.DB) {
				if tx.Statement.Table != q.table {
					return
				}

				call := calls.Add(1)
				entered <- call
				switch call {
				case 1:
					<-releaseFirst
				case 2:
					<-releaseSecond
				}
			}))

			q.log("first", 0)
			firstDone := make(chan error, 1)
			go func() { firstDone <- q.persist() }()
			require.EqualValues(t, 1, awaitAuditQueue(t, entered))

			secondDone := make(chan error, 1)
			go func() { secondDone <- q.persist() }()
			select {
			case <-entered:
				t.Fatal("concurrent persistence entered the database before the first batch completed")
			case <-time.After(50 * time.Millisecond):
			}

			admitted := make(chan struct{})
			go func() {
				q.log("later", 123)
				close(admitted)
			}()
			awaitAuditQueue(t, admitted)
			duringFirst := q.state()
			require.Equal(t, []string{"later"}, duringFirst.ids)

			releaseFirst <- struct{}{}
			require.NoError(t, awaitAuditQueue(t, firstDone))
			require.EqualValues(t, 2, awaitAuditQueue(t, entered))

			q.log("third", 0)
			releaseSecond <- struct{}{}
			require.NoError(t, awaitAuditQueue(t, secondDone))
			state := q.state()
			require.Equal(t, []string{"third"}, state.ids)
			require.NoError(t, q.persist())
			require.NoError(t, q.persist(), "empty flush must succeed")

			var stored []string
			require.NoError(t, c.db.WithContext(t.Context()).Table(q.table).Order("id").Pluck(q.idColumn, &stored).Error)
			require.Equal(t, []string{"first", "later", "third"}, stored)
		})
	}
}

func awaitAuditQueue[T any](t *testing.T, ch <-chan T) T {
	t.Helper()

	select {
	case result := <-ch:
		return result
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for audit queue operation")
		var zero T
		return zero
	}
}

// Explicit flush helpers are only needed by tests; production uses the loops and CloseContext.
func (c *Client) persistMCPAuditLogs() error {
	return c.writeAuditLogs(context.Background(), c.persistMCPAuditLogsContext)
}

func (c *Client) persistEnforcementDecisions() error {
	return c.writeAuditLogs(context.Background(), c.persistEnforcementDecisionsContext)
}
