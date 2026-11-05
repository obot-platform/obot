package llmcore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/maximhq/bifrost/core/schemas"
	llmcoreclient "github.com/obot-platform/obot/pkg/llmcore/client"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestSubscriptionLeasePendingCancelAndRepair(t *testing.T) {
	store, err := llmcoreclient.New(fake.NewClientBuilder().WithScheme(scheme.Scheme).Build(), retirementCodec{})
	require.NoError(t, err)
	require.NoError(t, store.CreateProvider(t.Context(), "anthropic", llmcoreclient.Provider{}))
	engine, err := New(t.Context(), store, Options{})
	require.NoError(t, err)
	defer engine.Shutdown()

	ctx, cancel := engine.requestContext(subscriptionRequest())
	// Cancel before Shutdown even when an assertion aborts with retirement waiting.
	defer cancel()
	plugin := &credentialPlugin{account: engine.account}
	_, short, err := plugin.PreLLMHook(ctx, subscriptionAttempt(schemas.Anthropic))
	require.NoError(t, err)
	require.Nil(t, short, "pause after successful enforcement and before core enqueue")

	require.NoError(t, store.MutateProvider(t.Context(), "anthropic", func(p *llmcoreclient.Provider) error {
		p.Config.NetworkConfig.BaseURL = "http://recording.invalid"
		return nil
	}))
	require.NoError(t, store.CreateProvider(t.Context(), "unrelated", llmcoreclient.Provider{Config: schemas.ProviderConfig{CustomProviderConfig: &schemas.CustomProviderConfig{BaseProviderType: schemas.OpenAI}}}))
	require.NoError(t, store.MutateSettings(t.Context(), func(settings map[string]json.RawMessage) error {
		settings["client"] = json.RawMessage(`{"max_request_body_size_mb":7}`)
		return nil
	}))

	// One asynchronous retirement must survive canceled retries. Neither a pending
	// lease nor an upstream drain may make Sync block unrelated configuration.
	started := time.Now()
	_, err = engine.Sync(t.Context())
	require.ErrorContains(t, err, "anthropic is still draining")
	require.Less(t, time.Since(started), 3*time.Second)
	pending := engine.retirements[schemas.Anthropic]
	require.NotNil(t, pending)
	_, short, _ = plugin.PreLLMHook(ctx, subscriptionAttempt(schemas.Anthropic))
	require.Equal(t, http.StatusServiceUnavailable, *short.Error.StatusCode)
	require.Equal(t, "api_error", *short.Error.Error.Type)
	newPending, cancelPending := engine.requestContext(subscriptionRequest())
	_, short, _ = plugin.PreLLMHook(newPending, subscriptionAttempt(schemas.Anthropic))
	cancelPending()
	require.NotNil(t, short)
	require.Equal(t, http.StatusServiceUnavailable, *short.Error.StatusCode)
	require.Equal(t, "api_error", *short.Error.Error.Type)
	require.NotContains(t, newPending.Value(schemas.BifrostContextKeyExtraHeaders).(map[string][]string), "authorization")
	// A pending gate does not turn an unmanaged direct context into an owner.
	direct := schemas.NewBifrostContext(t.Context(), schemas.NoDeadline)
	direct.SetValue(schemas.BifrostContextKeyExtraHeaders, map[string][]string{"authorization": {"Bearer sk-ant-oat-fixture"}})
	direct.SetValue(schemas.BifrostContextKeyUserAgent, "claude-cli/2.1.168")
	direct.SetValue(schemas.BifrostContextKeySkipKeySelection, true)
	_, short, _ = plugin.PreLLMHook(direct, subscriptionAttempt(schemas.Anthropic))
	require.Equal(t, http.StatusForbidden, *short.Error.StatusCode)
	require.Equal(t, "permission_denied", *short.Error.Error.Type)

	require.Equal(t, int64(7<<20), engine.MaxRequestBodyBytes())
	require.Contains(t, engine.applied.Providers, schemas.ModelProvider("unrelated"))
	require.Empty(t, engine.account.config.Load().Providers[schemas.Anthropic].NetworkConfig.BaseURL, "pending lease retains the old destination")

	retry, cancelRetry := context.WithCancel(t.Context())
	cancelRetry()
	_, err = engine.Sync(retry)
	require.Error(t, err)
	require.Same(t, pending, engine.retirements[schemas.Anthropic])
	// A canceled store read may stop before reconciliation; the original tracked
	// retirement must still await the lease rather than starting another removal.
	select {
	case <-pending.done:
		t.Fatal("retirement crossed an active credential lifetime")
	default:
	}
	cancel()
	cancel()
	select {
	case <-pending.done:
	case <-time.After(3 * time.Second):
		t.Fatal("owned cancel did not release retirement")
	}
	_, err = engine.Sync(t.Context())
	require.NoError(t, err)
	require.Equal(t, "http://recording.invalid", engine.account.config.Load().Providers[schemas.Anthropic].NetworkConfig.BaseURL)

	unsafe, cancelUnsafe := engine.requestContext(subscriptionRequest())
	_, short, _ = plugin.PreLLMHook(unsafe, subscriptionAttempt(schemas.Anthropic))
	cancelUnsafe()
	require.NotNil(t, short)
	require.NoError(t, store.MutateProvider(t.Context(), "anthropic", func(p *llmcoreclient.Provider) error {
		p.Config.NetworkConfig.BaseURL = "https://api.anthropic.com"
		return nil
	}))
	_, err = engine.Sync(t.Context())
	require.NoError(t, err)
	repaired, cancelRepaired := engine.requestContext(subscriptionRequest())
	defer cancelRepaired()
	_, short, _ = plugin.PreLLMHook(repaired, subscriptionAttempt(schemas.Anthropic))
	require.Nil(t, short, "a new owned context can use the repaired destination")
}

