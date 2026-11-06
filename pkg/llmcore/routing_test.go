package llmcore

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	bifrost "github.com/maximhq/bifrost/core"
	"github.com/maximhq/bifrost/core/schemas"
	"github.com/maximhq/bifrost/framework/configstore/tables"
	"github.com/maximhq/bifrost/plugins/routing/rules"
	llmcoreclient "github.com/obot-platform/obot/pkg/llmcore/client"
	"github.com/stretchr/testify/require"
)

// pausedReplacementStore exposes the removal/insertion gap inside Bifrost's
// UpsertRule without relying on catching that brief window with a stress loop.
type pausedReplacementStore struct {
	rules.Store
	removed chan struct{}
	resume  chan struct{}
}

// Pause after evaluation loads the old rule but before it fetches its compiled
// program. A writer here must not change the separately cached CEL program.
type pausedEvaluationLogger struct {
	schemas.Logger
	loaded chan struct{}
	resume chan struct{}
	once   sync.Once
}

func (s *pausedReplacementStore) UpsertRule(ctx context.Context, rule *tables.TableRoutingRule) error {
	if err := s.DeleteRule(ctx, rule.ID); err != nil {
		return err
	}
	close(s.removed)
	<-s.resume
	return s.Store.UpsertRule(ctx, rule)
}

func policyRule(model string) tables.TableRoutingRule {
	return tables.TableRoutingRule{
		ID:            "policy",
		Name:          "policy",
		Scope:         "global",
		CelExpression: "true",
		Targets:       []tables.TableRoutingTarget{{Model: &model, Weight: 1}},
	}
}

func policyRequest() *schemas.BifrostRequest {
	return &schemas.BifrostRequest{
		RequestType: schemas.ChatCompletionRequest,
		ChatRequest: &schemas.BifrostChatRequest{Provider: schemas.OpenAI, Model: "unrouted"},
	}
}

func waitRoutingSignal(t *testing.T, signal <-chan struct{}) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(3 * time.Second):
		t.Fatal("routing operation did not reach expected phase")
	}
}

func TestRuleReplacementDoesNotExposeEmptyPolicy(t *testing.T) {
	logger := bifrost.NewDefaultLogger(schemas.LogLevelWarn)
	store, err := rules.NewLocalStore(t.Context(), logger, nil)
	require.NoError(t, err)
	oldRule := policyRule("old-route")
	require.NoError(t, store.UpsertRule(t.Context(), &oldRule))
	paused := &pausedReplacementStore{Store: store, removed: make(chan struct{}), resume: make(chan struct{})}
	var resume sync.Once
	defer resume.Do(func() { close(paused.resume) })
	plugin, err := newRoutingPlugin(paused, logger, nil)
	require.NoError(t, err)
	replaced := make(chan error, 1)
	newRule := policyRule("new-route")
	go func() { replaced <- plugin.UpsertRule(t.Context(), &newRule) }()
	waitRoutingSignal(t, paused.removed)

	req := policyRequest()
	evaluated := make(chan error, 1)
	started := make(chan struct{})
	go func() {
		close(started)
		evaluated <- plugin.PreRequestHook(schemas.NewBifrostContext(t.Context(), schemas.NoDeadline), req)
	}()
	waitRoutingSignal(t, started)
	select {
	case err := <-evaluated:
		t.Fatalf("request evaluated an incomplete replacement: err=%v request=%+v", err, req.ChatRequest)
	case <-time.After(100 * time.Millisecond):
	}
	resume.Do(func() { close(paused.resume) })
	require.NoError(t, <-replaced)
	require.NoError(t, <-evaluated)
	require.Equal(t, "new-route", req.ChatRequest.Model)
}

func (l *pausedEvaluationLogger) Debug(msg string, args ...any) {
	if strings.HasPrefix(msg, "[Engine] Evaluating rule: ") {
		l.once.Do(func() { close(l.loaded); <-l.resume })
	}
	l.Logger.Debug(msg, args...)
}

func TestRuleMutationWaitsForFullEvaluation(t *testing.T) {
	for _, mutation := range []string{"replace", "delete"} {
		t.Run(mutation, func(t *testing.T) {
			logger := &pausedEvaluationLogger{
				Logger: bifrost.NewDefaultLogger(schemas.LogLevelWarn),
				loaded: make(chan struct{}),
				resume: make(chan struct{}),
			}
			var resume sync.Once
			defer resume.Do(func() { close(logger.resume) })
			store, err := rules.NewLocalStore(t.Context(), logger, nil)
			require.NoError(t, err)
			plugin, err := newRoutingPlugin(store, logger, nil)
			require.NoError(t, err)
			oldRule := policyRule("old-route")
			require.NoError(t, plugin.UpsertRule(t.Context(), &oldRule))
			req := policyRequest()
			evaluated := make(chan error, 1)
			go func() {
				evaluated <- plugin.PreRequestHook(schemas.NewBifrostContext(t.Context(), schemas.NoDeadline), req)
			}()
			waitRoutingSignal(t, logger.loaded)
			mutated := make(chan error, 1)
			started := make(chan struct{})
			go func() {
				close(started)
				if mutation == "delete" {
					mutated <- plugin.DeleteRule(t.Context(), oldRule.ID)
				} else {
					next := policyRule("new-route")
					next.CelExpression = "false"
					mutated <- plugin.UpsertRule(t.Context(), &next)
				}
			}()
			waitRoutingSignal(t, started)
			select {
			case err := <-mutated:
				t.Fatalf("rule mutation finished during evaluation: %v", err)
			case <-time.After(100 * time.Millisecond):
			}
			resume.Do(func() { close(logger.resume) })
			require.NoError(t, <-evaluated)
			require.NoError(t, <-mutated)
			require.Equal(t, "old-route", req.ChatRequest.Model)
			nextReq := policyRequest()
			require.NoError(t, plugin.PreRequestHook(schemas.NewBifrostContext(t.Context(), schemas.NoDeadline), nextReq))
			require.Equal(t, "unrouted", nextReq.ChatRequest.Model, "subsequent requests see the completed edit")
		})
	}
}

