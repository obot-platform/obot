package agentbackend

import (
	"context"
	"errors"
)

var ErrDisabled = errors.New("agent runtime backend is disabled")

// Disabled is a Backend that consistently reports that hosted-agent runtime
// management is unavailable. It lets service wiring inject a non-nil backend
// without giving disabled mode any synthetic behavior.
type Disabled struct{}

var _ Backend = Disabled{}

func (Disabled) ReconcileInstance(context.Context, DesiredInstance) (InstanceObservation, error) {
	return InstanceObservation{}, ErrDisabled
}

func (Disabled) ObserveInstance(context.Context, InstanceRef) (InstanceObservation, error) {
	return InstanceObservation{}, ErrDisabled
}

func (Disabled) DeleteInstance(context.Context, InstanceRef) (DeleteResult, error) {
	return DeleteResult{}, ErrDisabled
}

func (Disabled) ReconcilePool(context.Context, DesiredPool) (PoolObservation, error) {
	return PoolObservation{}, ErrDisabled
}

func (Disabled) ObservePool(context.Context, PoolRef) (PoolObservation, error) {
	return PoolObservation{}, ErrDisabled
}

func (Disabled) DeletePool(context.Context, PoolRef) (DeleteResult, error) {
	return DeleteResult{}, ErrDisabled
}

func (Disabled) GetPoolUtilization(context.Context, PoolRef) (UtilizationSnapshot, error) {
	return UtilizationSnapshot{}, ErrDisabled
}
