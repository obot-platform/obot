package client

import (
	"os"
	"strings"
	"testing"
	"time"
	"uuid"

	gatewaydb "github.com/obot-platform/obot/pkg/gateway/db"
	storageservices "github.com/obot-platform/obot/pkg/storage/services"
	"gorm.io/gorm"
)

func TestWithLockExcludesClientsOnPostgres(t *testing.T) {
	postgresDSN := os.Getenv(postgresUserLimitTestDSNEnv)
	if postgresDSN == "" {
		t.Skipf("set %s to a PostgreSQL URL whose user can create schemas", postgresUserLimitTestDSNEnv)
	}

	adminServices, err := storageservices.New(storageservices.Config{DSN: postgresDSN})
	if err != nil {
		t.Fatalf("opening PostgreSQL admin connection: %v", err)
	}
	schema := "obot_lock_" + strings.ReplaceAll(uuid.New().String(), "-", "")
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

	// Two clients on their own connections stand in for two replicas.
	scopedDSN := postgresUserLimitTestDSN(t, postgresDSN, schema)
	clientA := newPostgresUserLimitTestClient(newPostgresUserLimitTestDB(t, scopedDSN))
	clientB := newPostgresUserLimitTestClient(newPostgresUserLimitTestDB(t, scopedDSN))

	held := make(chan struct{})
	release := make(chan struct{})
	doneA := make(chan error, 1)
	go func() {
		doneA <- clientA.WithLock(t.Context(), gatewaydb.HostedResourcesLock, func(*gorm.DB) error {
			close(held)
			<-release
			return nil
		})
	}()
	<-held

	doneB := make(chan error, 1)
	go func() {
		doneB <- clientB.WithLock(t.Context(), gatewaydb.HostedResourcesLock, func(*gorm.DB) error {
			return nil
		})
	}()

	select {
	case err := <-doneB:
		t.Fatalf("second client took the lock while the first held it: %v", err)
	case <-time.After(500 * time.Millisecond):
	}

	close(release)
	if err := <-doneA; err != nil {
		t.Fatalf("first client: %v", err)
	}
	select {
	case err := <-doneB:
		if err != nil {
			t.Fatalf("second client: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("second client didn't take the lock after the first released it")
	}
}
