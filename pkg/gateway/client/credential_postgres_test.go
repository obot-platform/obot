package client

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	gatewaydb "github.com/obot-platform/obot/pkg/gateway/db"
	storageservices "github.com/obot-platform/obot/pkg/storage/services"
	"gorm.io/gorm"
)

func TestAcquireCredentialLockPostgres(t *testing.T) {
	scopedDSN, adminDB := newCredentialLockPostgresTest(t)

	t.Run("distinct held locks do not starve the shared pool", func(t *testing.T) {
		database := newPostgresUserLimitTestDB(t, scopedDSN)
		sqlDB, err := database.WithContext(t.Context()).DB()
		if err != nil {
			t.Fatalf("accessing PostgreSQL pool: %v", err)
		}
		sqlDB.SetMaxOpenConns(2)
		sqlDB.SetMaxIdleConns(2)

		clientA := newCredentialLockTestClient(t, database)
		clientB := newCredentialLockTestClient(t, database)
		releaseA, err := clientA.AcquireCredentialLock(t.Context(), "distinct-key-a")
		if err != nil {
			t.Fatalf("acquiring first distinct credential lock: %v", err)
		}
		releaseB, err := clientB.AcquireCredentialLock(t.Context(), "distinct-key-b")
		if err != nil {
			releaseA()
			t.Fatalf("acquiring second distinct credential lock: %v", err)
		}

		queryCtx, cancelQuery := context.WithTimeout(t.Context(), time.Second)
		queryErr := database.WithContext(queryCtx).Exec("SELECT 1").Error
		cancelQuery()
		releaseB()
		releaseA()

		if queryErr != nil {
			t.Fatalf("distinct credential locks starved the shared PostgreSQL pool: %v", queryErr)
		}
	})

	t.Run("does not starve the shared pool", func(t *testing.T) {
		database := newPostgresUserLimitTestDB(t, scopedDSN)
		sqlDB, err := database.WithContext(t.Context()).DB()
		if err != nil {
			t.Fatalf("accessing PostgreSQL pool: %v", err)
		}
		sqlDB.SetMaxOpenConns(2)
		sqlDB.SetMaxIdleConns(2)

		holder := newCredentialLockTestClient(t, database)
		waiter := newCredentialLockTestClient(t, database)
		releaseHolder, err := holder.AcquireCredentialLock(t.Context(), "shared-pool-key")
		if err != nil {
			t.Fatalf("acquiring initial credential lock: %v", err)
		}

		waiterCtx, cancelWaiter := context.WithTimeout(t.Context(), 10*time.Second)
		waiterResult := make(chan error, 1)
		go func() {
			release, err := waiter.AcquireCredentialLock(waiterCtx, "shared-pool-key")
			if err == nil {
				release()
			}
			waiterResult <- err
		}()

		time.Sleep(100 * time.Millisecond)
		queryCtx, cancelQuery := context.WithTimeout(t.Context(), time.Second)
		queryErr := database.WithContext(queryCtx).Exec("SELECT 1").Error
		cancelQuery()

		releaseHolder()
		select {
		case err := <-waiterResult:
			if err != nil {
				t.Fatalf("waiting for credential lock: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for credential lock waiter")
		}
		cancelWaiter()

		if queryErr != nil {
			t.Fatalf("shared PostgreSQL pool was starved while waiting for credential lock: %v", queryErr)
		}
	})

	t.Run("coordinates independent pools and honors cancellation", func(t *testing.T) {
		databaseA := newPostgresUserLimitTestDB(t, scopedDSN)
		databaseB := newPostgresUserLimitTestDB(t, scopedDSN)
		clientA := newCredentialLockTestClient(t, databaseA)
		clientB := newCredentialLockTestClient(t, databaseB)

		releaseA, err := clientA.AcquireCredentialLock(t.Context(), "independent-pool-key")
		if err != nil {
			t.Fatalf("acquiring credential lock from first pool: %v", err)
		}

		waitCtx, cancelWait := context.WithTimeout(t.Context(), 150*time.Millisecond)
		_, err = clientB.AcquireCredentialLock(waitCtx, "independent-pool-key")
		cancelWait()
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("waiting for held credential lock returned %v, want context deadline exceeded", err)
		}

		releaseA()
		acquireCtx, cancelAcquire := context.WithTimeout(t.Context(), 3*time.Second)
		releaseB, err := clientB.AcquireCredentialLock(acquireCtx, "independent-pool-key")
		cancelAcquire()
		if err != nil {
			t.Fatalf("acquiring credential lock after release: %v", err)
		}
		releaseB()
	})

	t.Run("connection loss releases the lock", func(t *testing.T) {
		databaseA := newPostgresUserLimitTestDB(t, scopedDSN)
		databaseB := newPostgresUserLimitTestDB(t, scopedDSN)
		clientA := newCredentialLockTestClient(t, databaseA)
		clientB := newCredentialLockTestClient(t, databaseB)
		const key = "connection-loss-key"

		releaseA, err := clientA.AcquireCredentialLock(t.Context(), key)
		if err != nil {
			t.Fatalf("acquiring credential lock from first connection: %v", err)
		}

		pid, err := credentialLockBackendPID(t.Context(), adminDB, credentialLockID(key))
		if err != nil {
			releaseA()
			t.Fatalf("finding credential lock backend: %v", err)
		}
		var terminated bool
		if err := adminDB.WithContext(t.Context()).Raw("SELECT pg_terminate_backend(?)", pid).Scan(&terminated).Error; err != nil {
			releaseA()
			t.Fatalf("terminating credential lock backend: %v", err)
		}
		if !terminated {
			releaseA()
			t.Fatal("PostgreSQL did not terminate the credential lock backend")
		}

		acquireCtx, cancelAcquire := context.WithTimeout(t.Context(), 3*time.Second)
		releaseB, err := clientB.AcquireCredentialLock(acquireCtx, key)
		cancelAcquire()
		if err != nil {
			releaseA()
			t.Fatalf("acquiring credential lock after connection loss: %v", err)
		}
		releaseB()
		releaseA()
	})
}

