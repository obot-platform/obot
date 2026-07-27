package client

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	gatewayllmaudit "github.com/obot-platform/obot/pkg/gateway/llmaudit"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"gorm.io/gorm"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/storage/value"
)

var llmAuditLogGroupResource = schema.GroupResource{
	Group:    "obot.obot.ai",
	Resource: "llmauditlogs",
}

const (
	defaultLLMAuditLogBatchSize  = 100
	defaultLLMAuditLogBufferSize = 3 * defaultLLMAuditLogBatchSize
)

type llmAuditEntry struct {
	log            types.LLMAuditLog
	responseStream []byte
}

func (c *Client) LLMAuditLogEnabled() bool {
	return c != nil && c.llmAuditEnabled
}

func (c *Client) LogLLMAuditEntry(auditLog types.LLMAuditLog, responseStream []byte) {
	if !c.LLMAuditLogEnabled() {
		return
	}
	if c.llmAuditEntries == nil {
		log.Warnf("dropping LLM audit log: writer is not configured")
		return
	}

	// Never let audit logging block an LLM request. A full channel means the
	// writer is behind for long enough that keeping request latency matters more.
	select {
	case c.llmAuditEntries <- llmAuditEntry{log: auditLog, responseStream: responseStream}:
	default:
		log.Warnf("dropping LLM audit log: buffer is full")
	}
}

func (c *Client) InsertLLMAuditLog(ctx context.Context, auditLog *types.LLMAuditLog) error {
	if auditLog == nil {
		return nil
	}
	return c.insertLLMAuditLogs(ctx, []types.LLMAuditLog{*auditLog})
}

