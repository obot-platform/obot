package client

import (
	"context"
	"fmt"
	"strconv"

	apitypes "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/auditlog"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"gorm.io/gorm"
)

type auditLogAPIKeySnapshot struct {
	APIKeyID   uint
	APIKeyName string
}

// GetMCPAuditLogAPIKeyFilterOptions returns the API keys present in the
// currently filtered and authorized MCP audit-log result set.
func (c *Client) GetMCPAuditLogAPIKeyFilterOptions(ctx context.Context, opts MCPAuditLogOptions) ([]apitypes.AuditLogAPIKeyFilterOption, error) {
	sources := auditlog.NormalizeSourceTypes(opts.SourceTypes)
	if err := ValidateAuditLogOptions(opts, sources); err != nil {
		return nil, err
	}
	if len(sources) != 1 || sources[0] != apitypes.AuditLogSourceTypeMCP {
		return nil, fmt.Errorf("API key filter options require the MCP audit log source")
	}

	db := c.db.WithContext(ctx).Model(&types.MCPAuditLog{}).Where("source_type = ?", apitypes.AuditLogSourceTypeMCP)
	if opts.Query != "" {
		var err error
		db, err = c.applyAuditLogSearch(ctx, db, opts.Query)
		if err != nil {
			return nil, err
		}
	}
	if len(opts.UserID) > 0 {
		db = db.Where("user_id IN ?", opts.UserID)
	}
	if len(opts.SessionID) > 0 {
		db = db.Where("session_id IN ?", opts.SessionID)
	}
	if len(opts.ClientIP) > 0 {
		db = db.Where("client_ip IN ?", opts.ClientIP)
	}
	if !opts.StartTime.IsZero() {
		db = db.Where("created_at >= ?", opts.StartTime.UTC())
	}
	if !opts.EndTime.IsZero() {
		db = db.Where("created_at < ?", opts.EndTime.UTC())
	}
	if opts.ProcessingTimeMin > 0 {
		db = db.Where("processing_time_ms >= ?", opts.ProcessingTimeMin)
	}
	if opts.ProcessingTimeMax > 0 {
		db = db.Where("processing_time_ms <= ?", opts.ProcessingTimeMax)
	}
	db = applyMCPAuditLogFilters(db, opts)
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

func (c *Client) scanAuditLogAPIKeyFilterOptions(ctx context.Context, db *gorm.DB, limit int) ([]apitypes.AuditLogAPIKeyFilterOption, error) {
	query := db.
		Where("api_key_id IS NOT NULL").
		Select("api_key_id, MAX(api_key_name) AS api_key_name").
		Group("api_key_id").
		Order("api_key_name, api_key_id")
	if limit > 0 {
		query = query.Limit(limit)
	}

	var snapshots []auditLogAPIKeySnapshot
	if err := query.Scan(&snapshots).Error; err != nil {
		return nil, err
	}
	return c.hydrateAuditLogAPIKeyFilterOptions(ctx, snapshots)
}

func (c *Client) hydrateAuditLogAPIKeyFilterOptions(ctx context.Context, snapshots []auditLogAPIKeySnapshot) ([]apitypes.AuditLogAPIKeyFilterOption, error) {
	if len(snapshots) == 0 {
		return []apitypes.AuditLogAPIKeyFilterOption{}, nil
	}

	keyIDs := make([]uint, 0, len(snapshots))
	for _, snapshot := range snapshots {
		keyIDs = append(keyIDs, snapshot.APIKeyID)
	}
	var keys []types.APIKey
	if err := c.db.WithContext(ctx).Where("id IN ?", keyIDs).Find(&keys).Error; err != nil {
		return nil, err
	}
	keysByID := make(map[uint]types.APIKey, len(keys))
	userIDs := make([]uint, 0, len(keys))
	for _, key := range keys {
		keysByID[key.ID] = key
		userIDs = append(userIDs, key.UserID)
	}

	var users []types.User
	if len(userIDs) > 0 {
		if err := c.db.WithContext(ctx).Where("id IN ?", userIDs).Find(&users).Error; err != nil {
			return nil, err
		}
	}
	usersByID := make(map[uint]types.User, len(users))
	for _, user := range users {
		usersByID[user.ID] = user
	}

	options := make([]apitypes.AuditLogAPIKeyFilterOption, 0, len(snapshots))
	for _, snapshot := range snapshots {
		option := apitypes.AuditLogAPIKeyFilterOption{
			Value: strconv.FormatUint(uint64(snapshot.APIKeyID), 10),
			Name:  snapshot.APIKeyName,
		}
		if key, ok := keysByID[snapshot.APIKeyID]; ok {
			option.UserID = strconv.FormatUint(uint64(key.UserID), 10)
			option.MaskedKey = fmt.Sprintf("ok1-%d-%d-*****", key.UserID, key.ID)
			option.Revoked = key.RevokedAt != nil
			option.UserDisplayName = auditLogAPIKeyUserDisplayName(usersByID[key.UserID], option.UserID)
		}
		options = append(options, option)
	}
	return options, nil
}

func auditLogAPIKeyUserDisplayName(user types.User, fallback string) string {
	if user.DisplayName != "" {
		return user.DisplayName
	}
	if user.Username != "" {
		return user.Username
	}
	if user.Email != "" {
		return user.Email
	}
	return fallback
}
