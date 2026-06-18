package db

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// legacyDeviceScanSkill mirrors the pre-attribution device_scan_skills
// schema: no skill_id column and no attributions table.
type legacyDeviceScanSkill struct {
	ID           uint `gorm:"primaryKey"`
	DeviceScanID uint
	Client       string
	Scope        string
	ProjectPath  string
	File         string
	Name         string
	GitRemoteURL string
}

func (legacyDeviceScanSkill) TableName() string { return "device_scan_skills" }

func TestMigrateDeviceScanSkillAttributions(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&legacyDeviceScanSkill{}); err != nil {
		t.Fatalf("auto migrate legacy schema: %v", err)
	}
	legacy := []legacyDeviceScanSkill{
		{DeviceScanID: 1, Client: "claude_code", File: "/home/a/.claude/skills/x/SKILL.md", Name: "x"},
		{DeviceScanID: 1, Client: "", File: "/home/a/unowned/SKILL.md", Name: "y"},
	}
	if err := db.Create(&legacy).Error; err != nil {
		t.Fatalf("insert legacy skills: %v", err)
	}

	if err := migrateDeviceScanSkillAttributions(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// Re-running must be idempotent (no duplicate attributions, no
	// unique index violations).
	if err := migrateDeviceScanSkillAttributions(db); err != nil {
		t.Fatalf("re-run migrate: %v", err)
	}

	var skills []types.DeviceScanSkill
	if err := db.Preload("Attributions").Order("id ASC").Find(&skills).Error; err != nil {
		t.Fatalf("load skills: %v", err)
	}
	if len(skills) != 2 {
		t.Fatalf("want 2 skills, got %d", len(skills))
	}
	for _, skill := range skills {
		want := types.ComputeDeviceScanSkillID(skill.File, skill.ProjectPath, skill.Name, skill.GitRemoteURL)
		if skill.SkillID != want {
			t.Errorf("skill %d skill_id: want %q, got %q", skill.ID, want, skill.SkillID)
		}
	}
	if len(skills[0].Attributions) != 1 || skills[0].Attributions[0].Client != "claude_code" {
		t.Errorf("skill 1 attributions: want one claude_code row, got %+v", skills[0].Attributions)
	}
	if len(skills[1].Attributions) != 0 {
		t.Errorf("skill 2 attributions: want none for empty client, got %+v", skills[1].Attributions)
	}
}
