package client

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"gorm.io/gorm"
	kuser "k8s.io/apiserver/pkg/authentication/user"
)

type adminActorKey struct{}

// AdminAuditEventInput describes an event to record. Details is marshaled to JSON, and must never hold a secret value.
type AdminAuditEventInput struct {
	// Actor is who made the change. RecordAdminAuditEvent defaults it to the administrator of its context
	// (WithAdminActor), and a transactional record always takes that administrator.
	Actor        string
	Action       string
	ResourceKind string
	ResourceID   string
	ParentID     string
	Details      any
}

// AdminAuditEventOptions filters and pages ListAdminAuditEvents. Empty fields match everything.
type AdminAuditEventOptions struct {
	// Each of these matches any of its values.
	ResourceKinds []string
	ResourceIDs   []string
	Actors        []string
	Actions       []string
	Since, Until  time.Time
	Limit         int
	Offset        int
	// Oldest lists the oldest events first, which an export needs to page through events that are still arriving.
	Oldest bool
}

// WithAdminActor returns a context whose gateway changes to agents and agent API keys are recorded as changes made by
// actor, the authenticated administrator's user ID. A change made with a context that has no actor, such as one a
// controller makes, is not recorded.
func WithAdminActor(ctx context.Context, actor string) context.Context {
	if actor == "" {
		return ctx
	}
	return context.WithValue(ctx, adminActorKey{}, actor)
}

// WithAdminUser is WithAdminActor for an authenticated user, and returns ctx unchanged when there is none.
func WithAdminUser(ctx context.Context, user kuser.Info) context.Context {
	if user == nil {
		return ctx
	}
	return WithAdminActor(ctx, user.GetUID())
}

func adminActor(ctx context.Context) string {
	actor, _ := ctx.Value(adminActorKey{}).(string)
	return actor
}

func newAdminAuditEvent(in AdminAuditEventInput) (*types.AdminAuditEvent, error) {
	event := &types.AdminAuditEvent{
		CreatedAt:    time.Now().UTC(),
		ActorUserID:  in.Actor,
		Action:       in.Action,
		ResourceKind: in.ResourceKind,
		ResourceID:   in.ResourceID,
		ParentID:     in.ParentID,
	}
	if in.Details != nil {
		details, err := json.Marshal(in.Details)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal the details of admin audit event %s: %w", in.Action, err)
		}
		event.Details = details
	}
	return event, nil
}

// recordAdminAuditEventTx writes an event within tx, so that it commits with the change it records, or not at all.
func recordAdminAuditEventTx(tx *gorm.DB, in AdminAuditEventInput) error {
	event, err := newAdminAuditEvent(in)
	if err != nil {
		return err
	}
	if err := tx.Create(event).Error; err != nil {
		return fmt.Errorf("failed to record admin audit event %s: %w", in.Action, err)
	}
	return nil
}

// recordContextAdminAuditEventTx records an event within tx as the administrator ctx names, and does nothing if it
// names none.
func recordContextAdminAuditEventTx(ctx context.Context, tx *gorm.DB, in AdminAuditEventInput) error {
	if in.Actor = adminActor(ctx); in.Actor == "" {
		return nil
	}
	return recordAdminAuditEventTx(tx, in)
}

// RecordAdminAuditEvent records an event for a change that was already made, in storage the gateway database cannot
// share a transaction with, as the change of in.Actor or, if that is empty, of the administrator ctx names. It is best
// effort: the change has happened and must not fail because its record could not be written, so a failure is logged
// at error level, with what the event said, and not returned.
func (c *Client) RecordAdminAuditEvent(ctx context.Context, in AdminAuditEventInput) {
	if in.Actor == "" {
		in.Actor = adminActor(ctx)
	}
	// The request may have been canceled by now, and the change it made is done.
	if err := recordAdminAuditEventTx(c.db.WithContext(context.WithoutCancel(ctx)), in); err != nil {
		slog.Error("Failed to record an admin audit event; the change it describes was made and is not in the admin audit",
			"action", in.Action, "actor", in.Actor, "resourceKind", in.ResourceKind, "resourceID", in.ResourceID, "parentID", in.ParentID, "error", err)
	}
}

// ListAdminAuditEvents returns the events matching opts, newest first unless opts.Oldest is set, and how many events match in all.
func (c *Client) ListAdminAuditEvents(ctx context.Context, opts AdminAuditEventOptions) ([]types.AdminAuditEvent, int64, error) {
	db := c.db.WithContext(ctx).Model(&types.AdminAuditEvent{})
	for column, values := range map[string][]string{
		"resource_kind": opts.ResourceKinds,
		"resource_id":   opts.ResourceIDs,
		"actor_user_id": opts.Actors,
		"action":        opts.Actions,
	} {
		if len(values) > 0 {
			db = db.Where(column+" IN ?", values)
		}
	}
	if !opts.Since.IsZero() {
		db = db.Where("created_at >= ?", opts.Since.UTC())
	}
	if !opts.Until.IsZero() {
		db = db.Where("created_at < ?", opts.Until.UTC())
	}

	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count admin audit events: %w", err)
	}
	// The ID breaks ties between events in the same instant, and orders as the events were written.
	if opts.Oldest {
		db = db.Order("created_at ASC").Order("id ASC")
	} else {
		db = db.Order("created_at DESC").Order("id DESC")
	}
	if opts.Limit > 0 {
		db = db.Limit(opts.Limit)
	}
	if opts.Offset > 0 {
		db = db.Offset(opts.Offset)
	}
	var events []types.AdminAuditEvent
	if err := db.Find(&events).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to list admin audit events: %w", err)
	}
	return events, total, nil
}

// deleteOldAdminAuditEvents deletes the events older than retentionDays, in batches. A retention of zero or less keeps
// them all.
func (c *Client) deleteOldAdminAuditEvents(ctx context.Context, now time.Time, retentionDays int) error {
	if retentionDays <= 0 {
		return nil
	}

	cutoff := now.Truncate(24*time.Hour).AddDate(0, 0, -retentionDays)
	for {
		result := c.db.WithContext(ctx).Exec(
			"DELETE FROM admin_audit_events WHERE id IN (SELECT id FROM admin_audit_events WHERE created_at < ? LIMIT ?)",
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
