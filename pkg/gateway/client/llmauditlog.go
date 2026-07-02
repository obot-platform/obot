package client

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	gatewayllmaudit "github.com/obot-platform/obot/pkg/gateway/llmaudit"
	"github.com/obot-platform/obot/pkg/gateway/types"
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
	return c.insertLLMAuditLogs(ctx, []types.LLMAuditLog{*auditLog})
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
	accumulator := gatewayllmaudit.NewResponseAccumulator(log.ModelProvider)
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

	var errs []error
	dataCtx := llmAuditLogDataCtx(log)
	encrypt := func(field string) string {
		if field == "" {
			return ""
		}
		b, err := transformer.TransformToStorage(ctx, []byte(field), dataCtx)
		if err != nil {
			errs = append(errs, err)
			return field
		}
		return base64.StdEncoding.EncodeToString(b)
	}

	log.RequestHeaders = encrypt(log.RequestHeaders)
	log.RequestBody = encrypt(log.RequestBody)
	log.RedactedRequestBody = encrypt(log.RedactedRequestBody)
	log.ResponseHeaders = encrypt(log.ResponseHeaders)
	log.ResponseBody = encrypt(log.ResponseBody)
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

	var errs []error
	dataCtx := llmAuditLogDataCtx(log)
	decrypt := func(field string) string {
		if field == "" {
			return ""
		}
		decoded, err := base64.StdEncoding.DecodeString(field)
		if err != nil {
			errs = append(errs, err)
			return field
		}
		out, _, err := transformer.TransformFromStorage(ctx, decoded, dataCtx)
		if err != nil {
			errs = append(errs, err)
			return field
		}
		return string(out)
	}

	log.RequestHeaders = decrypt(log.RequestHeaders)
	log.RequestBody = decrypt(log.RequestBody)
	log.RedactedRequestBody = decrypt(log.RedactedRequestBody)
	log.ResponseHeaders = decrypt(log.ResponseHeaders)
	log.ResponseBody = decrypt(log.ResponseBody)

	return errors.Join(errs...)
}

func llmAuditLogDataCtx(log *types.LLMAuditLog) value.Context {
	return value.DefaultContext(fmt.Sprintf("%s/%s/%s", llmAuditLogGroupResource.String(), log.ID, log.UserID))
}
