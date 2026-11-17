package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	gcontext "github.com/obot-platform/obot/pkg/gateway/context"
	llmcoreclient "github.com/obot-platform/obot/pkg/llmcore/client"
	"gorm.io/gorm"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/util/retry"
)

// MigrateLLMGatewaySubscriptions copies the legacy Claude flag before engine readiness.
// The marker avoids reading the retired property again after a successful startup.
func (c *Client) MigrateLLMGatewaySubscriptions(ctx context.Context, store *llmcoreclient.Client) error {
	return c.MigrateKinmIfNotRun(ctx, "provider-owned-subscription-passthrough", func() error {
		legacy := map[string]json.RawMessage{}
		property, err := c.GetProperty(ctx, "llm-gateway-settings")
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err == nil {
			if err := json.Unmarshal([]byte(property.Value), &legacy); err != nil || legacy == nil {
				gcontext.GetLogger(ctx).Warn("Invalid legacy LLM gateway settings object; migrating subscription passthrough as disabled")
				legacy = nil
			}
		}

		// Malformed legacy settings must neither enable subscriptions nor block startup.
		allowed := false
		if raw, ok := legacy["subscriptionPassthrough"]; ok {
			if string(raw) == "null" || json.Unmarshal(raw, &allowed) != nil {
				gcontext.GetLogger(ctx).Warn("Invalid legacy subscriptionPassthrough setting; migrating subscription passthrough as disabled")
				allowed = false
			}
		}

		// Replicas may run together. Re-read on conflict and preserve an explicit
		// migrated/admin value, even when another replica has since revoked it.
		err = retry.RetryOnConflict(retry.DefaultBackoff, func() error {
			return store.MutateProvider(ctx, "anthropic", func(provider *llmcoreclient.Provider) error {
				if provider.SubscriptionPassthrough == nil && provider.Config.CustomProviderConfig == nil {
					provider.SubscriptionPassthrough = new(allowed)
				}
				return nil
			})
		})
		if err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("migrate anthropic subscriptions: %w", err)
		}
		return nil
	})
}
