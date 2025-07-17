package client

import (
	"context"
	"time"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"gorm.io/gorm"
)

// InsertMCPAuditLog inserts a new MCP audit log entry
func (c *Client) InsertMCPAuditLog(ctx context.Context, log *types.MCPAuditLog) error {
	return c.db.WithContext(ctx).Create(log).Error
}

func (c *Client) UpdateMCPAuditLogByRequestID(ctx context.Context, log *types.MCPAuditLog) error {
	return c.db.WithContext(ctx).Where("request_id = ?", log.RequestID).Updates(log).Error
}

// GetMCPAuditLogs retrieves MCP audit logs with optional filters
func (c *Client) GetMCPAuditLogs(ctx context.Context, opts MCPAuditLogOptions) ([]types.MCPAuditLog, error) {
	var logs []types.MCPAuditLog

	db := c.db.WithContext(ctx).Model(&types.MCPAuditLog{})

	// Apply filters
	if opts.UserID != "" {
		db = db.Where("user_id = ?", opts.UserID)
	}
	if opts.MCPID != "" {
		db = db.Where("mcp_id = ?", opts.MCPID)
	}
	if opts.MCPServerDisplayName != "" {
		db = db.Where("mcp_server_display_name = ?", opts.MCPServerDisplayName)
	}
	if opts.MCPServerCatalogEntryName != "" {
		db = db.Where("mcp_server_catalog_entry_name = ?", opts.MCPServerCatalogEntryName)
	}
	if opts.Client != "" {
		db = db.Where("client = ?", opts.Client)
	}
	if opts.CallType != "" {
		db = db.Where("call_type = ?", opts.CallType)
	}
	if opts.SessionID != "" {
		db = db.Where("session_id = ?", opts.SessionID)
	}
	if !opts.StartTime.IsZero() {
		db = db.Where("created_at >= ?", opts.StartTime)
	}
	if !opts.EndTime.IsZero() {
		db = db.Where("created_at < ?", opts.EndTime)
	}

	// Apply pagination
	if opts.Limit > 0 {
		db = db.Limit(opts.Limit)
	}
	if opts.Offset > 0 {
		db = db.Offset(opts.Offset)
	}

	// Order by created_at descending by default
	db = db.Order("created_at DESC")

	return logs, db.Find(&logs).Error
}

