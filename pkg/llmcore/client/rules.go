package client

import (
	"context"
	"encoding/json"
	"fmt"

	tables "github.com/maximhq/bifrost/framework/configstore/tables"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func (s *Client) GetRule(ctx context.Context, id string) (tables.TableRoutingRule, error) {
	var obj v1.LLMGatewayRoutingRule
	if err := s.storage.Get(ctx, resourceKey(id), &obj); err != nil {
		return tables.TableRoutingRule{}, err
	}
	return decodeRule(obj.Spec)
}

// PutRule replaces or creates a complete routing rule in one attempt.
// Callers decide whether to retry conflicts or concurrent creation (AlreadyExists).
func (s *Client) PutRule(ctx context.Context, rule tables.TableRoutingRule) error {
	return s.editRule(ctx, rule.ID, true, func(current *tables.TableRoutingRule) error { *current = rule; return nil })
}

// MutateRule attempts one partial edit of the latest rule and returns conflicts
// to the caller. The callback must have no external side effects.
func (s *Client) MutateRule(ctx context.Context, id string, change func(*tables.TableRoutingRule) error) error {
	return s.editRule(ctx, id, false, change)
}

func (s *Client) editRule(ctx context.Context, id string, create bool, change func(*tables.TableRoutingRule) error) error {
	if id == "" {
		return fmt.Errorf("a routing rule needs an ID")
	}
	var obj v1.LLMGatewayRoutingRule
	err := s.storage.Get(ctx, resourceKey(id), &obj)
	exists := err == nil
	if err != nil && (!apierrors.IsNotFound(err) || !create) {
		return err
	}
	var rule tables.TableRoutingRule
	if exists {
		rule, err = decodeRule(obj.Spec)
		if err != nil {
			return err
		}
	}
	if err := change(&rule); err != nil {
		return err
	}
	rule.ID = id
	// Rules are non-secret configuration stored as plaintext JSON, without
	// credential-envelope identity binding. Callers must keep credentials
	// out of CEL expressions and header-match literals.
	payload, err := json.Marshal(rule)
	if err != nil {
		return err
	}
	if exists && obj.Spec.Payload == string(payload) {
		return nil
	}
	obj.Spec = v1.LLMGatewayResourceSpec{ID: id, Payload: string(payload)}
	if !exists {
		obj.Name = resourceName(id)
		obj.Namespace = system.DefaultNamespace
		return s.storage.Create(ctx, &obj)
	}
	return s.storage.Update(ctx, &obj)
}

func (s *Client) DeleteRule(ctx context.Context, id string) error {
	return kclient.IgnoreNotFound(s.storage.Delete(ctx, &v1.LLMGatewayRoutingRule{Name: resourceName(id), Namespace: system.DefaultNamespace}))
}

func decodeRule(spec v1.LLMGatewayResourceSpec) (tables.TableRoutingRule, error) {
	var rule tables.TableRoutingRule
	if err := json.Unmarshal([]byte(spec.Payload), &rule); err != nil {
		return rule, err
	}

	// The resource spec owns rule identity; any ID in the payload is ignored.
	// Target RuleIDs are omitted from JSON and must be restored for the engine.
	rule.ID = spec.ID
	for i := range rule.Targets {
		rule.Targets[i].RuleID = rule.ID
	}

	if rule.Scope == "" {
		rule.Scope = "global"
	}
	return rule, nil
}
