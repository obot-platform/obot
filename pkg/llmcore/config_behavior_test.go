package llmcore_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/maximhq/bifrost/core/schemas"
	"github.com/maximhq/bifrost/framework/configstore/tables"
	"github.com/obot-platform/obot/pkg/llmcore"
	llmcoreclient "github.com/obot-platform/obot/pkg/llmcore/client"
	"github.com/stretchr/testify/require"
)

const configTestChatResponse = `{"id":"test","object":"chat.completion","model":"test","choices":[{"index":0,"message":{"role":"assistant","content":"hello"},"finish_reason":"stop"}]}`

func configTestChat(engine *llmcore.Engine, query string) *httptest.ResponseRecorder {
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/v1/chat/completions"+query, strings.NewReader(`{"model":"openai/original","messages":[{"role":"user","content":"hello"}]}`)))
	return response
}

func TestEmptyProviderProxyUsesGlobalInferenceProxy(t *testing.T) {
	var directCalls, proxyCalls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		directCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, configTestChatResponse)
	}))
	defer upstream.Close()
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The HTTP dialer may establish a CONNECT tunnel before the inference POST.
		if r.Method == http.MethodPost {
			proxyCalls.Add(1)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, configTestChatResponse)
	}))
	defer proxy.Close()
	store := newTestStore(t)
	provider := transitionProvider(upstream.URL, "secret")
	provider.Config.ProxyConfig = &schemas.ProxyConfig{}
	require.NoError(t, store.CreateProvider(t.Context(), "openai", provider))
	require.NoError(t, store.MutateSettings(t.Context(), func(settings map[string]json.RawMessage) error {
		raw, err := json.Marshal(map[string]any{"enabled": true, "enable_for_inference": true, "url": proxy.URL})
		settings["proxy"] = raw
		return err
	}))
	engine, err := llmcore.New(t.Context(), store, llmcore.Options{})
	require.NoError(t, err)
	defer engine.Shutdown()
	response := configTestChat(engine, "")
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	require.Equal(t, int64(1), proxyCalls.Load(), "an empty provider proxy must inherit the global inference proxy")
	require.Zero(t, directCalls.Load(), "the global proxy must not be bypassed")
}

func newRoutingConfigEngine(t *testing.T, rule tables.TableRoutingRule) (*llmcore.Engine, *llmcoreclient.Client) {
	t.Helper()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, configTestChatResponse)
	}))
	t.Cleanup(upstream.Close)
	store := newTestStore(t)
	require.NoError(t, store.CreateProvider(t.Context(), "openai", transitionProvider(upstream.URL, "secret")))
	require.NoError(t, store.PutRule(t.Context(), rule))
	engine, err := llmcore.New(t.Context(), store, llmcore.Options{})
	require.NoError(t, err)
	t.Cleanup(engine.Shutdown)
	return engine, store
}

func configRoutingRule(expression, model string) tables.TableRoutingRule {
	return tables.TableRoutingRule{
		ID:            "policy",
		Name:          "policy",
		Scope:         "global",
		CelExpression: expression,
		Targets:       []tables.TableRoutingTarget{{Model: &model, Weight: 1}},
	}
}

func TestInvalidCELPreservesAppliedRoutingRule(t *testing.T) {
	engine, store := newRoutingConfigEngine(t, configRoutingRule("true", "old-route"))
	for _, expression := range []string{
		"params.tier ==",
		"unknown_variable == true",
		"(1)",
		"('text')",
		"[true]",
		"({'tier': true})",
		"headers['x-route']",
		"params.tier == 'premium' ? true : 1",
	} {
		require.NoError(t, store.PutRule(t.Context(), configRoutingRule(expression, "new-route")))
		for range 2 {
			changed, err := engine.Sync(t.Context())
			require.True(t, changed)
			require.ErrorContains(t, err, "policy")
			require.Error(t, engine.Err())
			response := configTestChat(engine, "")
			require.Equal(t, http.StatusOK, response.Code, response.Body.String())
			require.Equal(t, "old-route", response.Header().Get("X-Obot-LLM-Model"), "invalid replacements must preserve the working policy")
		}
	}
	require.NoError(t, store.PutRule(t.Context(), configRoutingRule("true", "new-route")))
	_, err := engine.Sync(t.Context())
	require.NoError(t, err)
	require.NoError(t, engine.Err())
	response := configTestChat(engine, "")
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	require.Equal(t, "new-route", response.Header().Get("X-Obot-LLM-Model"))
	changed, err := engine.Sync(t.Context())
	require.NoError(t, err)
	require.False(t, changed)
}

func TestRoutingUsesRequestQueryParameters(t *testing.T) {
	engine, _ := newRoutingConfigEngine(t, configRoutingRule(`"tier" in params && params.tier == 'premium'`, "premium-route"))
	for _, tt := range []struct {
		query string
		model string
	}{
		{
			query: "?tier=premium",
			model: "premium-route",
		},
		{
			query: "?TIER=premium",
			model: "premium-route",
		},
		{
			query: "?tier=standard&tier=premium",
			model: "premium-route",
		},
		{
			query: "?tier=standard",
			model: "original",
		},
		{
			query: "",
			model: "original",
		},
	} {
		t.Run(tt.query, func(t *testing.T) {
			response := configTestChat(engine, tt.query)
			require.Equal(t, http.StatusOK, response.Code, response.Body.String())
			require.Equal(t, tt.model, response.Header().Get("X-Obot-LLM-Model"))
		})
	}
}

func TestRoutingTargetProviderInheritanceConverges(t *testing.T) {
	for _, tt := range []struct {
		name     string
		provider *string
	}{
		{
			name:     "omitted provider",
			provider: nil,
		},
		{
			name:     "empty provider",
			provider: schemas.Ptr(""),
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			engine, store := newRoutingConfigEngine(t, configRoutingRule("true", "initial-route"))
			rule := configRoutingRule("true", "inherited-route")
			rule.Targets[0].Provider = tt.provider
			require.NoError(t, store.PutRule(t.Context(), rule))
			changed, err := engine.Sync(t.Context())
			require.NoError(t, err, "inheriting the incoming provider is not a missing dependency")
			require.True(t, changed)
			for range 2 {
				changed, err = engine.Sync(t.Context())
				require.NoError(t, err)
				require.False(t, changed, "inherited-provider rules must converge")
				require.NoError(t, engine.Err())
				response := configTestChat(engine, "")
				require.Equal(t, http.StatusOK, response.Code, response.Body.String())
				require.Equal(t, "openai", response.Header().Get("X-Obot-LLM-Provider"))
				require.Equal(t, "inherited-route", response.Header().Get("X-Obot-LLM-Model"))
			}

			// A named dependency still has to exist, and its policy stays live
			// while reconciliation reports the missing provider for retry.
			rule.Targets[0].Provider = schemas.Ptr("missing-provider")
			require.NoError(t, store.PutRule(t.Context(), rule))
			for range 2 {
				_, err = engine.Sync(t.Context())
				require.ErrorContains(t, err, "missing-provider")
				require.ErrorContains(t, engine.Err(), "missing-provider")
			}
			response := configTestChat(engine, "")
			require.GreaterOrEqual(t, response.Code, http.StatusBadRequest, "a missing named target must not silently bypass the rule")
		})
	}
}
