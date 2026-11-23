package llmengine

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/maximhq/bifrost/core/schemas"
	llmcoreclient "github.com/obot-platform/obot/pkg/llmcore/client"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

func TestProviderSubscriptionUpdates(t *testing.T) {
	e, ctx := startTestEngine(t)
	for _, name := range []string{"anthropic", "openai"} {
		created, err := e.CreateProvider(ctx, mustJSON(t, map[string]any{"provider": name}))
		require.NoError(t, err)
		require.False(t, created.SubscriptionPassthrough)
	}
	key, err := e.CreateKey(ctx, "anthropic", json.RawMessage(`{"name":"fixture","value":"secret-fixture","models":["*"],"weight":1}`))
	require.NoError(t, err)
	before, err := e.store.GetProvider(ctx, "anthropic")
	require.NoError(t, err)
	view, err := e.UpdateProvider(ctx, "anthropic", json.RawMessage(`{"subscription_passthrough":true}`), false)
	require.NoError(t, err)
	require.True(t, view.SubscriptionPassthrough)
	after, err := e.store.GetProvider(ctx, "anthropic")
	require.NoError(t, err)
	before.SubscriptionPassthrough = new(true)
	require.Equal(t, before, after, "a toggle preserves the entire aggregate")
	require.Equal(t, key.ID, after.Keys[0].ID)
	_, err = e.UpdateProvider(ctx, "anthropic", json.RawMessage(`{"network_config":{"max_retries":4}}`), false)
	require.NoError(t, err)
	view, err = e.GetProvider(ctx, "anthropic")
	require.NoError(t, err)
	require.True(t, view.SubscriptionPassthrough, "omission retains enablement")
	require.Equal(t, 4, view.NetworkConfig.MaxRetries)
	allowed, err := e.SubscriptionPassthrough(ctx, "openai")
	require.NoError(t, err)
	require.False(t, allowed, "Anthropic changes cannot enable OpenAI")
	_, err = e.UpdateProvider(ctx, "openai", json.RawMessage(`{"subscription_passthrough":true}`), false)
	require.NoError(t, err)
	_, err = e.UpdateProvider(ctx, "anthropic", json.RawMessage(`{"subscription_passthrough":false}`), false)
	require.NoError(t, err)
	allowed, err = e.SubscriptionPassthrough(ctx, "openai")
	require.NoError(t, err)
	require.True(t, allowed)
	require.NoError(t, e.DeleteProvider(ctx, "openai"))
	allowed, err = e.SubscriptionPassthrough(ctx, "openai")
	require.NoError(t, err)
	require.False(t, allowed)
	view, err = e.CreateProvider(ctx, json.RawMessage(`{"provider":"openai"}`))
	require.NoError(t, err)
	require.False(t, view.SubscriptionPassthrough)
}

func TestProviderSubscriptionUnmigratedEdits(t *testing.T) {
	e, ctx := startTestEngine(t)
	_, err := e.CreateProvider(ctx, json.RawMessage(`{"provider":"anthropic"}`))
	require.NoError(t, err)
	require.NoError(t, e.store.MutateProvider(ctx, "anthropic", func(provider *llmcoreclient.Provider) error {
		provider.SubscriptionPassthrough = nil
		return nil
	}))

	_, err = e.CreateKey(ctx, "anthropic", json.RawMessage(`{"name":"fixture","value":"fixture-secret"}`))
	require.NoError(t, err)
	provider, err := e.store.GetProvider(ctx, "anthropic")
	require.NoError(t, err)
	require.Nil(t, provider.SubscriptionPassthrough, "key edits must not suppress migration")

	view, err := e.UpdateProvider(ctx, "anthropic", json.RawMessage(`{"network_config":{"max_retries":4}}`), false)
	require.NoError(t, err)
	require.False(t, view.SubscriptionPassthrough)
	provider, err = e.store.GetProvider(ctx, "anthropic")
	require.NoError(t, err)
	require.Nil(t, provider.SubscriptionPassthrough, "unrelated provider edits must not suppress migration")
	require.Equal(t, 4, provider.Config.NetworkConfig.MaxRetries)

	_, err = e.UpdateProvider(ctx, "anthropic", json.RawMessage(`{"subscription_passthrough":null}`), false)
	require.NoError(t, err)
	provider, err = e.store.GetProvider(ctx, "anthropic")
	require.NoError(t, err)
	require.Nil(t, provider.SubscriptionPassthrough, "null must not suppress migration")

	_, err = e.UpdateProvider(ctx, "anthropic", json.RawMessage(`{"subscription_passthrough":false}`), false)
	require.NoError(t, err)
	provider, err = e.store.GetProvider(ctx, "anthropic")
	require.NoError(t, err)
	require.Equal(t, new(false), provider.SubscriptionPassthrough, "an explicit disable must survive a delayed migration")
}

