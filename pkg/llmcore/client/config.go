package client

import (
	"encoding/json"

	"github.com/maximhq/bifrost/core/schemas"
	configstoreTables "github.com/maximhq/bifrost/framework/configstore/tables"
)

// Config is a decoded desired configuration assembled from independent resources.
// Revision is a local content token, not an ordered database revision.
type Config struct {
	Revision  int64
	Providers map[schemas.ModelProvider]schemas.ProviderConfig
	Keys      map[schemas.ModelProvider][]schemas.Key
	Rules     map[string]configstoreTables.TableRoutingRule
	Settings  map[string]json.RawMessage
}
