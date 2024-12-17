package generic

import (
	"github.com/obot-platform/kinm/pkg/db"
	"github.com/obot-platform/kinm/pkg/stores"
	"github.com/obot-platform/kinm/pkg/strategy"
	"github.com/obot-platform/obot/pkg/storage/scheme"
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

func NewStore(db *db.Factory, obj kclient.Object) (rest.Storage, rest.Storage, error) {
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

	return stores.NewComplete(scheme.Scheme, newStorage), stores.NewStatus(scheme.Scheme, storage), err
}
