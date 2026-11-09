package llmcore

import (
	"context"

	llmcoreclient "github.com/obot-platform/obot/pkg/llmcore/client"
)

type modelListingConfigKey struct{}

// ModelListingConfig returns the immutable live snapshot read by the listing worker
// when it acquired keys. Hook-time snapshots cannot detect queued key-only rotations.
// Keyless and short-circuited listings do not acquire keys and return nil.
func ModelListingConfig(ctx context.Context) *llmcoreclient.Config {
	cfg, _ := ctx.Value(modelListingConfigKey{}).(*llmcoreclient.Config)
	return cfg
}