// Input failures happen before any LLM hook. Lifetime cleanup must therefore
// belong to the HTTP handler's cancel, including counting and streaming input.
func TestSubscriptionLeaseReleasedOnInputErrors(t *testing.T) {
	store, err := llmcoreclient.New(fake.NewClientBuilder().WithScheme(scheme.Scheme).Build(), retirementCodec{})
	require.NoError(t, err)
	require.NoError(t, store.CreateProvider(t.Context(), "anthropic", llmcoreclient.Provider{}))
	engine, err := New(t.Context(), store, Options{})
	require.NoError(t, err)
	defer engine.Shutdown()
	for _, path := range []string{"/v1/chat/completions", "/v1/responses", "/v1/messages", "/v1/messages/count_tokens", "/v1/responses/input_tokens"} {
		for _, body := range []string{"{", `{"model":"unknown/test","stream":true}`} {
			req := subscriptionRequest()
			req.URL.Path = path
			req.Body = io.NopCloser(strings.NewReader(body))
			out := httptest.NewRecorder()
			engine.ServeHTTP(out, req)
			require.Equal(t, http.StatusBadRequest, out.Code, out.Body.String())
			require.True(t, engine.account.subscriptionMu.TryLock(), "%s leaked a lease on input failure", path)
			engine.account.subscriptionMu.Unlock()
		}
	}
	// Body read errors occur before requestContext, so they cannot acquire a lease.
	req := subscriptionRequest()
	req.Body = &failedCredentialBody{}
	out := httptest.NewRecorder()
	engine.ServeHTTP(out, req)
	require.Equal(t, http.StatusBadRequest, out.Code)
	require.True(t, engine.account.subscriptionMu.TryLock())
	engine.account.subscriptionMu.Unlock()
}

type failedCredentialBody struct{}

func (*failedCredentialBody) Read([]byte) (int, error) { return 0, errors.New("interrupted upload") }
func (*failedCredentialBody) Close() error             { return nil }

