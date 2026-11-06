package llmengine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"slices"
	"time"

	"github.com/maximhq/bifrost/core/schemas"
	llmcoreclient "github.com/obot-platform/obot/pkg/llmcore/client"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/util/retry"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// legacyImportAnnotation marks the gateway's ModelProvider once the credentials of legacy
	// model providers have been imported, which happens once per installation.
	legacyImportAnnotation = "obot.ai/llm-gateway-legacy-import-v2"
)

var (
	// envProviders are the upstream providers configured from environment variables at startup.
	envProviders = []struct{ envVar, provider string }{
		{envVar: system.OpenAIAPIKeyEnvVar, provider: "openai"},
		{envVar: system.AnthropicAPIKeyEnvVar, provider: "anthropic"},
	}
)

// envConfigured returns the environment variable the upstream provider is configured from at
// startup, when it is set.
func envConfigured(provider string) string {
	for _, p := range envProviders {
		if p.provider == provider && os.Getenv(p.envVar) != "" {
			return p.envVar
		}
	}
	return ""
}

// TriggerModelSync flips the sync annotation on the gateway's ModelProvider so
// the provider controller re-lists models. The provider controller updates the same
// resource while it syncs, so a conflicting write is retried against the latest version.
func TriggerModelSync(ctx context.Context, c kclient.Client) error {
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		var mp v1.ModelProvider
		if err := c.Get(ctx, kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: system.LLMGatewayModelProvider}, &mp); err != nil {
			return err
		}
		if mp.Annotations[v1.ModelProviderSyncAnnotation] == "" {
			if mp.Annotations == nil {
				mp.Annotations = map[string]string{}
			}
			mp.Annotations[v1.ModelProviderSyncAnnotation] = "true"
		} else {
			delete(mp.Annotations, v1.ModelProviderSyncAnnotation)
		}
		return c.Update(ctx, &mp)
	})
}

// importEnvProviders adds the providers whose API key is set in the environment
// (OPENAI_API_KEY, ANTHROPIC_API_KEY), and updates their key when the variable changes. An
// annotation on the gateway's ModelProvider records a hash of each imported key, so an
// unchanged key is not written on every start.
func (e *Engine) importEnvProviders(ctx context.Context, c kclient.Client, mp *v1.ModelProvider) bool {
	changed := false
	for _, p := range envProviders {
		value := os.Getenv(p.envVar)
		if value == "" {
			continue
		}
		sum := sha256.Sum256([]byte(value))
		hash := hex.EncodeToString(sum[:])
		annotation := "obot.ai/llm-gateway-env-key-" + p.provider
		if mp.Annotations[annotation] == hash {
			continue
		}
		keyName := "env-" + p.envVar
		key := map[string]any{"name": keyName, "value": value, "models": []string{"*"}, "weight": 1}
		plainTextSecrets(key)

		// Retry edits against the latest aggregate. A concurrent creator wins intact;
		// reload it and upsert only the named environment key rather than replacing it.
		update := func(provider *llmcoreclient.Provider) error {
			cfg := llmcoreclient.Config{Providers: map[schemas.ModelProvider]schemas.ProviderConfig{schemas.ModelProvider(p.provider): provider.Config}, Keys: map[schemas.ModelProvider][]schemas.Key{schemas.ModelProvider(p.provider): provider.Keys}}
			id, method := "", http.MethodPost
			if i := slices.IndexFunc(provider.Keys, func(k schemas.Key) bool { return k.Name == keyName }); i >= 0 {
				id = provider.Keys[i].ID
				method = http.MethodPut
			}
			body, _ := json.Marshal(key)
			if _, err := updateKey(&cfg, schemas.ModelProvider(p.provider), provider.Config, id, method, body); err != nil {
				return err
			}
			provider.Keys = cfg.Keys[schemas.ModelProvider(p.provider)]
			return nil
		}
		// Storage attempts one write. This automatic import owns bounded conflict
		// retries, rerunning the provider lookup and key merge against fresh state.
		err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			if err := ctx.Err(); err != nil {
				return err
			}
			err := e.store.MutateProvider(ctx, p.provider, update)
			if apierrors.IsNotFound(err) {
				var provider llmcoreclient.Provider
				defaults := map[string]any{}
				ApplyProviderDefaults(defaults)
				raw, _ := json.Marshal(defaults)
				err = json.Unmarshal(raw, &provider.Config)
				if err == nil {
					provider.Config.CheckAndSetDefaults()
					err = update(&provider)
				}
				if err == nil {
					err = e.store.CreateProvider(ctx, p.provider, provider)
				}
				if apierrors.IsAlreadyExists(err) {
					err = e.store.MutateProvider(ctx, p.provider, update)
				}
			}
			return err
		})
		if err != nil {
			slog.Warn("failed to configure an LLM gateway provider from the environment", "provider", p.provider, "envVar", p.envVar, "error", err)
			continue
		}
		slog.Info("configured an LLM gateway provider from the environment", "provider", p.provider, "envVar", p.envVar)
		changed = true
		if err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			if err := c.Get(ctx, kclient.ObjectKeyFromObject(mp), mp); err != nil {
				return err
			}
			if mp.Annotations == nil {
				mp.Annotations = map[string]string{}
			}
			mp.Annotations[annotation] = hash
			return c.Update(ctx, mp)
		}); err != nil {
			slog.Warn("failed to record the LLM gateway provider configured from the environment", "provider", p.provider, "error", err)
		}
	}
	return changed
}