func newCredentialLockTestClient(t *testing.T, database *gatewaydb.DB) *Client {
	t.Helper()

	client := &Client{db: database}
	t.Cleanup(func() {
		if client.credentialLockPool != nil {
			if err := client.credentialLockPool.Close(); err != nil {
				t.Errorf("closing credential lock pool: %v", err)
			}
		}
	})
	return client
}

func newCredentialLockPostgresTest(t *testing.T) (string, *gorm.DB) {
	t.Helper()

	postgresDSN := os.Getenv(postgresUserLimitTestDSNEnv)
	if postgresDSN == "" {
		t.Skipf("set %s to a PostgreSQL URL whose user can create schemas", postgresUserLimitTestDSNEnv)
	}
	adminServices, err := storageservices.New(storageservices.Config{DSN: postgresDSN})
	if err != nil {
		t.Fatalf("opening PostgreSQL admin connection: %v", err)
	}

	schema := "obot_credential_lock_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if err := adminServices.DB.DB.Exec("CREATE SCHEMA " + schema).Error; err != nil {
		_ = adminServices.DB.SQLDB.Close()
		t.Fatalf("creating isolated PostgreSQL schema: %v", err)
	}
	t.Cleanup(func() {
		if err := adminServices.DB.DB.Exec("DROP SCHEMA " + schema + " CASCADE").Error; err != nil {
			t.Errorf("dropping isolated PostgreSQL schema: %v", err)
		}
		if err := adminServices.DB.SQLDB.Close(); err != nil {
			t.Errorf("closing PostgreSQL admin connection: %v", err)
		}
	})

	return postgresUserLimitTestDSN(t, postgresDSN, schema), adminServices.DB.DB
}

func credentialLockBackendPID(ctx context.Context, db *gorm.DB, lockID int64) (int, error) {
	lockClassID := uint64(lockID) >> 32
	lockObjectID := uint64(lockID) & 0xffffffff
	var pid int
	err := db.WithContext(ctx).Raw(`
		SELECT pid
		FROM pg_locks
		WHERE locktype = 'advisory'
		  AND database = (SELECT oid FROM pg_database WHERE datname = current_database())
		  AND classid = CAST(? AS oid)
		  AND objid = CAST(? AS oid)
		  AND objsubid = 1
		  AND granted
	`, lockClassID, lockObjectID).Scan(&pid).Error
	return pid, err
}
