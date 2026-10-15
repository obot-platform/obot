package storage

import (
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	sservices "github.com/obot-platform/obot/pkg/storage/services"
)

const (
	postgresTestDSNEnv = "OBOT_TEST_POSTGRES_DSN"
)

// Replicas that start together on a database without the storage tables all create them.
// Postgres doesn't make CREATE TABLE IF NOT EXISTS safe to run concurrently, so without the
// schema lock all but one fail with a duplicate key on pg_type_typname_nsp_index.
func TestBuildAPIGroupsConcurrentlyOnAFreshPostgresSchema(t *testing.T) {
	dsn := os.Getenv(postgresTestDSNEnv)
	if dsn == "" {
		t.Skipf("set %s to a PostgreSQL URL whose user can create schemas", postgresTestDSNEnv)
	}

	admin, err := sservices.New(sservices.Config{DSN: dsn})
	if err != nil {
		t.Fatalf("opening PostgreSQL admin connection: %v", err)
	}
	schema := "obot_storage_" + strings.ReplaceAll(uuid.New().String(), "-", "")
	if err := admin.DB.DB.Exec("CREATE SCHEMA " + schema).Error; err != nil {
		t.Fatalf("creating isolated PostgreSQL schema: %v", err)
	}
	t.Cleanup(func() {
		if err := admin.DB.DB.Exec("DROP SCHEMA " + schema + " CASCADE").Error; err != nil {
			t.Errorf("dropping isolated PostgreSQL schema: %v", err)
		}
		_ = admin.DB.SQLDB.Close()
	})

	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	query := u.Query()
	query.Set("search_path", schema)
	u.RawQuery = query.Encode()

	const replicas = 4
	var (
		wg   sync.WaitGroup
		errs = make([]error, replicas)
	)
	for i := range replicas {
		services, err := sservices.New(sservices.Config{DSN: u.String()})
		if err != nil {
			t.Fatalf("opening replica %d's connection: %v", i, err)
		}
		t.Cleanup(func() { _ = services.DB.SQLDB.Close() })
		wg.Go(func() {
			_, errs[i] = buildAPIGroups(t.Context(), services)
		})
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Errorf("replica %d failed to set up the storage tables: %v", i, err)
		}
	}
}
