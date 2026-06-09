package db

import (
	"fmt"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"gorm.io/gorm"
)

// migrateDeviceScanSkillAttributions backfills the skill_id column and
// the device_scan_skill_attributions table from pre-attribution skill
// rows. It runs once (recorded in the migrations table) inside the
// AutoMigrate transaction.
func migrateDeviceScanSkillAttributions(tx *gorm.DB) error {
	if err := tx.AutoMigrate(&types.DeviceScanSkill{}, &types.DeviceScanSkillAttribution{}); err != nil {
		return fmt.Errorf("auto migrate skill attribution tables: %w", err)
	}

	// Backfill skill_id in batches. The identity is computed in Go, so
	// each row needs its own UPDATE, but only rows missing one are
	// loaded and only the columns it derives from.
	var batch []types.DeviceScanSkill
	if err := tx.Model(&types.DeviceScanSkill{}).
		Select("id", "file", "project_path", "name", "git_remote_url").
		Where("skill_id IS NULL OR skill_id = ''").
		FindInBatches(&batch, 500, func(btx *gorm.DB, _ int) error {
			for _, skill := range batch {
				skillID := types.ComputeDeviceScanSkillID(skill.File, skill.ProjectPath, skill.Name, skill.GitRemoteURL)
				if err := btx.Model(&types.DeviceScanSkill{}).
					Where("id = ?", skill.ID).
					Update("skill_id", skillID).Error; err != nil {
					return fmt.Errorf("backfill skill_id for skill %d: %w", skill.ID, err)
				}
			}
			return nil
		}).Error; err != nil {
		return fmt.Errorf("backfill skill ids: %w", err)
	}

	// Backfill one attribution per row from the legacy client column in
	// a single set-based statement. NOT EXISTS keeps it idempotent.
	if err := tx.Exec(`INSERT INTO device_scan_skill_attributions (device_scan_skill_id, client, created_at)
		SELECT sk.id, sk.client, CURRENT_TIMESTAMP
		FROM device_scan_skills sk
		WHERE sk.client IS NOT NULL AND sk.client <> ''
		AND NOT EXISTS (
			SELECT 1 FROM device_scan_skill_attributions attr
			WHERE attr.device_scan_skill_id = sk.id AND attr.client = sk.client
		)`).Error; err != nil {
		return fmt.Errorf("backfill skill attributions: %w", err)
	}
	return nil
}
