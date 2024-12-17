package handlers

import (
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/otto.otto8.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type DefaultModelAliasHandler struct{}

func NewDefaultModelAliasHandler() *DefaultModelAliasHandler {
	return &DefaultModelAliasHandler{}
}

func (d *DefaultModelAliasHandler) Create(req api.Context) error {
	var manifest types.DefaultModelAliasManifest
	if err := req.Read(&manifest); err != nil {
		return err
	}

	dma := v1.DefaultModelAlias{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: system.DefaultModelAliasPrefix,
			Namespace:    req.Namespace(),
		},
		Spec: v1.DefaultModelAliasSpec{
			Manifest: manifest,
		},
	}

	if err := req.Create(&dma); err != nil {
		return err
	}

	return req.WriteCreated(convertDefaultModelAlias(dma))
}

func (d *DefaultModelAliasHandler) GetByID(req api.Context) error {
	var dma v1.DefaultModelAlias
	if err := req.Get(&dma, req.PathValue("id")); err != nil {
		return err
	}
	return req.Write(convertDefaultModelAlias(dma))
}

func (d *DefaultModelAliasHandler) List(req api.Context) error {
	var dmaList v1.DefaultModelAliasList
	if err := req.List(&dmaList); err != nil {
		return err
	}

	resp := make([]types.DefaultModelAlias, 0, len(dmaList.Items))
	for _, dma := range dmaList.Items {
		resp = append(resp, convertDefaultModelAlias(dma))
	}
	return req.Write(types.DefaultModelAliasList{Items: resp})
}

func (d *DefaultModelAliasHandler) Update(req api.Context) error {
	var dma v1.DefaultModelAlias
	if err := req.Get(&dma, req.PathValue("id")); err != nil {
		return err
	}

	var manifest types.DefaultModelAliasManifest
	if err := req.Read(&manifest); err != nil {
		return err
	}

	dma.Spec.Manifest = manifest
	if err := req.Update(&dma); err != nil {
		return err
	}

	return req.WriteCreated(convertDefaultModelAlias(dma))
}

func (d *DefaultModelAliasHandler) Delete(req api.Context) error {
	return req.Delete(&v1.DefaultModelAlias{
		ObjectMeta: metav1.ObjectMeta{
			Name:      req.PathValue("id"),
			Namespace: req.Namespace(),
		},
	})
}

func convertDefaultModelAlias(d v1.DefaultModelAlias) types.DefaultModelAlias {
	return types.DefaultModelAlias{
		DefaultModelAliasManifest: d.Spec.Manifest,
	}
}