// providerKeys lists a provider's keys, as the engine lists them (with secrets redacted).
func (e *Engine) providerKeys(ctx context.Context, provider string) ([]map[string]any, error) {
	keys, err := e.ListProviderKeys(ctx, provider)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(keys))
	for _, key := range keys {
		out = append(out, object(key))
	}
	return out, nil
}

// LegacyImportDone reports whether the credentials of legacy model providers have been
// imported into the gateway, whose ModelProvider is mp.
func LegacyImportDone(mp *v1.ModelProvider) bool {
	return mp.Annotations[legacyImportAnnotation] != ""
}

// RunStartupTasks configures environment keys and periodically asks Obot to refresh models.
func (e *Engine) RunStartupTasks(ctx context.Context, c kclient.Client) {
	if err := e.Wait(ctx); err != nil {
		return
	}

	var mp v1.ModelProvider
	for {
		err := c.Get(ctx, kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: system.LLMGatewayModelProvider}, &mp)
		if err == nil {
			break
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(2 * time.Second):
		}
	}

	changed, _ := e.startupChanges(ctx, c, &mp)()
	if changed {
		if _, err := e.Sync(ctx); err != nil {
			slog.Warn("LLM gateway startup sync failed", "error", err)
		}
	}

	// Bifrost refreshes provider model lists on its own schedule, so resync periodically.
	ticker := time.NewTicker(15 * time.Minute)
	defer ticker.Stop()
	for {
		if err := TriggerModelSync(ctx, c); err != nil {
			slog.Warn("failed to trigger LLM gateway model sync", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// startupChanges configures providers from the environment and applies the egress proxy.
// Legacy credential migration is disabled until upgrades from v0.27 are supported.
func (e *Engine) startupChanges(ctx context.Context, c kclient.Client, mp *v1.ModelProvider) func() (bool, bool) {
	return func() (bool, bool) {
		changed := e.importEnvProviders(ctx, c, mp)
		if e.ApplyEgressProxy(ctx) {
			changed = true
		}
		return changed, true
	}
}

// recordLegacyImport marks the legacy import as done on the gateway's ModelProvider. A
// conflicting write, as when the provider controller updates it at the same time, is retried:
// until the import is recorded, legacy models and providers aren't cleaned up, and the next
// start imports again.
func recordLegacyImport(ctx context.Context, c kclient.Client, mp *v1.ModelProvider) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		if err := c.Get(ctx, kclient.ObjectKeyFromObject(mp), mp); err != nil {
			return err
		}
		if mp.Annotations == nil {
			mp.Annotations = map[string]string{}
		}
		mp.Annotations[legacyImportAnnotation] = "true"
		return c.Update(ctx, mp)
	})
}
