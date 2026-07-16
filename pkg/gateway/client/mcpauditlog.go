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

	types2 "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/auditlog"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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
	if len(logs) == 0 {
		return nil
	}

	// Separate logs into three categories
	toInsert := make([]types.MCPAuditLog, 0, len(logs)/2)
	responseOnlyLogs := make([]types.MCPAuditLog, 0, len(logs)/2)

	for _, log := range logs {
		// Convert timestamp to UTC for consistency
		log.CreatedAt = log.CreatedAt.UTC()
		log.NormalizeMCPFields()
		if err := log.ValidateSourceFields(); err != nil {
			return fmt.Errorf("invalid audit log source fields: %w", err)
		}
		mcp := log.MCP()

		if !mcp.ResponseReceived {
			// Request-only logs
			toInsert = append(toInsert, log)
		} else if len(mcp.RequestBody) > 0 {
			// Complete logs (has both request and response data)
			toInsert = append(toInsert, log)
		} else {
			// Response-only logs (need to find and update existing request)
			responseOnlyLogs = append(responseOnlyLogs, log)
		}
	}

	// Use a transaction to ensure atomicity
	return c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Insert request-only and complete logs in batches
		if len(toInsert) > 0 {
			if err := tx.CreateInBatches(toInsert, 100).Error; err != nil {
				return fmt.Errorf("failed to insert audit logs: %w", err)
			}
		}

		// Process response-only logs
		for _, responseLog := range responseOnlyLogs {
			responseMCP := responseLog.MCP()
			// Find matching request log by RequestID and SessionID
			var existingLog types.MCPAuditLog
			err := tx.Where("request_id = ? AND session_id = ? AND response_received = ? AND source_type = ?", responseMCP.RequestID, responseMCP.SessionID, false, types2.AuditLogSourceTypeMCP).
				First(&existingLog).Error

			if err == nil {
				existingMCP := existingLog.MCP()
				// Found matching request - update with response data
				updates := map[string]any{
					"response_received": true,
				}

				// Update response-specific fields if they have values
				if len(responseMCP.MutatedRequestBody) > 0 {
					updates["mutated_request_body"] = responseMCP.MutatedRequestBody
					updates["request_mutated"] = true
				}
				if len(responseMCP.ResponseBody) > 0 {
					updates["response_body"] = responseMCP.ResponseBody
				}
				if len(responseMCP.OriginalResponseBody) > 0 {
					updates["original_response_body"] = responseMCP.OriginalResponseBody
					updates["response_mutated"] = true
				}
				if len(responseMCP.ResponseHeaders) > 0 {
					updates["response_headers"] = responseMCP.ResponseHeaders
				}
				if responseMCP.ResponseStatus != 0 {
					updates["response_status"] = responseMCP.ResponseStatus
				}
				if responseMCP.Error != "" {
					updates["error"] = responseMCP.Error
				}
				if len(responseMCP.WebhookStatuses) > 0 {
					updates["webhook_statuses"] = append(existingMCP.WebhookStatuses, responseMCP.WebhookStatuses...)
				}
				if existingLog.UserID == "" {
					updates["user_id"] = responseLog.UserID
				}
				if existingLog.ClientIP == "" {
					updates["client_ip"] = responseLog.ClientIP
				}
				if existingMCP.ClientName == "" {
					updates["client_name"] = responseMCP.ClientName
				}
				if existingMCP.ClientVersion == "" {
					updates["client_version"] = responseMCP.ClientVersion
				}

				// Calculate processing time as difference between response and request timestamps
				updates["processing_time_ms"] = responseLog.CreatedAt.Sub(existingLog.CreatedAt).Milliseconds()

				// Update the existing log
				if err := tx.Model(&existingLog).Updates(updates).Error; err != nil {
					return fmt.Errorf("failed to update audit log with response data: %w", err)
				}
			} else if errors.Is(err, gorm.ErrRecordNotFound) {
				// No matching request found - insert as new record
				if err := tx.Create(&responseLog).Error; err != nil {
					return fmt.Errorf("failed to insert orphaned response audit log: %w", err)
				}
			} else {
				// Database error
				return fmt.Errorf("failed to query for existing audit log: %w", err)
			}
		}

		return nil
	})
}

// InsertLocalAgentAuditLogs persists completed local-agent tool-call audit logs.
// Duplicate idempotency keys are treated as successful no-ops for transport retries.
func (c *Client) InsertLocalAgentAuditLogs(ctx context.Context, logs []types.MCPAuditLog) error {
	if len(logs) == 0 {
		return nil
	}

	toInsert := make([]types.MCPAuditLog, 0, len(logs))
	for i := range logs {
		log := logs[i]
		log.CreatedAt = log.CreatedAt.UTC()
		if log.SourceType != types2.AuditLogSourceTypeLocalAgentToolCall {
			return fmt.Errorf("local agent audit log source type is required")
		}
		if err := log.ValidateSourceFields(); err != nil {
			return fmt.Errorf("invalid local agent audit log source fields: %w", err)
		}
		if err := c.encryptMCPAuditLog(ctx, &log); err != nil {
			return fmt.Errorf("failed to encrypt local agent audit log: %w", err)
		}
		toInsert = append(toInsert, log)
	}

	return c.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "idempotency_key"}},
			DoNothing: true,
		}).
		CreateInBatches(toInsert, 100).Error
}

