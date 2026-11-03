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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/util/retry"
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
	err := s.decode(ctx, "provider/"+name, obj.Spec.Payload, &provider)
	return provider, err
}

// PutProvider replaces a provider and its keys, or creates the aggregate if absent.
func (s *Client) PutProvider(ctx context.Context, name string, provider Provider) error {
	if name == "" {
		return fmt.Errorf("a provider needs a name")
	}
	return s.editProvider(ctx, name, true, func(p *Provider) error { *p = provider; return nil })
}

// MutateProvider changes an existing provider atomically. On conflict, change is
// called again with the latest settings and keys; it must not have side effects.
func (s *Client) MutateProvider(ctx context.Context, name string, change func(*Provider) error) error {
	return s.editProvider(ctx, name, false, change)
}

func (s *Client) editProvider(ctx context.Context, name string, create bool, change func(*Provider) error) error {
	return retry.OnError(retry.DefaultRetry, isWriteConflict, func() error {
		obj := v1.LLMGatewayProvider{}
		err := s.storage.Get(ctx, resourceKey(name), &obj)
		exists := err == nil
		if err != nil && (!apierrors.IsNotFound(err) || !create) {
			return err
		}

		var provider Provider
		if exists {
			if err := s.decode(ctx, "provider/"+name, obj.Spec.Payload, &provider); err != nil {
				return err
			}
		}

		before, err := json.Marshal(provider)
		if err != nil {
			return err
		}
		if err := change(&provider); err != nil {
			return err
		}

		ids := map[string]bool{}
		for _, key := range provider.Keys {
			if key.ID == "" {
				return fmt.Errorf("a key needs an ID")
			}
			if ids[key.ID] {
				return fmt.Errorf("duplicate key ID %q", key.ID)
			}
			ids[key.ID] = true
		}

		after, err := json.Marshal(provider)
		if err != nil {
			return err
		}
		if exists && bytes.Equal(before, after) {
			return nil
		}

		payload, err := s.encode(ctx, "provider/"+name, provider)
		if err != nil {
			return err
		}

		obj.Name = resourceName(name)
		obj.Namespace = system.DefaultNamespace
		obj.Spec = v1.LLMGatewayResourceSpec{ID: name, Payload: payload}
		if !exists {
			return s.storage.Create(ctx, &obj)
		}
		return s.storage.Update(ctx, &obj)
	})
}

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

func (s *Client) DeleteKey(ctx context.Context, name, id string) error {
	return kclient.IgnoreNotFound(s.MutateProvider(ctx, name, func(provider *Provider) error {
		provider.Keys = slices.DeleteFunc(provider.Keys, func(key schemas.Key) bool { return key.ID == id })
		return nil
	}))
}

func resourceKey(name string) kclient.ObjectKey {
	return kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: resourceName(name)}
}

func isWriteConflict(err error) bool {
	return apierrors.IsConflict(err) || apierrors.IsAlreadyExists(err)
}
