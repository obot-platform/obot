package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"time"

	types2 "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/agentprofile"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/principal"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kuser "k8s.io/apiserver/pkg/authentication/user"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	// ErrInvalidAgentMetadata reports that CreateAgent or UpdateAgent was given metadata an agent must not have, such
	// as no display name.
	ErrInvalidAgentMetadata = errors.New("invalid agent metadata")

	// agentMetadataColumns are the agent columns that UpdateAgent writes.
	agentMetadataColumns = []string{"display_name", "description", "profiles"}
)

// agentLifecycleKey is what the agent lifecycle events delivered together share.
type agentLifecycleKey struct {
	agentID   uint
	eventType types.AgentLifecycleEventType
}

// CreateAgent creates an active agent with the metadata, profiles and creator of agent, and returns it. Nothing
// else of agent is written: the new agent gets a new ID, and is never created disabled or deleted. Every agent
// profile gets a new ID, and one that has an ID is refused. The entries are the caller's to validate
// (agentprofile.Validate).
func (c *Client) CreateAgent(ctx context.Context, agent types.Agent) (*types.Agent, error) {
	created := &types.Agent{
		DisplayName:   agent.DisplayName,
		Description:   agent.Description,
		CreatorUserID: agent.CreatorUserID,
		Profiles:      slices.Clone(agent.Profiles),
	}
	if err := validateAgentMetadata(*created); err != nil {
		return nil, err
	}
	if err := agentprofile.InitializeIDs(created.Profiles); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidAgentMetadata, err)
	}
	if err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(created).Error; err != nil {
			return fmt.Errorf("failed to create agent: %w", err)
		}
		var details map[string]any
		if len(created.Profiles) > 0 {
			details = map[string]any{types.AdminAuditAgentProfiles: profilesChange(nil, created.Profiles)}
		}
		return recordContextAdminAuditEventTx(ctx, tx, AdminAuditEventInput{
			Action:       types.AdminAuditActionAgentCreate,
			ResourceKind: types.AdminAuditResourceAgent,
			ResourceID:   strconv.FormatUint(uint64(created.ID), 10),
			Details:      details,
		})
	}); err != nil {
		return nil, err
	}
	return created, nil
}

// GetAgent returns the agent with id. A deleted agent is gorm.ErrRecordNotFound.
func (c *Client) GetAgent(ctx context.Context, id uint) (*types.Agent, error) {
	agent := new(types.Agent)
	if err := c.db.WithContext(ctx).Where("id = ?", id).First(agent).Error; err != nil {
		return nil, err
	}
	return agent, nil
}

// ListAgents returns the agents in the order they were created. Deleted agents are left out unless includeDeleted is
// set.
func (c *Client) ListAgents(ctx context.Context, includeDeleted bool) ([]types.Agent, error) {
	tx := c.db.WithContext(ctx)
	if includeDeleted {
		tx = tx.Unscoped()
	}
	var agents []types.Agent
	if err := tx.Order("id").Find(&agents).Error; err != nil {
		return nil, fmt.Errorf("failed to list agents: %w", err)
	}
	return agents, nil
}

