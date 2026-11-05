package llmcore

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/maximhq/bifrost/core/schemas"
	llmcoreclient "github.com/obot-platform/obot/pkg/llmcore/client"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestSubscriptionLeaseRootOwnership(t *testing.T) {
	a := &account{}
	a.config.Store(&llmcoreclient.Config{Providers: map[schemas.ModelProvider]schemas.ProviderConfig{schemas.Anthropic: {}}})
	owner, release := (&Engine{account: a}).requestContext(subscriptionRequest())
	defer release()
	plugin := &credentialPlugin{account: a}

	// Plugin scopes share the managed root, including across refused fallbacks.
	scope := owner.WithPluginScope(schemas.Ptr(plugin.GetName()))
	defer scope.ReleasePluginScope()
	require.Same(t, owner.Root(), scope.Root())
	for _, provider := range []schemas.ModelProvider{schemas.Anthropic, schemas.OpenAI, schemas.Anthropic} {
		_, short, err := plugin.PreLLMHook(scope, subscriptionAttempt(provider))
		require.NoError(t, err)
		require.Equal(t, provider == schemas.Anthropic, short == nil)
	}

	for _, tt := range []struct {
		name   string
		parent context.Context
	}{
		{
			name:   "direct parent",
			parent: owner,
		},
		{
			name:   "WithValue parent",
			parent: context.WithValue(owner, struct{}{}, true),
		},
		{
			name:   "WithoutCancel parent",
			parent: context.WithoutCancel(owner),
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			derived, cancel := schemas.NewBifrostContextWithCancel(tt.parent)
			defer cancel()
			require.NotSame(t, owner.Root(), derived.Root())
			require.Same(t, owner.Value(subscriptionLeaseKey{}), derived.Value(subscriptionLeaseKey{}))

			_, short, err := plugin.PreLLMHook(derived, subscriptionAttempt(schemas.Anthropic))
			require.NoError(t, err)
			require.NotNil(t, short, "inherited private values do not confer lifetime ownership")
			require.Equal(t, http.StatusForbidden, *short.Error.StatusCode)
			require.Equal(t, "permission_denied", *short.Error.Error.Type)
			require.False(t, hasCallerCredentials(derived.Value(schemas.BifrostContextKeyExtraHeaders).(map[string][]string)))
		})
	}

	// Core/parent cancellation must refuse reuse without releasing the reader
	// before the owning handler has finished its cleanup.
	owner.Cancel()
	_, short, err := plugin.PreLLMHook(scope, subscriptionAttempt(schemas.Anthropic))
	require.NoError(t, err)
	require.NotNil(t, short)
	require.Equal(t, http.StatusForbidden, *short.Error.StatusCode)
	require.False(t, a.subscriptionMu.TryLock())
	release()
	require.True(t, a.subscriptionMu.TryLock())
	a.subscriptionMu.Unlock()
}

// This is the reviewer's real post-hook queue-reroute scheduling seam. It never
// changes headers or policy, and always resumes before the engine shuts down.
type securityQueuePauseTracer struct {
	schemas.Tracer
	entered chan struct{}
	resume  chan struct{}
	once    sync.Once
}

func (p *securityQueuePauseTracer) StartSpanID(ctx context.Context, name string, kind schemas.SpanKind) (string, schemas.SpanHandle) {
	if name == "queue-wait" {
		p.once.Do(func() {
			close(p.entered)
			<-p.resume
		})
	}

	return p.Tracer.StartSpanID(ctx, name, kind)
}

func TestSubscriptionDetachedLeaseCannotReroute(t *testing.T) {
	received := make(chan http.Header, 8)
	alternate := credentialRecordingServer(received)
	defer alternate.Close()
	store, err := llmcoreclient.New(fake.NewClientBuilder().WithScheme(scheme.Scheme).Build(), retirementCodec{})
	require.NoError(t, err)
	require.NoError(t, store.CreateProvider(t.Context(), "anthropic", llmcoreclient.Provider{}))
	engine, err := New(t.Context(), store, Options{})
	require.NoError(t, err)
	pause := &securityQueuePauseTracer{Tracer: engine.tracer, entered: make(chan struct{}), resume: make(chan struct{})}
	engine.Client().SetTracer(pause)
	var resume sync.Once
	defer func() {
		resume.Do(func() { close(pause.resume) })
		engine.Shutdown()
	}()
	owner, release := engine.requestContext(subscriptionRequest())
	defer release()
	detached, cancel := schemas.NewBifrostContextWithCancel(context.WithoutCancel(owner))
	defer cancel()
	done := make(chan *schemas.BifrostError, 1)
	go func() { done <- credentialChatCall(engine, detached) }()

	// Fixed code must refuse before enqueue. Vulnerable code pauses here, letting
	// the exact owner-release/replacement sequence run against loopback only.
	var result *schemas.BifrostError
	finished := false
	select {
	case result = <-done:
		finished = true
	case <-pause.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("request neither refused nor reached queue boundary")
	}

	require.NoError(t, store.MutateProvider(t.Context(), "anthropic", func(p *llmcoreclient.Provider) error {
		p.Config.NetworkConfig.BaseURL = alternate.URL
		p.Config.NetworkConfig.AllowPrivateNetwork = true
		return nil
	}))
	_, err = engine.Sync(t.Context())
	require.ErrorContains(t, err, "anthropic is still draining")
	retirement := engine.retirements[schemas.Anthropic]
	require.NotNil(t, retirement)
	release()
	require.NoError(t, detached.Err(), "detached lifetime outlasts its alleged owner")
	select {
	case <-retirement.done:
	case <-time.After(5 * time.Second):
		t.Fatal("retirement did not finish after owner release")
	}
	_, err = engine.Sync(t.Context())
	require.NoError(t, err)
	require.False(t, engine.account.subscriptionDestinationSafe())
	resume.Do(func() { close(pause.resume) })
	if !finished {
		select {
		case result = <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("request did not finish")
		}
	}

	require.Empty(t, received, "unsafe replacement must receive zero requests")
	require.NotNil(t, result)
	require.Equal(t, http.StatusForbidden, *result.StatusCode)
	require.Equal(t, "permission_denied", *result.Error.Type)
}

