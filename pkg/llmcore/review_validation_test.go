package llmcore

import (
	"math"
	"net/http"
	"testing"
	"time"

	bifrost "github.com/maximhq/bifrost/core"
	"github.com/maximhq/bifrost/core/schemas"
	"github.com/maximhq/bifrost/framework/configstore/tables"
	"github.com/maximhq/bifrost/plugins/routing/rules"
	llmcoreclient "github.com/obot-platform/obot/pkg/llmcore/client"
	"github.com/stretchr/testify/require"
)

func TestAccountRejectsNegativeQueues(t *testing.T) {
	for _, tt := range []struct {
		field string
		queue schemas.ConcurrencyAndBufferSize
	}{
		{
			field: "concurrency",
			queue: schemas.ConcurrencyAndBufferSize{Concurrency: -1},
		},
		{
			field: "buffer_size",
			queue: schemas.ConcurrencyAndBufferSize{BufferSize: -1},
		},
	} {
		t.Run(tt.field, func(t *testing.T) {
			a := &account{}
			a.config.Store(&llmcoreclient.Config{Providers: map[schemas.ModelProvider]schemas.ProviderConfig{
				"invalid": {ConcurrencyAndBufferSize: tt.queue},
				"default": {},
			}})

			_, err := a.GetConfigForProvider("invalid")
			require.ErrorContains(t, err, "invalid")
			require.ErrorContains(t, err, tt.field)

			cfg, err := a.GetConfigForProvider("default")
			require.NoError(t, err)
			require.Equal(t, schemas.DefaultConcurrencyAndBufferSize, cfg.ConcurrencyAndBufferSize)
		})
	}
}

func TestInvalidTargetsPreserveRuleAndProgram(t *testing.T) {
	for _, tt := range []struct {
		name    string
		weights []float64
	}{
		{
			name: "empty",
		},
		{
			name:    "negative",
			weights: []float64{1, -1},
		},
		{
			name:    "nan",
			weights: []float64{math.NaN()},
		},
		{
			name:    "positive infinity",
			weights: []float64{math.Inf(1)},
		},
		{
			name:    "negative infinity",
			weights: []float64{math.Inf(-1)},
		},
		{
			name:    "overflow",
			weights: []float64{math.MaxFloat64, math.MaxFloat64},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			logger := bifrost.NewDefaultLogger(schemas.LogLevelWarn)
			store, err := rules.NewLocalStore(t.Context(), logger, nil)
			require.NoError(t, err)
			plugin, err := newRoutingPlugin(store, logger, nil)
			require.NoError(t, err)
			old := policyRule("old-route")
			require.NoError(t, plugin.UpsertRule(t.Context(), &old))

			next := policyRule("new-route")
			next.CelExpression = "false"
			next.Targets = nil
			for _, weight := range tt.weights {
				next.Targets = append(next.Targets, tables.TableRoutingTarget{Weight: weight})
			}
			require.ErrorContains(t, plugin.UpsertRule(t.Context(), &next), "policy")

			req := policyRequest()
			require.NoError(t, plugin.PreRequestHook(schemas.NewBifrostContext(t.Context(), schemas.NoDeadline), req))
			require.Equal(t, "old-route", req.ChatRequest.Model)
		})
	}
}

func TestRoutingUsesBifrostPriorityOrder(t *testing.T) {
	a, b := policyRule("a"), policyRule("b")
	a.ID, b.ID = "a", "b"
	a.CreatedAt = time.Date(2026, 11, 4, 0, 0, 0, 0, time.UTC)
	b.CreatedAt = a.CreatedAt.Add(time.Second)
	b.Priority = 1
	for _, order := range [][]tables.TableRoutingRule{{a, b}, {b, a}} {
		logger := bifrost.NewDefaultLogger(schemas.LogLevelWarn)
		store, err := rules.NewLocalStore(t.Context(), logger, nil)
		require.NoError(t, err)
		plugin, err := newRoutingPlugin(store, logger, nil)
		require.NoError(t, err)
		for _, rule := range order {
			require.NoError(t, plugin.UpsertRule(t.Context(), &rule))
		}
		assertWinner := func(model string) {
			t.Helper()
			req := policyRequest()
			require.NoError(t, plugin.PreRequestHook(schemas.NewBifrostContext(t.Context(), schemas.NoDeadline), req))
			require.Equal(t, model, req.ChatRequest.Model)
		}
		assertWinner("a")
		replacement := b
		replacement.Priority = -1
		require.NoError(t, plugin.UpsertRule(t.Context(), &replacement))
		assertWinner("b")
		require.NoError(t, plugin.DeleteRule(t.Context(), "b"))
		assertWinner("a")
		require.NoError(t, plugin.DeleteRule(t.Context(), "a"))
		assertWinner("unrouted")
	}
}

