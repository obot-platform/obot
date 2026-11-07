package client

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"

	"github.com/maximhq/bifrost/core/schemas"
	tables "github.com/maximhq/bifrost/framework/configstore/tables"
	"github.com/obot-platform/nah/pkg/name"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// Hash logical IDs so punctuation and case cannot produce invalid resource names;
// SafeConcatName applies the shared naming length and suffix conventions.
func resourceName(id string) string {
	return name.SafeConcatName("gateway", fmt.Sprintf("%x", sha256.Sum256([]byte(id))))
}

// Load assembles independently versioned resources. It does not provide a
// transactional snapshot; runtime reconciliation must tolerate temporary
// missing dependencies and retry until the desired configuration converges.
// Resource watches drive reconciliation; the engine compares desired state
// against what it has successfully applied.
// Unreadable providers are omitted together with all their keys and reported in
// ProviderErrors. Collection, routing-rule, and settings errors are blocking:
// callers must discard the result when the returned error is non-nil.
func (s *Client) Load(ctx context.Context) (LoadResult, error) {
	cfg := Config{
		Providers: map[schemas.ModelProvider]schemas.ProviderConfig{},
		Keys:      map[schemas.ModelProvider][]schemas.Key{},
		Rules:     map[string]tables.TableRoutingRule{},
		Settings:  map[string]json.RawMessage{},
	}

	result := LoadResult{}
	var providers v1.LLMGatewayProviderList
	var rules v1.LLMGatewayRoutingRuleList
	for _, list := range []kclient.ObjectList{&providers, &rules} {
		if err := s.storage.List(ctx, list, kclient.InNamespace(system.DefaultNamespace)); err != nil {
			return LoadResult{}, err
		}
	}

	for i := range providers.Items {
		p := &providers.Items[i]

		var data Provider
		if err := s.decode(ctx, providerConfigID(p.Spec.ID), p.Spec.Payload, &data); err != nil {
			result.ProviderErrors = append(result.ProviderErrors, ProviderLoadError{Provider: schemas.ModelProvider(p.Spec.ID), Namespace: p.Namespace, ResourceName: p.Name, Err: err})
			continue
		}

		name := schemas.ModelProvider(p.Spec.ID)
		if cfg.AzureAccounts == nil {
			cfg.AzureAccounts = map[schemas.ModelProvider]map[string]AzureAccount{}
		}

		cfg.AzureAccounts[name] = data.AzureAccounts
		if cfg.AzureDiscoveries == nil {
			cfg.AzureDiscoveries = map[schemas.ModelProvider]map[string]AzureDiscoverySnapshot{}
		}

		cfg.AzureDiscoveries[name] = data.AzureDiscoveries
		cfg.Providers[name] = data.Config
		cfg.Keys[name] = data.Keys
	}

	for i := range rules.Items {
		r := &rules.Items[i]

		rule, err := decodeRule(r.Spec)
		if err != nil {
			return LoadResult{}, fmt.Errorf("routing rule %q (%s/%s): %w", r.Spec.ID, r.Namespace, r.Name, err)
		}

		cfg.Rules[rule.ID] = rule
	}

	settings, err := s.GetSettings(ctx)
	if err != nil {
		return LoadResult{}, fmt.Errorf("global settings %s/%s: %w", system.DefaultNamespace, system.LLMGatewayConfigName, err)
	}
	cfg.Settings = settings
	result.Config = cfg
	return result, nil
}
