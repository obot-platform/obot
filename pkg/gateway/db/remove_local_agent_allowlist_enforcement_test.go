package db

import (
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestRemoveLocalAgentAllowlistEnforcement(t *testing.T) {
	type legacyDecision struct {
		ID uint `gorm:"primaryKey"`
	}

	database, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := database.Exec(`CREATE TABLE mdm_configurations (id integer primary key, enforcement_enabled numeric, enforcement_allowlist text)`).Error; err != nil {
		t.Fatalf("create legacy MDM table: %v", err)
	}
	if err := database.Table("enforcement_decision_logs").AutoMigrate(&legacyDecision{}); err != nil {
		t.Fatalf("create decision table: %v", err)
	}
	if err := database.Table("enforcement_decision_logs").Create(&legacyDecision{ID: 1}).Error; err != nil {
		t.Fatalf("insert decision: %v", err)
	}

	if err := removeLocalAgentAllowlistEnforcement(database); err != nil {
		t.Fatalf("remove enforcement: %v", err)
	}
	if database.Migrator().HasTable("enforcement_decision_logs") {
		t.Fatal("enforcement decision table still exists")
	}
	if database.Migrator().HasColumn("mdm_configurations", "enforcement_enabled") ||
		database.Migrator().HasColumn("mdm_configurations", "enforcement_allowlist") {
		t.Fatal("legacy MDM enforcement columns still exist")
	}
	if err := removeLocalAgentAllowlistEnforcement(database); err != nil {
		t.Fatalf("second removal should be safe: %v", err)
	}
}
