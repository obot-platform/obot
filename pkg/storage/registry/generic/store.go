package generic

import (
	"context"
	"errors"

	"github.com/obot-platform/kinm/pkg/db"
	"github.com/obot-platform/kinm/pkg/stores"
	"github.com/obot-platform/kinm/pkg/strategy"
	kinmtypes "github.com/obot-platform/kinm/pkg/types"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/storage/services"
	"github.com/obot-platform/obot/pkg/storage/tables"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apiserver/pkg/registry/rest"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type withTable struct {
	strategy.CompleteStrategy
	*strategy.TableAdapter
}

type hasColumns interface {
	GetColumns() [][]string
}

// limited makes Create and Update writes within limits. Status writes go through the status store, which isn't
// limited.
type limited struct {
	strategy.CompleteStrategy
	limits services.Limiter
}

// NewStore returns obj's store and status store. With limits, the store's Create and Update writes, which patches
// become, are made within them.
func NewStore(db *db.Factory, obj kclient.Object, limits services.Limiter) (rest.Storage, rest.Storage, error) {
	storage, err := db.NewDBStrategy(obj)
	if err != nil {
		return nil, nil, err
	}

	var tableStrategy any
	if obj, ok := obj.(hasColumns); ok {
		tableStrategy, err = tables.NewConverter(obj.GetColumns())
		if err != nil {
			return nil, nil, err
		}
	}

	newStorage := withTable{
		CompleteStrategy: storage,
		TableAdapter:     strategy.NewTable(tableStrategy),
	}
	if limits != nil {
		newStorage.CompleteStrategy = limited{
			CompleteStrategy: storage,
			limits:           limits,
		}
	}

	return stores.NewComplete(scheme.Scheme, newStorage), stores.NewStatus(scheme.Scheme, storage), err
}

func (l limited) Create(ctx context.Context, obj kinmtypes.Object) (kinmtypes.Object, error) {
	var created kinmtypes.Object
	err := l.limits.Within(ctx, obj, func() (err error) {
		created, err = l.CompleteStrategy.Create(ctx, obj)
		return err
	})
	return created, statusError(err)
}

func (l limited) Update(ctx context.Context, obj kinmtypes.Object) (kinmtypes.Object, error) {
	var updated kinmtypes.Object
	err := l.limits.Within(ctx, obj, func() (err error) {
		updated, err = l.CompleteStrategy.Update(ctx, obj)
		return err
	})
	return updated, statusError(err)
}

// statusError keeps a limit's HTTP status in the API, so clients see the refusal as it was made.
func statusError(err error) error {
	if httpErr, ok := errors.AsType[*types.ErrHTTP](err); ok {
		return &apierrors.StatusError{ErrStatus: metav1.Status{
			Status:  metav1.StatusFailure,
			Code:    int32(httpErr.Code),
			Message: httpErr.Message,
		}}
	}
	return err
}
