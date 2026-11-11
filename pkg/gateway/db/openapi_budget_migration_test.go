package db

import (
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	storageservices "github.com/obot-platform/obot/pkg/storage/services"
	"gorm.io/gorm"
)

func TestOpenAPIBudgetMigrationConcurrentPostgres(t *testing.T) {
	dsn := os.Getenv("OBOT_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("OBOT_TEST_POSTGRES_DSN is not set")
	}
	admin, err := storageservices.New(storageservices.Config{DSN: dsn})
	if err != nil {
		t.Fatal(err)
	}
	schema := "openapi_budget_migration_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if err := admin.DB.DB.Exec("CREATE SCHEMA " + schema).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = admin.DB.DB.Exec("DROP SCHEMA " + schema + " CASCADE").Error
		_ = admin.DB.SQLDB.Close()
	})
	parsed, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()

	const replicas = 3
	databases := make([]*gorm.DB, replicas)
	var wg sync.WaitGroup
	errs := make([]error, replicas)
	for index := range replicas {
		services, err := storageservices.New(storageservices.Config{DSN: parsed.String()})
		if err != nil {
			t.Fatal(err)
		}
		databases[index] = services.DB.DB
		t.Cleanup(func() { _ = services.DB.SQLDB.Close() })
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			errs[index] = AutoMigrateOpenAPIBudgets(databases[index])
		}(index)
	}
	wg.Wait()
	for index, err := range errs {
		if err != nil {
			t.Fatalf("replica %d migration failed: %v", index, err)
		}
	}

	var indexes []string
	if err := databases[0].Raw(`
		SELECT indexname
		FROM pg_indexes
		WHERE schemaname = current_schema() AND tablename IN ('openapi_budget_states','openapi_execution_requests')
		ORDER BY indexname`).Scan(&indexes).Error; err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(indexes, "\n")
	for _, required := range []string{"idx_openapi_budget_states_scope_key", "idx_openapi_requests_budget_state_id", "idx_openapi_requests_state_deadline"} {
		if !strings.Contains(joined, required) {
			t.Fatalf("missing required index %s in %v", required, indexes)
		}
	}
}
