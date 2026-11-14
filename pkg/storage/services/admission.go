package services

import (
	"context"
	"sync"

	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// Admitter decides whether storage makes a write.
type Admitter interface {
	// Admit runs write, storage's write of obj, if obj may be written.
	Admit(ctx context.Context, obj kclient.Object, write func() error) error
}

// Admission admits storage's Create and Update writes. Its Admitter is set after storage starts, since it depends on
// storage, and until then every write is made.
type Admission struct {
	lock     sync.RWMutex
	admitter Admitter
}

func (a *Admission) Set(admitter Admitter) {
	a.lock.Lock()
	defer a.lock.Unlock()
	a.admitter = admitter
}

func (a *Admission) Admit(ctx context.Context, obj kclient.Object, write func() error) error {
	a.lock.RLock()
	admitter := a.admitter
	a.lock.RUnlock()
	if admitter == nil {
		return write()
	}
	return admitter.Admit(ctx, obj, write)
}
