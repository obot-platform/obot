package handlers

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/substrate"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// SQLite cannot take PostgreSQL advisory locks. Count lock attempts to prove a
// stalled body never reaches the shared lock, while another request can take it.
func TestClaudeRunnerSlowBodyDoesNotHoldProvisionLock(t *testing.T) {
	for _, endpoint := range []string{"status", "finished"} {
		t.Run(endpoint, func(t *testing.T) {
			gateway, db := newHandlerTestGatewayWithDB(t)
			var locks atomic.Int32
			require.NoError(t, db.Callback().Raw().Before("gorm:raw").Register("test_advisory_lock", func(tx *gorm.DB) {
				if strings.Contains(tx.Statement.SQL.String(), "pg_advisory_xact_lock") {
					locks.Add(1)
					tx.Statement.SQL.Reset()
					tx.Statement.SQL.WriteString("SELECT 1")
					tx.Statement.Vars = nil
				}
			}))
			h := &ClaudeRunnerHandler{Substrate: &substrate.Client{}}
			read, write := io.Pipe()
			defer read.Close()
			defer write.Close()
			request := httptest.NewRequest(http.MethodPost, "/", read)
			request.Header.Set("X-Obot-Runner-Token", "runner-secret")
			done := make(chan error, 1)
			go func() {
				req := api.Context{Request: request, ResponseWriter: httptest.NewRecorder(), GatewayClient: gateway}
				if endpoint == "status" {
					done <- h.Status(req)
				} else {
					done <- h.Finished(req)
				}
			}()
			// Pipe.Write returns once the handler starts consuming the body.
			_, err := write.Write([]byte(`{"unfinished":`))
			require.NoError(t, err)
			require.Zero(t, locks.Load())
			require.NoError(t, gateway.WithClaudeProvisionLock(t.Context(), func() error { return nil }))
			require.EqualValues(t, 1, locks.Load())
			require.NoError(t, write.Close())
			select {
			case err := <-done:
				require.Error(t, err)
			case <-time.After(5 * time.Second):
				t.Fatal("handler did not finish after the upload closed")
			}
			require.EqualValues(t, 1, locks.Load(), "malformed body must not acquire the lock")
		})
	}
}
