package server

import (
	"context"
	"errors"
	"net"
	"net/http"
	"path/filepath"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaydb "github.com/obot-platform/obot/pkg/gateway/db"
	"github.com/obot-platform/obot/pkg/gateway/types"
	sservices "github.com/obot-platform/obot/pkg/storage/services"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type failingListener struct {
	err error
}

func (l failingListener) Accept() (net.Conn, error) {
	return nil, l.err
}

func (failingListener) Close() error {
	return nil
}

func (failingListener) Addr() net.Addr {
	return &net.TCPAddr{}
}

func newServerAuditClient(t *testing.T) (*gateway.Client, *gorm.DB, *gorm.DB, context.CancelFunc) {
	t.Helper()

	dsn := "sqlite://" + filepath.Join(t.TempDir(), "audit.db")
	storage, err := sservices.New(sservices.Config{
		DSN: dsn,
	})
	require.NoError(t, err)
	db, err := gatewaydb.New(storage.DB.DB, storage.DB.SQLDB, true)
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate())

	// Keep an independent connection for checking committed rows after the writer closes its DB.
	observer, err := sservices.New(sservices.Config{
		DSN: dsn,
	})
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	c := gateway.New(ctx, db, nil, nil, nil, time.Hour, 1000, 0, 0, 0, true)
	t.Cleanup(func() {
		cancel()
		_ = c.Close()
		_ = observer.DB.SQLDB.Close()
	})

	c.LogLLMAuditEntry(types.LLMAuditLog{ID: "partial-llm"})
	// Observe consumption into the real loop's partial batch without adding a production test hook.
	require.Eventually(t, func() bool {
		return reflect.ValueOf(c).Elem().FieldByName("llmAuditEntries").Len() == 0
	}, time.Second, time.Millisecond)
	var count int64
	require.NoError(t, observer.DB.DB.Model(&types.LLMAuditLog{}).Count(&count).Error)
	require.Zero(t, count, "the partial batch must still need the final drain")

	return c, storage.DB.DB, observer.DB.DB, cancel
}

func finalCredentialCapture(ctx context.Context, c *gateway.Client) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		<-ctx.Done()
		// Credential shutdown joins handlers whose finalizers enqueue interrupted captures.
		c.LogEgressAuditEntry(types.EgressAuditLog{
			ID:      "final-credential-capture",
			UserID:  "7",
			Outcome: "interrupted",
		})
	}()

	return done
}

func requireServerAuditRows(t *testing.T, db *gorm.DB) {
	t.Helper()

	var egress []types.EgressAuditLog
	require.NoError(t, db.Find(&egress).Error)
	require.Len(t, egress, 1)
	require.Equal(t, "final-credential-capture", egress[0].ID)
	require.Equal(t, "interrupted", egress[0].Outcome)

	var llm []types.LLMAuditLog
	require.NoError(t, db.Find(&llm).Error)
	require.Len(t, llm, 1)
	require.Equal(t, "partial-llm", llm[0].ID)
}

func TestServeFailureDrainsAuditLogs(t *testing.T) {
	c, _, observer, servicesCancel := newServerAuditClient(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	credentialDone := finalCredentialCapture(ctx, c)
	serveErr := errors.New("permanent listener accept failure")
	s := &http.Server{}
	var shutdownCalls int
	var shutdownCtx context.Context
	var closeErr error
	started := time.Now()

	err := serveAndShutdown(ctx, cancel, func() error {
		return s.Serve(failingListener{err: serveErr})
	}, func() {
		shutdownCalls++
		var stop context.CancelFunc
		shutdownCtx, stop = context.WithTimeout(context.Background(), 20*time.Second)
		defer stop()
		defer servicesCancel()

		closeErr = s.Shutdown(shutdownCtx)
		<-credentialDone
		closeErr = errors.Join(closeErr, c.Close())
	})

	require.ErrorIs(t, err, serveErr)
	require.NoError(t, closeErr)
	require.Equal(t, 1, shutdownCalls)
	require.ErrorIs(t, ctx.Err(), context.Canceled)
	deadline, ok := shutdownCtx.Deadline()
	require.True(t, ok)
	require.WithinDuration(t, started.Add(20*time.Second), deadline, time.Second)
	require.ErrorIs(t, shutdownCtx.Err(), context.Canceled)
	requireServerAuditRows(t, observer)
	require.Error(t, c.FlushEgressAuditLogs(), "the synchronous drain has already closed the writer")
}

func TestServeJoinsNormalShutdown(t *testing.T) {
	for _, test := range []struct {
		name     string
		serveErr error
	}{
		{
			name:     "normal return",
			serveErr: http.ErrServerClosed,
		},
		{
			name:     "error during shutdown",
			serveErr: errors.New("listener failed during shutdown"),
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			c, writer, observer, servicesCancel := newServerAuditClient(t)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			credentialDone := finalCredentialCapture(ctx, c)
			serving := make(chan struct{})
			returnServe := make(chan struct{})
			shutdownStarted := make(chan context.Context, 1)
			allowDrain := make(chan struct{})
			defer close(allowDrain)
			returned := make(chan error, 1)
			var shutdownCalls atomic.Int32
			var writes atomic.Int32
			var closed atomic.Bool
			var postCloseWrites atomic.Bool
			var closeErr error
			var writeDeadlines []time.Time
			require.NoError(t, writer.Callback().Create().Before("gorm:create").Register("test_deadline", func(tx *gorm.DB) {
				writes.Add(1)
				if closed.Load() {
					postCloseWrites.Store(true)
				}
				deadline, _ := tx.Statement.Context.Deadline()
				writeDeadlines = append(writeDeadlines, deadline)
			}))

			go func() {
				returned <- serveAndShutdown(ctx, cancel, func() error {
					close(serving)
					<-returnServe
					return test.serveErr
				}, func() {
					shutdownCalls.Add(1)
					// Simulate HTTP shutdown consuming its entire budget before the audit drain.
					shutdownCtx, stop := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
					defer stop()
					defer servicesCancel()
					shutdownStarted <- shutdownCtx
					<-credentialDone
					<-allowDrain
					closeErr = c.Close()
					closed.Store(true)
				})
			}()
			<-serving
			cancel()
			originalCtx := <-shutdownStarted
			originalDeadline, ok := originalCtx.Deadline()
			require.True(t, ok)
			close(returnServe)

			select {
			case err := <-returned:
				t.Fatalf("Serve returned before the ongoing shutdown drained audit work: %v", err)
			case <-time.After(30 * time.Millisecond):
			}
			require.EqualValues(t, 1, shutdownCalls.Load())
			allowDrain <- struct{}{}
			select {
			case err := <-returned:
				if errors.Is(test.serveErr, http.ErrServerClosed) {
					require.NoError(t, err)
				} else {
					require.ErrorIs(t, err, test.serveErr)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("shutdown did not finish")
			}

			require.NoError(t, closeErr)
			require.EqualValues(t, 1, shutdownCalls.Load())
			require.ErrorIs(t, originalCtx.Err(), context.DeadlineExceeded)
			require.Len(t, writeDeadlines, 2)
			require.Equal(t, writeDeadlines[0], writeDeadlines[1], "all final audit writes share one fresh budget")
			require.True(t, writeDeadlines[0].After(originalDeadline))
			require.WithinDuration(t, time.Now().Add(20*time.Second), writeDeadlines[0], time.Second)
			requireServerAuditRows(t, observer)
			require.Error(t, c.FlushEgressAuditLogs())
			require.EqualValues(t, 2, writes.Load())
			require.False(t, postCloseWrites.Load())
		})
	}
}
