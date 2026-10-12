package client

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"gorm.io/gorm"
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
// the failure. It records nothing for a connection that no longer exists.
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

	failure := &types.SCIMRequestFailure{
		ConnectionID: connectionID,
		CreatedAt:    now,
		Method:       outcome.Method,
		Resource:     strings.ToValidUTF8(outcome.Resource, "\uFFFD"),
		Status:       outcome.Status,
		SCIMType:     outcome.SCIMType,
		Detail:       truncateUTF8(outcome.Detail, maxSCIMFailureDetailLength),
	}
	if err := c.encryptSCIMRequestFailure(ctx, failure); err != nil {
		return err
	}
	return db.Transaction(func(tx *gorm.DB) error {
		// A request can fail because its connection was deleted while it was handled. The connection is locked, as
		// deleting it does, so that a failure is either recorded first and deleted with the connection, or not at all.
		if _, err := scimConnectionTx(tx, connectionID, true); errors.Is(err, ErrSCIMConnectionNotFound) {
			return nil
		} else if err != nil {
			return err
		}
		if err := tx.Create(failure).Error; err != nil {
			return fmt.Errorf("failed to record SCIM request failure: %w", err)
		}

		// Only the most recent failures are kept.
		if err := tx.Where("connection_id = ? AND id NOT IN (?)", connectionID,
			tx.Model(new(types.SCIMRequestFailure)).
				Select("id").
				Where("connection_id = ?", connectionID).
				Order("id DESC").
				Limit(scimRequestFailuresKept),
		).Delete(new(types.SCIMRequestFailure)).Error; err != nil {
			return fmt.Errorf("failed to prune SCIM request failures: %w", err)
		}
		return nil
	})
}

// truncateUTF8 returns s, as valid UTF-8, cut to at most n bytes without splitting a character. A failure's details
// can quote what the request sent, and PostgreSQL refuses text that is not valid UTF-8.
func truncateUTF8(s string, n int) string {
	s = strings.ToValidUTF8(s, "\uFFFD")
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
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
