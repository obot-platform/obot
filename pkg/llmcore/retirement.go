package llmcore

import (
	"context"
	"fmt"
	"time"

	"github.com/maximhq/bifrost/core/schemas"
)

// Bound the whole batch, not each provider. A stalled upstream must not hold
// reconciliation indefinitely or multiply its delay by the provider count.
const (
	providerDrainWait = time.Second
)

type providerRetirement struct {
	done chan struct{}
	err  error // published by closing done
}

// retireProviders requires syncLock. Only the removal goroutines run outside
// the lock; they publish results through done and never mutate engine state.
func (e *Engine) retireProviders(ctx context.Context, providers map[schemas.ModelProvider]struct{}, remove func(schemas.ModelProvider) error) (map[schemas.ModelProvider]struct{}, []error) {
	if e.retirements == nil {
		e.retirements = make(map[schemas.ModelProvider]*providerRetirement)
	}
	for name := range providers {
		if _, running := e.retirements[name]; running {
			continue
		}
		drain := &providerRetirement{done: make(chan struct{})}
		e.retirements[name] = drain
		go func() {
			// Core can reroute a producer paused after hooks onto a replacement queue.
			// Wait inside the tracked job so Sync's shared drain budget still applies.
			if name == schemas.Anthropic {
				e.subscriptionLifetime.Lock()
				defer e.subscriptionLifetime.Unlock()
			}
			drain.err = remove(name)
			close(drain.done)
		}()
	}
	waitCtx, cancel := context.WithTimeout(ctx, providerDrainWait)
	defer cancel()
	blocked := make(map[schemas.ModelProvider]struct{})
	var errs []error
	for name := range providers {
		drain := e.retirements[name]
		// Prefer an already completed result even if the shared wait budget
		// expired while another provider was draining.
		select {
		case <-drain.done:
		case <-waitCtx.Done():
			// Both may become ready together; recheck completion before deferring it.
			select {
			case <-drain.done:
			default:
				blocked[name] = struct{}{}
				errs = append(errs, fmt.Errorf("provider %s is still draining: %w", name, waitCtx.Err()))
				continue
			}
		}
		delete(e.retirements, name)
		if drain.err != nil {
			blocked[name] = struct{}{}
			errs = append(errs, fmt.Errorf("retire provider %s: %w", name, drain.err))
		}
	}
	return blocked, errs
}
