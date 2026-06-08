package client

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/storage/value"
)

var localAgentAuditLogGroupResource = schema.GroupResource{
	Group:    "obot.obot.ai",
	Resource: "localagentauditlogs",
}

func (c *Client) CreateLocalAgentAuditLogs(ctx context.Context, logs []types.LocalAgentAuditLog) ([]types.LocalAgentAuditLog, int, error) {
	if len(logs) == 0 {
		return nil, 0, nil
	}

	toInsert := make([]types.LocalAgentAuditLog, 0, len(logs))
	for _, auditLog := range logs {
		if auditLog.EventID == "" {
			return nil, 0, fmt.Errorf("event ID is required")
		}
		if auditLog.CreatedAt.IsZero() {
			auditLog.CreatedAt = time.Now().UTC()
		} else {
			auditLog.CreatedAt = auditLog.CreatedAt.UTC()
		}
		if err := c.encryptLocalAgentAuditLog(ctx, &auditLog); err != nil {
			return nil, 0, fmt.Errorf("failed to encrypt local agent audit log: %w", err)
		}
		toInsert = append(toInsert, auditLog)
	}

	result := c.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "event_id"}},
		DoNothing: true,
	}).Create(&toInsert)
	if result.Error != nil {
		return nil, 0, result.Error
	}

	inserted := int(result.RowsAffected)
	if inserted == len(toInsert) {
		return toInsert, inserted, nil
	}

	eventIDs := make([]string, 0, len(toInsert))
	for _, auditLog := range toInsert {
		eventIDs = append(eventIDs, auditLog.EventID)
	}

	var stored []types.LocalAgentAuditLog
	if err := c.db.WithContext(ctx).Where("event_id IN (?)", eventIDs).Find(&stored).Error; err != nil {
		return nil, 0, err
	}
	return stored, inserted, nil
}

func (c *Client) GetLocalAgentAuditLogs(ctx context.Context, opts LocalAgentAuditLogOptions) ([]types.LocalAgentAuditLog, int64, error) {
	var logs []types.LocalAgentAuditLog

	db := c.db.WithContext(ctx).Model(&types.LocalAgentAuditLog{})
	db, err := c.applyLocalAgentAuditLogFilters(ctx, db, opts)
	if err != nil {
		return nil, 0, err
	}

	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if opts.Limit > 0 {
		db = db.Limit(opts.Limit)
	}
	if opts.Offset > 0 {
		db = db.Offset(opts.Offset)
	}

	validSortFields := map[string]bool{
		"created_at":         true,
		"user_id":            true,
		"client_name":        true,
		"client_version":     true,
		"client_ip":          true,
		"tool_name":          true,
		"tool_type":          true,
		"event_name":         true,
		"success":            true,
		"status":             true,
		"exit_code":          true,
		"duration_ms":        true,
		"session_id":         true,
		"conversation_id":    true,
		"request_id":         true,
		"workspace_hash":     true,
		"workspace_basename": true,
		"payload_truncated":  true,
	}
	if opts.SortBy != "" && validSortFields[opts.SortBy] {
		sortOrder := "DESC"
		if opts.SortOrder == "asc" {
			sortOrder = "ASC"
		}
		db = db.Order(opts.SortBy + " " + sortOrder)
	} else {
		db = db.Order("created_at DESC")
	}

	if err := db.Find(&logs).Error; err != nil {
		return nil, 0, err
	}

	for i := range logs {
		if opts.WithPayloads {
			if err := c.decryptLocalAgentAuditLog(ctx, &logs[i]); err != nil {
				return nil, 0, fmt.Errorf("failed to decrypt local agent audit log: %w", err)
			}
			continue
		}
		blankLocalAgentAuditPayloads(&logs[i])
	}

	return logs, total, nil
}

func (c *Client) GetLocalAgentAuditLog(ctx context.Context, id uint, withPayloads bool) (*types.LocalAgentAuditLog, error) {
	var log types.LocalAgentAuditLog

	if err := c.db.WithContext(ctx).Model(&types.LocalAgentAuditLog{}).Where("id = ?", id).First(&log).Error; err != nil {
		return nil, err
	}

	if withPayloads {
		if err := c.decryptLocalAgentAuditLog(ctx, &log); err != nil {
			return nil, fmt.Errorf("failed to decrypt local agent audit log: %w", err)
		}
	} else {
		blankLocalAgentAuditPayloads(&log)
	}

	return &log, nil
}

