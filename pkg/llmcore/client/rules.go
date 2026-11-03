package client

import (
	"context"
	"encoding/json"
	"fmt"

	tables "github.com/maximhq/bifrost/framework/configstore/tables"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/util/retry"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func (s *Client) GetRule(ctx context.Context, id string) (tables.TableRoutingRule, error) {
	var obj v1.LLMGatewayRoutingRule
	if err := s.storage.Get(ctx, resourceKey(id), &obj); err != nil {
		return tables.TableRoutingRule{}, err
	}
	return decodeRule(obj.Spec)
}

func (s *Client) PutRule(ctx context.Context, rule tables.TableRoutingRule) error {
	if rule.ID == "" {
		return fmt.Errorf("a routing rule needs an ID")
	}

	payload, err := json.Marshal(rule)
	if err != nil {
		return err
	}

	return retry.OnError(retry.DefaultRetry, isWriteConflict, func() error {
		var obj v1.LLMGatewayRoutingRule
		err := s.storage.Get(ctx, resourceKey(rule.ID), &obj)
		if err != nil && !apierrors.IsNotFound(err) {
			return err
		}

		exists := err == nil
		if exists && obj.Spec.Payload == string(payload) {
			return nil
		}

		obj.Name = resourceName(rule.ID)
		obj.Namespace = system.DefaultNamespace
		obj.Spec = v1.LLMGatewayResourceSpec{ID: rule.ID, Payload: string(payload)}
		if !exists {
			return s.storage.Create(ctx, &obj)
		}
		return s.storage.Update(ctx, &obj)
	})
}

func (s *Client) DeleteRule(ctx context.Context, id string) error {
	return kclient.IgnoreNotFound(s.storage.Delete(ctx, &v1.LLMGatewayRoutingRule{Name: resourceName(id), Namespace: system.DefaultNamespace}))
}

func decodeRule(spec v1.LLMGatewayResourceSpec) (tables.TableRoutingRule, error) {
	var rule tables.TableRoutingRule
	if err := json.Unmarshal([]byte(spec.Payload), &rule); err != nil {
		return rule, err
	}

	rule.ID = spec.ID
	for i := range rule.Targets {
		rule.Targets[i].RuleID = rule.ID
	}

	if rule.Scope == "" {
		rule.Scope = "global"
	}
	return rule, nil
}
