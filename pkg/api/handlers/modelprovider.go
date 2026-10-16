package handlers

import (
	"fmt"
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/api/handlers/providers"
	"github.com/obot-platform/obot/pkg/gateway/server/dispatcher"
	"github.com/obot-platform/obot/pkg/i18n"
	"github.com/obot-platform/obot/pkg/license"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type ModelProviderHandler struct {
	dispatcher *dispatcher.Dispatcher
	license    *license.Provider
}

func NewModelProviderHandler(dispatcher *dispatcher.Dispatcher, licenseProvider *license.Provider) *ModelProviderHandler {
	return &ModelProviderHandler{
		dispatcher: dispatcher,
		license:    licenseProvider,
	}
}

func (mp *ModelProviderHandler) ByID(req api.Context) error {
	var modelProvider v1.ModelProvider
	if err := req.Get(&modelProvider, req.PathValue("model_provider_id")); err != nil {
		return err
	}

	mps, err := providers.ModelProviderStatus(req.Context(), modelProvider, nil, mp.license)
	if err != nil {
		return err
	}

	locale := providerResponseLocale(req)
	return req.Write(mp.convertModelProvider(modelProvider, *mps, locale))
}

func (mp *ModelProviderHandler) List(req api.Context) error {
	var modelProviders v1.ModelProviderList
	if err := req.List(&modelProviders, &kclient.ListOptions{
		Namespace: req.Namespace(),
	}); err != nil {
		return err
	}

	resp := make([]types.ModelProvider, 0, len(modelProviders.Items))
	locale := providerResponseLocale(req)
	for _, modelProvider := range modelProviders.Items {
		mps, err := providers.ModelProviderStatus(req.Context(), modelProvider, nil, mp.license)
		if err != nil {
			return err
		}

		resp = append(resp, mp.convertModelProvider(modelProvider, *mps, locale))
	}

	return req.Write(types.ModelProviderList{Items: resp})
}

func (mp *ModelProviderHandler) RefreshModels(req api.Context) error {
	var modelProvider v1.ModelProvider
	if err := req.Get(&modelProvider, req.PathValue("model_provider_id")); err != nil {
		return err
	}

	mps, err := providers.ModelProviderStatus(req.Context(), modelProvider, nil, mp.license)
	if err != nil {
		return err
	}

	if !mps.Configured {
		return types.NewErrBadRequest("%s", apiMessage(req, "model_not_configured",
			"provider", i18n.Resolve(providerResponseLocale(req), modelProvider.Spec.Name),
			"parameters", strings.Join(mps.MissingConfigurationParameters, ", ")))
	}

	if modelProvider.Annotations[v1.ModelProviderSyncAnnotation] == "" {
		if modelProvider.Annotations == nil {
			modelProvider.Annotations = make(map[string]string, 1)
		}
		modelProvider.Annotations[v1.ModelProviderSyncAnnotation] = "true"
	} else {
		delete(modelProvider.Annotations, v1.ModelProviderSyncAnnotation)
	}

	if err := req.Update(&modelProvider); err != nil {
		return fmt.Errorf("failed to sync models for model provider %q: %w", modelProvider.Name, err)
	}

	locale := providerResponseLocale(req)
	return req.Write(mp.convertModelProvider(modelProvider, *mps, locale))
}

func (mp *ModelProviderHandler) convertModelProvider(
	modelProvider v1.ModelProvider,
	modelProviderStatus types.ModelProviderStatus,
	locale string,
) types.ModelProvider {
	manifest := *modelProvider.Spec.ModelProviderManifest.DeepCopy()
	localizeProviderMetadata(&manifest.CommonProviderMetadata, locale)
	return types.ModelProvider{
		Metadata:              MetadataFrom(&modelProvider),
		ModelProviderManifest: manifest,
		ModelProviderStatus:   modelProviderStatus,
	}
}