func (c *Client) GetLocalAgentAuditLogFilterOptions(ctx context.Context, option string, opts LocalAgentAuditLogOptions, exclude ...any) ([]string, error) {
	db := c.db.WithContext(ctx).Model(&types.LocalAgentAuditLog{}).Distinct(option)

	var err error
	db, err = c.applyLocalAgentAuditLogFilters(ctx, db, opts)
	if err != nil {
		return nil, err
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

func (c *Client) applyLocalAgentAuditLogFilters(ctx context.Context, db *gorm.DB, opts LocalAgentAuditLogOptions) (*gorm.DB, error) {
	if opts.Query != "" {
		searchTerm := "%" + opts.Query + "%"
		like := "LIKE"
		if db.Name() == "postgres" {
			like = "ILIKE"
		}

		users, err := c.UsersIncludeDeleted(ctx, types.UserQuery{})
		if err != nil {
			return nil, fmt.Errorf("failed to get users: %w", err)
		}

		var userIDs []string
		userQuery := strings.ToLower(opts.Query)
		for _, u := range users {
			if strings.Contains(strings.ToLower(u.DisplayName), userQuery) {
				userIDs = append(userIDs, strconv.FormatUint(uint64(u.ID), 10))
			}
		}

		// Each %[1]s in the query will be replaced with LIKE or ILIKE.
		query := `user_id in (?) OR event_id %[1]s ? OR client_name %[1]s ? OR client_version %[1]s ? OR
client_ip %[1]s ? OR tool_name %[1]s ? OR tool_type %[1]s ? OR event_name %[1]s ? OR status %[1]s ? OR
session_id %[1]s ? OR conversation_id %[1]s ? OR request_id %[1]s ? OR workspace_hash %[1]s ? OR
workspace_basename %[1]s ? OR error %[1]s ?`
		args := append([]any{userIDs}, slicesOfAny(searchTerm, strings.Count(query, "%[1]s ?"))...)

		if exitCode, err := strconv.Atoi(opts.Query); err == nil {
			query += " OR exit_code = ?"
			args = append(args, exitCode)
		}
		if durationMs, err := strconv.ParseInt(opts.Query, 10, 64); err == nil {
			query += " OR duration_ms = ?"
			args = append(args, durationMs)
		}
		if success, ok := parseBoolQuery(opts.Query); ok {
			query += " OR success = ?"
			args = append(args, success)
		}

		db = db.Where(fmt.Sprintf(query, like), args...)
	}

	if len(opts.UserID) > 0 {
		db = db.Where("user_id IN (?)", opts.UserID)
	}
	if len(opts.EventID) > 0 {
		db = db.Where("event_id IN (?)", opts.EventID)
	}
	if len(opts.ClientName) > 0 {
		db = db.Where("client_name IN (?)", opts.ClientName)
	}
	if len(opts.ClientVersion) > 0 {
		db = db.Where("client_version IN (?)", opts.ClientVersion)
	}
	if len(opts.ClientIP) > 0 {
		db = db.Where("client_ip IN (?)", opts.ClientIP)
	}
	if len(opts.ToolName) > 0 {
		db = db.Where("tool_name IN (?)", opts.ToolName)
	}
	if len(opts.ToolType) > 0 {
		db = db.Where("tool_type IN (?)", opts.ToolType)
	}
	if len(opts.EventName) > 0 {
		db = db.Where("event_name IN (?)", opts.EventName)
	}
	if len(opts.Success) > 0 {
		db = db.Where("success IN (?)", opts.Success)
	}
	if len(opts.Status) > 0 {
		db = db.Where("status IN (?)", opts.Status)
	}
	if len(opts.ExitCode) > 0 {
		db = db.Where("exit_code IN (?)", opts.ExitCode)
	}
	if len(opts.SessionID) > 0 {
		db = db.Where("session_id IN (?)", opts.SessionID)
	}
	if len(opts.ConversationID) > 0 {
		db = db.Where("conversation_id IN (?)", opts.ConversationID)
	}
	if len(opts.RequestID) > 0 {
		db = db.Where("request_id IN (?)", opts.RequestID)
	}
	if len(opts.WorkspaceHash) > 0 {
		db = db.Where("workspace_hash IN (?)", opts.WorkspaceHash)
	}
	if len(opts.WorkspaceBasename) > 0 {
		db = db.Where("workspace_basename IN (?)", opts.WorkspaceBasename)
	}
	if len(opts.PayloadTruncated) > 0 {
		db = db.Where("payload_truncated IN (?)", opts.PayloadTruncated)
	}
	if opts.DurationMsMin > 0 {
		db = db.Where("duration_ms >= ?", opts.DurationMsMin)
	}
	if opts.DurationMsMax > 0 {
		db = db.Where("duration_ms <= ?", opts.DurationMsMax)
	}
	if !opts.StartTime.IsZero() {
		db = db.Where("created_at >= ?", opts.StartTime.UTC())
	}
	if !opts.EndTime.IsZero() {
		db = db.Where("created_at < ?", opts.EndTime.UTC())
	}

	return db, nil
}

func (c *Client) encryptLocalAgentAuditLog(ctx context.Context, log *types.LocalAgentAuditLog) error {
	if c.encryptionConfig == nil {
		return nil
	}

	transformer := c.encryptionConfig.Transformers[localAgentAuditLogGroupResource]
	if transformer == nil {
		return nil
	}

	var errs []error
	dataCtx := localAgentAuditLogDataCtx(log)

	encrypt := func(field *json.RawMessage) {
		if len(*field) == 0 {
			return
		}
		b, err := transformer.TransformToStorage(ctx, *field, dataCtx)
		if err != nil {
			errs = append(errs, err)
			return
		}
		*field = json.RawMessage(base64.StdEncoding.EncodeToString(b))
	}

	encrypt(&log.RawClientHookEvent)
	encrypt(&log.RawToolInput)
	encrypt(&log.RawToolOutput)
	encrypt(&log.RawError)

	log.Encrypted = true
	return errors.Join(errs...)
}

func (c *Client) decryptLocalAgentAuditLog(ctx context.Context, log *types.LocalAgentAuditLog) error {
	if !log.Encrypted || c.encryptionConfig == nil {
		return nil
	}

	transformer := c.encryptionConfig.Transformers[localAgentAuditLogGroupResource]
	if transformer == nil {
		return nil
	}

	var errs []error
	dataCtx := localAgentAuditLogDataCtx(log)

	decrypt := func(field *json.RawMessage) {
		if len(*field) == 0 {
			return
		}
		decoded := make([]byte, base64.StdEncoding.DecodedLen(len(*field)))
		n, err := base64.StdEncoding.Decode(decoded, *field)
		if err != nil {
			errs = append(errs, err)
			return
		}
		out, _, err := transformer.TransformFromStorage(ctx, decoded[:n], dataCtx)
		if err != nil {
			errs = append(errs, err)
			return
		}
		*field = json.RawMessage(out)
	}

	decrypt(&log.RawClientHookEvent)
	decrypt(&log.RawToolInput)
	decrypt(&log.RawToolOutput)
	decrypt(&log.RawError)

	return errors.Join(errs...)
}

func blankLocalAgentAuditPayloads(log *types.LocalAgentAuditLog) {
	log.RawClientHookEvent = nil
	log.RawToolInput = nil
	log.RawToolOutput = nil
	log.RawError = nil
}

func slicesOfAny(value any, count int) []any {
	result := make([]any, count)
	for i := range result {
		result[i] = value
	}
	return result
}

func parseBoolQuery(value string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "true", "success", "succeeded":
		return true, true
	case "false", "failure", "failed":
		return false, true
	default:
		return false, false
	}
}

type LocalAgentAuditLogOptions struct {
	WithPayloads      bool
	UserID            []string
	EventID           []string
	ClientName        []string
	ClientVersion     []string
	ClientIP          []string
	ToolName          []string
	ToolType          []string
	EventName         []string
	Success           []bool
	Status            []string
	ExitCode          []int
	SessionID         []string
	ConversationID    []string
	RequestID         []string
	WorkspaceHash     []string
	WorkspaceBasename []string
	PayloadTruncated  []bool
	DurationMsMin     int64
	DurationMsMax     int64
	Query             string
	StartTime         time.Time
	EndTime           time.Time
	Limit             int
	Offset            int
	SortBy            string
	SortOrder         string
}

func localAgentAuditLogDataCtx(log *types.LocalAgentAuditLog) value.Context {
	return value.DefaultContext(fmt.Sprintf("%s/%s/%s", localAgentAuditLogGroupResource.String(), log.EventID, log.UserID))
}