func (c *Client) GetLLMAuditLogs(ctx context.Context, opts LLMAuditLogOptions) ([]types.LLMAuditLog, int64, error) {
	var logs []types.LLMAuditLog

	db := c.db.WithContext(ctx).Model(&types.LLMAuditLog{})
	db = applyLLMAuditLogOptions(db, opts)
	if !opts.WithSensitiveFields {
		db = omitLLMAuditLogSensitiveFields(db)
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

	sortBy := opts.SortBy
	validSortFields := map[string]struct{}{
		"created_at":        {},
		"user_id":           {},
		"model_provider":    {},
		"target_model":      {},
		"request_path":      {},
		"response_status":   {},
		"outcome":           {},
		"user_agent":        {},
		"client_session_id": {},
	}
	if _, ok := validSortFields[sortBy]; !ok {
		sortBy = "created_at"
	}
	sortOrder := "DESC"
	if opts.SortOrder == "asc" {
		sortOrder = "ASC"
	}
	db = db.Order(sortBy + " " + sortOrder)
	if err := db.Find(&logs).Error; err != nil {
		return nil, 0, err
	}
	for i := range logs {
		if err := c.prepareLLMAuditLog(ctx, &logs[i], opts.WithSensitiveFields); err != nil {
			return nil, 0, err
		}
	}

	return logs, total, nil
}

func (c *Client) GetLLMAuditLog(ctx context.Context, id string, withSensitiveFields bool) (*types.LLMAuditLog, error) {
	var log types.LLMAuditLog
	db := c.db.WithContext(ctx)
	if !withSensitiveFields {
		db = omitLLMAuditLogSensitiveFields(db)
	}
	if err := db.First(&log, "id = ?", id).Error; err != nil {
		return nil, err
	}
	if err := c.prepareLLMAuditLog(ctx, &log, withSensitiveFields); err != nil {
		return nil, err
	}
	return &log, nil
}

func (c *Client) GetLLMAuditLogFilterOptions(ctx context.Context, option string, opts LLMAuditLogOptions, exclude ...any) ([]string, error) {
	db := c.db.WithContext(ctx).Model(&types.LLMAuditLog{}).Distinct(option)
	db = applyLLMAuditLogOptions(db, opts)

	if len(exclude) > 0 {
		db = db.Where(option+" NOT IN ?", exclude)
	}
	if opts.Limit > 0 {
		db = db.Order(option).Limit(opts.Limit)
	}

	if option == "message_policy_triggered" {
		var values []bool
		if err := db.Select(option).Scan(&values).Error; err != nil {
			return nil, err
		}

		result := make([]string, 0, len(values))
		for _, value := range values {
			result = append(result, strconv.FormatBool(value))
		}
		return result, nil
	}

	var result []string
	return result, db.Select(option).Scan(&result).Error
}

func (c *Client) prepareLLMAuditLog(ctx context.Context, log *types.LLMAuditLog, withSensitiveFields bool) error {
	if withSensitiveFields {
		if err := c.decryptLLMAuditLog(ctx, log); err != nil {
			return fmt.Errorf("failed to decrypt LLM audit log: %w", err)
		}
		return nil
	}
	log.RequestHeaders = nil
	log.RequestBody = nil
	log.PolicyModifiedRequestBody = nil
	log.ResponseHeaders = nil
	log.ResponseBody = nil
	return nil
}

func omitLLMAuditLogSensitiveFields(db *gorm.DB) *gorm.DB {
	return db.Omit("request_headers", "request_body", "policy_modified_request_body", "response_headers", "response_body")
}

func (c *Client) runLLMAuditPersistenceLoop(ctx context.Context, batchSize int, flushInterval time.Duration) {
	if c.llmAuditEntries == nil {
		return
	}
	if batchSize <= 0 {
		batchSize = defaultLLMAuditLogBatchSize
	}
	if flushInterval <= 0 {
		flushInterval = time.Second
	}

	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()

	batch := make([]llmAuditEntry, 0, batchSize)
	for {
		select {
		case <-ctx.Done():
			for {
				batch = c.drainQueuedLLMAuditEntries(batch, batchSize)
				if len(batch) < batchSize {
					c.flushLLMAuditBatch(batch)
					return
				}
				batch = c.flushLLMAuditBatch(batch)
			}
		case entry := <-c.llmAuditEntries:
			batch = append(batch, entry)
			batch = c.drainQueuedLLMAuditEntries(batch, batchSize)
			if len(batch) >= batchSize {
				batch = c.flushLLMAuditBatch(batch)
			}
		case <-ticker.C:
			batch = c.flushLLMAuditBatch(batch)
		}
	}
}

func applyLLMAuditLogOptions(db *gorm.DB, opts LLMAuditLogOptions) *gorm.DB {
	if opts.HideModelsRequests {
		db = db.Where("request_path NOT LIKE ? AND request_path NOT LIKE ?", "%/models", "%/models/")
	}
	if opts.Query != "" {
		searchTerm := "%" + opts.Query + "%"
		like := "LIKE"
		if db.Name() == "postgres" {
			like = "ILIKE"
		}
		query := `user_id %[1]s ? OR model_provider %[1]s ? OR model_id %[1]s ? OR target_model %[1]s ? OR request_path %[1]s ? OR response_id %[1]s ? OR outcome %[1]s ? OR error %[1]s ? OR request_id %[1]s ? OR user_agent %[1]s ? OR client_session_id %[1]s ? OR client_ip %[1]s ?`
		args := make([]any, strings.Count(query, "%[1]s ?"))
		for i := range args {
			args[i] = searchTerm
		}
		if responseStatus, err := strconv.Atoi(opts.Query); err == nil {
			query += " OR response_status = ?"
			args = append(args, responseStatus)
		}
		db = db.Where(fmt.Sprintf(query, like), args...)
	}
	if len(opts.UserID) > 0 {
		db = db.Where("user_id IN (?)", opts.UserID)
	}
	if len(opts.ModelProvider) > 0 {
		db = db.Where("model_provider IN (?)", opts.ModelProvider)
	}
	if len(opts.TargetModel) > 0 {
		db = db.Where("target_model IN (?)", opts.TargetModel)
	}
	if len(opts.RequestPath) > 0 {
		db = db.Where("request_path IN (?)", opts.RequestPath)
	}
	if len(opts.ResponseStatus) > 0 {
		db = db.Where("response_status IN (?)", opts.ResponseStatus)
	}
	if len(opts.Outcome) > 0 {
		db = db.Where("outcome IN (?)", opts.Outcome)
	}
	if len(opts.UserAgent) > 0 {
		db = db.Where("user_agent IN (?)", opts.UserAgent)
	}
	if len(opts.ClientSessionID) > 0 {
		db = db.Where("client_session_id IN (?)", opts.ClientSessionID)
	}
	if len(opts.MessagePolicyTriggered) > 0 {
		db = db.Where("message_policy_triggered IN (?)", opts.MessagePolicyTriggered)
	}
	if !opts.StartTime.IsZero() {
		db = db.Where("created_at >= ?", opts.StartTime.UTC())
	}
	if !opts.EndTime.IsZero() {
		db = db.Where("created_at < ?", opts.EndTime.UTC())
	}
	return db
}

func (c *Client) drainQueuedLLMAuditEntries(batch []llmAuditEntry, batchSize int) []llmAuditEntry {
	for len(batch) < batchSize {
		select {
		case entry := <-c.llmAuditEntries:
			batch = append(batch, entry)
		default:
			return batch
		}
	}
	return batch
}

func (c *Client) flushLLMAuditBatch(batch []llmAuditEntry) []llmAuditEntry {
	if len(batch) == 0 {
		return batch
	}
	if err := c.persistLLMAuditLogs(batch); err != nil {
		log.Errorf("Failed to persist LLM audit logs: %v", err)
	}
	return batch[:0]
}

func (c *Client) persistQueuedLLMAuditLogs() error {
	if c.llmAuditEntries == nil {
		return nil
	}

	batchSize := c.llmAuditBatchSize
	if batchSize <= 0 {
		batchSize = defaultLLMAuditLogBatchSize
	}
	batch := make([]llmAuditEntry, 0, batchSize)
	for {
		select {
		case entry := <-c.llmAuditEntries:
			batch = append(batch, entry)
			if len(batch) >= batchSize {
				if err := c.persistLLMAuditLogs(batch); err != nil {
					return err
				}
				batch = batch[:0]
			}
		default:
			return c.persistLLMAuditLogs(batch)
		}
	}
}

func (c *Client) persistLLMAuditLogs(entries []llmAuditEntry) error {
	if len(entries) == 0 {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	logs := make([]types.LLMAuditLog, len(entries))
	for i, entry := range entries {
		logs[i] = entry.log
		logs[i].CreatedAt = logs[i].CreatedAt.UTC()
		aggregateLLMAuditResponse(&logs[i], entry.responseStream)
	}
	return c.insertLLMAuditLogs(ctx, logs)
}

func aggregateLLMAuditResponse(log *types.LLMAuditLog, responseStream []byte) {
	if log == nil || len(responseStream) == 0 {
		return
	}
	accumulator := gatewayllmaudit.NewResponseAccumulator(log.ModelProvider, log.RequestPath)
	accumulator.Write(responseStream)
	log.ResponseBody = accumulator.JSON()
	if log.ResponseID == "" {
		log.ResponseID = accumulator.ResponseID()
	}
}

func (c *Client) insertLLMAuditLogs(ctx context.Context, logs []types.LLMAuditLog) error {
	if len(logs) == 0 {
		return nil
	}
	if c.encryptionConfig != nil && c.encryptionConfig.Transformers[llmAuditLogGroupResource] != nil {
		for i := range logs {
			if err := c.encryptLLMAuditLog(ctx, &logs[i]); err != nil {
				return err
			}
		}
	}
	batchSize := c.llmAuditBatchSize
	if batchSize <= 0 {
		batchSize = defaultLLMAuditLogBatchSize
	}
	return c.db.WithContext(ctx).CreateInBatches(logs, batchSize).Error
}

func (c *Client) runLLMAuditLogCleanup(ctx context.Context, retentionDays int) {
	if retentionDays <= 0 {
		return
	}

	err := c.deleteOldLLMAuditLogs(ctx, time.Now().UTC(), retentionDays)
	if err != nil && !errors.Is(err, context.Canceled) {
		log.Errorf("Failed to delete old LLM audit logs: %v", err)
	}

	ticker := time.NewTicker(c.auditLogCleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			err = c.deleteOldLLMAuditLogs(ctx, now.UTC(), retentionDays)
			if err != nil && !errors.Is(err, context.Canceled) {
				log.Errorf("Failed to delete old LLM audit logs: %v", err)
			}
		}
	}
}

func (c *Client) deleteOldLLMAuditLogs(ctx context.Context, now time.Time, retentionDays int) error {
	if retentionDays <= 0 {
		return nil
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}

	cutoff := now.Truncate(24*time.Hour).AddDate(0, 0, -retentionDays)

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		result := c.db.WithContext(ctx).Exec(
			"DELETE FROM llm_audit_logs WHERE id IN (SELECT id FROM llm_audit_logs WHERE created_at < ? LIMIT ?)",
			cutoff, c.auditLogDeleteBatchSize,
		)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected < int64(c.auditLogDeleteBatchSize) {
			return nil
		}
	}
}

