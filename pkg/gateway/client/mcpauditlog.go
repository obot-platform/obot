package client

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"gorm.io/gorm"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/storage/value"
)

var (
	mcpAuditLogGroupResource = schema.GroupResource{
		Group:    "obot.obot.ai",
		Resource: "mcpauditlogs",
	}
)

func (c *Client) insertMCPAuditLogs(ctx context.Context, logs []types.MCPAuditLog) error {
	return c.db.WithContext(ctx).CreateInBatches(logs, 100).Error
}

// GetMCPAuditLogs retrieves MCP audit logs with optional filters
func (c *Client) GetMCPAuditLogs(ctx context.Context, opts MCPAuditLogOptions) ([]types.MCPAuditLog, int64, error) {
	var logs []types.MCPAuditLog

	db := c.db.WithContext(ctx).Model(&types.MCPAuditLog{})

	// Apply text search across multiple fields
	if opts.Query != "" {
		searchTerm := "%" + opts.Query + "%"
		like := "LIKE"
		if db.Name() == "postgres" {
			like = "ILIKE"
		}

		// First, get any potential users that match the search term.

		users, err := c.UsersIncludeDeleted(ctx, types.UserQuery{})
		if err != nil {
			return nil, 0, fmt.Errorf("failed to get users: %w", err)
		}

		var userIDs []string
		// Don't lowercase the query for the selecting from the database. Some databases aren't case-sensitive,
		// and we want to preserve the case of the search term.
		userQuery := strings.ToLower(opts.Query)
		for _, u := range users {
			if strings.Contains(strings.ToLower(u.DisplayName), userQuery) {
				userIDs = append(userIDs, strconv.FormatUint(uint64(u.ID), 10))
			}
		}

		// Check if the query is a valid integer for response_status search
		query := `user_id in (?) OR mcp_id %[1]s ? OR mcp_server_display_name %[1]s ? OR
mcp_server_catalog_entry_name %[1]s ? OR client_name %[1]s ? OR client_version %[1]s ? OR
client_ip %[1]s ? OR call_type %[1]s ? OR call_identifier %[1]s ? OR error %[1]s ? OR
session_id %[1]s ? OR request_id %[1]s ? OR user_agent %[1]s ?`

		args := append([]any{userIDs}, slices.Repeat([]any{searchTerm}, strings.Count(query, "%[1]s ?"))...)

		if responseStatus, err := strconv.Atoi(opts.Query); err == nil {
			query += " OR response_status = ?"
			args = append(args, responseStatus)
		}

		db = db.Where(fmt.Sprintf(query, like), args...)
	}

	// Apply filters
	if len(opts.UserID) > 0 {
		db = db.Where("user_id IN (?)", opts.UserID)
	}
	if len(opts.MCPID) > 0 {
		db = db.Where("mcp_id IN (?)", opts.MCPID)
	}
	if len(opts.MCPServerDisplayName) > 0 {
		db = db.Where("mcp_server_display_name IN (?)", opts.MCPServerDisplayName)
	}
	if len(opts.MCPServerCatalogEntryName) > 0 {
		db = db.Where("mcp_server_catalog_entry_name IN (?)", opts.MCPServerCatalogEntryName)
	}
	if len(opts.CallType) > 0 {
		db = db.Where("call_type IN (?)", opts.CallType)
	}
	if len(opts.CallIdentifier) > 0 {
		db = db.Where("call_identifier IN (?)", opts.CallIdentifier)
	}
	if len(opts.SessionID) > 0 {
		db = db.Where("session_id IN (?)", opts.SessionID)
	}
	if len(opts.ClientName) > 0 {
		db = db.Where("client_name IN (?)", opts.ClientName)
	}
	if len(opts.ClientVersion) > 0 {
		db = db.Where("client_version IN (?)", opts.ClientVersion)
	}
	if len(opts.ResponseStatus) > 0 {
		db = db.Where("response_status IN (?)", opts.ResponseStatus)
	}
	if len(opts.ClientIP) > 0 {
		db = db.Where("client_ip IN (?)", opts.ClientIP)
	}
	if opts.ProcessingTimeMin > 0 {
		db = db.Where("processing_time_ms >= ?", opts.ProcessingTimeMin)
	}
	if opts.ProcessingTimeMax > 0 {
		db = db.Where("processing_time_ms <= ?", opts.ProcessingTimeMax)
	}
	if !opts.StartTime.IsZero() {
		db = db.Where("created_at >= ?", opts.StartTime.Local())
	}
	if !opts.EndTime.IsZero() {
		db = db.Where("created_at < ?", opts.EndTime.Local())
	}

	// Get the total before applying the limit
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Apply pagination
	if opts.Limit > 0 {
		db = db.Limit(opts.Limit)
	}
	if opts.Offset > 0 {
		db = db.Offset(opts.Offset)
	}

	// Apply sorting
	if opts.SortBy != "" {
		// Validate sort field to prevent SQL injection
		validSortFields := map[string]bool{
			"created_at":                    true,
			"mcp_id":                        true,
			"mcp_server_display_name":       true,
			"mcp_server_catalog_entry_name": true,
			"call_type":                     true,
			"call_identifier":               true,
			"processing_time_ms":            true,
			"client_name":                   true,
			"client_version":                true,
			"response_status":               true,
			"client_ip":                     true,
		}

		if validSortFields[opts.SortBy] {
			sortOrder := "DESC" // default to descending
			if opts.SortOrder == "asc" {
				sortOrder = "ASC"
			}
			db = db.Order(opts.SortBy + " " + sortOrder)
		} else {
			// Fallback to default sorting if invalid field
			db = db.Order("created_at DESC")
		}
	} else {
		// Default sorting by created_at descending
		db = db.Order("created_at DESC")
	}

	err := db.Find(&logs).Error
	if err != nil {
		return nil, 0, err
	}

	// Decrypt the logs after fetching
	for i := range logs {
		if !opts.WithRequestAndResponse {
			// These are the only fields that are encrypted right now.
			// So, just blank them out and skip decryption.
			logs[i].RequestBody = nil
			logs[i].ResponseBody = nil
			logs[i].RequestHeaders = nil
			logs[i].ResponseHeaders = nil
		} else {
			if err := c.decryptMCPAuditLog(ctx, &logs[i]); err != nil {
				return nil, 0, fmt.Errorf("failed to decrypt MCP audit log: %w", err)
			}
		}
	}

	return logs, total, nil
}

