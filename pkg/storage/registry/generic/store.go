package generic

import (
	"github.com/acorn-io/acorn/pkg/storage/scheme"
	"github.com/acorn-io/acorn/pkg/storage/tables"
	"github.com/acorn-io/kinm/pkg/db"
	"github.com/acorn-io/kinm/pkg/stores"
	"github.com/acorn-io/kinm/pkg/strategy"
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