// UpdateAgent replaces the metadata and profiles of the agent with agent.ID with those of agent, and returns the
// agent. Nothing else of agent is written, so it can neither change the agent's lifecycle nor its creator. A deleted or
// missing agent is gorm.ErrRecordNotFound.
//
// Profiles are matched to the stored ones by ID, under the agent's lock (agentprofile.ReconcileIDs): an entry
// without an ID gets a new one, and an ID the agent does not have is refused. The entries are the caller's to validate
// (agentprofile.Validate).
func (c *Client) UpdateAgent(ctx context.Context, agent types.Agent) (*types.Agent, error) {
	if err := validateAgentMetadata(agent); err != nil {
		return nil, err
	}
	profiles := slices.Clone(agent.Profiles)

	var updated *types.Agent
	if err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		if updated, err = lockAgent(tx, agent.ID); err != nil {
			return err
		}
		var changed []string
		if updated.DisplayName != agent.DisplayName {
			changed = append(changed, "displayName")
		}
		if updated.Description != agent.Description {
			changed = append(changed, "description")
		}
		if err := agentprofile.ReconcileIDs(updated.Profiles, profiles); err != nil {
			return fmt.Errorf("%w: %w", ErrInvalidAgentMetadata, err)
		}
		details := map[string]any{}
		if !profilesEqual(updated.Profiles, profiles) {
			changed = append(changed, types.AdminAuditAgentProfiles)
			details[types.AdminAuditAgentProfiles] = profilesChange(updated.Profiles, profiles)
		}
		updated.DisplayName = agent.DisplayName
		updated.Description = agent.Description
		updated.Profiles = profiles

		// Select writes the metadata even where it is now empty, which a struct update skips. Omit is redundant with
		// it, and keeps the lifecycle columns out should the selection ever grow.
		if err := tx.Select(agentMetadataColumns).Omit(types.AgentLifecycleColumns...).Updates(updated).Error; err != nil {
			return fmt.Errorf("failed to update agent %d: %w", agent.ID, err)
		}
		if len(changed) == 0 {
			// Saving what is already stored changes nothing, so there is nothing to record.
			return nil
		}
		// Field names, and the before and after of profiles, which hold only names, subjects and IDs: the
		// display name and description are free text, which could hold anything.
		details["changedFields"] = changed
		return recordContextAdminAuditEventTx(ctx, tx, AdminAuditEventInput{
			Action:       types.AdminAuditActionAgentUpdate,
			ResourceKind: types.AdminAuditResourceAgent,
			ResourceID:   strconv.FormatUint(uint64(agent.ID), 10),
			Details:      details,
		})
	}); err != nil {
		return nil, err
	}
	return updated, nil
}

// DisableAgent denies an agent access, and records why. Disabling a disabled agent only updates the reason. A deleted
// or missing agent is gorm.ErrRecordNotFound.
func (c *Client) DisableAgent(ctx context.Context, id uint, reason string) (*types.Agent, error) {
	return c.changeAgentLifecycle(ctx, func(tx *gorm.DB) (*types.Agent, bool, error) {
		agent, changed, reasonChanged, err := disableAgentTx(tx, id, reason)
		if err != nil {
			return nil, false, err
		}
		// Disabling a disabled agent with another reason changes only the reason, which is still a change an
		// administrator made. Repeating the same disable changes nothing and records nothing.
		if changed || reasonChanged {
			// Whether there is a reason, not the reason: it is free text, which could hold anything.
			err = recordContextAdminAuditEventTx(ctx, tx, AdminAuditEventInput{
				Action:       types.AdminAuditActionAgentDisable,
				ResourceKind: types.AdminAuditResourceAgent,
				ResourceID:   strconv.FormatUint(uint64(id), 10),
				Details:      map[string]any{"accessChanged": changed, "hasReason": agent.DisabledReason != ""},
			})
		}
		return agent, changed, err
	})
}

// EnableAgent restores the access of a disabled agent. Enabling an agent that is not disabled changes nothing. A
// deleted or missing agent is gorm.ErrRecordNotFound.
func (c *Client) EnableAgent(ctx context.Context, id uint) (*types.Agent, error) {
	return c.changeAgentLifecycle(ctx, func(tx *gorm.DB) (*types.Agent, bool, error) {
		agent, changed, err := enableAgentTx(tx, id)
		if err != nil || !changed {
			// Enabling an agent that is not disabled changes nothing, so it records nothing.
			return agent, changed, err
		}
		return agent, changed, recordContextAdminAuditEventTx(ctx, tx, AdminAuditEventInput{
			Action:       types.AdminAuditActionAgentEnable,
			ResourceKind: types.AdminAuditResourceAgent,
			ResourceID:   strconv.FormatUint(uint64(id), 10),
		})
	})
}

