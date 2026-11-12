package db

import (
	"fmt"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"gorm.io/gorm"
)

var (
	// unusedMCPAuditLogIndexes are single-column indexes on mcp_audit_logs that no query can use. Their
	// columns are either never part of a predicate or ordering, or are only matched by the free-text
	// search, whose substring patterns a B-tree index cannot serve.
	unusedMCPAuditLogIndexes = []string{
		"idx_mcp_audit_logs_started_at",
		"idx_mcp_audit_logs_actor_type",
		"idx_mcp_audit_logs_actor_id",
		"idx_mcp_audit_logs_target_type",
		"idx_mcp_audit_logs_target_name",
		"idx_mcp_audit_logs_target_parent_type",
		"idx_mcp_audit_logs_outcome_reason",
		"idx_mcp_audit_logs_tool_use_id",
		"idx_mcp_audit_logs_turn_id",
		"idx_mcp_audit_logs_agent_version",
		"idx_mcp_audit_logs_cli_version",
		"idx_mcp_audit_logs_model",
		"idx_mcp_audit_logs_model_id",
		"idx_mcp_audit_logs_permission_mode",
		"idx_mcp_audit_logs_device_deployment_id",
		"idx_mcp_audit_logs_os",
		"idx_mcp_audit_logs_architecture",
		"idx_mcp_audit_logs_git_commit",
	}
)

// dropUnusedMCPAuditLogIndexes drops the indexes that were removed from the MCPAuditLog model.
// AutoMigrate creates indexes but never drops them, so databases created by earlier versions keep
// paying to maintain them on every audit log write until they are dropped here.
//
// It checks for each index and only drops the ones that exist, so it is safe to run on every
// startup. That matters because an earlier version's AutoMigrate creates them again.
func dropUnusedMCPAuditLogIndexes(tx *gorm.DB) error {
	migrator := tx.Migrator()
	if !migrator.HasTable(&types.MCPAuditLog{}) {
		return nil
	}
	for _, index := range unusedMCPAuditLogIndexes {
		if !migrator.HasIndex(&types.MCPAuditLog{}, index) {
			continue
		}
		if err := migrator.DropIndex(&types.MCPAuditLog{}, index); err != nil {
			return fmt.Errorf("failed to drop index %s from mcp_audit_logs: %w", index, err)
		}
	}
	return nil
}