const (
	// effectiveAuditLogTimeSQL selects the timestamp used to order and time-filter a row. For MCP
	// rows this is the server-assigned created_at, but for local-agent rows it is occurred_at, which
	// is client-reported. This is intentional so an event sorts by when it actually happened on the
	// client, but it means a skewed or hostile client can position its local-agent events anywhere in
	// the merged timeline relative to server-timestamped MCP rows. The server-controlled created_at is
	// preserved separately (and surfaced as RecordedAt) so this ordering is never mistaken for a
	// tamper-resistant record of when Obot received the event.
	effectiveAuditLogTimeSQL = "CASE WHEN source_type = 'local_agent_tool_call' THEN occurred_at ELSE created_at END"
)

var (
	mcpAuditLogSortColumns = map[string]string{
		"created_at":                    "created_at",
		"mcp_id":                        "mcp_id",
		"mcp_server_display_name":       "mcp_server_display_name",
		"mcp_server_catalog_entry_name": "mcp_server_catalog_entry_name",
		"call_type":                     "call_type",
		"call_identifier":               "call_identifier",
		"processing_time_ms":            "processing_time_ms",
		"client_name":                   "client_name",
		"client_version":                "client_version",
		"response_status":               "response_status",
		"client_ip":                     "client_ip",
	}
	localAgentAuditLogSortColumns = map[string]string{
		"created_at":     "created_at",
		"occurred_at":    "occurred_at",
		"agent_provider": "agent_provider",
		"status":         "outcome_status",
		"tool_name":      "action_name",
		"tool_kind":      "action_kind",
		"duration_ms":    "duration_ms",
		"client_ip":      "client_ip",
	}
)

