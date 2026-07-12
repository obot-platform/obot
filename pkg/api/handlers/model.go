package handlers

import (
	"errors"
	"fmt"
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/modelaccesspolicy"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type ModelHandler struct {
	mapHelper *modelaccesspolicy.Helper
}

func NewModelHandler(mapHelper *modelaccesspolicy.Helper) *ModelHandler {
	return &ModelHandler{
		mapHelper: mapHelper,
	}
}

func (a *ModelHandler) List(req api.Context) error {
	var modelList v1.ModelList
	if err := req.List(&modelList); err != nil {
		return err
	}

	var (
		allowAll      = req.URL.Query().Get("all") == "true" && (req.UserIsAdmin() || req.UserIsAuditor())
		allowedModels map[string]bool
		err           error
	)

	if !allowAll {
		allowedModels, allowAll, err = a.mapHelper.GetUserAllowedModels(req.User)
		if err != nil {
			return err
		}
	}

	var modelProviders v1.ModelProviderList
	if err := req.Storage.List(req.Context(), &modelProviders, &kclient.ListOptions{
		Namespace: req.Namespace(),
	}); err != nil {
		return err
	}

	modelProviderMap := make(map[string]v1.ModelProvider)
	for _, mp := range modelProviders.Items {
		modelProviderMap[mp.Name] = mp
	}

	respList := make([]types.Model, 0, len(modelList.Items))
	for _, model := range modelList.Items {
		if !allowAll && !allowedModels[model.Name] {
			// Skip models the user is not allowed to access
			continue
		}

		modelProvider, ok := modelProviderMap[model.Spec.Manifest.ModelProvider]
		if !ok {
			return types.NewErrNotFound("model provider %s not found", model.Spec.Manifest.ModelProvider)
		}

		resp, err := convertModel(model, modelProvider)
		if err != nil {
			return err
		}

		respList = append(respList, resp)
	}

	return req.Write(types.ModelList{Items: respList})
}

func (a *ModelHandler) ByID(req api.Context) error {
	var model v1.Model
	if err := req.Get(&model, req.PathValue("id")); err != nil {
		return err
	}

	var modelProvider v1.ModelProvider
	if err := req.Storage.Get(req.Context(), kclient.ObjectKey{Namespace: model.Namespace, Name: model.Spec.Manifest.ModelProvider}, &modelProvider); err != nil {
		return err
	}

	resp, err := convertModel(model, modelProvider)
	if err != nil {
		return err
	}

	return req.Write(resp)
}

func (a *ModelHandler) Update(req api.Context) error {
	var model types.ModelManifest
	if err := req.Read(&model); err != nil {
		return err
	}

	var existing v1.Model
	if err := req.Get(&existing, req.PathValue("id")); err != nil {
		return err
	}

	existing.Spec.Manifest = model

	if err := validateModelManifestAndSetDefaults(&existing); err != nil {
		return err
	}

	if err := req.Update(&existing); err != nil {
		return err
	}

	var modelProvider v1.ModelProvider
	if err := req.Storage.Get(req.Context(), kclient.ObjectKey{Namespace: existing.Namespace, Name: existing.Spec.Manifest.ModelProvider}, &modelProvider); err != nil {
		return err
	}

	resp, err := convertModel(existing, modelProvider)
	if err != nil {
		return err
	}

	return req.Write(resp)
}

func (a *ModelHandler) Create(req api.Context) error {
	var modelManifest types.ModelManifest
	if err := req.Read(&modelManifest); err != nil {
		return err
	}

	if modelManifest.ModelProvider == "" {
		return types.NewErrBadRequest("model provider is required")
	}

	var modelProvider v1.ModelProvider
	if err := req.Get(&modelProvider, modelManifest.ModelProvider); err != nil {
		return err
	}

	model := &v1.Model{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: system.ModelPrefix,
			Namespace:    req.Namespace(),
		},
		Spec: v1.ModelSpec{
			Manifest: modelManifest,
		},
	}

	if err := validateModelManifestAndSetDefaults(model); err != nil {
		return err
	}

	if err := req.Create(model); err != nil {
		return err
	}

	resp, err := convertModel(*model, modelProvider)
	if err != nil {
		return err
	}

	return req.Write(resp)
}

func (a *ModelHandler) Delete(req api.Context) error {
	return req.Delete(&v1.Model{
		ObjectMeta: metav1.ObjectMeta{
			Name:      req.PathValue("id"),
			Namespace: req.Namespace(),
		},
	})
}

func convertModel(model v1.Model, modelProvider v1.ModelProvider) (types.Model, error) {
	var aliasAssigned *bool
	if model.Generation == model.Status.ObservedGeneration {
		aliasAssigned = &model.Status.AliasAssigned
	}

	return types.Model{
		Metadata:      MetadataFrom(&model),
		ModelManifest: model.Spec.Manifest,
		ModelStatus: types.ModelStatus{
			AliasAssigned:     aliasAssigned,
			ModelProviderName: modelProvider.Name,
			Icon:              modelProvider.Spec.Icon,
			IconDark:          modelProvider.Spec.IconDark,
			Cost:              model.Status.Cost,
		},
	}, nil
}

func validateModelManifestAndSetDefaults(newModel *v1.Model) error {
	var errs []error
	newModel.Spec.Manifest.Name = strings.TrimSpace(newModel.Spec.Manifest.Name)
	if newModel.Spec.Manifest.Name == "" {
		newModel.Spec.Manifest.Name = strings.ReplaceAll(strings.TrimSpace(newModel.Spec.Manifest.TargetModel), "/", "-")
	}
	if strings.Contains(newModel.Spec.Manifest.Name, "/") {
		errs = append(errs, fmt.Errorf("field name must be a single path segment and must not contain '/'"))
	}
	if strings.TrimSpace(newModel.Spec.Manifest.TargetModel) == "" {
		errs = append(errs, fmt.Errorf("field targetModel is required"))
	}
	if strings.TrimSpace(newModel.Spec.Manifest.ModelProvider) == "" {
		errs = append(errs, fmt.Errorf("field modelProvider is required"))
	}

	return errors.Join(errs...)
}