func (c *Client) GetAuditLogFilterOptions(ctx context.Context, option string, opts MCPAuditLogOptions, exclude ...any) ([]string, error) {
	db := c.db.WithContext(ctx).Model(&types.MCPAuditLog{}).Distinct(option)

	// Apply the same filters as GetMCPAuditLogs (excluding sorting, limit, offset)
	if len(opts.UserID) > 0 {
		db = db.Where("user_id IN (?)", opts.UserID)
	}
	if len(opts.MCPID) > 0 {
		db = db.Where("mcp_id IN (?)", opts.MCPID)
	}
	if len(opts.MCPServerDisplayName) > 0 {
		db = db.Where("mcp_server_display_name IN (?)", opts.MCPServerDisplayName)
	}
	if len(opts.MCPServerCatalogEntryName) > 0 {
		db = db.Where("mcp_server_catalog_entry_name IN (?)", opts.MCPServerCatalogEntryName)
	}
	if len(opts.CallType) > 0 {
		db = db.Where("call_type IN (?)", opts.CallType)
	}
	if len(opts.CallIdentifier) > 0 {
		db = db.Where("call_identifier IN (?)", opts.CallIdentifier)
	}
	if len(opts.SessionID) > 0 {
		db = db.Where("session_id IN (?)", opts.SessionID)
	}
	if len(opts.ClientName) > 0 {
		db = db.Where("client_name IN (?)", opts.ClientName)
	}
	if len(opts.ClientVersion) > 0 {
		db = db.Where("client_version IN (?)", opts.ClientVersion)
	}
	if len(opts.ResponseStatus) > 0 {
		db = db.Where("response_status IN (?)", opts.ResponseStatus)
	}
	if len(opts.ClientIP) > 0 {
		db = db.Where("client_ip IN (?)", opts.ClientIP)
	}
	if !opts.StartTime.IsZero() {
		db = db.Where("created_at >= ?", opts.StartTime.Local())
	}
	if !opts.EndTime.IsZero() {
		db = db.Where("created_at < ?", opts.EndTime.Local())
	}

	var result []string
	if len(exclude) != 0 {
		db = db.Where(option+" NOT IN ?", exclude)
	}
	return result, db.Select(option).Scan(&result).Error
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
		if len(opts.UserIDs) > 0 {
			tx = tx.Where("user_id IN (?)", opts.UserIDs)
		}
		if len(opts.MCPServerDisplayNames) > 0 {
			tx = tx.Where("mcp_server_display_name IN (?)", opts.MCPServerDisplayNames)
		}
		if len(opts.MCPServerCatalogEntryNames) > 0 {
			tx = tx.Where("mcp_server_catalog_entry_name IN (?)", opts.MCPServerCatalogEntryNames)
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

			// Get tool call items and build stats from them
			var toolItems []types.MCPToolCallStatsItem
			if err := base.
				Select("call_identifier as tool_name, created_at, user_id, processing_time_ms, response_status, error").
				Where("mcp_id = ? AND call_type = ? AND created_at >= ? AND created_at < ?",
					basic.MCPID, "tools/call", opts.StartTime, opts.EndTime).
				Where("call_identifier != ''").
				Scan(&toolItems).Error; err != nil {
				return err
			}

			// Build tool stats from items using a map for efficiency
			toolStatsMap := make(map[string][]types.MCPToolCallStatsItem)
			for _, item := range toolItems {
				toolStatsMap[item.ToolName] = append(toolStatsMap[item.ToolName], item)
			}

			// Convert map to slice of MCPToolCallStats
			var toolStats []types.MCPToolCallStats
			for toolName, items := range toolStatsMap {
				toolStats = append(toolStats, types.MCPToolCallStats{
					ToolName:  toolName,
					CallCount: int64(len(items)),
					Items:     items,
				})
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

// MCPAuditLogOptions represents options for querying MCP audit logs
type MCPAuditLogOptions struct {
	WithRequestAndResponse    bool
	UserID                    []string
	MCPID                     []string
	MCPServerDisplayName      []string
	MCPServerCatalogEntryName []string
	CallType                  []string
	CallIdentifier            []string
	SessionID                 []string
	ClientName                []string
	ClientVersion             []string
	ResponseStatus            []string
	ClientIP                  []string
	ProcessingTimeMin         int64
	ProcessingTimeMax         int64
	Query                     string // Search term for text search across multiple fields
	StartTime                 time.Time
	EndTime                   time.Time
	Limit                     int
	Offset                    int
	SortBy                    string // Field to sort by (e.g., "created_at", "user_id", "call_type")
	SortOrder                 string // Sort order: "asc" or "desc"
}

// MCPUsageStatsOptions represents options for querying MCP usage statistics
type MCPUsageStatsOptions struct {
	MCPID                      string
	UserIDs                    []string
	MCPServerDisplayNames      []string
	MCPServerCatalogEntryNames []string
	StartTime                  time.Time
	EndTime                    time.Time
}

func (c *Client) encryptMCPAuditLog(ctx context.Context, log *types.MCPAuditLog) error {
	if c.encryptionConfig == nil {
		return nil
	}

	transformer := c.encryptionConfig.Transformers[mcpAuditLogGroupResource]
	if transformer == nil {
		return nil
	}

	var (
		b    []byte
		err  error
		errs []error

		dataCtx = mcpAuditLogDataCtx(log)
	)

	if len(log.RequestBody) > 0 {
		if b, err = transformer.TransformToStorage(ctx, log.RequestBody, dataCtx); err != nil {
			errs = append(errs, err)
		} else {
			log.RequestBody = json.RawMessage(base64.StdEncoding.EncodeToString(b))
		}
	}

	if len(log.ResponseBody) > 0 {
		if b, err = transformer.TransformToStorage(ctx, log.ResponseBody, dataCtx); err != nil {
			errs = append(errs, err)
		} else {
			log.ResponseBody = json.RawMessage(base64.StdEncoding.EncodeToString(b))
		}
	}

	if len(log.RequestHeaders) > 0 {
		if b, err = transformer.TransformToStorage(ctx, log.RequestHeaders, dataCtx); err != nil {
			errs = append(errs, err)
		} else {
			log.RequestHeaders = json.RawMessage(base64.StdEncoding.EncodeToString(b))
		}
	}

	if len(log.ResponseHeaders) > 0 {
		if b, err = transformer.TransformToStorage(ctx, log.ResponseHeaders, dataCtx); err != nil {
			errs = append(errs, err)
		} else {
			log.ResponseHeaders = json.RawMessage(base64.StdEncoding.EncodeToString(b))
		}
	}

	log.Encrypted = true

	return errors.Join(errs...)
}

func (c *Client) decryptMCPAuditLog(ctx context.Context, log *types.MCPAuditLog) error {
	if !log.Encrypted || c.encryptionConfig == nil {
		return nil
	}

	transformer := c.encryptionConfig.Transformers[mcpAuditLogGroupResource]
	if transformer == nil {
		return nil
	}

	var (
		out, decoded []byte
		n            int
		err          error
		errs         []error

		dataCtx = mcpAuditLogDataCtx(log)
	)

	if len(log.RequestBody) > 0 {
		decoded = make([]byte, base64.StdEncoding.DecodedLen(len(log.RequestBody)))
		n, err = base64.StdEncoding.Decode(decoded, log.RequestBody)
		if err == nil {
			if out, _, err = transformer.TransformFromStorage(ctx, decoded[:n], dataCtx); err != nil {
				errs = append(errs, err)
			} else {
				log.RequestBody = json.RawMessage(out)
			}
		} else {
			errs = append(errs, err)
		}
	}

	if len(log.ResponseBody) > 0 {
		decoded = make([]byte, base64.StdEncoding.DecodedLen(len(log.ResponseBody)))
		n, err = base64.StdEncoding.Decode(decoded, log.ResponseBody)
		if err == nil {
			if out, _, err = transformer.TransformFromStorage(ctx, decoded[:n], dataCtx); err != nil {
				errs = append(errs, err)
			} else {
				log.ResponseBody = json.RawMessage(out)
			}
		} else {
			errs = append(errs, err)
		}
	}

	if len(log.RequestHeaders) > 0 {
		decoded = make([]byte, base64.StdEncoding.DecodedLen(len(log.RequestHeaders)))
		n, err = base64.StdEncoding.Decode(decoded, log.RequestHeaders)
		if err == nil {
			if out, _, err = transformer.TransformFromStorage(ctx, decoded[:n], dataCtx); err != nil {
				errs = append(errs, err)
			} else {
				log.RequestHeaders = json.RawMessage(out)
			}
		} else {
			errs = append(errs, err)
		}
	}

	if len(log.ResponseHeaders) > 0 {
		decoded = make([]byte, base64.StdEncoding.DecodedLen(len(log.ResponseHeaders)))
		n, err = base64.StdEncoding.Decode(decoded, log.ResponseHeaders)
		if err == nil {
			if out, _, err = transformer.TransformFromStorage(ctx, decoded[:n], dataCtx); err != nil {
				errs = append(errs, err)
			} else {
				log.ResponseHeaders = json.RawMessage(out)
			}
		} else {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

func mcpAuditLogDataCtx(log *types.MCPAuditLog) value.Context {
	return value.DefaultContext(fmt.Sprintf("%s/%s/%s", mcpAuditLogGroupResource.String(), log.MCPID, log.UserID))
}
