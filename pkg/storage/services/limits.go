package services

import (
	"context"
	"sync"

	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// Limiter decides whether storage makes a write.
type Limiter interface {
	// Within runs write, storage's write of obj, if the limits allow it.
	Within(ctx context.Context, obj kclient.Object, write func() error) error
}

// Limits is the limiter on storage's Create and Update writes. It's set after storage starts, since limiters depend
// on it, and until then every write is made.
type Limits struct {
	lock    sync.RWMutex
	limiter Limiter
}

func (l *Limits) Set(limiter Limiter) {
	l.lock.Lock()
	defer l.lock.Unlock()
	l.limiter = limiter
}

func (l *Limits) Within(ctx context.Context, obj kclient.Object, write func() error) error {
	l.lock.RLock()
	limiter := l.limiter
	l.lock.RUnlock()
	if limiter == nil {
		return write()
	}
	return limiter.Within(ctx, obj, write)
}
