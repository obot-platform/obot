package client

import (
	"context"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	gatewaydb "github.com/obot-platform/obot/pkg/gateway/db"
	"github.com/obot-platform/obot/pkg/gateway/types"
	sservices "github.com/obot-platform/obot/pkg/storage/services"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestClaudeLLMCredential(t *testing.T) {
	c := newTestClient(t)
	ctx := t.Context()
	user := types.User{Username: "claude", HashedUsername: "claude", Email: "claude@example.com"}
	require.NoError(t, c.db.WithContext(ctx).Create(&user).Error)

	mcp, err := c.EnsureClaudeCredential(ctx, user.ID, "acr-test", []string{"vmcp_test"}, time.Now().Add(24*time.Hour))
	require.NoError(t, err)
	llm, err := c.EnsureClaudeLLMCredential(ctx, user.ID, "acr-test", time.Now().Add(24*time.Hour))
	require.NoError(t, err)
	require.Equal(t, ClaudeLLMCredentialContext, llm.Context)
	require.NotEqual(t, mcp.Secrets["keyID"], llm.Secrets["keyID"], "the session's keys are separate")
	key, err := c.ValidateAPIKey(ctx, llm.Secrets["key"])
	require.NoError(t, err)
	require.Equal(t, user.ID, key.UserID)
	require.Equal(t, types.APIKeyScopes{CanAccessLLMProxy: true}, key.APIKeyScopes)
	require.True(t, key.ClaudeEnvironment)

	// A revoked LLM key stays as the session's revocation marker through retention cleanup.
	require.NoError(t, c.RevokeClaudeCredentials(ctx, "acr-test"))
	_, err = c.ValidateAPIKey(ctx, llm.Secrets["key"])
	require.ErrorIs(t, err, ErrInvalidAPIKey)
	require.NoError(t, c.db.WithContext(ctx).Model(&types.APIKey{}).Where("id = ?", key.ID).Update("expires_at", time.Now().Add(-time.Minute)).Error)
	require.NoError(t, c.cleanupRetainedData(ctx, time.Now(), 0, 0))
	require.NoError(t, c.deleteOldRevokedAPIKeys(ctx, time.Now().AddDate(0, 0, 100), 90))
	_, err = c.GetAPIKeyByID(ctx, key.ID)
	require.NoError(t, err)
	_, err = c.EnsureClaudeLLMCredential(ctx, user.ID, "acr-test", time.Now().Add(24*time.Hour))
	require.ErrorIs(t, err, ErrClaudeCredentialRevoked)

	_, err = c.DeleteCredential(ctx, ClaudeLLMCredentialContext, "acr-test")
	require.NoError(t, err)
	require.NoError(t, c.cleanupRetainedData(ctx, time.Now(), 0, 0))
	_, err = c.GetAPIKeyByID(ctx, key.ID)
	require.Error(t, err)
}

// newProvisionLockTestClient returns a client whose database has a pool of poolSize
// connections. It counts the provisioning locks taken in locks.
func newProvisionLockTestClient(t *testing.T, poolSize int, locks *atomic.Int64) *Client {
	t.Helper()
	services, err := sservices.New(sservices.Config{DSN: "sqlite://" + filepath.Join(t.TempDir(), "lock.db")})
	require.NoError(t, err)
	services.DB.SQLDB.SetMaxOpenConns(poolSize)
	db, err := gatewaydb.New(services.DB.DB, services.DB.SQLDB, true)
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate())
	// SQLite has no advisory locks. The lock's transaction still holds a connection, as on
	// PostgreSQL, which is what the pool tests need.
	require.NoError(t, services.DB.DB.Callback().Raw().Before("gorm:raw").Register("test_advisory_lock", func(tx *gorm.DB) {
		if strings.Contains(tx.Statement.SQL.String(), "pg_advisory_xact_lock") {
			locks.Add(1)
			tx.Statement.SQL.Reset()
			tx.Statement.SQL.WriteString("SELECT 1")
			tx.Statement.Vars = nil
		}
	}))
	t.Cleanup(func() { _ = services.DB.SQLDB.Close() })
	return &Client{db: db}
}

