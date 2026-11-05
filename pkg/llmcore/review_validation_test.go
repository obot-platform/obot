package llmcore

import (
	"context"
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
			plugin, err := newRoutingPlugin(store, logger)
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

func TestScopedRoutingOrderIsStable(t *testing.T) {
	now := time.Date(2026, 11, 4, 0, 0, 0, 0, time.UTC)
	for _, tt := range []struct {
		name           string
		firstCreated   time.Time
		secondCreated  time.Time
		firstPriority  int
		secondPriority int
		winner         string
	}{
		{
			name:          "newest first",
			firstCreated:  now,
			secondCreated: now.Add(time.Second),
			winner:        "b",
		},
		{
			name:          "equal timestamps",
			firstCreated:  now,
			secondCreated: now,
			winner:        "a",
		},
		{
			name:   "zero timestamps",
			winner: "a",
		},
		{
			name:           "priority before timestamp",
			firstCreated:   now,
			secondCreated:  now.Add(time.Second),
			secondPriority: 1,
			winner:         "a",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			a, b := policyRule("a"), policyRule("b")
			a.ID, b.ID = "a", "b"
			a.CreatedAt, b.CreatedAt = tt.firstCreated, tt.secondCreated
			a.Priority, b.Priority = tt.firstPriority, tt.secondPriority

			for _, order := range [][]tables.TableRoutingRule{{a, b}, {b, a}} {
				logger := bifrost.NewDefaultLogger(schemas.LogLevelWarn)
				store, err := rules.NewLocalStore(t.Context(), logger, nil)
				require.NoError(t, err)
				plugin, err := newRoutingPlugin(store, logger)
				require.NoError(t, err)
				for _, rule := range order {
					require.NoError(t, plugin.UpsertRule(t.Context(), &rule))
				}

				assertWinner := func() {
					t.Helper()
					req := policyRequest()
					require.NoError(t, plugin.PreRequestHook(schemas.NewBifrostContext(t.Context(), schemas.NoDeadline), req))
					require.Equal(t, tt.winner, req.ChatRequest.Model)
				}
				assertWinner()

				// Metadata edits and delete/reinsert must not turn insertion order into policy.
				for _, rule := range order {
					rule.Name = "renamed"
					require.NoError(t, plugin.UpsertRule(t.Context(), &rule))
					assertWinner()
					require.NoError(t, plugin.DeleteRule(t.Context(), rule.ID))
					require.NoError(t, plugin.UpsertRule(t.Context(), &rule))
					assertWinner()
				}

				seed := policyRule("intermediate")
				seed.ID = "seed"
				seed.Priority = -1
				seed.CelExpression = `model == 'unrouted'`
				seed.ChainRule = true
				require.NoError(t, plugin.UpsertRule(t.Context(), &seed))
				for _, rule := range order {
					rule.CelExpression = `model == 'intermediate'`
					require.NoError(t, plugin.UpsertRule(t.Context(), &rule))
				}
				assertWinner()
			}
		})
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
					Model:  schemas.Ptr("a"),
					Weight: 0,
				},
				{
					Model:  schemas.Ptr("b"),
					Weight: 0,
				},
			},
			models: []string{"a", "b"},
		},
		{
			name: "weighted target",
			targets: []tables.TableRoutingTarget{
				{
					Model:  schemas.Ptr("a"),
					Weight: 0,
				},
				{
					Model:  schemas.Ptr("b"),
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
				Provider: schemas.Ptr(""),
				Model:    schemas.Ptr(""),
				Weight:   1,
			}},
			models: []string{"unrouted"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			logger := bifrost.NewDefaultLogger(schemas.LogLevelWarn)
			store, err := rules.NewLocalStore(t.Context(), logger, nil)
			require.NoError(t, err)
			plugin, err := newRoutingPlugin(store, logger)
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

type sharedScopedStore struct {
	rules.Store
	scoped []*tables.TableRoutingRule
}

func (s sharedScopedStore) GetScopedRules(context.Context, string, string) []*tables.TableRoutingRule {
	return s.scoped
}

func TestScopedOrderDoesNotMutateStoreSlice(t *testing.T) {
	a, b := policyRule("a"), policyRule("b")
	a.ID, b.ID = "a", "b"
	shared := []*tables.TableRoutingRule{&b, &a}
	store := orderedRuleStore{Store: sharedScopedStore{scoped: shared}}

	ordered := store.GetScopedRules(t.Context(), "global", "")
	require.Equal(t, []*tables.TableRoutingRule{&a, &b}, ordered)
	require.Equal(t, []*tables.TableRoutingRule{&b, &a}, shared)
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
				StatusCode:  tt.status,
				Error:       &schemas.ErrorField{Message: tt.message},
				ExtraFields: schemas.BifrostErrorExtraFields{Provider: tt.provider},
			}
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
