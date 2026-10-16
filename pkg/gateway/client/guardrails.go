package client

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	// guardrailSecretContext is the credential context holding guardrail secrets.
	guardrailSecretContext = "guardrail"
)

func (c *Client) ListGuardrailPolicies(ctx context.Context) ([]types.GuardrailPolicy, error) {
	return c.ListGuardrailPoliciesForProfile(ctx, "")
}

// ListGuardrailPoliciesForProfile lists the policies of one gateway profile, or every policy if
// profileID is empty.
func (c *Client) ListGuardrailPoliciesForProfile(ctx context.Context, profileID string) ([]types.GuardrailPolicy, error) {
	var policies []types.GuardrailPolicy
	q := c.db.WithContext(ctx).Order("created_at")
	if profileID != "" {
		q = q.Where("profile_id = ?", profileID)
	}
	return policies, q.Find(&policies).Error
}

func (c *Client) GetGuardrailPolicy(ctx context.Context, id string) (*types.GuardrailPolicy, error) {
	var policy types.GuardrailPolicy
	if err := c.db.WithContext(ctx).Where("id = ?", id).First(&policy).Error; err != nil {
		return nil, err
	}
	return &policy, nil
}

func (c *Client) CreateGuardrailPolicy(ctx context.Context, policy *types.GuardrailPolicy) error {
	if policy.ID == "" {
		policy.ID = uuid.NewString()
	}
	return c.db.WithContext(ctx).Create(policy).Error
}

func (c *Client) UpdateGuardrailPolicy(ctx context.Context, policy *types.GuardrailPolicy) error {
	return c.db.WithContext(ctx).Model(policy).
		Select("name", "description", "enabled", "target", "apply_to", "action", "condition", "sampling_rate",
			"input_scope", "fail_closed", "block_message", "guardrail_ids", "profile_id").
		Updates(policy).Error
}

func (c *Client) DeleteGuardrailPolicy(ctx context.Context, id string) error {
	return c.db.WithContext(ctx).Where("id = ?", id).Delete(&types.GuardrailPolicy{}).Error
}

// DeleteGuardrailPoliciesForProfile deletes the policies of a gateway profile that was deleted.
func (c *Client) DeleteGuardrailPoliciesForProfile(ctx context.Context, profileID string) error {
	return c.db.WithContext(ctx).Where("profile_id = ?", profileID).Delete(&types.GuardrailPolicy{}).Error
}

func (c *Client) ListGuardrails(ctx context.Context) ([]types.Guardrail, error) {
	var guardrails []types.Guardrail
	return guardrails, c.db.WithContext(ctx).Order("created_at").Find(&guardrails).Error
}

func (c *Client) GetGuardrail(ctx context.Context, id string) (*types.Guardrail, error) {
	var guardrail types.Guardrail
	if err := c.db.WithContext(ctx).Where("id = ?", id).First(&guardrail).Error; err != nil {
		return nil, err
	}
	return &guardrail, nil
}

// CreateGuardrail saves a guardrail and its secrets.
// CreateGuardrail stores a guardrail and its secrets together.
func (c *Client) CreateGuardrail(ctx context.Context, guardrail *types.Guardrail, secrets map[string]string) error {
	if guardrail.ID == "" {
		guardrail.ID = uuid.NewString()
	}
	return c.saveGuardrail(ctx, guardrail.ID, secrets, func(tx *gorm.DB) error {
		return tx.Create(guardrail).Error
	})
}

// UpdateGuardrail replaces a guardrail's settings and its secrets together, so a
// guardrail is never left with a new destination and the old credentials, or the reverse.
func (c *Client) UpdateGuardrail(ctx context.Context, guardrail *types.Guardrail, secrets map[string]string) error {
	return c.saveGuardrail(ctx, guardrail.ID, secrets, func(tx *gorm.DB) error {
		return tx.Model(guardrail).Select("name", "description", "config").Updates(guardrail).Error
	})
}

// saveGuardrail writes a guardrail with save and replaces its secrets (an empty map
// removes them) in one transaction.
func (c *Client) saveGuardrail(ctx context.Context, id string, secrets map[string]string, save func(*gorm.DB) error) error {
	var cred *types.Credential
	if len(secrets) > 0 {
		cred = &types.Credential{Context: guardrailSecretContext, Name: id, Secrets: secrets}
		if err := c.encryptCredential(ctx, cred); err != nil {
			return fmt.Errorf("failed to encrypt the guardrail's secrets: %w", err)
		}
	}
	return c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := save(tx); err != nil {
			return err
		}
		if cred == nil {
			return tx.Where("context = ? AND name = ?", guardrailSecretContext, id).Delete(&types.Credential{}).Error
		}
		return tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "context"}, {Name: "name"}},
			DoUpdates: clause.AssignmentColumns([]string{"secrets", "encrypted"}),
		}).Create(cred).Error
	})
}

func (c *Client) DeleteGuardrail(ctx context.Context, id string) error {
	if _, err := c.DeleteCredential(ctx, guardrailSecretContext, id); err != nil {
		return err
	}
	return c.db.WithContext(ctx).Where("id = ?", id).Delete(&types.Guardrail{}).Error
}

// GuardrailSecrets returns a guardrail's secrets, or an empty map if it has none.
func (c *Client) GuardrailSecrets(ctx context.Context, id string) (map[string]string, error) {
	cred, err := c.RevealCredential(ctx, []string{guardrailSecretContext}, id)
	if errors.As(err, &CredentialNotFoundError{}) {
		return map[string]string{}, nil
	} else if err != nil {
		return nil, err
	}
	if cred.Secrets == nil {
		return map[string]string{}, nil
	}
	return cred.Secrets, nil
}

// ErrGuardrailNotFound reports whether err is a missing policy or guardrail.
func ErrGuardrailNotFound(err error) bool {
	return errors.Is(err, gorm.ErrRecordNotFound)
}
