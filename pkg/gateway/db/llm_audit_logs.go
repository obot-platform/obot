package db

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"gorm.io/gorm"
)

const (
	llmAuditLogTableName           = "llm_audit_logs"
	llmAuditLogMaintenanceLockID   = 55602834805647 // "llm_audit_logs" on a phone keypad, with underscores as 0.
	llmAuditLogFuturePartitions    = 2
	llmAuditLogPartitionDateFormat = "_2006_01"
)

const llmAuditLogPostgresSchema = `
CREATE TABLE IF NOT EXISTS llm_audit_logs (
    id uuid NOT NULL,
    created_at timestamptz NOT NULL,
    duration bigint NOT NULL DEFAULT 0,
    user_id text NOT NULL DEFAULT '',
    model_provider text NOT NULL DEFAULT '',
    model_id text NOT NULL DEFAULT '',
    target_model text NOT NULL DEFAULT '',
    request_headers text NOT NULL DEFAULT '',
    request_body text,
    response_headers text NOT NULL DEFAULT '',
    response_body text,
    response_text text NOT NULL DEFAULT '',
    response_status integer NOT NULL DEFAULT 0,
    outcome text NOT NULL DEFAULT '',
    error text NOT NULL DEFAULT '',
    input_tokens integer NOT NULL DEFAULT 0,
    output_tokens integer NOT NULL DEFAULT 0,
    request_id text NOT NULL DEFAULT '',
    user_agent text NOT NULL DEFAULT '',
    client_ip text NOT NULL DEFAULT '',
    PRIMARY KEY (id, created_at)
) PARTITION BY RANGE (created_at);

ALTER TABLE llm_audit_logs ADD COLUMN IF NOT EXISTS request_headers text NOT NULL DEFAULT '';
ALTER TABLE llm_audit_logs ADD COLUMN IF NOT EXISTS response_headers text NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS llm_audit_logs_created_at_idx ON llm_audit_logs (created_at DESC);
CREATE INDEX IF NOT EXISTS llm_audit_logs_user_created_idx ON llm_audit_logs (user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS llm_audit_logs_provider_created_idx ON llm_audit_logs (model_provider, created_at DESC);
`

// migrateLLMAuditLogs creates the LLM audit log table for the active database dialect.
func migrateLLMAuditLogs(tx *gorm.DB) error {
	if tx.Name() != "postgres" {
		return tx.AutoMigrate(&types.LLMAuditLog{})
	}

	return tx.Exec(llmAuditLogPostgresSchema).Error
}

// MaintainLLMAuditLogPartitions prepares PostgreSQL partitions and applies retention cleanup.
func MaintainLLMAuditLogPartitions(ctx context.Context, tx *gorm.DB, retentionDays int, now time.Time) error {
	if tx.Name() != "postgres" {
		return deleteOldLLMAuditLogs(ctx, tx, retentionDays, now)
	}

	locked, err := tryLLMAuditLogAdvisoryLock(ctx, tx)
	if err != nil || !locked {
		return err
	}

	if err := ensureLLMAuditLogFuturePartitions(ctx, tx, now); err != nil {
		return err
	}
	if retentionDays > 0 {
		return dropOldLLMAuditLogPartitions(ctx, tx, retentionDays, now)
	}
	return nil
}

// deleteOldLLMAuditLogs applies row-based retention for non-partitioned databases.
func deleteOldLLMAuditLogs(ctx context.Context, tx *gorm.DB, retentionDays int, now time.Time) error {
	if retentionDays <= 0 {
		return nil
	}
	cutoff := now.AddDate(0, 0, -retentionDays)
	return tx.WithContext(ctx).Where("created_at < ?", cutoff).Delete(&types.LLMAuditLog{}).Error
}

// ensureLLMAuditLogFuturePartitions creates the current monthly partition plus configured future months.
func ensureLLMAuditLogFuturePartitions(ctx context.Context, tx *gorm.DB, now time.Time) error {
	start := llmAuditLogPartitionStart(now)
	for i := range llmAuditLogFuturePartitions + 1 {
		from := start.AddDate(0, i, 0)
		to := from.AddDate(0, 1, 0)
		if err := tx.WithContext(ctx).Exec(createLLMAuditLogPartitionSQL(from), from, to).Error; err != nil {
			return err
		}
	}
	return nil
}

// dropOldLLMAuditLogPartitions drops monthly partitions whose full range is older than retention.
func dropOldLLMAuditLogPartitions(ctx context.Context, tx *gorm.DB, retentionDays int, now time.Time) error {
	cutoff := now.AddDate(0, 0, -retentionDays)
	rows, err := tx.WithContext(ctx).Raw(`
SELECT c.relname
FROM pg_inherits i
JOIN pg_class c ON c.oid = i.inhrelid
JOIN pg_class p ON p.oid = i.inhparent
WHERE p.relname = $1`, llmAuditLogTableName).Rows()
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return err
		}
		from, ok := llmAuditLogPartitionStartFromName(name)
		if !ok {
			continue
		}
		to := from.AddDate(0, 1, 0)
		if to.After(cutoff) {
			continue
		}
		if err := tx.WithContext(ctx).Exec(fmt.Sprintf(`DROP TABLE IF EXISTS "%s"`, name)).Error; err != nil {
			return err
		}
	}
	return rows.Err()
}

// tryLLMAuditLogAdvisoryLock serializes partition DDL across horizontally scaled instances.
func tryLLMAuditLogAdvisoryLock(ctx context.Context, tx *gorm.DB) (bool, error) {
	var locked bool
	err := tx.WithContext(ctx).Raw("SELECT pg_try_advisory_xact_lock($1)", llmAuditLogMaintenanceLockID).Scan(&locked).Error
	return locked, err
}

// createLLMAuditLogPartitionSQL builds the CREATE TABLE statement for one monthly partition.
func createLLMAuditLogPartitionSQL(from time.Time) string {
	return fmt.Sprintf(
		`CREATE TABLE IF NOT EXISTS "%s" PARTITION OF "%s" FOR VALUES FROM ($1) TO ($2)`,
		from.Format(llmAuditLogTableName+llmAuditLogPartitionDateFormat), llmAuditLogTableName,
	)
}

// llmAuditLogPartitionStart returns the UTC calendar-month start containing t.
func llmAuditLogPartitionStart(t time.Time) time.Time {
	y, m, _ := t.UTC().Date()
	return time.Date(y, m, 1, 0, 0, 0, 0, time.UTC)
}

// llmAuditLogPartitionStartFromName parses the partition start month from a child table name.
func llmAuditLogPartitionStartFromName(name string) (time.Time, bool) {
	s, ok := strings.CutPrefix(name, llmAuditLogTableName)
	if !ok {
		return time.Time{}, false
	}
	t, err := time.Parse(llmAuditLogPartitionDateFormat, s)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}