func TestProviderSubscriptionValidation(t *testing.T) {
	for _, tc := range []struct {
		name    string
		initial string
		patch   string
	}{
		{
			name:    "unsupported",
			initial: `{"provider":"gemini"}`,
			patch:   `{"subscription_passthrough":true}`,
		},
		{
			name:    "custom",
			initial: `{"provider":"custom","custom_provider_config":{"base_provider_type":"openai"},"network_config":{"base_url":"https://8.8.8.8/v1"}}`,
			patch:   `{"subscription_passthrough":true}`,
		},
		{
			name:    "string",
			initial: `{"provider":"openai"}`,
			patch:   `{"subscription_passthrough":"true"}`,
		},
		{
			name:    "number",
			initial: `{"provider":"openai"}`,
			patch:   `{"subscription_passthrough":1}`,
		},
		{
			name:    "insecure TLS on enable",
			initial: `{"provider":"openai","network_config":{"insecure_skip_verify":true}}`,
			patch:   `{"subscription_passthrough":true}`,
		},
		{
			name:    "proxy CA on enable",
			initial: `{"provider":"openai","proxy_config":{"type":"http","url":"http://proxy.invalid:3128","ca_cert_pem":"fixture"}}`,
			patch:   `{"subscription_passthrough":true}`,
		},
		{
			name:    "credential override while enabled",
			initial: `{"provider":"openai","subscription_passthrough":true}`,
			patch:   `{"network_config":{"extra_headers":{"Authorization":"fixture"}}}`,
		},
		{
			name:    "destination while enabled",
			initial: `{"provider":"anthropic","subscription_passthrough":true}`,
			patch:   `{"network_config":{"base_url":"https://8.8.8.8/v1"}}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e, ctx := startTestEngine(t)
			view, err := e.CreateProvider(ctx, json.RawMessage(tc.initial))
			require.NoError(t, err)
			before, err := e.store.GetProvider(ctx, string(view.Name))
			require.NoError(t, err)
			_, err = e.UpdateProvider(ctx, string(view.Name), json.RawMessage(tc.patch), false)
			require.True(t, apierrors.IsBadRequest(err), "expected bad request, got %v", err)
			after, err := e.store.GetProvider(ctx, string(view.Name))
			require.NoError(t, err)
			require.Equal(t, before, after)
		})
	}
	e, ctx := startTestEngine(t)
	_, err := e.CreateProvider(ctx, json.RawMessage(`{"provider":"gemini","subscription_passthrough":false}`))
	require.NoError(t, err)
	_, err = e.CreateProvider(ctx, json.RawMessage(`{"provider":"openai","subscription_passthrough":true,"network_config":{"insecure_skip_verify":true}}`))
	require.True(t, apierrors.IsBadRequest(err))
	_, err = e.CreateProvider(ctx, json.RawMessage(`{"provider":"openai","subscription_passthrough":null}`))
	require.NoError(t, err)
	view, err := e.GetProvider(ctx, "openai")
	require.NoError(t, err)
	require.False(t, view.SubscriptionPassthrough)
}

func TestProviderSubscriptionNullRetainsValue(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run(map[bool]string{false: "disabled", true: "enabled"}[enabled], func(t *testing.T) {
			e, ctx := startTestEngine(t)
			_, err := e.CreateProvider(ctx, mustJSON(t, map[string]any{
				"provider":                 "openai",
				"subscription_passthrough": enabled,
			}))
			require.NoError(t, err)
			before, err := e.store.GetProvider(ctx, "openai")
			require.NoError(t, err)

			view, err := e.UpdateProvider(ctx, "openai", json.RawMessage(`{"subscription_passthrough":null}`), false)
			require.NoError(t, err)
			require.Equal(t, enabled, view.SubscriptionPassthrough)
			after, err := e.store.GetProvider(ctx, "openai")
			require.NoError(t, err)
			require.Equal(t, before, after)
		})
	}
}

func TestSubscriptionProviderReadFailure(t *testing.T) {
	e, ctx := startTestEngine(t)
	_, err := e.CreateProvider(ctx, json.RawMessage(`{"provider":"openai","subscription_passthrough":true}`))
	require.NoError(t, err)
	allowed, err := e.SubscriptionPassthrough(ctx, "openai")
	require.NoError(t, err)
	require.True(t, allowed)

	// Replace only the reader, keeping the previously enabled runtime alive.
	broken := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithInterceptorFuncs(interceptor.Funcs{
		Get: func(context.Context, kclient.WithWatch, kclient.ObjectKey, kclient.Object, ...kclient.GetOption) error {
			return errors.New("provider unreadable")
		},
	}).Build()
	e.store, err = llmcoreclient.New(broken, plaintextConfigCodec{})
	require.NoError(t, err)
	allowed, err = e.SubscriptionPassthrough(ctx, "openai")
	require.ErrorContains(t, err, "provider unreadable")
	require.False(t, allowed)
}

func TestSubscriptionUpdateConflict(t *testing.T) {
	e, ctx := startTestEngine(t)
	_, err := e.CreateProvider(ctx, json.RawMessage(`{"provider":"openai"}`))
	require.NoError(t, err)
	// A concurrent key edit advances the resource version during the flag write.
	storage := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
	base, err := llmcoreclient.New(storage, plaintextConfigCodec{})
	require.NoError(t, err)
	require.NoError(t, base.CreateProvider(ctx, "openai", llmcoreclient.Provider{}))
	injected := false
	wrapped := interceptor.NewClient(storage, interceptor.Funcs{
		Update: func(ctx context.Context, c kclient.WithWatch, obj kclient.Object, opts ...kclient.UpdateOption) error {
			if _, ok := obj.(*v1.LLMGatewayProvider); ok && !injected {
				injected = true
				require.NoError(t, base.PutKey(ctx, "openai", schemas.Key{ID: "concurrent"}))
			}
			return c.Update(ctx, obj, opts...)
		},
	})
	e.store, err = llmcoreclient.New(wrapped, plaintextConfigCodec{})
	require.NoError(t, err)
	_, err = e.UpdateProvider(ctx, "openai", json.RawMessage(`{"subscription_passthrough":true}`), false)
	require.True(t, apierrors.IsConflict(err), "expected 409, got %v", err)
	p, err := base.GetProvider(ctx, "openai")
	require.NoError(t, err)
	require.Equal(t, "concurrent", p.Keys[0].ID)
	require.Equal(t, new(false), p.SubscriptionPassthrough)
}

func TestSubscriptionProvidersUpdateIndependently(t *testing.T) {
	e, ctx := startTestEngine(t)
	for _, name := range []string{"anthropic", "openai"} {
		_, err := e.CreateProvider(ctx, mustJSON(t, map[string]any{"provider": name}))
		require.NoError(t, err)
	}
	for _, allowed := range []bool{true, false, true} {
		start := make(chan struct{})
		errs := make(chan error, 2)
		for _, name := range []string{"anthropic", "openai"} {
			go func() {
				<-start
				_, err := e.UpdateProvider(ctx, name, mustJSON(t, map[string]any{"subscription_passthrough": allowed}), false)
				errs <- err
			}()
		}
		close(start)
		require.NoError(t, <-errs)
		require.NoError(t, <-errs)
		for _, name := range []string{"anthropic", "openai"} {
			got, err := e.SubscriptionPassthrough(ctx, name)
			require.NoError(t, err)
			require.Equal(t, allowed, got)
		}
	}
}