// GetMCPUsageStats retrieves usage statistics for MCP servers
func (c *Client) GetMCPUsageStats(ctx context.Context, opts MCPUsageStatsOptions) (types.MCPUsageStatsList, error) {
	type totalCallsAndUniqueUsers struct {
		TotalCalls  int64
		UniqueUsers int64
	}

	var (
		callsAndUsers totalCallsAndUniqueUsers
		stats         []types.MCPUsageStatItem
	)

	// Get basic stats for each server
	if err := c.db.WithContext(ctx).Transaction(func(base *gorm.DB) error {
		base = base.Model(&types.MCPAuditLog{}).Session(&gorm.Session{})
		tx := base.Where("created_at >= ? AND created_at < ?", opts.StartTime, opts.EndTime)

		if opts.MCPID != "" {
			tx = tx.Where("mcp_id = ?", opts.MCPID)
		}
		if opts.MCPServerDisplayName != "" {
			tx = tx.Where("mcp_server_display_name = ?", opts.MCPServerDisplayName)
		}
		if opts.MCPServerCatalogEntryName != "" {
			tx = tx.Where("mcp_server_catalog_entry_name = ?", opts.MCPServerCatalogEntryName)
		}

		type basicStats struct {
			MCPID                     string
			MCPServerDisplayName      string
			MCPServerCatalogEntryName string
		}

		if err := tx.Select("COUNT(*) AS total_calls, COUNT(DISTINCT user_id) AS unique_users").Scan(&callsAndUsers).Error; err != nil {
			return err
		}

		var basicStatsList []basicStats
		if err := tx.Select("mcp_id, mcp_server_display_name, mcp_server_catalog_entry_name").
			Group("mcp_id, mcp_server_display_name, mcp_server_catalog_entry_name").
			Scan(&basicStatsList).Error; err != nil {
			return err
		}

		var stat types.MCPUsageStatItem
		stats = make([]types.MCPUsageStatItem, 0, len(basicStatsList))
		// Build the full stats with tool call breakdown
		for _, basic := range basicStatsList {
			stat = types.MCPUsageStatItem{
				MCPID:                     basic.MCPID,
				MCPServerDisplayName:      basic.MCPServerDisplayName,
				MCPServerCatalogEntryName: basic.MCPServerCatalogEntryName,
			}

			// Get tool call breakdown for this server
			var toolStats []types.MCPToolCallStats
			if err := base.
				Select("call_identifier as tool_name, COUNT(*) as call_count").
				Where("mcp_id = ? AND call_type = ? AND created_at >= ? AND created_at < ?",
					basic.MCPID, "tools/call", opts.StartTime, opts.EndTime).
				Where("call_identifier != ''").
				Group("call_identifier").
				Scan(&toolStats).Error; err != nil {
				return err
			}

			// Get resource read breakdown for this server
			var resourceStats []types.MCPResourceReadStats
			if err := base.
				Select("call_identifier as resource_uri, COUNT(*) as read_count").
				Where("mcp_id = ? AND call_type = ? AND created_at >= ? AND created_at < ?",
					basic.MCPID, "resources/read", opts.StartTime, opts.EndTime).
				Where("call_identifier != ''").
				Group("call_identifier").
				Scan(&resourceStats).Error; err != nil {
				return err
			}

			// Get prompt read breakdown for this server
			var promptStats []types.MCPPromptReadStats
			if err := base.
				Select("call_identifier as prompt_name, COUNT(*) as read_count").
				Where("mcp_id = ? AND call_type = ? AND created_at >= ? AND created_at < ?",
					basic.MCPID, "prompts/get", opts.StartTime, opts.EndTime).
				Where("call_identifier != ''").
				Group("call_identifier").
				Scan(&promptStats).Error; err != nil {
				return err
			}

			stat.ToolCalls = toolStats
			stat.ResourceReads = resourceStats
			stat.PromptReads = promptStats
			stats = append(stats, stat)
		}

		return nil
	}); err != nil {
		return types.MCPUsageStatsList{}, err
	}

	return types.MCPUsageStatsList{
		TimeStart:   opts.StartTime,
		TimeEnd:     opts.EndTime,
		TotalCalls:  callsAndUsers.TotalCalls,
		UniqueUsers: callsAndUsers.UniqueUsers,
		Items:       stats,
	}, nil
}

// CountMCPAuditLogs counts the total number of audit logs matching the given criteria
func (c *Client) CountMCPAuditLogs(ctx context.Context, opts MCPAuditLogOptions) (int64, error) {
	var count int64

	db := c.db.WithContext(ctx).Model(&types.MCPAuditLog{})

	// Apply filters
	if opts.UserID != "" {
		db = db.Where("user_id = ?", opts.UserID)
	}
	if opts.MCPID != "" {
		db = db.Where("mcp_id = ?", opts.MCPID)
	}
	if opts.MCPServerDisplayName != "" {
		db = db.Where("mcp_server_display_name = ?", opts.MCPServerDisplayName)
	}
	if opts.MCPServerCatalogEntryName != "" {
		db = db.Where("mcp_server_catalog_entry_name = ?", opts.MCPServerCatalogEntryName)
	}
	if opts.Client != "" {
		db = db.Where("client = ?", opts.Client)
	}
	if opts.CallType != "" {
		db = db.Where("call_type = ?", opts.CallType)
	}
	if opts.SessionID != "" {
		db = db.Where("session_id = ?", opts.SessionID)
	}
	if !opts.StartTime.IsZero() {
		db = db.Where("created_at >= ?", opts.StartTime)
	}
	if !opts.EndTime.IsZero() {
		db = db.Where("created_at < ?", opts.EndTime)
	}

	return count, db.Count(&count).Error
}

// MCPAuditLogOptions represents options for querying MCP audit logs
type MCPAuditLogOptions struct {
	UserID                    string
	MCPID                     string
	MCPServerDisplayName      string
	MCPServerCatalogEntryName string
	Client                    string
	CallType                  string
	SessionID                 string
	StartTime                 time.Time
	EndTime                   time.Time
	Limit                     int
	Offset                    int
}

// MCPUsageStatsOptions represents options for querying MCP usage statistics
type MCPUsageStatsOptions struct {
	MCPID                     string
	MCPServerDisplayName      string
	MCPServerCatalogEntryName string
	StartTime                 time.Time
	EndTime                   time.Time
}