// DeleteAgent deletes an agent and revokes its API keys. The agent's row is kept, so that audit history can still
// show it, and the controller removes what it owns. A deleted or missing agent is gorm.ErrRecordNotFound.
func (c *Client) DeleteAgent(ctx context.Context, id uint) error {
	_, err := c.changeAgentLifecycle(ctx, func(tx *gorm.DB) (*types.Agent, bool, error) {
		agent, keysRevoked, err := deleteAgentTx(tx, id)
		if err != nil {
			return nil, false, err
		}
		return agent, true, recordContextAdminAuditEventTx(ctx, tx, AdminAuditEventInput{
			Action:       types.AdminAuditActionAgentDelete,
			ResourceKind: types.AdminAuditResourceAgent,
			ResourceID:   strconv.FormatUint(uint64(id), 10),
			Details:      map[string]any{"keysRevoked": keysRevoked},
		})
	})
	return err
}

// ActiveAgent returns the agent with id if it is active, for an authenticator of agents to build its principal from.
// It is the one check of an agent's access: an agent that is disabled, deleted or missing is an
// *AgentAccessDeniedError, and one that cannot be read, so that its access is unknown, is an *AgentAccessLookupError.
func (c *Client) ActiveAgent(ctx context.Context, id uint) (*types.Agent, error) {
	agent, err := c.GetAgent(ctx, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, &AgentAccessDeniedError{
			AgentID: id,
			Status:  types2.UserStatusDeleted,
		}
	} else if err != nil {
		return nil, &AgentAccessLookupError{
			AgentID: id,
			Err:     err,
		}
	}
	if status := agent.Status(); status != types2.UserStatusActive {
		return nil, &AgentAccessDeniedError{
			AgentID: id,
			Status:  status,
		}
	}
	return agent, nil
}

// AgentUserInfo returns the principal of agent, as every authenticator of agents builds it. Its UID and its name are
// both the agent's encoded ID (principal.Agent(id).String()), never a username, so that it is never mistaken for the
// user with the same numeric ID. It records the agent kind and lifecycle status, as principal.RecordKind and
// principal.RecordUserStatus do.
//
// An agent has no role and belongs to no group, so groups are only what the caller's credential grants. extra holds
// the rest of what the credential establishes, such as its API key attribution and MCP scope; AgentUserInfo
// replaces whatever extra holds under the keys it records, and must be given a map that no one else uses.
func AgentUserInfo(agent *types.Agent, groups []string, extra map[string][]string) *kuser.DefaultInfo {
	if extra == nil {
		extra = map[string][]string{}
	}
	principal.RecordPrincipal(extra, principal.KindAgent, agent.Status())

	uid := principal.Agent(agent.ID).String()
	return &kuser.DefaultInfo{
		Name:   uid,
		UID:    uid,
		Groups: groups,
		Extra:  extra,
	}
}

// changeAgentLifecycle runs change in a transaction of its own, and kicks lifecycle delivery after the commit when
// change reports that it recorded a lifecycle event.
func (c *Client) changeAgentLifecycle(ctx context.Context, change func(tx *gorm.DB) (*types.Agent, bool, error)) (*types.Agent, error) {
	var (
		agent   *types.Agent
		changed bool
	)
	if err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		agent, changed, err = change(tx)
		return err
	}); err != nil {
		return nil, err
	}
	if changed {
		c.kickLifecycleEventDelivery()
	}
	return agent, nil
}

// disableAgentTx disables an agent within tx, and reports whether the agent lost access, in which case it recorded a
// disabled event, and whether only the reason of an agent that was already disabled changed. The caller must kick
// lifecycle delivery after committing when it reports that the agent lost access.
func disableAgentTx(tx *gorm.DB, id uint, reason string) (agent *types.Agent, accessChanged, reasonChanged bool, err error) {
	agent, err = lockAgent(tx, id)
	if err != nil {
		return nil, false, false, err
	}

	if agent.DisabledAt != nil {
		if agent.DisabledReason == reason {
			return agent, false, false, nil
		}
		// Access is unchanged, so only the reason is updated, and there is nothing to deliver.
		if err := tx.Model(agent).UpdateColumn("disabled_reason", reason).Error; err != nil {
			return nil, false, false, fmt.Errorf("failed to update the disable reason of agent %d: %w", id, err)
		}
		agent.DisabledReason = reason
		return agent, false, true, nil
	}

	now := time.Now()
	if err := tx.Model(agent).UpdateColumns(map[string]any{
		"disabled_at":     now,
		"disabled_reason": reason,
	}).Error; err != nil {
		return nil, false, false, fmt.Errorf("failed to disable agent %d: %w", id, err)
	}
	if err := recordAgentLifecycleEvent(tx, id, types.AgentLifecycleEventDisabled); err != nil {
		return nil, false, false, err
	}

	agent.DisabledAt = &now
	agent.DisabledReason = reason
	return agent, true, false, nil
}

