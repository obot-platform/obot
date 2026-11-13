package client

import (
	"strings"
	"time"

	"gorm.io/gorm"
)

// AuditLogExportQuery selects server update time and ID-keyset pagination for
// scheduled exports. These queries skip the unused count and return zero for total.
// Browsing and one-time exports retain their event-time selection and ordering.
type AuditLogExportQuery struct {
	AfterID string
}

func applyScheduledAuditLogQuery(db *gorm.DB, query *AuditLogExportQuery, start, end time.Time) *gorm.DB {
	// Keep timestamp bounds on the columns themselves so ordinary updated_at and
	// created_at indexes can serve the two branches, including historical NULL/zero rows.
	const zeroTime = "0001-01-01 00:00:00+00:00"
	updated := []string{"updated_at > ?"}
	updatedArgs := []any{zeroTime}
	legacy := []string{"(updated_at IS NULL OR updated_at <= ?)"}
	legacyArgs := []any{zeroTime}
	if !start.IsZero() {
		updated = append(updated, "updated_at >= ?")
		updatedArgs = append(updatedArgs, start.UTC())
		legacy = append(legacy, "created_at >= ?")
		legacyArgs = append(legacyArgs, start.UTC())
	}
	if !end.IsZero() {
		updated = append(updated, "updated_at < ?")
		updatedArgs = append(updatedArgs, end.UTC())
		legacy = append(legacy, "created_at < ?")
		legacyArgs = append(legacyArgs, end.UTC())
	}
	db = db.Where("("+strings.Join(updated, " AND ")+") OR ("+strings.Join(legacy, " AND ")+")", append(updatedArgs, legacyArgs...)...)
	if query.AfterID != "" {
		// Immutable IDs anchor pages when updates move earlier rows out of this fixed
		// window. IDs never track coverage between exports; the schedule's EndTime does.
		db = db.Where("id > ?", query.AfterID)
	}
	return db
}
