package llmcore

import (
	"context"
	"fmt"
	"net/http"
	"sync"
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

type subscriptionLifetime struct {
	sync.RWMutex
	// Admission registers cancellation before retirement can snapshot the generation.
	mu         sync.Mutex
	generation context.Context
	cancel     context.CancelFunc
}

func (l *subscriptionLifetime) admit(ctx context.Context, w http.ResponseWriter, transitioning func() bool) (context.Context, func(), bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if transitioning() || !l.TryRLock() {
		return nil, nil, false
	}
	if l.generation == nil {
		l.generation, l.cancel = context.WithCancel(context.Background())
	}
	ctx, cancel := context.WithCancel(ctx)
	interrupted := make(chan struct{})
	stop := context.AfterFunc(l.generation, func() {
		defer close(interrupted)
		cancel()
		controller := http.NewResponseController(w)
		expired := time.Now()
		// Context cancellation alone cannot interrupt a transport write or body read.
		_ = controller.SetWriteDeadline(expired)
		_ = controller.SetReadDeadline(expired)
	})
	return ctx, func() {
		// Join the callback before net/http can reuse the writer or connection.
		if !stop() {
			<-interrupted
		}
		cancel()
		// The handler has joined stream cleanup, so core cannot reroute its
		// subscription request onto a replacement queue.
		l.RUnlock()
	}, true
}

func (l *subscriptionLifetime) lockForRetirement() {
	l.mu.Lock()
	cancel := l.cancel
	l.mu.Unlock()
	var timer *time.Timer
	if cancel != nil {
		// The tracked retirement outlives any individual Sync caller.
		timer = time.AfterFunc(providerDrainWait, cancel)
	}
	l.Lock()
	if timer != nil {
		timer.Stop()
		cancel()
	}
	l.mu.Lock()
	l.generation, l.cancel = nil, nil
	l.mu.Unlock()
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
			lifetime := e.subscriptionLock(name)
			if lifetime != nil {
				lifetime.lockForRetirement()
			}
			drain.err = remove(name)
			if lifetime != nil {
				lifetime.Unlock()
			}
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

// Separate readers let unrelated providers reconcile while a subscription streams.
func (e *Engine) subscriptionLock(provider schemas.ModelProvider) *subscriptionLifetime {
	switch provider {
	case schemas.Anthropic:
		return &e.claudeSubscriptionLifetime
	case schemas.OpenAI:
		return &e.chatgptSubscriptionLifetime
	default:
		return nil
	}
}
