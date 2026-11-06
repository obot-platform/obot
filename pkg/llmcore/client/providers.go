package client

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/maximhq/bifrost/core/schemas"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"k8s.io/client-go/util/retry"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// AzureDiscoveryReservationAnnotation distinguishes ordering writes from discovery/configuration changes.
const AzureDiscoveryReservationAnnotation = "obot.ai/azure-discovery-reservation"

// AzureAccount identifies the account used for management-plane deployment discovery.
type AzureAccount struct {
	SubscriptionID string `json:"azure_subscription_id,omitempty"`
	ResourceGroup  string `json:"azure_resource_group,omitempty"`
	AccountName    string `json:"azure_account_name,omitempty"`
}

func (a AzureAccount) ResourceID() string {
	if a == (AzureAccount{}) {
		return ""
	}

	return "/subscriptions/" + a.SubscriptionID + "/resourceGroups/" + a.ResourceGroup + "/providers/Microsoft.CognitiveServices/accounts/" + a.AccountName
}

// AzureDiscoverySnapshot records only successful discovery, including a confirmed empty result.
// Keeping it with the key allows another replica to retain deployments during an ARM outage.
type AzureDiscoverySnapshot struct {
	Fingerprint [32]byte
	Names       []string
	BaseModels  map[string]string
}

// AzureDiscoveryFingerprint binds a snapshot to the effective authentication and account.
// Policies are applied when listing, so policy edits need no new ARM request.
func AzureDiscoveryFingerprint(key schemas.Key, account AzureAccount) [32]byte {
	// Hash effective values, not SecretVar encoding: an empty value gains a type
	// during decoding, and that representation change must not invalidate a snapshot.
	var credentials, scopes []string
	if c := key.AzureKeyConfig; c != nil {
		credentials = []string{c.Endpoint.GetValue(), c.ClientID.GetValue(), c.ClientSecret.GetValue(), c.TenantID.GetValue()}
		scopes = c.Scopes
	}

	raw, _ := json.Marshal(struct {
		Account     AzureAccount
		Value       string
		Credentials []string
		Scopes      []string `json:",omitempty"`
	}{Account: account, Value: key.Value.GetValue(), Credentials: credentials, Scopes: scopes})

	return sha256.Sum256(raw)
}

// Provider is one atomic configuration aggregate: provider settings and its keys.
type Provider struct {
	// Ordering is storage-only: reservations must not change the runtime view or trigger model-sync loops.
	AzureRefreshGenerations map[string]uint64                 `json:",omitempty"`
	AzureAccounts           map[string]AzureAccount           `json:",omitempty"`
	AzureDiscoveries        map[string]AzureDiscoverySnapshot `json:",omitempty"`
	Config                  schemas.ProviderConfig
	Keys                    []schemas.Key
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
	return s.mutateProvider(ctx, name, change, false)
}

func (s *Client) mutateProvider(ctx context.Context, name string, change func(*Provider) error, reservation bool) error {
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

	// Remove snapshots as part of the same atomic edit that changes or removes a key.

	for id, snapshot := range provider.AzureDiscoveries {
		i := slices.IndexFunc(provider.Keys, func(key schemas.Key) bool { return key.ID == id })
		if i < 0 || provider.AzureAccounts[id] == (AzureAccount{}) || snapshot.Fingerprint != AzureDiscoveryFingerprint(provider.Keys[i], provider.AzureAccounts[id]) {
			delete(provider.AzureDiscoveries, id)
		}
	}

	for id := range provider.AzureRefreshGenerations {
		if !slices.ContainsFunc(provider.Keys, func(key schemas.Key) bool { return key.ID == id }) {
			delete(provider.AzureRefreshGenerations, id)
		}
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
	if reservation {
		if obj.Annotations == nil {
			obj.Annotations = map[string]string{}
		}
		obj.Annotations[AzureDiscoveryReservationAnnotation] = "true"
	} else {
		delete(obj.Annotations, AzureDiscoveryReservationAnnotation)
	}

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
		delete(provider.AzureAccounts, id)
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

// BeginAzureDiscovery reserves publication order in shared storage before fetching ARM.
// A newer reservation supersedes older workers even if that newer fetch subsequently fails.
func (s *Client) BeginAzureDiscovery(ctx context.Context, name, id string, fingerprint [32]byte) (uint64, error) {
	var generation uint64
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		generation = 0

		return s.mutateProvider(ctx, name, func(provider *Provider) error {
			for _, key := range provider.Keys {
				if key.ID == id && provider.AzureAccounts[id] != (AzureAccount{}) && (key.Enabled == nil || *key.Enabled) && fingerprint == AzureDiscoveryFingerprint(key, provider.AzureAccounts[id]) {
					if provider.AzureRefreshGenerations == nil {
						provider.AzureRefreshGenerations = map[string]uint64{}
					}

					provider.AzureRefreshGenerations[id]++
					generation = provider.AzureRefreshGenerations[id]
					break
				}
			}

			return nil
		}, true)
	})

	return generation, err
}

// SaveAzureDiscovery rejects superseded refreshes and concurrent key/account edits.
// MutateProvider skips identical payloads, so unchanged listings do not trigger watch loops.
func (s *Client) SaveAzureDiscovery(ctx context.Context, name, id string, generation uint64, snapshot AzureDiscoverySnapshot) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		return s.MutateProvider(ctx, name, func(provider *Provider) error {
			if generation == 0 || provider.AzureRefreshGenerations[id] != generation {
				return nil
			}

			for _, key := range provider.Keys {
				if key.ID == id && provider.AzureAccounts[id] != (AzureAccount{}) && (key.Enabled == nil || *key.Enabled) && snapshot.Fingerprint == AzureDiscoveryFingerprint(key, provider.AzureAccounts[id]) {
					if provider.AzureDiscoveries == nil {
						provider.AzureDiscoveries = map[string]AzureDiscoverySnapshot{}
					}

					provider.AzureDiscoveries[id] = snapshot
					break
				}
			}

			return nil
		})
	})
}
