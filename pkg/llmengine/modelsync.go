package llmengine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strings"
	"time"

	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
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

// providerKey is a gateway provider key as listed by the engine.
type providerKey struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

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
// unchanged key isn't written, and announced to other replicas, on every start.
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

		var keys struct {
			Keys []providerKey `json:"keys"`
		}
		var err error
		if err = e.Do(ctx, http.MethodGet, "/api/providers/"+url.PathEscape(p.provider)+"/keys", nil, &keys); err != nil {
			provider := map[string]any{"provider": p.provider}
			ApplyProviderDefaults(provider)
			if err = e.Do(ctx, http.MethodPost, "/api/providers", provider, nil); err == nil {
				err = e.Do(ctx, http.MethodPost, "/api/providers/"+url.PathEscape(p.provider)+"/keys", key, nil)
			}
		} else if i := slices.IndexFunc(keys.Keys, func(k providerKey) bool { return k.Name == keyName }); i >= 0 {
			err = e.Do(ctx, http.MethodPut, "/api/providers/"+url.PathEscape(p.provider)+"/keys/"+url.PathEscape(keys.Keys[i].ID), key, nil)
		} else {
			err = e.Do(ctx, http.MethodPost, "/api/providers/"+url.PathEscape(p.provider)+"/keys", key, nil)
		}
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

// removeAutoDetectedKeys removes the keys the engine creates on its own, when it starts with no
// providers, from API key environment variables such as OPENAI_API_KEY. Obot configures
// providers from the environment itself (see importEnvProviders), with keys that hold the
// value rather than refer to the variable, which the admin API can then update. The
// engine's keys would be a second key for the same variable, which can't be changed through
// the admin API, and would bring back providers an administrator removed. A provider left
// with no keys is removed too.
//
// It must run while holding the lock every replica takes to change the gateway's
// configuration, right after this engine booted from the stores: it acts on the engine's copy
// of the configuration, and removing a provider removes it from the shared stores by name, so
// with a stale copy it would remove a provider another replica has just configured.
func (e *Engine) removeAutoDetectedKeys(ctx context.Context) {
	var resp struct {
		Providers []struct {
			Name string `json:"name"`
		} `json:"providers"`
	}
	if err := e.Do(ctx, http.MethodGet, "/api/providers", nil, &resp); err != nil {
		slog.Warn("failed to list LLM gateway providers", "error", err)
		return
	}
	for _, p := range resp.Providers {
		keys, err := e.providerKeys(ctx, p.Name)
		if err != nil {
			continue
		}
		var detected []string
		for _, key := range keys {
			name, _ := key["name"].(string)
			id, _ := key["id"].(string)
			value, _ := json.Marshal(key["value"])
			if id != "" && strings.HasSuffix(name, "_auto_detected") && IsSecretReference(value) {
				detected = append(detected, id)
			}
		}
		if len(detected) == 0 {
			continue
		}
		if len(detected) == len(keys) {
			if err := e.Do(ctx, http.MethodDelete, "/api/providers/"+url.PathEscape(p.Name), nil, nil); err != nil {
				slog.Warn("failed to remove an LLM gateway provider the engine configured from the environment", "provider", p.Name, "error", err)
			}
			continue
		}
		for _, id := range detected {
			if err := e.Do(ctx, http.MethodDelete, "/api/providers/"+url.PathEscape(p.Name)+"/keys/"+url.PathEscape(id), nil, nil); err != nil {
				slog.Warn("failed to remove an LLM gateway key the engine configured from the environment", "provider", p.Name, "error", err)
			}
		}
	}
}

// providerKeys lists a provider's keys, as the engine lists them (with secrets redacted).
func (e *Engine) providerKeys(ctx context.Context, provider string) ([]map[string]any, error) {
	var resp struct {
		Keys []map[string]any `json:"keys"`
	}
	if err := e.Do(ctx, http.MethodGet, "/api/providers/"+url.PathEscape(provider)+"/keys", nil, &resp); err != nil {
		return nil, err
	}
	return resp.Keys, nil
}

// LegacyImportDone reports whether the credentials of legacy model providers have been
// imported into the gateway, whose ModelProvider is mp.
func LegacyImportDone(mp *v1.ModelProvider) bool {
	return mp.Annotations[legacyImportAnnotation] != ""
}

// RunStartupTasks waits for the engine to start, imports legacy model provider credentials
// once, and then keeps Obot's model list in step with Bifrost's. announce tells the other
// replicas when it changed the gateway's configuration, so they reload it.
//
// Replicas that start together take turns: each holds a lock every replica shares while it
// makes its changes, and first reloads the configuration the others announced
// (currentRevision returns the latest), so it never writes back a stale copy of it.
func (e *Engine) RunStartupTasks(ctx context.Context, c kclient.Client, gatewayClient *gateway.Client, currentRevision, announce func(context.Context) (int64, error)) {
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

	if err := gatewayClient.WithLLMGatewayLock(ctx, func() error {
		if revision, err := currentRevision(ctx); err != nil {
			return fmt.Errorf("failed to read the LLM gateway configuration revision: %w", err)
		} else if _, err := e.Reload(revision); err != nil {
			return fmt.Errorf("failed to reload the LLM gateway configuration: %w", err)
		}
		// Another replica may have imported legacy providers while this one waited.
		if err := c.Get(ctx, kclient.ObjectKeyFromObject(&mp), &mp); err != nil {
			return err
		}
		return e.Change(ctx, e.startupChanges(ctx, c, gatewayClient, &mp), announce)
	}); err != nil {
		slog.Warn("LLM gateway startup changes failed or were not announced to other replicas", "error", err)
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

// startupChanges imports legacy provider credentials once, configures providers from the
// environment and applies the egress proxy. mp is the gateway's ModelProvider.
func (e *Engine) startupChanges(ctx context.Context, c kclient.Client, gatewayClient *gateway.Client, mp *v1.ModelProvider) func() (bool, bool) {
	return func() (bool, bool) {
		changed := false
		if !LegacyImportDone(mp) {
			results, err := e.ImportLegacyModelProviders(ctx, c, gatewayClient)
			changed = slices.ContainsFunc(results, func(r LegacyImportResult) bool { return r.Status == "imported" })
			failed := err != nil
			if err != nil {
				slog.Warn("failed to import legacy model providers into the LLM gateway", "error", err)
			}
			for _, r := range results {
				slog.Info("LLM gateway legacy provider import", "modelProvider", r.ModelProvider, "gatewayProvider", r.GatewayProvider, "status", r.Status, "message", r.Message)
				failed = failed || r.Status == "error"
			}
			// Only record the import once it fully succeeded, so a failure is retried next start.
			if !failed {
				if err := recordLegacyImport(ctx, c, mp); err != nil {
					slog.Warn("failed to record LLM gateway legacy import", "error", err)
				}
			}
		}

		if e.importEnvProviders(ctx, c, mp) {
			changed = true
		}
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