// Callers waiting for the provisioning lock must not hold database connections. The lock
// holder's own queries use another connection from the pool, so waiters that fill the pool
// stall the holder, and every other database user, until their requests time out.
func TestClaudeProvisionLockWaitersDoNotExhaustThePool(t *testing.T) {
	const poolSize = 2
	var locks atomic.Int64
	c := newProvisionLockTestClient(t, poolSize, &locks)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	callers := 4 * poolSize
	errs := make(chan error, callers)
	start := make(chan struct{})
	for range callers {
		go func() {
			<-start
			errs <- c.WithClaudeProvisionLock(ctx, func() error {
				// Give the other callers time to take the pool's connections, as callers
				// waiting for the PostgreSQL advisory lock do.
				time.Sleep(20 * time.Millisecond)
				// Like credential issuance, read through the pool, not the lock's transaction.
				var count int64
				return c.db.WithContext(ctx).Model(&types.Credential{}).Count(&count).Error
			})
		}()
	}
	close(start)
	for range callers {
		require.NoError(t, <-errs)
	}
	require.EqualValues(t, callers, locks.Load())
}

func TestClaudeProvisionLockWaiterGivesUpWithItsRequest(t *testing.T) {
	var locks atomic.Int64
	c := newProvisionLockTestClient(t, 2, &locks)
	held, release := make(chan struct{}), make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- c.WithClaudeProvisionLock(t.Context(), func() error {
			close(held)
			<-release
			return nil
		})
	}()
	<-held

	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	err := c.WithClaudeProvisionLock(ctx, func() error {
		t.Error("a canceled waiter must not run")
		return nil
	})
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.EqualValues(t, 1, locks.Load(), "a waiter does not take a database lock")

	close(release)
	require.NoError(t, <-done)
	require.NoError(t, c.WithClaudeProvisionLock(t.Context(), func() error { return nil }), "the lock is free again")
}

// A rotation that lands between reading a session's credential and reading its key revokes
// the key the lookup found. The lookup must not report that as the session's revocation.
func TestCurrentClaudeCredentialDuringRotation(t *testing.T) {
	var locks atomic.Int64
	c := newProvisionLockTestClient(t, 2, &locks)
	ctx := t.Context()
	user := types.User{Username: "claude", HashedUsername: "claude", Email: "claude@example.com"}
	require.NoError(t, c.db.WithContext(ctx).Create(&user).Error)
	// The key expires too soon to reuse, so the next issue rotates it.
	old, err := c.EnsureClaudeLLMCredential(ctx, user.ID, "acr-test", time.Now().Add(20*time.Second))
	require.NoError(t, err)

	var rotate atomic.Bool
	var rotated types.Credential
	require.NoError(t, c.db.WithContext(ctx).Callback().Query().Before("gorm:query").Register("test_rotate", func(tx *gorm.DB) {
		if tx.Statement.Table == "api_keys" && rotate.CompareAndSwap(true, false) {
			rotated, err = c.EnsureClaudeLLMCredential(ctx, user.ID, "acr-test", time.Now().Add(5*time.Minute))
			require.NoError(t, err)
		}
	}))
	rotate.Store(true)
	credential, ok, err := c.CurrentClaudeLLMCredential(ctx, user.ID, "acr-test", time.Now().Add(5*time.Minute))
	require.NoError(t, err, "a rotation is not a revocation")
	require.False(t, ok, "the caller issues under the lock, which returns the rotated key")
	require.Empty(t, credential.Secrets)
	require.NotEqual(t, old.Secrets["keyID"], rotated.Secrets["keyID"], "the hook rotated the key")

	// A revoked key the session still refers to is reported as revoked.
	require.NoError(t, c.RevokeClaudeCredentials(ctx, "acr-test"))
	_, _, err = c.CurrentClaudeLLMCredential(ctx, user.ID, "acr-test", time.Now().Add(5*time.Minute))
	require.ErrorIs(t, err, ErrClaudeCredentialRevoked)
}