func TestReusedDirectContextChecksNewCredentials(t *testing.T) {
	received := make(chan http.Header, 32)
	upstream := credentialRecordingServer(received)
	defer upstream.Close()
	store, err := llmcoreclient.New(fake.NewClientBuilder().WithScheme(scheme.Scheme).Build(), retirementCodec{})
	require.NoError(t, err)
	require.NoError(t, store.CreateProvider(t.Context(), "anthropic", llmcoreclient.Provider{
		Config: schemas.ProviderConfig{NetworkConfig: schemas.NetworkConfig{BaseURL: upstream.URL, AllowPrivateNetwork: true}},
		Keys: []schemas.Key{{
			ID:     "configured",
			Name:   "configured",
			Value:  *schemas.NewSecretVar("configured-fixture"),
			Models: schemas.WhiteList{"*"},
			Weight: 1,
		}},
	}))
	engine, err := New(t.Context(), store, Options{})
	require.NoError(t, err)
	defer engine.Shutdown()

	for _, tt := range []struct {
		name    string
		ua      string
		headers map[string][]string
		direct  bool
	}{
		{
			name:    "subscription",
			ua:      "claude-cli/2.1.168",
			headers: map[string][]string{"authorization": {"Bearer sk-ant-oat-fixture"}},
		},
		{
			name:    "missing UA",
			headers: map[string][]string{"authorization": {"Bearer sk-ant-oat-fixture"}},
		},
		{
			name:    "other UA",
			ua:      "other-client",
			headers: map[string][]string{"authorization": {"Bearer sk-ant-oat-fixture"}},
		},
		{
			name:    "duplicate auth",
			ua:      "claude-cli/2.1.168",
			headers: map[string][]string{"Authorization": {"Bearer sk-ant-oat-fixture", "Bearer sk-ant-oat-other"}},
		},
		{
			name:    "mixed auth",
			ua:      "claude-cli/2.1.168",
			headers: map[string][]string{"authorization": {"Bearer sk-ant-oat-fixture"}, "X-Api-Key": {"caller-fixture"}},
		},
		{
			name:    "API key only",
			ua:      "claude-cli/2.1.168",
			headers: map[string][]string{"x-api-key": {"caller-fixture"}},
		},
		{
			name:    "direct key override",
			ua:      "claude-cli/2.1.168",
			headers: map[string][]string{"authorization": {"Bearer sk-ant-oat-fixture"}},
			direct:  true,
		},
	} {
		for _, skip := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/skip=%t", tt.name, skip), func(t *testing.T) {
				ctx, cancel := schemas.NewBifrostContextWithCancel(t.Context())
				defer cancel()
				// Multiple credential-free calls must keep using the configured key.
				for range 2 {
					require.Nil(t, credentialChatCall(engine, ctx))
					select {
					case headers := <-received:
						require.Equal(t, "configured-fixture", headers.Get("X-Api-Key"))
						require.Empty(t, headers.Get("Authorization"))
					case <-time.After(5 * time.Second):
						t.Fatal("configured-key request did not arrive")
					}
				}

				ctx.SetValue(schemas.BifrostContextKeyExtraHeaders, tt.headers)
				ctx.SetValue(schemas.BifrostContextKeyUserAgent, tt.ua)
				ctx.SetValue(schemas.BifrostContextKeySkipKeySelection, skip)
				if tt.direct {
					ctx.SetValue(schemas.BifrostContextKeyDirectKey, schemas.Key{Value: *schemas.NewSecretVar("override-fixture")})
				}
				require.Nil(t, ctx.Value(subscriptionLeaseKey{}))
				result := credentialChatCall(engine, ctx)

				// Consume unexpected recordings so a failing case cannot contaminate
				// the next warmup, and report counts rather than credential values.
				outbound := len(received)
				for range outbound {
					<-received
				}
				require.Zero(t, outbound, "new credentials must be refused before another outbound call")
				require.NotNil(t, result)
				require.Equal(t, http.StatusForbidden, *result.StatusCode)
				require.Equal(t, "permission_denied", *result.Error.Type)
				require.True(t, *result.AllowFallbacks)
				require.False(t, hasCallerCredentials(ctx.Value(schemas.BifrostContextKeyExtraHeaders).(map[string][]string)))
			})
		}
	}
}