func (c *Client) encryptLLMAuditLog(ctx context.Context, log *types.LLMAuditLog) error {
	if c.encryptionConfig == nil {
		return nil
	}

	transformer := c.encryptionConfig.Transformers[llmAuditLogGroupResource]
	if transformer == nil {
		return nil
	}

	dataCtx := llmAuditLogDataCtx(log)
	errs := []error{
		encryptRawMessageField(ctx, transformer, dataCtx, &log.RequestHeaders),
		encryptRawMessageField(ctx, transformer, dataCtx, &log.RequestBody),
		encryptRawMessageField(ctx, transformer, dataCtx, &log.PolicyModifiedRequestBody),
		encryptRawMessageField(ctx, transformer, dataCtx, &log.ResponseHeaders),
		encryptRawMessageField(ctx, transformer, dataCtx, &log.ResponseBody),
	}
	log.Encrypted = true

	return errors.Join(errs...)
}

func (c *Client) decryptLLMAuditLog(ctx context.Context, log *types.LLMAuditLog) error {
	if !log.Encrypted || c.encryptionConfig == nil {
		return nil
	}

	transformer := c.encryptionConfig.Transformers[llmAuditLogGroupResource]
	if transformer == nil {
		return nil
	}

	dataCtx := llmAuditLogDataCtx(log)
	errs := []error{
		decryptRawMessageField(ctx, transformer, dataCtx, &log.RequestHeaders),
		decryptRawMessageField(ctx, transformer, dataCtx, &log.RequestBody),
		decryptRawMessageField(ctx, transformer, dataCtx, &log.PolicyModifiedRequestBody),
		decryptRawMessageField(ctx, transformer, dataCtx, &log.ResponseHeaders),
		decryptRawMessageField(ctx, transformer, dataCtx, &log.ResponseBody),
	}

	return errors.Join(errs...)
}

func llmAuditLogDataCtx(log *types.LLMAuditLog) value.Context {
	return value.DefaultContext(fmt.Sprintf("%s/%s/%s", llmAuditLogGroupResource.String(), log.ID, log.UserID))
}

type LLMAuditLogOptions struct {
	WithSensitiveFields    bool
	HideModelsRequests     bool
	UserID                 []string
	ModelProvider          []string
	TargetModel            []string
	RequestPath            []string
	ResponseStatus         []int
	Outcome                []string
	UserAgent              []string
	ClientSessionID        []string
	MessagePolicyTriggered []bool
	Query                  string
	StartTime              time.Time
	EndTime                time.Time
	Limit                  int
	Offset                 int
	SortBy                 string
	SortOrder              string
}
