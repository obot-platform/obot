package generic

import (
	"context"

	"github.com/obot-platform/kinm/pkg/db"
	"github.com/obot-platform/kinm/pkg/stores"
	"github.com/obot-platform/kinm/pkg/strategy"
	kinmtypes "github.com/obot-platform/kinm/pkg/types"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/storage/services"
	"github.com/obot-platform/obot/pkg/storage/tables"
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

// admitted makes Create and Update writes only once they're admitted. Status writes go through the status store, which
// admits everything.
type admitted struct {
	strategy.CompleteStrategy
	admitter services.Admitter
}

// NewStore returns obj's store and status store. With an admitter, the store's Create and Update writes, which patches
// become, are made only once it admits them.
func NewStore(db *db.Factory, obj kclient.Object, admitter services.Admitter) (rest.Storage, rest.Storage, error) {
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
	if admitter != nil {
		newStorage.CompleteStrategy = admitted{
			CompleteStrategy: storage,
			admitter:         admitter,
		}
	}

	return stores.NewComplete(scheme.Scheme, newStorage), stores.NewStatus(scheme.Scheme, storage), err
}

func (a admitted) Create(ctx context.Context, obj kinmtypes.Object) (kinmtypes.Object, error) {
	var created kinmtypes.Object
	err := a.admitter.Admit(ctx, obj, func() (err error) {
		created, err = a.CompleteStrategy.Create(ctx, obj)
		return err
	})
	return created, err
}

func (a admitted) Update(ctx context.Context, obj kinmtypes.Object) (kinmtypes.Object, error) {
	var updated kinmtypes.Object
	err := a.admitter.Admit(ctx, obj, func() (err error) {
		updated, err = a.CompleteStrategy.Update(ctx, obj)
		return err
	})
	return updated, err
}
