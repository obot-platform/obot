package client

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/storage/value"
)

var llmAuditLogGroupResource = schema.GroupResource{
	Group:    "obot.obot.ai",
	Resource: "llmauditlogs",
}

func (c *Client) InsertLLMAuditLog(ctx context.Context, auditLog *types.LLMAuditLog) error {
	if err := c.encryptLLMAuditLog(ctx, auditLog); err != nil {
		return err
	}
	return c.db.WithContext(ctx).Create(auditLog).Error
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