func TestTargetWeightsAndInheritanceRemainUsable(t *testing.T) {
	for _, tt := range []struct {
		name    string
		targets []tables.TableRoutingTarget
		models  []string
	}{
		{
			name: "all zero weights",
			targets: []tables.TableRoutingTarget{
				{
					Model:  new("a"),
					Weight: 0,
				},
				{
					Model:  new("b"),
					Weight: 0,
				},
			},
			models: []string{"a", "b"},
		},
		{
			name: "weighted target",
			targets: []tables.TableRoutingTarget{
				{
					Model:  new("a"),
					Weight: 0,
				},
				{
					Model:  new("b"),
					Weight: 2,
				},
			},
			models: []string{"b"},
		},
		{
			name:    "omitted provider and model",
			targets: []tables.TableRoutingTarget{{Weight: 1}},
			models:  []string{"unrouted"},
		},
		{
			name: "empty provider and model",
			targets: []tables.TableRoutingTarget{{
				Provider: new(""),
				Model:    new(""),
				Weight:   1,
			}},
			models: []string{"unrouted"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			logger := bifrost.NewDefaultLogger(schemas.LogLevelWarn)
			store, err := rules.NewLocalStore(t.Context(), logger, nil)
			require.NoError(t, err)
			plugin, err := newRoutingPlugin(store, logger, nil)
			require.NoError(t, err)
			rule := policyRule("unused")
			rule.Targets = tt.targets
			require.NoError(t, plugin.UpsertRule(t.Context(), &rule))

			for range 20 {
				req := policyRequest()
				require.NoError(t, plugin.PreRequestHook(schemas.NewBifrostContext(t.Context(), schemas.NoDeadline), req))
				require.Equal(t, schemas.OpenAI, req.ChatRequest.Provider)
				require.Contains(t, tt.models, req.ChatRequest.Model)
			}
		})
	}
}

func TestConfigurationGateHTTPClassificationIsNarrow(t *testing.T) {
	for _, tt := range []struct {
		name     string
		provider schemas.ModelProvider
		message  string
		status   *int
		want     int
	}{
		{
			name:     "local queue gate",
			provider: "pending",
			message:  "failed to get config for provider pending: provider pending is being reconfigured",
			want:     http.StatusServiceUnavailable,
		},
		{
			name:     "upstream 400 with same message",
			provider: "pending",
			message:  "failed to get config for provider pending: provider pending is being reconfigured",
			status:   schemas.Ptr(http.StatusBadRequest),
			want:     http.StatusBadRequest,
		},
		{
			name:     "another provider",
			provider: "other",
			message:  "failed to get config for provider pending: provider pending is being reconfigured",
			want:     http.StatusBadRequest,
		},
		{
			name:     "missing configuration",
			provider: "pending",
			message:  "failed to get config for provider pending: provider pending is not configured",
			want:     http.StatusBadRequest,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := &schemas.BifrostError{
				StatusCode: tt.status,
				Error:      &schemas.ErrorField{Message: tt.message},
			}
			err.PopulateExtraFields(schemas.ChatCompletionRequest, tt.provider, "", "")
			status, kind, _ := errorParts(err)
			require.Equal(t, tt.want, status)
			if tt.want == http.StatusServiceUnavailable {
				require.Equal(t, "api_error", kind)
			}
		})
	}
}

func TestConfiguredModelParsingPreservesEmptyAndNamespacedInputs(t *testing.T) {
	cfg := &llmcoreclient.Config{Providers: map[schemas.ModelProvider]schemas.ProviderConfig{"pending": {}}}
	for _, tt := range []struct {
		raw      string
		provider schemas.ModelProvider
		model    string
	}{
		{
			raw: "",
		},
		{
			raw:      "pending/",
			provider: "pending",
		},
		{
			raw:      "pending/namespace/model",
			provider: "pending",
			model:    "namespace/model",
		},
		{
			raw:      "openai/namespace/model",
			provider: schemas.OpenAI,
			model:    "namespace/model",
		},
		{
			raw:   "namespace/model",
			model: "namespace/model",
		},
		{
			raw:   "/model",
			model: "/model",
		},
		{
			raw:   " pending/model",
			model: " pending/model",
		},
	} {
		t.Run(tt.raw, func(t *testing.T) {
			provider, model := parseConfiguredModel(cfg, tt.raw)
			require.Equal(t, tt.provider, provider)
			require.Equal(t, tt.model, model)
		})
	}
}
