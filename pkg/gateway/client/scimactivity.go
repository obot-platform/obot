package client

import (
	"context"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"k8s.io/apiserver/pkg/storage/value"
)

const (
	// scimActivityInterval is how often the last request times are written, so that a burst of SCIM requests
	// does not write the connection row for every one of them.
	scimActivityInterval = 5 * time.Second

	// scimRequestFailuresKept is how many recent failures are kept for each connection.
	scimRequestFailuresKept = 50

	// maxSCIMFailureDetailLength bounds the stored detail of a failure.
	maxSCIMFailureDetailLength = 1000
)

// SCIMRequestOutcome describes a handled SCIM request.
type SCIMRequestOutcome struct {
	Method   string
	Resource string
	Status   int
	SCIMType string
	Detail   string
}

// RecordSCIMRequest records an authenticated SCIM request of the connection: its time, and for a failed request,
// the failure.
func (c *Client) RecordSCIMRequest(ctx context.Context, connectionID string, outcome SCIMRequestOutcome) error {
	now := time.Now()
	stale := now.Add(-scimActivityInterval)
	success := outcome.Status < 400

	columns := map[string]any{
		"last_request_at": now,
	}
	condition := "last_request_at IS NULL OR last_request_at < ?"
	args := []any{stale}
	if success {
		columns["last_success_at"] = now
		condition += " OR last_success_at IS NULL OR last_success_at < ?"
		args = append(args, stale)
	}

	db := c.db.WithContext(ctx)
	if err := db.Model(new(types.SCIMConnection)).
		Where("id = ?", connectionID).
		Where(condition, args...).
		UpdateColumns(columns).Error; err != nil {
		return fmt.Errorf("failed to record SCIM request time: %w", err)
	}
	if success {
		return nil
	}

	detail := outcome.Detail
	if len(detail) > maxSCIMFailureDetailLength {
		detail = detail[:maxSCIMFailureDetailLength]
	}
	failure := &types.SCIMRequestFailure{
		ConnectionID: connectionID,
		CreatedAt:    now,
		Method:       outcome.Method,
		Resource:     outcome.Resource,
		Status:       outcome.Status,
		SCIMType:     outcome.SCIMType,
		Detail:       detail,
	}
	if err := c.encryptSCIMRequestFailure(ctx, failure); err != nil {
		return err
	}
	if err := db.Create(failure).Error; err != nil {
		return fmt.Errorf("failed to record SCIM request failure: %w", err)
	}

	// Only the most recent failures are kept.
	if err := db.Where("connection_id = ? AND id NOT IN (?)", connectionID,
		db.Model(new(types.SCIMRequestFailure)).
			Select("id").
			Where("connection_id = ?", connectionID).
			Order("id DESC").
			Limit(scimRequestFailuresKept),
	).Delete(new(types.SCIMRequestFailure)).Error; err != nil {
		return fmt.Errorf("failed to prune SCIM request failures: %w", err)
	}
	return nil
}

// SCIMRequestFailures returns the connection's most recent failed requests, newest first.
func (c *Client) SCIMRequestFailures(ctx context.Context, connectionID string, limit int) ([]types.SCIMRequestFailure, error) {
	var failures []types.SCIMRequestFailure
	if err := c.db.WithContext(ctx).
		Where("connection_id = ?", connectionID).
		Order("id DESC").
		Limit(limit).
		Find(&failures).Error; err != nil {
		return nil, fmt.Errorf("failed to list SCIM request failures: %w", err)
	}

	for i := range failures {
		if err := c.decryptSCIMRequestFailure(ctx, &failures[i]); err != nil {
			return nil, err
		}
	}
	return failures, nil
}

func (c *Client) encryptSCIMRequestFailure(ctx context.Context, failure *types.SCIMRequestFailure) error {
	if c.encryptionConfig == nil || failure.Detail == "" {
		return nil
	}
	transformer := c.encryptionConfig.Transformers[userGroupResource]
	if transformer == nil {
		return nil
	}

	b, err := transformer.TransformToStorage(ctx, []byte(failure.Detail), scimRequestFailureDataCtx(failure))
	if err != nil {
		return fmt.Errorf("failed to encrypt SCIM request failure: %w", err)
	}
	failure.Detail = base64.StdEncoding.EncodeToString(b)
	failure.Encrypted = true
	return nil
}

func (c *Client) decryptSCIMRequestFailure(ctx context.Context, failure *types.SCIMRequestFailure) error {
	if !failure.Encrypted || c.encryptionConfig == nil {
		return nil
	}
	transformer := c.encryptionConfig.Transformers[userGroupResource]
	if transformer == nil {
		return nil
	}

	decoded, err := base64.StdEncoding.DecodeString(failure.Detail)
	if err != nil {
		return fmt.Errorf("failed to decode SCIM request failure: %w", err)
	}
	out, _, err := transformer.TransformFromStorage(ctx, decoded, scimRequestFailureDataCtx(failure))
	if err != nil {
		return fmt.Errorf("failed to decrypt SCIM request failure: %w", err)
	}
	failure.Detail = string(out)
	failure.Encrypted = false
	return nil
}

// scimRequestFailureDataCtx binds a failure's encrypted detail to its connection. The row ID is not known until
// the row is created.
func scimRequestFailureDataCtx(failure *types.SCIMRequestFailure) value.Context {
	return value.DefaultContext(fmt.Sprintf("%s/scim-failure/%s", userGroupResource.String(), failure.ConnectionID))
}
