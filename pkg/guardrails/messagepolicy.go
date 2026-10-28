package guardrails

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/obot-platform/obot/apiclient/types"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
)

// LegacyPolicy is a message policy to convert into a guardrail.
type LegacyPolicy struct {
	ID       string
	Manifest types.MessagePolicyManifest
}

// MigrationStore creates the guardrails message policies become.
type MigrationStore interface {
	GetGuardrailPolicy(ctx context.Context, id string) (*gatewaytypes.GuardrailPolicy, error)
	GetGuardrail(ctx context.Context, id string) (*gatewaytypes.Guardrail, error)
	CreateGuardrail(ctx context.Context, guardrail *gatewaytypes.Guardrail, secrets map[string]string) error
	CreateGuardrailPolicy(ctx context.Context, policy *gatewaytypes.GuardrailPolicy) error
}

// ProfileFor returns the gateway profile a converted message policy's guardrail policy belongs to: one
// for everyone if the policy applied to everyone, otherwise one with the policy's subjects
// (none, for a policy that applied to no one).
type ProfileFor func(ctx context.Context, policy LegacyPolicy) (string, error)

// MigrateMessagePolicies converts message policies into guardrails: each becomes an LLM
// judge guardrail with its definition and a blocking guardrail policy in a profile with its subjects. Policies on user
// messages check model input, and policies on tool calls check model output. The guardrail policies
// fail closed, as message policies did.
//
// The guardrail policies are created disabled: message policies were an opt-in feature, so
// an administrator reviews the converted guardrail policies and enables the ones they want.
func MigrateMessagePolicies(ctx context.Context, store MigrationStore, policies []LegacyPolicy, profileFor ProfileFor) error {
	for _, p := range policies {
		// Skip policies converted by an earlier, interrupted run.
		if existing, err := store.GetGuardrailPolicy(ctx, "mp-"+p.ID); err == nil && existing != nil {
			continue
		}
		m := p.Manifest
		config, err := json.Marshal(map[string]any{"definition": m.Definition})
		if err != nil {
			return err
		}
		guardrail := &gatewaytypes.Guardrail{
			ID:          "mp-" + p.ID,
			Name:        m.DisplayName,
			Description: "Converted from a message policy.",
			Type:        JudgeType,
			Config:      config,
		}
		// The guardrail may exist from an earlier, interrupted run.
		if existing, err := store.GetGuardrail(ctx, guardrail.ID); err != nil || existing == nil {
			if err := store.CreateGuardrail(ctx, guardrail, nil); err != nil {
				return fmt.Errorf("failed to convert message policy %s: %w", p.ID, err)
			}
		}

		applyTo := gatewaytypes.GuardrailApplyBoth
		switch m.Direction {
		case types.PolicyDirectionUserMessage:
			applyTo = gatewaytypes.GuardrailApplyInput
		case types.PolicyDirectionToolCalls:
			applyTo = gatewaytypes.GuardrailApplyOutput
		}
		profileID, err := profileFor(ctx, p)
		if err != nil {
			return fmt.Errorf("failed to create a profile for message policy %s: %w", p.ID, err)
		}
		converted := &gatewaytypes.GuardrailPolicy{
			ID:           "mp-" + p.ID,
			Name:         m.DisplayName,
			Description:  "Converted from a message policy. Enable it to apply it.",
			Enabled:      false,
			Target:       gatewaytypes.GuardrailTargetLLM,
			ApplyTo:      applyTo,
			Action:       gatewaytypes.GuardrailActionBlock,
			SamplingRate: 100,
			InputScope:   gatewaytypes.GuardrailScopeLatest,
			// Message policies blocked when the judge couldn't decide, for example because
			// its model was unavailable; keep that, rather than letting traffic through.
			FailClosed:   true,
			GuardrailIDs: []string{guardrail.ID},
			ProfileID:    profileID,
		}
		if err := store.CreateGuardrailPolicy(ctx, converted); err != nil {
			return fmt.Errorf("failed to convert message policy %s: %w", p.ID, err)
		}
	}
	return nil
}
