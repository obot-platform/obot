// Package fake provides a process-local agent backend for development and
// provider contract tests. It deliberately stores no secret values.
package fake

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"sort"
	"sync"
	"time"

	"github.com/obot-platform/obot/pkg/agentbackend"
)

var ErrInvalidDesiredState = errors.New("invalid desired state")

type Config struct {
	TransitionDelay time.Duration
	Now             func() time.Time
}

type Backend struct {
	mu        sync.Mutex
	delay     time.Duration
	now       func() time.Time
	pools     map[string]*pool
	instances map[string]*instance
	events    chan agentbackend.Event
}

type pool struct {
	desired    agentbackend.DesiredPool
	generation int64
	state      agentbackend.State
	readyAt    time.Time
	deleteAt   time.Time
}

type instance struct {
	desired    agentbackend.DesiredInstance
	generation int64
	state      agentbackend.State
	readyAt    time.Time
	deleteAt   time.Time
	url        string
	urlReady   bool
}

var _ agentbackend.Backend = (*Backend)(nil)
var _ agentbackend.Subscriber = (*Backend)(nil)

func New(config Config) *Backend {
	now := config.Now
	if now == nil {
		now = time.Now
	}
	return &Backend{
		delay:     config.TransitionDelay,
		now:       now,
		pools:     map[string]*pool{},
		instances: map[string]*instance{},
		events:    make(chan agentbackend.Event, 128),
	}
}