func TestCredentialSnapshotRejectsChangedHeaders(t *testing.T) {
	for _, headers := range []map[string][]string{
		{"authorization": {"Bearer sk-ant-oat-replacement"}},
		{"authorization": {"Bearer sk-ant-oat-fixture", "Bearer sk-ant-oat-other"}},
		{"authorization": {"Bearer sk-ant-oat-fixture"}, "x-api-key": {"caller-fixture"}},
		{"X-Api-Key": {"caller-fixture"}},
	} {
		a := &account{}
		a.config.Store(&llmcoreclient.Config{Providers: map[schemas.ModelProvider]schemas.ProviderConfig{schemas.Anthropic: {}}})
		ctx, cancel := (&Engine{account: a}).requestContext(subscriptionRequest())
		defer cancel()
		plugin := &credentialPlugin{account: a}
		original := ctx.Value(schemas.BifrostContextKeyExtraHeaders).(map[string][]string)
		_, short, err := plugin.PreLLMHook(ctx, subscriptionAttempt(schemas.Anthropic))
		require.NoError(t, err)
		require.Nil(t, short)

		ctx.SetValue(schemas.BifrostContextKeyExtraHeaders, headers)
		_, short, err = plugin.PreLLMHook(ctx, subscriptionAttempt(schemas.Anthropic))
		require.NoError(t, err)
		require.NotNil(t, short, "saved auth cannot mask changed incoming credentials")
		require.Equal(t, http.StatusForbidden, *short.Error.StatusCode)
		require.False(t, hasCallerCredentials(ctx.Value(schemas.BifrostContextKeyExtraHeaders).(map[string][]string)))

		// The refused map is credential-free, but the original admission survives.
		_, short, err = plugin.PreLLMHook(ctx, subscriptionAttempt(schemas.OpenAI))
		require.NoError(t, err)
		require.NotNil(t, short)
		_, short, err = plugin.PreLLMHook(ctx, subscriptionAttempt(schemas.Anthropic))
		require.NoError(t, err)
		require.Nil(t, short)
		require.Equal(t, original["authorization"], ctx.Value(schemas.BifrostContextKeyExtraHeaders).(map[string][]string)["authorization"])
		require.Equal(t, []string{"Bearer sk-ant-oat-fixture"}, original["authorization"])
		cancel()
	}
}

func TestCredentialSnapshotSurvivesCurrentHeaderMutation(t *testing.T) {
	a := &account{}
	a.config.Store(&llmcoreclient.Config{Providers: map[schemas.ModelProvider]schemas.ProviderConfig{schemas.Anthropic: {}}})
	ctx, cancel := (&Engine{account: a}).requestContext(subscriptionRequest())
	defer cancel()
	plugin := &credentialPlugin{account: a}
	_, short, err := plugin.PreLLMHook(ctx, subscriptionAttempt(schemas.Anthropic))
	require.NoError(t, err)
	require.Nil(t, short)

	// A caller editing current headers must not also rewrite the saved admission.
	current := ctx.Value(schemas.BifrostContextKeyExtraHeaders).(map[string][]string)
	current["authorization"][0] = "Bearer sk-ant-oat-replacement"
	_, short, err = plugin.PreLLMHook(ctx, subscriptionAttempt(schemas.Anthropic))
	require.NoError(t, err)
	require.NotNil(t, short)
	require.Equal(t, http.StatusForbidden, *short.Error.StatusCode)

	_, short, err = plugin.PreLLMHook(ctx, subscriptionAttempt(schemas.Anthropic))
	require.NoError(t, err)
	require.Nil(t, short)
	require.Equal(t, []string{"Bearer sk-ant-oat-fixture"}, ctx.Value(schemas.BifrostContextKeyExtraHeaders).(map[string][]string)["authorization"])
	require.Equal(t, []string{"Bearer sk-ant-oat-replacement"}, current["authorization"], "the hook must not edit a previously published map")
}

func credentialRecordingServer(received chan<- http.Header) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received <- r.Header.Clone()
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"msg_mock","type":"message","role":"assistant","model":"test","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`)
	}))
}

func credentialChatCall(engine *Engine, ctx *schemas.BifrostContext) *schemas.BifrostError {
	_, err := engine.Client().ChatCompletionRequest(ctx, &schemas.BifrostChatRequest{
		Provider: schemas.Anthropic,
		Model:    "test",
		Input: []schemas.ChatMessage{{
			Role:    schemas.ChatMessageRoleUser,
			Content: &schemas.ChatMessageContent{ContentStr: schemas.Ptr("hi")},
		}},
	})
	return err
}
