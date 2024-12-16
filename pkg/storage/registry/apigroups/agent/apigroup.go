package agent

import (
	"strings"

	v1 "github.com/acorn-io/acorn/pkg/storage/apis/otto.otto8.ai/v1"
	"github.com/acorn-io/acorn/pkg/storage/registry/generic"
	"github.com/acorn-io/acorn/pkg/storage/scheme"
	"github.com/acorn-io/acorn/pkg/storage/services"
	"github.com/acorn-io/kinm/pkg/apigroup"
	"github.com/acorn-io/nah/pkg/typed"
	coordinationv1 "k8s.io/api/coordination/v1"
	"k8s.io/apiserver/pkg/registry/rest"
	genericapiserver "k8s.io/apiserver/pkg/server"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func Stores(services *services.Services) (map[string]rest.Storage, error) {
	var (
		result   = map[string]rest.Storage{}
		generics = map[string]kclient.Object{}
	)

	for gvk := range services.DB.Scheme().AllKnownTypes() {
		if gvk.Group == v1.SchemeGroupVersion.Group && gvk.Version == v1.SchemeGroupVersion.Version {
			obj, err := services.DB.Scheme().New(gvk)
			if err != nil {
				return nil, err
			}
			if o, ok := obj.(kclient.Object); ok {
				generics[strings.ToLower(gvk.Kind)+"s"] = o
			}
		}
	}

	for _, name := range typed.SortedKeys(generics) {
		store, statusStore, err := generic.NewStore(services.DB, generics[name])
		if err != nil {
			return nil, err
		}

		result[name] = store
		result[name+"/status"] = statusStore
	}

	return result, nil
}

func LeasesAPIGroup(services *services.Services) (*genericapiserver.APIGroupInfo, error) {
	store, _, err := generic.NewStore(services.DB, &coordinationv1.Lease{})
	if err != nil {
		return nil, err
	}
	return apigroup.ForStores(scheme.AddToScheme, map[string]rest.Storage{
		"leases": store,
	}, coordinationv1.SchemeGroupVersion)
}

func APIGroup(services *services.Services) (*genericapiserver.APIGroupInfo, error) {
	stores, err := Stores(services)
	if err != nil {
		return nil, err
	}
	return apigroup.ForStores(scheme.AddToScheme, stores, v1.SchemeGroupVersion)
}