func (b *Backend) ReconcilePool(_ context.Context, desired agentbackend.DesiredPool) (agentbackend.PoolObservation, error) {
	if desired.Ref.ID == "" || desired.Revision == "" || desired.Capacity.CPUVCPUs <= 0 ||
		desired.Capacity.MemoryBytes <= 0 || desired.Capacity.StorageBytes <= 0 {
		return agentbackend.PoolObservation{}, fmt.Errorf("%w: pool ID, revision, CPU, memory, and storage are required", ErrInvalidDesiredState)
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	b.progressLocked()

	current, ok := b.pools[desired.Ref.ID]
	if !ok {
		current = &pool{}
		b.pools[desired.Ref.ID] = current
	}
	desired.Ref.BackendID = backendID("pool", desired.Ref.ID)
	if !ok || !reflect.DeepEqual(current.desired, desired) {
		current.desired = clonePool(desired)
		current.generation++
		current.state = agentbackend.StatePending
		current.readyAt = b.now().Add(b.delay)
		b.emitLocked(poolEvent(desired.Ref))
	}
	return poolObservation(current), nil
}

func (b *Backend) ObservePool(_ context.Context, ref agentbackend.PoolRef) (agentbackend.PoolObservation, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.progressLocked()
	current := b.pools[ref.ID]
	if current == nil {
		return agentbackend.PoolObservation{Ref: ref}, nil
	}
	return poolObservation(current), nil
}

func (b *Backend) DeletePool(_ context.Context, ref agentbackend.PoolRef) (agentbackend.DeleteResult, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.progressLocked()
	current := b.pools[ref.ID]
	if current == nil {
		return agentbackend.DeleteResult{Complete: true}, nil
	}
	for _, sandbox := range b.instances {
		if sandbox.desired.Pool.ID == ref.ID {
			return agentbackend.DeleteResult{}, fmt.Errorf("%w: pool still has instances", ErrInvalidDesiredState)
		}
	}
	if current.state != agentbackend.StateDeleting {
		current.state = agentbackend.StateDeleting
		current.deleteAt = b.now().Add(b.delay)
		b.emitLocked(poolEvent(current.desired.Ref))
	}
	return agentbackend.DeleteResult{}, nil
}

func (b *Backend) ReconcileInstance(_ context.Context, desired agentbackend.DesiredInstance) (agentbackend.InstanceObservation, error) {
	if desired.Ref.ID == "" || desired.Pool.ID == "" || desired.Revision == "" {
		return agentbackend.InstanceObservation{}, fmt.Errorf("%w: instance ID, pool ID, and revision are required", ErrInvalidDesiredState)
	}

	// A real backend uses the secret values to write them wherever it keeps
	// secrets; this one has nowhere to write them and never reads them, so it
	// drops them at the door rather than holding them in a process-local map for
	// the lifetime of the instance.
	//
	// Redacting before the comparison below, not just before storing, is what
	// keeps a rotation from looking like a permanent difference: Version still
	// changes with the value, so a rotated secret is still seen as a change.
	desired = desired.Redacted()

	b.mu.Lock()
	defer b.mu.Unlock()
	b.progressLocked()
	pool := b.pools[desired.Pool.ID]
	if pool == nil || pool.state == agentbackend.StateDeleting {
		return agentbackend.InstanceObservation{}, fmt.Errorf("%w: pool %q does not exist", ErrInvalidDesiredState, desired.Pool.ID)
	}

	current, ok := b.instances[desired.Ref.ID]
	if !ok && pool.desired.Suspended {
		return errorInstanceObservation(desired.Ref, "PoolSuspended", "the pool does not permit new instances"), nil
	}
	desired.Ref.BackendID = backendID("instance", desired.Ref.ID)
	desired.Pool.BackendID = pool.desired.Ref.BackendID
	if !ok {
		current = &instance{url: "fake://hosted-agent/" + desired.Ref.ID}
		b.instances[desired.Ref.ID] = current
	}
	if !ok || !reflect.DeepEqual(current.desired, desired) {
		if pool.desired.Suspended {
			if !ok || current.state != agentbackend.StateReady {
				return errorInstanceObservation(desired.Ref, "PoolSuspended", "the pool does not permit instance starts"), nil
			}
			// Preserve the running sandbox without restarting it. Its observed
			// revision remains old until the pool is resumed.
			return instanceObservation(current), nil
		}
		current.desired = cloneInstance(desired)
		current.generation++
		current.state = agentbackend.StatePending
		current.readyAt = b.now().Add(b.delay)
		b.emitLocked(instanceEvent(desired.Ref))
	}
	return instanceObservation(current), nil
}

func (b *Backend) ObserveInstance(_ context.Context, ref agentbackend.InstanceRef) (agentbackend.InstanceObservation, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.progressLocked()
	current := b.instances[ref.ID]
	if current == nil {
		return agentbackend.InstanceObservation{Ref: ref}, nil
	}
	return instanceObservation(current), nil
}

func (b *Backend) DeleteInstance(_ context.Context, ref agentbackend.InstanceRef) (agentbackend.DeleteResult, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.progressLocked()
	current := b.instances[ref.ID]
	if current == nil {
		return agentbackend.DeleteResult{Complete: true}, nil
	}
	if current.state != agentbackend.StateDeleting {
		current.state = agentbackend.StateDeleting
		current.deleteAt = b.now().Add(b.delay)
		b.emitLocked(instanceEvent(current.desired.Ref))
	}
	return agentbackend.DeleteResult{}, nil
}

func (b *Backend) GetPoolUtilization(_ context.Context, ref agentbackend.PoolRef) (agentbackend.UtilizationSnapshot, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.progressLocked()
	pool := b.pools[ref.ID]
	if pool == nil {
		return agentbackend.UtilizationSnapshot{}, fmt.Errorf("%w: pool %q does not exist", ErrInvalidDesiredState, ref.ID)
	}
	var result agentbackend.UtilizationSnapshot
	result.Timestamp = b.now()
	for _, sandbox := range b.instances {
		if sandbox.desired.Pool.ID != ref.ID || sandbox.state == agentbackend.StateDeleting {
			continue
		}
		ordinal := float64((sandbox.generation%5)+1) / 10
		usage := agentbackend.ResourceUtilization{
			CPUVCPUs:     min(pool.desired.Capacity.CPUVCPUs, ordinal),
			MemoryBytes:  min(pool.desired.Capacity.MemoryBytes, int64(ordinal*float64(pool.desired.Capacity.MemoryBytes))),
			StorageBytes: min(pool.desired.Capacity.StorageBytes, int64(ordinal*float64(pool.desired.Capacity.StorageBytes))),
		}
		result.Instances = append(result.Instances, agentbackend.InstanceUtilization{
			Ref: sandbox.desired.Ref, State: sandbox.state, Utilization: usage,
		})
		result.Pool.CPUVCPUs += usage.CPUVCPUs
		result.Pool.MemoryBytes += usage.MemoryBytes
		result.Pool.StorageBytes += usage.StorageBytes
	}
	result.Pool.CPUVCPUs = min(result.Pool.CPUVCPUs, pool.desired.Capacity.CPUVCPUs)
	result.Pool.MemoryBytes = min(result.Pool.MemoryBytes, pool.desired.Capacity.MemoryBytes)
	result.Pool.StorageBytes = min(result.Pool.StorageBytes, pool.desired.Capacity.StorageBytes)
	sort.Slice(result.Instances, func(i, j int) bool { return result.Instances[i].Ref.ID < result.Instances[j].Ref.ID })
	return result, nil
}

func (b *Backend) Subscribe(ctx context.Context, handler func(context.Context, agentbackend.Event) error) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case event := <-b.events:
			if err := handler(ctx, event); err != nil {
				return err
			}
		}
	}
}