// enableAgentTx enables a disabled agent within tx, and reports whether the agent regained access, in which case it
// recorded an enabled event. The caller must kick lifecycle delivery after committing when it reports a change.
func enableAgentTx(tx *gorm.DB, id uint) (*types.Agent, bool, error) {
	agent, err := lockAgent(tx, id)
	if err != nil {
		return nil, false, err
	}
	if agent.DisabledAt == nil {
		return agent, false, nil
	}

	// Explicit columns, because struct updates skip the zero values that mark an agent as enabled.
	if err := tx.Model(agent).UpdateColumns(map[string]any{
		"disabled_at":     nil,
		"disabled_reason": "",
	}).Error; err != nil {
		return nil, false, fmt.Errorf("failed to enable agent %d: %w", id, err)
	}
	if err := recordAgentLifecycleEvent(tx, id, types.AgentLifecycleEventEnabled); err != nil {
		return nil, false, err
	}

	agent.DisabledAt = nil
	agent.DisabledReason = ""
	return agent, true, nil
}

// deleteAgentTx deletes an agent within tx, revokes its API keys, and records a deleted event. It returns the number
// of keys it revoked. The caller must kick lifecycle delivery after committing.
func deleteAgentTx(tx *gorm.DB, id uint) (*types.Agent, int64, error) {
	agent, err := lockAgent(tx, id)
	if err != nil {
		return nil, 0, err
	}

	keysRevoked, err := revokeAgentAPIKeysTx(tx, id)
	if err != nil {
		return nil, 0, err
	}
	// A soft delete, because the agent has a gorm.DeletedAt: the row stays for audit history.
	if err := tx.Delete(agent).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to delete agent %d: %w", id, err)
	}
	if err := recordAgentLifecycleEvent(tx, id, types.AgentLifecycleEventDeleted); err != nil {
		return nil, 0, err
	}
	return agent, keysRevoked, nil
}

// revokeAgentAPIKeysTx revokes, within tx, every API key of an agent that is being deleted, so that none of them
// works once the deletion commits, and returns how many it revoked. Keys that are already revoked keep their
// revocation time and are not counted. No cached validation needs invalidating: a cache hit reads the key's
// revocation again, so a revoked key is refused at once on every replica.
func revokeAgentAPIKeysTx(tx *gorm.DB, agentID uint) (int64, error) {
	result := whereAPIKeyOwner(tx.Model(&types.APIKey{}), principal.Agent(agentID)).
		Where("revoked_at IS NULL").
		Update("revoked_at", time.Now().UTC())
	if result.Error != nil {
		return 0, fmt.Errorf("failed to revoke the API keys of agent %d: %w", agentID, result.Error)
	}
	return result.RowsAffected, nil
}

// lockAgent loads and locks an agent within tx. A deleted or missing agent is gorm.ErrRecordNotFound.
func lockAgent(tx *gorm.DB, id uint) (*types.Agent, error) {
	agent := new(types.Agent)
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", id).First(agent).Error; err != nil {
		return nil, err
	}
	return agent, nil
}

// profilesEqual reports whether a and b are the same profiles, as they are stored: compared as JSON, in which a
// list that is empty and one that is missing are the same.
func profilesEqual(a, b []types2.AgentProfile) bool {
	if len(a) == 0 || len(b) == 0 {
		return len(a) == len(b)
	}
	aJSON, aErr := json.Marshal(a)
	bJSON, bErr := json.Marshal(b)
	return aErr == nil && bErr == nil && bytes.Equal(aJSON, bJSON)
}

// profilesChange is how a change to an agent's profiles is recorded in the admin audit: what it was before and
// after the change, always as lists. Subjects and IDs are not secret.
func profilesChange(before, after []types2.AgentProfile) map[string]any {
	if before == nil {
		before = []types2.AgentProfile{}
	}
	if after == nil {
		after = []types2.AgentProfile{}
	}
	return map[string]any{"before": before, "after": after}
}

