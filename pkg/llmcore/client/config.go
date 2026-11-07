package client

import (
	"encoding/json"
	"fmt"

	"github.com/maximhq/bifrost/core/schemas"
	configstoreTables "github.com/maximhq/bifrost/framework/configstore/tables"
)

// Config is a decoded desired configuration assembled from independent resources.
type Config struct {
	AzureDiscoveries map[schemas.ModelProvider]map[string]AzureDiscoverySnapshot
	AzureAccounts    map[schemas.ModelProvider]map[string]AzureAccount
	Providers        map[schemas.ModelProvider]schemas.ProviderConfig
	Keys             map[schemas.ModelProvider][]schemas.Key
	Rules            map[string]configstoreTables.TableRoutingRule
	Settings         map[string]json.RawMessage
}

// LoadResult separates usable desired state from providers that could not be read.
// A non-nil error from Load makes the entire result unusable.
type LoadResult struct {
	Config         Config
	ProviderErrors []ProviderLoadError
}

// ProviderLoadError identifies an unavailable aggregate and preserves its underlying error.
// It is distinct from an absent provider, whose resource was successfully observed missing.
type ProviderLoadError struct {
	Provider     schemas.ModelProvider
	Namespace    string
	ResourceName string
	Err          error
}

func (e ProviderLoadError) Error() string {
	return fmt.Sprintf("provider %q (%s/%s): %v", e.Provider, e.Namespace, e.ResourceName, e.Err)
}
func (e ProviderLoadError) Unwrap() error { return e.Err }