func (b *Backend) progressLocked() {
	now := b.now()
	for id, current := range b.instances {
		switch {
		case current.state == agentbackend.StateDeleting && !now.Before(current.deleteAt):
			delete(b.instances, id)
			b.emitLocked(instanceEvent(current.desired.Ref))
		case current.state == agentbackend.StatePending && !now.Before(current.readyAt):
			current.state = agentbackend.StateReady
			current.urlReady = true
			b.emitLocked(instanceEvent(current.desired.Ref))
		}
	}
	for id, current := range b.pools {
		switch {
		case current.state == agentbackend.StateDeleting && !now.Before(current.deleteAt):
			delete(b.pools, id)
			b.emitLocked(poolEvent(current.desired.Ref))
		case current.state == agentbackend.StatePending && !now.Before(current.readyAt):
			current.state = agentbackend.StateReady
			b.emitLocked(poolEvent(current.desired.Ref))
		}
	}
}

func (b *Backend) emitLocked(event agentbackend.Event) {
	select {
	case b.events <- event:
	default:
		// Events are hints; polling remains the reliability path.
	}
}

func backendID(kind, id string) string { return "fake-" + kind + "-" + id }
func poolObservation(value *pool) agentbackend.PoolObservation {
	return agentbackend.PoolObservation{
		Ref: value.desired.Ref, Exists: true, ObservedRevision: observedRevision(value.state, value.desired.Revision),
		State: value.state, Schedulable: value.state == agentbackend.StateReady && !value.desired.Suspended,
		Capacity: value.desired.Capacity, BackendGeneration: value.generation,
	}
}

func instanceObservation(value *instance) agentbackend.InstanceObservation {
	url := ""
	if value.urlReady {
		url = value.url
	}
	return agentbackend.InstanceObservation{
		Ref: value.desired.Ref, Exists: true, ObservedRevision: observedRevision(value.state, value.desired.Revision),
		State: value.state, URL: url, BackendGeneration: value.generation,
	}
}

func errorInstanceObservation(ref agentbackend.InstanceRef, reason, message string) agentbackend.InstanceObservation {
	return agentbackend.InstanceObservation{
		Ref: ref, State: agentbackend.StateError, Reason: reason, Message: message,
	}
}

func observedRevision(state agentbackend.State, revision string) string {
	if state == agentbackend.StateReady {
		return revision
	}
	return ""
}

func poolEvent(ref agentbackend.PoolRef) agentbackend.Event {
	return agentbackend.Event{Kind: agentbackend.ResourceKindPool, Pool: ref}
}

func instanceEvent(ref agentbackend.InstanceRef) agentbackend.Event {
	return agentbackend.Event{Kind: agentbackend.ResourceKindInstance, Instance: ref}
}

func clonePool(value agentbackend.DesiredPool) agentbackend.DesiredPool {
	return value
}

func cloneInstance(value agentbackend.DesiredInstance) agentbackend.DesiredInstance {
	value.Files = slices.Clone(value.Files)
	for i := range value.Files {
		value.Files[i].Content = slices.Clone(value.Files[i].Content)
	}
	value.Secrets = slices.Clone(value.Secrets)
	if value.Env != nil {
		env := make(map[string]string, len(value.Env))
		maps.Copy(env, value.Env)
		value.Env = env
	}
	return value
}
