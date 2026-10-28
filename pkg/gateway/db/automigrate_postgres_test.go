package db

import (
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	storageservices "github.com/obot-platform/obot/pkg/storage/services"
)

// TestAutoMigrateConcurrentPostgres runs AutoMigrate from two connections at once on an empty
// schema, as replicas starting together do. Set OBOT_TEST_POSTGRES_DSN to a Postgres URL whose
// user can create schemas.
func TestAutoMigrateConcurrentPostgres(t *testing.T) {
	dsn := os.Getenv("OBOT_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("OBOT_TEST_POSTGRES_DSN is not set")
	}
	admin, err := storageservices.New(storageservices.Config{DSN: dsn})
	if err != nil {
		t.Fatal(err)
	}
	schema := "obot_automigrate_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if err := admin.DB.DB.Exec("CREATE SCHEMA " + schema).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = admin.DB.DB.Exec("DROP SCHEMA " + schema + " CASCADE").Error
		_ = admin.DB.SQLDB.Close()
	})
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()

	const replicas = 3
	dbs := make([]*DB, replicas)
	for i := range dbs {
		services, err := storageservices.New(storageservices.Config{DSN: u.String()})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = services.DB.SQLDB.Close() })
		if dbs[i], err = New(services.DB.DB, services.DB.SQLDB, true); err != nil {
			t.Fatal(err)
		}
	}
	var wg sync.WaitGroup
	errs := make([]error, replicas)
	for i, db := range dbs {
		wg.Go(func() { errs[i] = db.AutoMigrate() })
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Errorf("replica %d: %v", i, err)
		}
	}
}
