package client

import (
	"context"
	"strconv"

	apitypes "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/auditlog"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/principal"
	"gorm.io/gorm"
)

type auditLogAPIKeyOptionRow struct {
	APIKeyID        uint
	APIKeyName      string
	UserID          uint
	Revoked         bool
	UserDisplayName string
	Username        string
	Email           string
}

// GetMCPAuditLogAPIKeyFilterOptions returns the API keys present in the
// currently filtered and authorized MCP and local-agent audit-log result set.
func (c *Client) GetMCPAuditLogAPIKeyFilterOptions(ctx context.Context, opts MCPAuditLogOptions) ([]apitypes.AuditLogAPIKeyFilterOption, error) {
	sources := auditlog.NormalizeSourceTypes(opts.SourceTypes)
	if err := ValidateAuditLogOptions(opts, sources); err != nil {
		return nil, err
	}

	db, _, err := c.auditLogBaseQuery(ctx, opts, sources)
	if err != nil {
		return nil, err
	}
	if hasMCPAuditLogFilters(opts) {
		db = applyMCPAuditLogFilters(db.Where("source_type = ?", apitypes.AuditLogSourceTypeMCP), opts)
	} else if hasLocalAgentAuditLogFilters(opts) {
		db = applyLocalAgentAuditLogFilters(db.Where("source_type = ?", apitypes.AuditLogSourceTypeLocalAgentToolCall), opts)
	}
	db = applyUnifiedAuditLogFilters(db, opts)

	return c.scanAuditLogAPIKeyFilterOptions(ctx, db, opts.Limit)
}

// GetLLMAuditLogAPIKeyFilterOptions returns the API keys present in the
// currently filtered LLM audit-log result set.
func (c *Client) GetLLMAuditLogAPIKeyFilterOptions(ctx context.Context, opts LLMAuditLogOptions) ([]apitypes.AuditLogAPIKeyFilterOption, error) {
	db := c.db.WithContext(ctx).Model(&types.LLMAuditLog{})
	db = applyLLMAuditLogOptions(db, opts)
	return c.scanAuditLogAPIKeyFilterOptions(ctx, db, opts.Limit)
}

// scanAuditLogAPIKeyFilterOptions returns the unique API keys represented by
// the filtered audit-log query in db, enriched with current key and user data.
func (c *Client) scanAuditLogAPIKeyFilterOptions(ctx context.Context, db *gorm.DB, limit int) ([]apitypes.AuditLogAPIKeyFilterOption, error) {
	// API key names are immutable, so every snapshot for a key should have the
	// same name. MAX selects that value while grouping the snapshots by key ID.
	snapshots := db.
		Where("api_key_id IS NOT NULL").
		Select("api_key_id, MAX(api_key_name) AS api_key_name").
		Group("api_key_id")
	query := c.db.WithContext(ctx).
		Table("(?) AS audit_key_snapshots", snapshots).
		Select(`audit_key_snapshots.api_key_id,
			audit_key_snapshots.api_key_name,
			COALESCE(api_keys.user_id, 0) AS user_id,
			(api_keys.revoked_at IS NOT NULL) AS revoked,
			COALESCE(users.display_name, '') AS user_display_name,
			COALESCE(users.username, '') AS username,
			COALESCE(users.email, '') AS email`).
		Joins("LEFT JOIN api_keys ON api_keys.id = audit_key_snapshots.api_key_id").
		Joins("LEFT JOIN users ON users.id = api_keys.user_id").
		Order("audit_key_snapshots.api_key_name, audit_key_snapshots.api_key_id")
	if limit > 0 {
		query = query.Limit(limit)
	}

	var rows []auditLogAPIKeyOptionRow
	if err := query.Scan(&rows).Error; err != nil {
		return nil, err
	}

	options := make([]apitypes.AuditLogAPIKeyFilterOption, 0, len(rows))
	for _, row := range rows {
		option := apitypes.AuditLogAPIKeyFilterOption{
			Value:   strconv.FormatUint(uint64(row.APIKeyID), 10),
			Name:    row.APIKeyName,
			Revoked: row.Revoked,
		}
		if row.UserID != 0 {
			option.UserID = strconv.FormatUint(uint64(row.UserID), 10)
			option.MaskedKey = principal.MaskedAPIKeyName(option.UserID, row.APIKeyID)
			option.UserDisplayName = auditLogAPIKeyUserDisplayName(
				row.UserDisplayName, row.Username, row.Email, option.UserID)
		}
		options = append(options, option)
	}
	return options, nil
}

func auditLogAPIKeyUserDisplayName(displayName, username, email, fallback string) string {
	if displayName != "" {
		return displayName
	}
	if username != "" {
		return username
	}
	if email != "" {
		return email
	}
	return fallback
}
