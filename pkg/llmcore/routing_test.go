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
	"github.com/stretchr/testify/require"
)

// pausedReplacementStore exposes the removal/insertion gap inside Bifrost's
// UpsertRule without relying on catching that brief window with a stress loop.
type pausedReplacementStore struct {
	rules.Store
	removed chan struct{}
	resume  chan struct{}
}

func (s *pausedReplacementStore) UpsertRule(ctx context.Context, rule *tables.TableRoutingRule) error {
	if err := s.Store.DeleteRule(ctx, rule.ID); err != nil {
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
	plugin, err := newRoutingPlugin(paused, logger)
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

// Pause after evaluation loads the old rule but before it fetches its compiled
// program. A writer here must not change the separately cached CEL program.
type pausedEvaluationLogger struct {
	schemas.Logger
	loaded chan struct{}
	resume chan struct{}
	once   sync.Once
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
			plugin, err := newRoutingPlugin(store, logger)
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