// A pre-hook error can end a request before credentialPlugin/PostLLMHook runs.
// Cancellation must use the same HTTP cleanup path rather than a post-hook lease.
type abortedCredentialAttempt struct {
	cancel context.CancelFunc
}

func (*abortedCredentialAttempt) GetName() string { return "abort-credential-attempt" }
func (*abortedCredentialAttempt) Cleanup() error  { return nil }
func (p *abortedCredentialAttempt) PreRequestHook(*schemas.BifrostContext, *schemas.BifrostRequest) error {
	if p.cancel != nil {
		p.cancel()
	}
	return nil
}
func (*abortedCredentialAttempt) PreLLMHook(_ *schemas.BifrostContext, req *schemas.BifrostRequest) (*schemas.BifrostRequest, *schemas.LLMPluginShortCircuit, error) {
	return req, &schemas.LLMPluginShortCircuit{Error: &schemas.BifrostError{StatusCode: schemas.Ptr(http.StatusServiceUnavailable), Error: &schemas.ErrorField{Message: "injected transport failure"}}}, nil
}
func (*abortedCredentialAttempt) PostLLMHook(_ *schemas.BifrostContext, resp *schemas.BifrostResponse, err *schemas.BifrostError) (*schemas.BifrostResponse, *schemas.BifrostError, error) {
	return resp, err, nil
}

func TestSubscriptionLeaseReleasedOnAbortAndCancellation(t *testing.T) {
	for _, canceled := range []bool{false, true} {
		t.Run(fmt.Sprintf("canceled=%t", canceled), func(t *testing.T) {
			store, err := llmcoreclient.New(fake.NewClientBuilder().WithScheme(scheme.Scheme).Build(), retirementCodec{})
			require.NoError(t, err)
			require.NoError(t, store.CreateProvider(t.Context(), "anthropic", llmcoreclient.Provider{}))
			parent, cancelParent := context.WithCancel(t.Context())
			defer cancelParent()
			plugin := &abortedCredentialAttempt{}
			if canceled {
				plugin.cancel = cancelParent
			}
			engine, err := New(t.Context(), store, Options{Plugins: []schemas.LLMPlugin{plugin}})
			require.NoError(t, err)
			defer engine.Shutdown()
			req := subscriptionRequest().WithContext(parent)
			req.URL.Path = "/v1/chat/completions"
			req.Body = io.NopCloser(strings.NewReader(`{"model":"anthropic/test","messages":[{"role":"user","content":"hi"}]}`))
			out := httptest.NewRecorder()
			engine.ServeHTTP(out, req)
			require.GreaterOrEqual(t, out.Code, http.StatusBadRequest)
			require.True(t, engine.account.subscriptionMu.TryLock(), "request termination must release the HTTP lease without relying on post-hooks")
			engine.account.subscriptionMu.Unlock()
			require.NoError(t, store.MutateProvider(t.Context(), "anthropic", func(p *llmcoreclient.Provider) error {
				p.Config.NetworkConfig.BaseURL = "https://api.anthropic.com"
				return nil
			}))
			_, err = engine.Sync(t.Context())
			require.NoError(t, err)
		})
	}
}

func TestConfiguredKeyContextDoesNotAcquireSubscriptionLease(t *testing.T) {
	a := &account{}
	a.config.Store(&llmcoreclient.Config{})
	ctx, cancel := (&Engine{account: a}).requestContext(httptest.NewRequest(http.MethodPost, "/v1/messages", nil))
	defer cancel()
	require.Nil(t, ctx.Value(subscriptionLeaseKey{}))
	require.Nil(t, ctx.Value(schemas.BifrostContextKeySkipKeySelection))
	require.True(t, a.subscriptionMu.TryLock(), "configured-key requests must not hold an Anthropic read lease")
	a.subscriptionMu.Unlock()
}
