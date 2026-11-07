package db

import (
	"testing"

	"github.com/obot-platform/obot/pkg/gateway/types"
	storageservices "github.com/obot-platform/obot/pkg/storage/services"
)

func TestAutoMigrateAuditLogExportTimeIndexesPreserveLegacyTimestamps(t *testing.T) {
	services, err := storageservices.New(storageservices.Config{DSN: "sqlite://:memory:"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = services.DB.SQLDB.Close() })
	tx := services.DB.DB
	for _, table := range []string{"mcp_audit_logs", "llm_audit_logs"} {
		idType := "TEXT"
		if table == "mcp_audit_logs" {
			idType = "INTEGER"
		}
		if err := tx.Exec("CREATE TABLE " + table + " (id " + idType + " PRIMARY KEY, created_at datetime, updated_at datetime)").Error; err != nil {
			t.Fatal(err)
		}
		if err := tx.Exec("INSERT INTO " + table + " VALUES ('1', '2025-01-01 00:00:00+00:00', NULL), ('2', '2025-01-01 00:00:00+00:00', '0001-01-01 00:00:00+00:00')").Error; err != nil {
			t.Fatal(err)
		}
	}
	for range 2 {
		if err := tx.AutoMigrate(&types.MCPAuditLog{}, &types.LLMAuditLog{}); err != nil {
			t.Fatal(err)
		}
	}
	for _, table := range []string{"mcp_audit_logs", "llm_audit_logs"} {
		for _, column := range []string{"updated_at", "created_at"} {
			if !tx.Migrator().HasIndex(table, "idx_"+table+"_"+column) {
				t.Fatalf("%s has no %s index", table, column)
			}
		}
		var preserved int64
		if err := tx.Raw("SELECT COUNT(*) FROM " + table + " WHERE created_at = '2025-01-01 00:00:00+00:00' AND ((id = '1' AND updated_at IS NULL) OR (id = '2' AND updated_at = '0001-01-01 00:00:00+00:00'))").Scan(&preserved).Error; err != nil {
			t.Fatal(err)
		}
		if preserved != 2 {
			t.Fatalf("%s migration rewrote legacy timestamps", table)
		}
	}
}