func validateAgentMetadata(agent types.Agent) error {
	if strings.TrimSpace(agent.DisplayName) == "" {
		return fmt.Errorf("%w: agent display name is required", ErrInvalidAgentMetadata)
	}
	return nil
}

// recordAgentLifecycleEvent records, within tx, a lifecycle change of an agent that must be delivered once tx
// commits.
func recordAgentLifecycleEvent(tx *gorm.DB, agentID uint, eventType types.AgentLifecycleEventType) error {
	if err := tx.Create(&types.AgentLifecycleEvent{
		AgentID: agentID,
		Type:    eventType,
	}).Error; err != nil {
		return fmt.Errorf("failed to record the %s event of agent %d: %w", eventType, agentID, err)
	}
	return nil
}

// agentLifecycleOutbox returns the outbox of agent lifecycle events.
func (c *Client) agentLifecycleOutbox() lifecycleOutbox[types.AgentLifecycleEvent, agentLifecycleKey] {
	return lifecycleOutbox[types.AgentLifecycleEvent, agentLifecycleKey]{
		db:   c.db,
		name: "agent lifecycle",
		id: func(event types.AgentLifecycleEvent) uint {
			return event.ID
		},
		key: func(event types.AgentLifecycleEvent) agentLifecycleKey {
			return agentLifecycleKey{
				agentID:   event.AgentID,
				eventType: event.Type,
			}
		},
		deliverer: func(context.Context) func(context.Context, types.AgentLifecycleEvent) error {
			return c.deliverAgentLifecycleEvent
		},
		logAttrs: func(event types.AgentLifecycleEvent) []any {
			return []any{"agentID", event.AgentID, "type", event.Type}
		},
	}
}

// deliverAgentLifecycleEvents delivers the pending agent lifecycle events and prunes old delivered ones.
func (c *Client) deliverAgentLifecycleEvents(ctx context.Context) error {
	return c.agentLifecycleOutbox().deliver(ctx)
}

// deliverAgentLifecycleEvent creates the AgentIdentityChange or AgentIdentityDelete that tells the controller about
// an agent's lifecycle change. Its name comes from the event, so a repeated delivery creates nothing new while it
// exists, and its handler must tolerate a repeat if it does.
func (c *Client) deliverAgentLifecycleEvent(ctx context.Context, event types.AgentLifecycleEvent) error {
	var obj kclient.Object
	switch event.Type {
	case types.AgentLifecycleEventDisabled, types.AgentLifecycleEventEnabled:
		obj = &v1.AgentIdentityChange{
			Name:      agentLifecycleObjectName(system.AgentIdentityChangePrefix, event.ID),
			Namespace: system.DefaultNamespace,
			Spec: v1.AgentIdentityChangeSpec{
				AgentID: event.AgentID,
			},
		}
	case types.AgentLifecycleEventDeleted:
		obj = &v1.AgentIdentityDelete{
			Name:      agentLifecycleObjectName(system.AgentIdentityDeletePrefix, event.ID),
			Namespace: system.DefaultNamespace,
			Spec: v1.AgentIdentityDeleteSpec{
				AgentID: event.AgentID,
			},
		}
	default:
		return fmt.Errorf("unknown agent lifecycle event type %q", event.Type)
	}

	if err := c.storageClient.Create(ctx, obj); apierrors.IsAlreadyExists(err) {
		return nil
	} else if err != nil {
		return fmt.Errorf("failed to deliver the %s event of agent %d: %w", event.Type, event.AgentID, err)
	}
	if event.Type == types.AgentLifecycleEventDeleted {
		slog.Info("Scheduled agent cleanup after deletion", "agentID", event.AgentID)
	}
	return nil
}

// agentLifecycleObjectName names the controller object delivered for an agent outbox event. The name cannot collide
// with one generated from the same prefix, whose random suffix never contains a vowel.
func agentLifecycleObjectName(prefix string, eventID uint) string {
	return fmt.Sprintf("%sagent-lifecycle-%d", prefix, eventID)
}