// GetMCPAuditLogs retrieves a single, globally ordered page of audit rows from the selected
// sources. An empty SourceTypes selection preserves the MCP-only default. Mixed-source queries use
// each source's effective event time before applying one count, order, limit, and offset window.
// Sensitive payload fields are blanked unless WithRequestAndResponse is true.
func (c *Client) GetMCPAuditLogs(ctx context.Context, opts MCPAuditLogOptions) ([]types.MCPAuditLog, int64, error) {
	sources := auditlog.NormalizeSourceTypes(opts.SourceTypes)
	if err := ValidateAuditLogOptions(opts, sources); err != nil {
		return nil, 0, err
	}

	db := c.db.WithContext(ctx).Model(&types.MCPAuditLog{}).Where("source_type IN ?", sources)
	if opts.Query != "" {
		var err error
		db, err = c.applyAuditLogSearch(ctx, db, opts.Query)
		if err != nil {
			return nil, 0, err
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

	eventTime := auditLogEventTimeExpression(sources)
	if !opts.StartTime.IsZero() {
		db = db.Where(eventTime+" >= ?", opts.StartTime.UTC())
	}
	if !opts.EndTime.IsZero() {
		db = db.Where(eventTime+" < ?", opts.EndTime.UTC())
	}
	if opts.ProcessingTimeMin > 0 {
		db = db.Where("CASE WHEN source_type = ? THEN duration_ms ELSE processing_time_ms END >= ?",
			types2.AuditLogSourceTypeLocalAgentToolCall, opts.ProcessingTimeMin)
	}
	if opts.ProcessingTimeMax > 0 {
		db = db.Where("CASE WHEN source_type = ? THEN duration_ms ELSE processing_time_ms END <= ?",
			types2.AuditLogSourceTypeLocalAgentToolCall, opts.ProcessingTimeMax)
	}

	if hasMCPAuditLogFilters(opts) {
		db = applyMCPAuditLogFilters(db.Where("source_type = ?", types2.AuditLogSourceTypeMCP), opts)
	} else if hasLocalAgentAuditLogFilters(opts) {
		db = applyLocalAgentAuditLogFilters(db.Where("source_type = ?", types2.AuditLogSourceTypeLocalAgentToolCall), opts)
	}

	// When payloads aren't requested, avoid reading/transferring the large sensitive columns that
	// prepareAuditLogPayload would blank anyway.
	if !opts.WithRequestAndResponse {
		db = omitMCPAuditLogSensitiveFields(db)
	}

	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	sortExpression, err := auditLogSortExpression(opts, sources)
	if err != nil {
		return nil, 0, err
	}
	order := "DESC"
	if opts.SortOrder == "asc" {
		order = "ASC"
	}
	db = db.Order(sortExpression + " " + order)
	if sortExpression != eventTime {
		db = db.Order(eventTime + " " + order)
	}
	db = db.Order("id " + order)
	if opts.Limit > 0 {
		db = db.Limit(opts.Limit)
	}
	if opts.Offset > 0 {
		db = db.Offset(opts.Offset)
	}

	var logs []types.MCPAuditLog
	if err := db.Find(&logs).Error; err != nil {
		return nil, 0, err
	}
	for i := range logs {
		if err := c.prepareAuditLogPayload(ctx, &logs[i], opts.WithRequestAndResponse); err != nil {
			return nil, 0, err
		}
	}
	return logs, total, nil
}

// ValidateAuditLogOptions verifies source selection, source-specific filter compatibility, and sort
// parameters. If sources is empty, validation uses opts.SourceTypes and applies the MCP-only default
// when that field is also empty. Validation does not perform any authorization checks; callers must
// authorize the selected sources separately.
func ValidateAuditLogOptions(opts MCPAuditLogOptions, sources []types2.AuditLogSourceType) error {
	if len(sources) == 0 {
		sources = auditlog.NormalizeSourceTypes(opts.SourceTypes)
	}
	for _, source := range sources {
		if source != types2.AuditLogSourceTypeMCP && source != types2.AuditLogSourceTypeLocalAgentToolCall {
			return fmt.Errorf("invalid audit log source type %q", source)
		}
	}
	hasMCP, hasLocal := hasMCPAuditLogFilters(opts), hasLocalAgentAuditLogFilters(opts)
	if hasMCP && hasLocal {
		return errors.New("MCP and local-agent-specific audit log filters cannot be combined")
	}
	// Source-specific filters narrow the query to a single source, so combining them with a
	// multi-source selection would silently drop the other source's rows. Require the caller to
	// scope the selection to the one source those filters apply to.
	if (hasMCP || hasLocal) && len(sources) > 1 {
		return errors.New("source-specific audit log filters require selecting a single audit log source")
	}
	if hasMCP && !slices.Contains(sources, types2.AuditLogSourceTypeMCP) {
		return errors.New("MCP-specific filters require the MCP audit log source")
	}
	if hasLocal && !slices.Contains(sources, types2.AuditLogSourceTypeLocalAgentToolCall) {
		return errors.New("local-agent-specific filters require the local-agent audit log source")
	}
	if opts.SortOrder != "" && opts.SortOrder != "asc" && opts.SortOrder != "desc" {
		return fmt.Errorf("invalid audit log sort direction %q", opts.SortOrder)
	}
	_, err := auditLogSortExpression(opts, sources)
	return err
}

func hasMCPAuditLogFilters(opts MCPAuditLogOptions) bool {
	return len(opts.PowerUserWorkspaceID) > 0 || len(opts.OwnServerMCPIDs) > 0 || len(opts.MCPID) > 0 ||
		len(opts.MCPServerDisplayName) > 0 || len(opts.MCPServerCatalogEntryName) > 0 || len(opts.CallType) > 0 ||
		len(opts.CallIdentifier) > 0 || len(opts.ClientName) > 0 || len(opts.ClientVersion) > 0 || len(opts.ResponseStatus) > 0
}

func hasLocalAgentAuditLogFilters(opts MCPAuditLogOptions) bool {
	return len(opts.AgentProvider) > 0 || len(opts.Status) > 0 || len(opts.ToolName) > 0 || len(opts.ToolKind) > 0 || len(opts.DeviceID) > 0
}

func applyMCPAuditLogFilters(db *gorm.DB, opts MCPAuditLogOptions) *gorm.DB {
	if len(opts.PowerUserWorkspaceID) > 0 || len(opts.OwnServerMCPIDs) > 0 {
		var conditions []string
		var args []any
		if len(opts.PowerUserWorkspaceID) > 0 {
			conditions, args = append(conditions, "power_user_workspace_id IN ?"), append(args, opts.PowerUserWorkspaceID)
		}
		if len(opts.OwnServerMCPIDs) > 0 {
			conditions, args = append(conditions, "mcp_id IN ?"), append(args, opts.OwnServerMCPIDs)
		}
		db = db.Where("("+strings.Join(conditions, " OR ")+")", args...)
	}
	for _, filter := range []struct {
		column string
		values []string
	}{
		{"mcp_id", opts.MCPID}, {"mcp_server_display_name", opts.MCPServerDisplayName},
		{"mcp_server_catalog_entry_name", opts.MCPServerCatalogEntryName}, {"call_type", opts.CallType},
		{"call_identifier", opts.CallIdentifier}, {"client_name", opts.ClientName},
		{"client_version", opts.ClientVersion}, {"response_status", opts.ResponseStatus},
	} {
		if len(filter.values) > 0 {
			db = db.Where(filter.column+" IN ?", filter.values)
		}
	}
	return db
}

func applyLocalAgentAuditLogFilters(db *gorm.DB, opts MCPAuditLogOptions) *gorm.DB {
	for _, filter := range []struct {
		column string
		values []string
	}{
		{"agent_provider", opts.AgentProvider}, {"outcome_status", opts.Status}, {"action_name", opts.ToolName},
		{"action_kind", opts.ToolKind}, {"device_id", opts.DeviceID},
	} {
		if len(filter.values) > 0 {
			db = db.Where(filter.column+" IN ?", filter.values)
		}
	}
	return db
}

func (c *Client) applyAuditLogSearch(ctx context.Context, db *gorm.DB, queryValue string) (*gorm.DB, error) {
	users, err := c.UsersIncludeDeleted(ctx, types.UserQuery{})
	if err != nil {
		return nil, fmt.Errorf("failed to get users: %w", err)
	}
	var userIDs []string
	for _, user := range users {
		if strings.Contains(strings.ToLower(user.DisplayName), strings.ToLower(queryValue)) {
			userIDs = append(userIDs, strconv.FormatUint(uint64(user.ID), 10))
		}
	}
	like := "LIKE"
	if db.Name() == "postgres" {
		like = "ILIKE"
	}
	columns := []string{
		"mcp_id", "mcp_server_display_name", "mcp_server_catalog_entry_name", "client_name", "client_version",
		"client_ip", "call_type", "call_identifier", "error", "session_id", "request_id", "user_agent",
		"agent_provider", "agent_version", "cli_name", "cli_version", "outcome_status", "outcome_reason", "action_name",
		"action_kind", "target_name", "target_parent_name", "tool_use_id", "turn_id", "device_id", "os", "architecture",
		"model", "model_id", "permission_mode", "git_commit",
	}
	parts := []string{"user_id IN ?"}
	args := []any{userIDs}
	for _, column := range columns {
		parts = append(parts, column+" "+like+" ?")
		args = append(args, "%"+queryValue+"%")
	}
	if status, err := strconv.Atoi(queryValue); err == nil {
		parts, args = append(parts, "response_status = ?"), append(args, status)
	}
	return db.Where("("+strings.Join(parts, " OR ")+")", args...), nil
}

func auditLogSortExpression(opts MCPAuditLogOptions, sources []types2.AuditLogSourceType) (string, error) {
	sortBy := opts.SortBy
	if sortBy == "" || sortBy == "timestamp" {
		return auditLogEventTimeExpression(sources), nil
	}
	switch sortBy {
	case "event_type":
		return "CASE source_type WHEN 'mcp' THEN 0 ELSE 1 END", nil
	case "duration":
		return "CASE WHEN source_type = 'local_agent_tool_call' THEN duration_ms ELSE processing_time_ms END", nil
	}
	if len(sources) != 1 {
		return "", fmt.Errorf("sort key %q is not supported for mixed audit logs", sortBy)
	}
	if sources[0] == types2.AuditLogSourceTypeMCP {
		if column, ok := mcpAuditLogSortColumns[sortBy]; ok {
			return column, nil
		}
	} else if column, ok := localAgentAuditLogSortColumns[sortBy]; ok {
		return column, nil
	}
	return "", fmt.Errorf("invalid audit log sort key %q", sortBy)
}

func auditLogEventTimeExpression(sources []types2.AuditLogSourceType) string {
	if len(sources) == 1 {
		if sources[0] == types2.AuditLogSourceTypeLocalAgentToolCall {
			return "occurred_at"
		}
		return "created_at"
	}
	return effectiveAuditLogTimeSQL
}

func (c *Client) prepareAuditLogPayload(ctx context.Context, log *types.MCPAuditLog, includePayload bool) error {
	if log.SourceType == types2.AuditLogSourceTypeLocalAgentToolCall {
		log.MCPFields = nil
		if includePayload {
			if err := c.decryptMCPAuditLog(ctx, log); err != nil {
				return fmt.Errorf("failed to decrypt local-agent audit log: %w", err)
			}
		} else if log.LocalAgentToolCallFields != nil {
			blankLocalAgentSensitiveFields(log.LocalAgentToolCallFields)
		}
		return nil
	}

	log.LocalAgentToolCallFields = nil
	if includePayload {
		if err := c.decryptMCPAuditLog(ctx, log); err != nil {
			return fmt.Errorf("failed to decrypt MCP audit log: %w", err)
		}
		return nil
	}
	mcp := log.MCP()
	mcp.RequestBody, mcp.MutatedRequestBody, mcp.ResponseBody, mcp.OriginalResponseBody = nil, nil, nil, nil
	mcp.RequestHeaders, mcp.ResponseHeaders = nil, nil
	return nil
}

// blankLocalAgentSensitiveFields clears the encrypted/sensitive local-agent fields so metadata can
// be returned to callers without exposing payloads.
func blankLocalAgentSensitiveFields(local *types.LocalAgentToolCallAuditLogFields) {
	local.OutcomeError = ""
	local.Hostname = ""
	local.LocalUsername = ""
	local.ReportedUserEmail = ""
	local.CWD = ""
	local.GitRoot = ""
	local.GitRemotes = nil
	local.GitBranch = ""
	local.TranscriptPath = ""
	local.RequestBody = nil
	local.ResponseBody = nil
	local.RawEvent = nil
}

// omitMCPAuditLogSensitiveFields excludes the large/sensitive payload columns from the SELECT so
// they are never read from or transferred by the DB when payloads are not requested. It mirrors the
// fields that prepareAuditLogPayload blanks; that blanking is retained as a safety net. Columns for
// both source types are listed because a single list query spans both.
func omitMCPAuditLogSensitiveFields(db *gorm.DB) *gorm.DB {
	return db.Omit(
		// MCP fields
		"request_body", "mutated_request_body", "response_body", "original_response_body",
		"request_headers", "response_headers",
		// Local-agent fields
		"local_agent_error", "hostname", "local_username", "reported_user_email", "cwd",
		"git_root", "git_remotes", "git_branch", "transcript_path",
		"local_agent_request_body", "local_agent_response_body", "local_agent_raw_event",
	)
}

// GetMCPAuditLog retrieves a single MCP audit log by ID
func (c *Client) GetMCPAuditLog(ctx context.Context, id uint, withRequestAndResponse bool) (*types.MCPAuditLog, error) {
	var log types.MCPAuditLog

	db := c.db.WithContext(ctx).Model(&types.MCPAuditLog{})

	if err := db.Where("id = ?", id).First(&log).Error; err != nil {
		return nil, err
	}
	if log.SourceType == types2.AuditLogSourceTypeLocalAgentToolCall {
		log.MCPFields = nil
	} else {
		log.LocalAgentToolCallFields = nil
	}

	// Decrypt if requested
	if err := c.decryptMCPAuditLog(ctx, &log); err != nil {
		return nil, fmt.Errorf("failed to decrypt MCP audit log: %w", err)
	}
	if !withRequestAndResponse {
		mcp := log.MCP()
		if mcp != nil {
			// Blank out encrypted fields
			mcp.RequestBody = nil
			mcp.MutatedRequestBody = nil
			mcp.ResponseBody = nil
			mcp.OriginalResponseBody = nil
			// Request and response headers are intentionally kept non-nil, since sensitive values are redacted
		}
		if local := log.LocalAgentToolCallFields; local != nil {
			blankLocalAgentSensitiveFields(local)
		}
	}

	return &log, nil
}

// GetAuditLogFilterOptions returns distinct values for an allowed audit-log filter column under
// the same source, scope, common-filter, and time constraints used by the list query. The caller
// must validate option against its public allowlist before calling this method.
func (c *Client) GetAuditLogFilterOptions(ctx context.Context, option string, opts MCPAuditLogOptions, exclude ...any) ([]string, error) {
	sources := auditlog.NormalizeSourceTypes(opts.SourceTypes)
	if err := ValidateAuditLogOptions(opts, sources); err != nil {
		return nil, err
	}
	if len(sources) > 1 && !hasMCPAuditLogFilters(opts) && !hasLocalAgentAuditLogFilters(opts) &&
		(option == "user_id" || option == "session_id" || option == "client_ip") {
		return c.getMixedAuditLogFilterOptions(ctx, option, opts, exclude...)
	}
	if (len(sources) == 1 && sources[0] == types2.AuditLogSourceTypeLocalAgentToolCall) ||
		(len(sources) > 1 && (hasLocalAgentAuditLogFilters(opts) || isLocalAgentFilterOption(option))) {
		return c.getLocalAgentAuditLogFilterOptions(ctx, option, opts, exclude...)
	}

	db := c.db.WithContext(ctx).Model(&types.MCPAuditLog{}).Where("source_type = ?", types2.AuditLogSourceTypeMCP).Distinct(option)

	// Apply the same filters as GetMCPAuditLogs (excluding sorting, offset)
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
	// Apply scope filtering (union of workspace servers OR own servers)
	if len(opts.PowerUserWorkspaceID) > 0 || len(opts.OwnServerMCPIDs) > 0 {
		var (
			conditions []string
			args       []any
		)

		if len(opts.PowerUserWorkspaceID) > 0 {
			conditions = append(conditions, "power_user_workspace_id IN (?)")
			args = append(args, opts.PowerUserWorkspaceID)
		}
		if len(opts.OwnServerMCPIDs) > 0 {
			conditions = append(conditions, "mcp_id IN (?)")
			args = append(args, opts.OwnServerMCPIDs)
		}

		db = db.Where(strings.Join(conditions, " OR "), args...)
	}
	if !opts.StartTime.IsZero() {
		db = db.Where("created_at >= ?", opts.StartTime.UTC())
	}
	if !opts.EndTime.IsZero() {
		db = db.Where("created_at < ?", opts.EndTime.UTC())
	}

	var result []string
	if len(exclude) != 0 {
		db = db.Where(option+" NOT IN ?", exclude)
	}
	if opts.Limit > 0 {
		// Ensure deterministic subset when using DISTINCT + LIMIT by ordering on the same option
		db = db.Order(option).Limit(opts.Limit)
	}
	return result, db.Select(option).Scan(&result).Error
}

var localAgentFilterOptionColumns = map[string]string{
	"agent_provider": "agent_provider", "status": "outcome_status", "tool_name": "action_name",
	"tool_kind": "action_kind", "device_id": "device_id",
}

func isLocalAgentFilterOption(option string) bool {
	_, ok := localAgentFilterOptionColumns[option]
	return ok
}

func (c *Client) getMixedAuditLogFilterOptions(ctx context.Context, option string, opts MCPAuditLogOptions, exclude ...any) ([]string, error) {
	db := c.db.WithContext(ctx).Model(&types.MCPAuditLog{}).
		Where("source_type IN ?", auditlog.NormalizeSourceTypes(opts.SourceTypes)).Distinct(option)
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
		db = db.Where("((source_type = ? AND created_at >= ?) OR (source_type = ? AND occurred_at >= ?))",
			types2.AuditLogSourceTypeMCP, opts.StartTime.UTC(), types2.AuditLogSourceTypeLocalAgentToolCall, opts.StartTime.UTC())
	}
	if !opts.EndTime.IsZero() {
		db = db.Where("((source_type = ? AND created_at < ?) OR (source_type = ? AND occurred_at < ?))",
			types2.AuditLogSourceTypeMCP, opts.EndTime.UTC(), types2.AuditLogSourceTypeLocalAgentToolCall, opts.EndTime.UTC())
	}
	if len(exclude) > 0 {
		db = db.Where(option+" NOT IN ?", exclude)
	}
	if opts.Limit > 0 {
		db = db.Order(option).Limit(opts.Limit)
	}
	var result []string
	return result, db.Select(option).Scan(&result).Error
}

// getLocalAgentAuditLogFilterOptions returns the distinct values for a local-agent filter column.
func (c *Client) getLocalAgentAuditLogFilterOptions(ctx context.Context, option string, opts MCPAuditLogOptions, exclude ...any) ([]string, error) {
	column := localAgentFilterOptionColumns[option]
	if column == "" {
		column = option
	}
	db := c.db.WithContext(ctx).Model(&types.MCPAuditLog{}).Where("source_type = ?", types2.AuditLogSourceTypeLocalAgentToolCall).Distinct(column)

	if len(opts.UserID) > 0 {
		db = db.Where("user_id IN ?", opts.UserID)
	}
	if len(opts.ClientIP) > 0 {
		db = db.Where("client_ip IN ?", opts.ClientIP)
	}
	if len(opts.SessionID) > 0 {
		db = db.Where("session_id IN ?", opts.SessionID)
	}
	if len(opts.AgentProvider) > 0 {
		db = db.Where("agent_provider IN ?", opts.AgentProvider)
	}
	if len(opts.Status) > 0 {
		db = db.Where("outcome_status IN ?", opts.Status)
	}
	if len(opts.ToolName) > 0 {
		db = db.Where("action_name IN ?", opts.ToolName)
	}
	if len(opts.ToolKind) > 0 {
		db = db.Where("action_kind IN ?", opts.ToolKind)
	}
	if len(opts.DeviceID) > 0 {
		db = db.Where("device_id IN ?", opts.DeviceID)
	}
	if !opts.StartTime.IsZero() {
		db = db.Where("occurred_at >= ?", opts.StartTime.UTC())
	}
	if !opts.EndTime.IsZero() {
		db = db.Where("occurred_at < ?", opts.EndTime.UTC())
	}

	var result []string
	if len(exclude) != 0 {
		db = db.Where(column+" NOT IN ?", exclude)
	}
	if opts.Limit > 0 {
		db = db.Order(column).Limit(opts.Limit)
	}
	return result, db.Select(column).Scan(&result).Error
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
		base = base.Model(&types.MCPAuditLog{}).Where("source_type = ?", types2.AuditLogSourceTypeMCP).Session(&gorm.Session{})
		tx := base.Where("created_at >= ? AND created_at < ?", opts.StartTime, opts.EndTime)

		if opts.MCPID != "" {
			tx = tx.Where("mcp_id = ?", opts.MCPID)
		}
		// Apply scope filtering (union of workspace servers OR own servers)
		if len(opts.PowerUserWorkspaceID) > 0 || len(opts.OwnServerMCPIDs) > 0 {
			var conditions []string
			var args []any

			if len(opts.PowerUserWorkspaceID) > 0 {
				conditions = append(conditions, "power_user_workspace_id IN (?)")
				args = append(args, opts.PowerUserWorkspaceID)
			}
			if len(opts.OwnServerMCPIDs) > 0 {
				conditions = append(conditions, "mcp_id IN (?)")
				args = append(args, opts.OwnServerMCPIDs)
			}

			tx = tx.Where(strings.Join(conditions, " OR "), args...)
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

// MCPAuditLogOptions configures audit-row queries across MCP and local-agent sources. Common
// filters apply to every selected source. MCP-only and local-agent-only filters are mutually
// exclusive; ValidateAuditLogOptions enforces those rules before a query runs.
type MCPAuditLogOptions struct {
	// WithRequestAndResponse requests decrypted payload and sensitive environment fields. It must
	// only be set after the caller has passed the corresponding authorization check.
	WithRequestAndResponse bool
	// SourceTypes selects the persisted source kinds. Empty preserves the MCP-only default.
	SourceTypes []types2.AuditLogSourceType
	// PowerUserWorkspaceID and OwnServerMCPIDs define the authorized MCP-server scope. When both
	// are present, a row may match either scope.
	PowerUserWorkspaceID []string
	OwnServerMCPIDs      []string
	// UserID, SessionID, and ClientIP are common filters shared by both sources.
	UserID    []string
	SessionID []string
	ClientIP  []string
	// ProcessingTimeMin and ProcessingTimeMax filter the normalized duration field, using MCP
	// processing time or local-agent duration as appropriate.
	ProcessingTimeMin int64
	ProcessingTimeMax int64

	// MCP-only filters.
	MCPID                     []string
	MCPServerDisplayName      []string
	MCPServerCatalogEntryName []string
	CallType                  []string
	CallIdentifier            []string
	ClientName                []string
	ClientVersion             []string
	ResponseStatus            []string

	// Local-agent tool-call filters.
	AgentProvider []string
	Status        []string
	ToolName      []string
	ToolKind      []string
	DeviceID      []string

	// Query searches the non-sensitive text columns of every selected source and matching user
	// display names.
	Query     string
	StartTime time.Time
	EndTime   time.Time
	Limit     int
	Offset    int
	// SortBy accepts timestamp, event_type, outcome, and duration for mixed queries. A single-source
	// query may additionally use that source's allowlisted storage columns.
	SortBy string
	// SortOrder accepts asc or desc. Empty defaults to descending.
	SortOrder string
}

// MCPUsageStatsOptions represents options for querying MCP usage statistics
type MCPUsageStatsOptions struct {
	MCPID                      string
	PowerUserWorkspaceID       []string // Workspace filtering support (same as audit logs)
	OwnServerMCPIDs            []string // MCPIDs for user's own servers (union with PowerUserWorkspaceID)
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
		errs []error

		dataCtx = mcpAuditLogDataCtx(log)
	)

	if mcp := log.MCP(); mcp != nil {
		errs = append(errs,
			encryptRawMessageField(ctx, transformer, dataCtx, &mcp.RequestBody),
			encryptRawMessageField(ctx, transformer, dataCtx, &mcp.MutatedRequestBody),
			encryptRawMessageField(ctx, transformer, dataCtx, &mcp.ResponseBody),
			encryptRawMessageField(ctx, transformer, dataCtx, &mcp.OriginalResponseBody),
			encryptRawMessageField(ctx, transformer, dataCtx, &mcp.RequestHeaders),
			encryptRawMessageField(ctx, transformer, dataCtx, &mcp.ResponseHeaders),
		)
	}

	if local := log.LocalAgentToolCallFields; local != nil {
		errs = append(errs,
			encryptStringField(ctx, transformer, dataCtx, &local.OutcomeError),
			encryptStringField(ctx, transformer, dataCtx, &local.Hostname),
			encryptStringField(ctx, transformer, dataCtx, &local.LocalUsername),
			encryptStringField(ctx, transformer, dataCtx, &local.ReportedUserEmail),
			encryptStringField(ctx, transformer, dataCtx, &local.CWD),
			encryptStringField(ctx, transformer, dataCtx, &local.GitRoot),
			encryptStringSliceField(ctx, transformer, dataCtx, []string(local.GitRemotes)),
			encryptStringField(ctx, transformer, dataCtx, &local.GitBranch),
			encryptStringField(ctx, transformer, dataCtx, &local.TranscriptPath),
			encryptRawMessageField(ctx, transformer, dataCtx, &local.RequestBody),
			encryptRawMessageField(ctx, transformer, dataCtx, &local.ResponseBody),
			encryptRawMessageField(ctx, transformer, dataCtx, &local.RawEvent),
		)
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
		errs    []error
		dataCtx = mcpAuditLogDataCtx(log)
	)

	if mcp := log.MCP(); mcp != nil {
		errs = append(errs,
			decryptRawMessageField(ctx, transformer, dataCtx, &mcp.RequestBody),
			decryptRawMessageField(ctx, transformer, dataCtx, &mcp.MutatedRequestBody),
			decryptRawMessageField(ctx, transformer, dataCtx, &mcp.ResponseBody),
			decryptRawMessageField(ctx, transformer, dataCtx, &mcp.OriginalResponseBody),
			decryptRawMessageField(ctx, transformer, dataCtx, &mcp.RequestHeaders),
			decryptRawMessageField(ctx, transformer, dataCtx, &mcp.ResponseHeaders),
		)
	}

	if local := log.LocalAgentToolCallFields; local != nil {
		errs = append(errs,
			decryptStringField(ctx, transformer, dataCtx, &local.OutcomeError),
			decryptStringField(ctx, transformer, dataCtx, &local.Hostname),
			decryptStringField(ctx, transformer, dataCtx, &local.LocalUsername),
			decryptStringField(ctx, transformer, dataCtx, &local.ReportedUserEmail),
			decryptStringField(ctx, transformer, dataCtx, &local.CWD),
			decryptStringField(ctx, transformer, dataCtx, &local.GitRoot),
			decryptStringSliceField(ctx, transformer, dataCtx, []string(local.GitRemotes)),
			decryptStringField(ctx, transformer, dataCtx, &local.GitBranch),
			decryptStringField(ctx, transformer, dataCtx, &local.TranscriptPath),
			decryptRawMessageField(ctx, transformer, dataCtx, &local.RequestBody),
			decryptRawMessageField(ctx, transformer, dataCtx, &local.ResponseBody),
			decryptRawMessageField(ctx, transformer, dataCtx, &local.RawEvent),
		)
	}

	return errors.Join(errs...)
}

func encryptRawMessageField(ctx context.Context, transformer value.Transformer, dataCtx value.Context, field *json.RawMessage) error {
	if len(*field) == 0 {
		return nil
	}
	b, err := transformer.TransformToStorage(ctx, *field, dataCtx)
	if err != nil {
		return err
	}
	*field = base64.StdEncoding.AppendEncode(nil, b)
	return nil
}

func decryptRawMessageField(ctx context.Context, transformer value.Transformer, dataCtx value.Context, field *json.RawMessage) error {
	if len(*field) == 0 {
		return nil
	}
	decoded := make([]byte, base64.StdEncoding.DecodedLen(len(*field)))
	n, err := base64.StdEncoding.Decode(decoded, *field)
	if err != nil {
		return err
	}
	out, _, err := transformer.TransformFromStorage(ctx, decoded[:n], dataCtx)
	if err != nil {
		return err
	}
	*field = out
	return nil
}

func encryptStringField(ctx context.Context, transformer value.Transformer, dataCtx value.Context, field *string) error {
	if *field == "" {
		return nil
	}
	b, err := transformer.TransformToStorage(ctx, []byte(*field), dataCtx)
	if err != nil {
		return err
	}
	*field = base64.StdEncoding.EncodeToString(b)
	return nil
}

func decryptStringField(ctx context.Context, transformer value.Transformer, dataCtx value.Context, field *string) error {
	if *field == "" {
		return nil
	}
	decoded := make([]byte, base64.StdEncoding.DecodedLen(len(*field)))
	n, err := base64.StdEncoding.Decode(decoded, []byte(*field))
	if err != nil {
		return err
	}
	out, _, err := transformer.TransformFromStorage(ctx, decoded[:n], dataCtx)
	if err != nil {
		return err
	}
	*field = string(out)
	return nil
}

func encryptStringSliceField(ctx context.Context, transformer value.Transformer, dataCtx value.Context, field []string) error {
	for i := range field {
		if err := encryptStringField(ctx, transformer, dataCtx, &field[i]); err != nil {
			return err
		}
	}
	return nil
}

func decryptStringSliceField(ctx context.Context, transformer value.Transformer, dataCtx value.Context, field []string) error {
	for i := range field {
		if err := decryptStringField(ctx, transformer, dataCtx, &field[i]); err != nil {
			return err
		}
	}
	return nil
}

func mcpAuditLogDataCtx(log *types.MCPAuditLog) value.Context {
	mcp := log.MCP()
	if mcp == nil {
		return value.DefaultContext(fmt.Sprintf("%s/%s/%s", mcpAuditLogGroupResource.String(), log.SourceType, log.UserID))
	}
	return value.DefaultContext(fmt.Sprintf("%s/%s/%s", mcpAuditLogGroupResource.String(), mcp.MCPID, log.UserID))
}