func TestRoutingValidationAcceptsBooleanExpressions(t *testing.T) {
	logger := bifrost.NewDefaultLogger(schemas.LogLevelWarn)
	store, err := rules.NewLocalStore(t.Context(), logger, nil)
	require.NoError(t, err)
	plugin, err := newRoutingPlugin(store, logger, nil)
	require.NoError(t, err)
	for _, expression := range []string{
		"",
		"true",
		"false",
		"[true, false][0]",
		`params.tier == 'premium' ? true : false`,
		`headers['X-Route'] == 'yes' && params['Tier'] == 'premium'`,
		`model != '' && provider != '' && request_type != ''`,
		`virtual_key_id == '' && virtual_key_name == '' && user_id == ''`,
		`team_id == '' && team_name == '' && customer_id == '' && customer_name == ''`,
		`tokens_used < 1.0 && request < 1.0 && budget_used < 1.0`,
		`complexity_tier == 'high'`,
	} {
		t.Run(expression, func(t *testing.T) {
			rule := policyRule("selected")
			rule.CelExpression = expression
			require.NoError(t, plugin.UpsertRule(t.Context(), &rule))
			// Validate with the real Bifrost store too: declarations and key
			// normalization must agree with the runtime compiler.
			_, err := store.GetProgram(t.Context(), &rule)
			require.NoError(t, err)
		})
	}
}

func TestRuleLegacyFallbackAfterUnregister(t *testing.T) {
	a := &account{}
	a.config.Store(&llmcoreclient.Config{Providers: map[schemas.ModelProvider]schemas.ProviderConfig{"local-rule": {}}})
	logger := bifrost.NewDefaultLogger(schemas.LogLevelWarn)
	store, err := rules.NewLocalStore(t.Context(), logger, nil)
	require.NoError(t, err)
	plugin, err := newRoutingPlugin(store, logger, a)
	require.NoError(t, err)
	rule := policyRule("failed-primary")
	var fallback tables.RoutingFallback
	require.NoError(t, fallback.UnmarshalJSON([]byte(`"local-rule/namespace/backup"`)))
	rule.ParsedFallbacks = []tables.RoutingFallback{fallback}
	require.NoError(t, plugin.UpsertRule(t.Context(), &rule))
	schemas.UnregisterKnownProvider("local-rule")
	req := policyRequest()
	require.NoError(t, plugin.PreRequestHook(schemas.NewBifrostContext(t.Context(), schemas.NoDeadline), req))
	require.Equal(t, []schemas.Fallback{{Provider: "local-rule", Model: "namespace/backup"}}, req.ChatRequest.Fallbacks)
}

func TestRuleFallbackParsingSnapshotAndNamespaces(t *testing.T) {
	a := &account{}
	configured := &llmcoreclient.Config{Providers: map[schemas.ModelProvider]schemas.ProviderConfig{"local-rule": {}}}
	a.config.Store(configured)
	logger := bifrost.NewDefaultLogger(schemas.LogLevelWarn)
	store, err := rules.NewLocalStore(t.Context(), logger, nil)
	require.NoError(t, err)
	plugin, err := newRoutingPlugin(store, logger, a)
	require.NoError(t, err)
	schemas.RegisterKnownProvider("foreign-rule")
	defer schemas.UnregisterKnownProvider("foreign-rule")
	rule := policyRule("selected")
	for _, raw := range []string{`"unknown/backup"`, `"foreign-rule/backup"`, `"local-rule/namespace/backup"`, `{"provider":"local-rule","key_id":"pinned"}`, `{"provider":"missing-custom","model":"namespace/model","key_id":"missing-pin"}`} {
		var fb tables.RoutingFallback
		require.NoError(t, fb.UnmarshalJSON([]byte(raw)))
		rule.ParsedFallbacks = append(rule.ParsedFallbacks, fb)
	}
	require.NoError(t, plugin.UpsertRule(t.Context(), &rule))
	schemas.UnregisterKnownProvider("local-rule")
	ctx := schemas.NewBifrostContext(t.Context(), schemas.NoDeadline)
	ctx.SetValue(modelConfigKey{}, configured)
	// Primary HTTP conversion and rule parsing share the captured view even if
	// Sync publishes another config in between. Credential safety does not.
	a.config.Store(&llmcoreclient.Config{})
	req := policyRequest()
	require.NoError(t, plugin.PreRequestHook(ctx, req))
	require.Equal(t, []schemas.Fallback{
		{
			Provider: "local-rule",
			Model:    "namespace/backup",
		},
		{
			Provider: "local-rule",
			Model:    "unrouted",
			KeyID:    "pinned",
		},
		{
			Provider: "missing-custom",
			Model:    "namespace/model",
			KeyID:    "missing-pin",
		},
	}, req.ChatRequest.Fallbacks)
}
