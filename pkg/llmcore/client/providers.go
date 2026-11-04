package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/maximhq/bifrost/core/schemas"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// Provider is one atomic configuration aggregate: provider settings and its keys.
type Provider struct {
	Config schemas.ProviderConfig
	Keys   []schemas.Key
}

func (s *Client) GetProvider(ctx context.Context, name string) (Provider, error) {
	var obj v1.LLMGatewayProvider
	if err := s.storage.Get(ctx, resourceKey(name), &obj); err != nil {
		return Provider{}, err
	}

	var provider Provider
	err := s.decode(ctx, providerConfigID(name), obj.Spec.Payload, &provider)
	return provider, err
}

// CreateProvider creates an aggregate without replacing an existing provider or its keys.
// Callers handling concurrent creation must reload and mutate the winning aggregate.
func (s *Client) CreateProvider(ctx context.Context, name string, provider Provider) error {
	if name == "" {
		return fmt.Errorf("a provider needs a name")
	}
	if err := validateKeys(provider.Keys); err != nil {
		return err
	}
	payload, err := s.encode(ctx, providerConfigID(name), provider)
	if err != nil {
		return err
	}
	return s.storage.Create(ctx, &v1.LLMGatewayProvider{
		Name:      resourceName(name),
		Namespace: system.DefaultNamespace,
		Spec:      v1.LLMGatewayResourceSpec{ID: name, Payload: payload},
	})
}

// MutateProvider attempts one atomic edit of the latest provider and its keys.
// Conflicts are returned to the caller, which can explicitly retry the operation.
// The callback must not have side effects because its write may fail.
func (s *Client) MutateProvider(ctx context.Context, name string, change func(*Provider) error) error {
	obj := v1.LLMGatewayProvider{}
	err := s.storage.Get(ctx, resourceKey(name), &obj)
	if err != nil {
		return err
	}

	var provider Provider
	if err := s.decode(ctx, providerConfigID(name), obj.Spec.Payload, &provider); err != nil {
		return err
	}

	before, err := json.Marshal(provider)
	if err != nil {
		return err
	}
	if err := change(&provider); err != nil {
		return err
	}

	if err := validateKeys(provider.Keys); err != nil {
		return err
	}

	after, err := json.Marshal(provider)
	if err != nil {
		return err
	}
	if bytes.Equal(before, after) {
		return nil
	}

	payload, err := s.encode(ctx, providerConfigID(name), provider)
	if err != nil {
		return err
	}

	obj.Spec = v1.LLMGatewayResourceSpec{ID: name, Payload: payload}
	return s.storage.Update(ctx, &obj)
}

// DeleteProvider removes the provider and its keys but retains referencing rules.
// Reconcilers must tolerate missing targets without silently bypassing routing policy.
func (s *Client) DeleteProvider(ctx context.Context, name string) error {
	return kclient.IgnoreNotFound(s.storage.Delete(ctx, &v1.LLMGatewayProvider{Name: resourceName(name), Namespace: system.DefaultNamespace}))
}

func (s *Client) PutKey(ctx context.Context, name string, key schemas.Key) error {
	return s.MutateProvider(ctx, name, func(provider *Provider) error {
		for i, k := range provider.Keys {
			if k.ID == key.ID {
				provider.Keys[i] = key
				return nil
			}
		}
		provider.Keys = append(provider.Keys, key)
		return nil
	})
}

// DeleteKey requires an existing provider. An already-absent key is a successful
// no-op, but a missing provider or one removed during the update returns NotFound.
func (s *Client) DeleteKey(ctx context.Context, name, id string) error {
	return s.MutateProvider(ctx, name, func(provider *Provider) error {
		provider.Keys = slices.DeleteFunc(provider.Keys, func(key schemas.Key) bool { return key.ID == id })
		return nil
	})
}

func resourceKey(name string) kclient.ObjectKey {
	return kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: resourceName(name)}
}

// providerConfigID is the stable codec identity used to seal and open a provider.
func providerConfigID(name string) string {
	return "provider/" + name
}

// validateKeys enforces storage identity, not provider-specific authentication.
// Providers may have no keys, and key values may be empty when credentials come
// from provider-specific fields or ambient authentication. Callers and the runtime
// are responsible for validating authentication for the selected provider.
func validateKeys(keys []schemas.Key) error {
	ids := map[string]struct{}{}
	for _, key := range keys {
		if key.ID == "" {
			return fmt.Errorf("a key needs an ID")
		}
		if _, exists := ids[key.ID]; exists {
			return fmt.Errorf("duplicate key ID %q", key.ID)
		}
		ids[key.ID] = struct{}{}
	}
	return nil
}
