package client

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"

	"github.com/maximhq/bifrost/core/schemas"
	tables "github.com/maximhq/bifrost/framework/configstore/tables"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// Hash arbitrary logical IDs into resource names. The 56-character result
// (prefix plus 48 hex digits) stays below the 63-character DNS-label limit.
func resourceName(id string) string { return fmt.Sprintf("gateway-%x", sha256.Sum256([]byte(id)))[:56] }

// Load assembles independently versioned resources. It does not provide a
// transactional snapshot; runtime reconciliation must tolerate temporary
// missing dependencies and retry until the desired configuration converges.
// Resource watches drive reconciliation; the engine compares desired state
// against what it has successfully applied.
func (s *Client) Load(ctx context.Context) (Config, error) {
	cfg := Config{
		Providers: map[schemas.ModelProvider]schemas.ProviderConfig{},
		Keys:      map[schemas.ModelProvider][]schemas.Key{},
		Rules:     map[string]tables.TableRoutingRule{},
		Settings:  map[string]json.RawMessage{},
	}

	var providers v1.LLMGatewayProviderList
	var rules v1.LLMGatewayRoutingRuleList
	for _, list := range []kclient.ObjectList{&providers, &rules} {
		if err := s.storage.List(ctx, list, kclient.InNamespace(system.DefaultNamespace)); err != nil {
			return cfg, err
		}
	}

	for i := range providers.Items {
		p := &providers.Items[i]

		var data Provider
		if err := s.decode(ctx, providerConfigID(p.Spec.ID), p.Spec.Payload, &data); err != nil {
			return cfg, err
		}

		name := schemas.ModelProvider(p.Spec.ID)
		cfg.Providers[name] = data.Config
		cfg.Keys[name] = data.Keys
	}

	for i := range rules.Items {
		r := &rules.Items[i]

		rule, err := decodeRule(r.Spec)
		if err != nil {
			return cfg, err
		}

		cfg.Rules[rule.ID] = rule
	}

	settings, err := s.GetSettings(ctx)
	if err != nil {
		return cfg, err
	}
	cfg.Settings = settings
	return cfg, nil
}
