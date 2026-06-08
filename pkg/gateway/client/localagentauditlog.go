package client

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/obot-platform/obot/pkg/gateway/types"
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

func localAgentAuditLogDataCtx(log *types.LocalAgentAuditLog) value.Context {
	return value.DefaultContext(fmt.Sprintf("%s/%s/%s", localAgentAuditLogGroupResource.String(), log.EventID, log.UserID))
}
