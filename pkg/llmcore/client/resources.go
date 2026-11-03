package client

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/maximhq/bifrost/core/schemas"
	tables "github.com/maximhq/bifrost/framework/configstore/tables"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func resourceName(id string) string { return fmt.Sprintf("gateway-%x", sha256.Sum256([]byte(id)))[:56] }

func (s *Client) loadResources(ctx context.Context) (Config, error) {
	cfg := Config{Providers: map[schemas.ModelProvider]schemas.ProviderConfig{}, Keys: map[schemas.ModelProvider][]schemas.Key{}, Rules: map[string]tables.TableRoutingRule{}, Settings: map[string]json.RawMessage{}}
	var providers v1.LLMGatewayProviderList
	var rules v1.LLMGatewayRoutingRuleList
	var settings v1.LLMGatewayConfigList
	for _, list := range []kclient.ObjectList{&providers, &rules, &settings} {
		if err := s.storage.List(ctx, list, kclient.InNamespace(system.DefaultNamespace)); err != nil {
			return cfg, err
		}
	}
	var objects []kclient.Object
	for i := range providers.Items {
		p := &providers.Items[i]
		var data Provider
		if err := s.decode(ctx, "provider/"+p.Spec.ID, p.Spec.Payload, &data); err != nil {
			return cfg, err
		}
		name := schemas.ModelProvider(p.Spec.ID)
		cfg.Providers[name] = data.Config
		cfg.Keys[name] = data.Keys
		objects = append(objects, p)
	}
	for i := range rules.Items {
		r := &rules.Items[i]
		rule, err := decodeRule(r.Spec)
		if err != nil {
			return cfg, err
		}
		cfg.Rules[rule.ID] = rule
		objects = append(objects, r)
	}
	for i := range settings.Items {
		r := &settings.Items[i]
		if err := s.decode(ctx, "settings", r.Spec.Payload, &cfg.Settings); err != nil {
			return cfg, err
		}
		objects = append(objects, r)
	}
	// Revision is process-local content identity, never a parsed resourceVersion.
	identities := make([]string, 0, len(objects))
	for _, o := range objects {
		identities = append(identities, fmt.Sprintf("%T/%s/%s/%s", o, o.GetName(), o.GetUID(), o.GetResourceVersion()))
	}
	sort.Strings(identities)
	raw, _ := json.Marshal(struct {
		Config     Config
		Identities []string
	}{cfg, identities})
	hash := sha256.Sum256(raw)
	for _, b := range hash[:7] {
		cfg.Revision = cfg.Revision<<8 | int64(b)
	}
	return cfg, nil
}
