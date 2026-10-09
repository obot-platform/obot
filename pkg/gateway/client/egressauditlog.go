package client

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/storage/value"
)

var (
	egressAuditResource  = schema.GroupResource{Group: "obot.obot.ai", Resource: "egressauditlogs"}
	egressPayloadColumns = []string{"request_headers", "response_headers", "request_body", "response_body"}
)

type EgressAuditLogOptions struct {
	UserID, ClaudeAccount, EnvironmentID, ClaudeSession, Host, Outcome string
	StartTime, EndTime                                                 time.Time
	Limit, Offset                                                      int
}

// EgressAuditEnvironments exposes names without loading configuration or secrets.
func (c *Client) EgressAuditEnvironments(ctx context.Context) (map[string]string, error) {
	var environments []struct {
		ID          string
		DisplayName string
	}
	if err := c.db.WithContext(ctx).Model(&types.ClaudeEnvironment{}).Select("id", "display_name").Find(&environments).Error; err != nil {
		return nil, err
	}

	names := make(map[string]string, len(environments))
	for _, environment := range environments {
		names[environment.ID] = environment.DisplayName
	}
	return names, nil
}

// SaveEgressAuditLog persists a capture without overwriting final transport status.
func (c *Client) SaveEgressAuditLog(ctx context.Context, entry types.EgressAuditLog) error {
	return c.insertEgressAuditLogs(ctx, []types.EgressAuditLog{entry})
}

func (c *Client) insertEgressAuditLogs(ctx context.Context, entries []types.EgressAuditLog) error {
	captures := make([]types.EgressAuditLog, 0, len(entries))
	finalByID := make(map[string]types.EgressAuditLog)
	for _, entry := range entries {
		if entry.Finalized {
			finalByID[entry.ID] = entry
			continue
		}
		if err := c.transformEgressPayload(ctx, &entry, false); err != nil {
			return err
		}
		captures = append(captures, entry)
	}

	finals := make([]types.EgressAuditLog, 0, len(finalByID))
	for _, entry := range finalByID {
		finals = append(finals, entry)
	}

	return c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if len(captures) > 0 {
			updates := clause.AssignmentColumns([]string{"created_at", "updated_at", "user_id", "claude_account", "environment_id", "claude_session", "actor", "method", "scheme", "host", "path", "request_bytes", "response_bytes", "request_truncated", "response_truncated", "request_headers", "response_headers", "request_body", "response_body", "encrypted"})
			for _, column := range []string{"response_status", "duration_ms", "outcome"} {
				updates = append(updates, clause.Assignment{Column: clause.Column{Name: column}, Value: gorm.Expr("CASE WHEN egress_audit_logs.finalized THEN egress_audit_logs." + column + " ELSE excluded." + column + " END")})
			}
			if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "id"}}, DoUpdates: updates}).CreateInBatches(captures, 100).Error; err != nil {
				return err
			}
		}
		if len(finals) > 0 {
			// Final events can arrive on another replica before the capture is flushed.
			return tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "id"}}, DoUpdates: clause.AssignmentColumns([]string{"response_status", "duration_ms", "outcome", "updated_at", "finalized"})}).CreateInBatches(finals, 100).Error
		}
		return nil
	})
}

func (c *Client) transformEgressPayload(ctx context.Context, entry *types.EgressAuditLog, decrypt bool) error {
	if c.encryptionConfig == nil {
		if decrypt && entry.Encrypted {
			return fmt.Errorf("egress audit encryption key unavailable")
		}
		return nil
	}
	transformer := c.encryptionConfig.Transformers[egressAuditResource]
	if transformer == nil {
		if decrypt && entry.Encrypted {
			return fmt.Errorf("egress audit encryption key unavailable")
		}
		return nil
	}
	if decrypt && !entry.Encrypted {
		return nil
	}
	dataCtx := value.DefaultContext(fmt.Sprintf("%s/%s/%s", egressAuditResource.String(), entry.ID, entry.UserID))
	for _, field := range []*json.RawMessage{&entry.RequestHeaders, &entry.ResponseHeaders, &entry.RequestBody, &entry.ResponseBody} {
		var err error
		if decrypt {
			err = decryptRawMessageField(ctx, transformer, dataCtx, field)
		} else {
			err = encryptRawMessageField(ctx, transformer, dataCtx, field)
		}
		if err != nil {
			return err
		}
	}
	entry.Encrypted = !decrypt
	return nil
}

func (c *Client) ListEgressAuditLogs(ctx context.Context, opts EgressAuditLogOptions) ([]types.EgressAuditLog, int64, error) {
	query := c.db.WithContext(ctx).Model(&types.EgressAuditLog{}).Where("user_id <> ?", "")
	for column, val := range map[string]string{"user_id": opts.UserID, "claude_account": opts.ClaudeAccount, "environment_id": opts.EnvironmentID, "claude_session": opts.ClaudeSession, "host": opts.Host, "outcome": opts.Outcome} {
		if val != "" {
			query = query.Where(column+" = ?", val)
		}
	}
	if !opts.StartTime.IsZero() {
		query = query.Where("created_at >= ?", opts.StartTime)
	}
	if !opts.EndTime.IsZero() {
		query = query.Where("created_at <= ?", opts.EndTime)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var entries []types.EgressAuditLog
	err := query.Omit(egressPayloadColumns...).Order("created_at DESC, id DESC").Limit(min(max(opts.Limit, 1), 100)).Offset(max(opts.Offset, 0)).Find(&entries).Error
	return entries, total, err
}

func (c *Client) GetEgressAuditLog(ctx context.Context, id string, auditor bool) (*types.EgressAuditLog, error) {
	query := c.db.WithContext(ctx).Where("user_id <> ?", "")
	if !auditor {
		query = query.Omit(egressPayloadColumns...)
	}
	var entry types.EgressAuditLog
	if err := query.First(&entry, "id = ?", id).Error; err != nil {
		return nil, err
	}
	if auditor {
		if err := c.transformEgressPayload(ctx, &entry, true); err != nil {
			return nil, err
		}
	}
	return &entry, nil
}

func (c *Client) RunEgressAuditCleanup(ctx context.Context, days int) {
	if days <= 0 {
		return
	}
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		cutoff := time.Now().UTC().AddDate(0, 0, -days)
		for {
			result := c.db.WithContext(ctx).Exec("DELETE FROM egress_audit_logs WHERE id IN (SELECT id FROM egress_audit_logs WHERE updated_at < ? LIMIT 1000)", cutoff)
			if result.Error != nil {
				slog.ErrorContext(ctx, "Failed to clean up egress audit logs", "error", result.Error)
				break
			}
			if result.RowsAffected < 1000 {
				break
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
