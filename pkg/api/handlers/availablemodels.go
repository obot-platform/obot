package handlers

import (
	"strings"

	"github.com/obot-platform/obot/pkg/api/handlers/providers"

	openai "github.com/obot-platform/chat-completion-client"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/gateway/server/dispatcher"
	"github.com/obot-platform/obot/pkg/license"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type AvailableModelsHandler struct {
	dispatcher      *dispatcher.Dispatcher
	licenseProvider *license.Provider
}

func NewAvailableModelsHandler(dispatcher *dispatcher.Dispatcher, licenseProvider *license.Provider) *AvailableModelsHandler {
	return &AvailableModelsHandler{
		dispatcher:      dispatcher,
		licenseProvider: licenseProvider,
	}
}

func (a *AvailableModelsHandler) List(req api.Context) error {
	var modelProviders v1.ModelProviderList
	if err := req.List(&modelProviders, &kclient.ListOptions{
		Namespace: req.Namespace(),
	}); err != nil {
		return err
	}

	var oModels openai.ModelsList
	for _, modelProvider := range modelProviders.Items {
		mps, err := providers.ModelProviderStatus(req.Context(), modelProvider, nil, a.licenseProvider)
		if err != nil {
			return err
		}
		if !mps.Configured {
			continue
		}

		m, err := a.dispatcher.ModelsForProvider(req.Context(), modelProvider)
		if err != nil {
			return err
		}

		for _, model := range m.Models {
			if model.Metadata == nil {
				model.Metadata = make(map[string]string)
			}
			model.Metadata["model-provider"] = modelProvider.Name
			oModels.Models = append(oModels.Models, model)
		}
	}

	return req.Write(oModels)
}

func (a *AvailableModelsHandler) ListForModelProvider(req api.Context) error {
	modelProviderID := req.PathValue("model_provider_id")
	var modelProvider v1.ModelProvider
	if err := req.Get(&modelProvider, modelProviderID); err != nil {
		return err
	}

	if len(modelProvider.Status.MissingConfigurationParameters) != 0 {
		return types.NewErrBadRequest("model provider %s is not configured, missing configuration parameters: %s", modelProvider.Name, strings.Join(modelProvider.Status.MissingConfigurationParameters, ", "))
	}

	oModels, err := a.dispatcher.ModelsForProvider(req.Context(), modelProvider)
	if err != nil {
		return err
	}

	return req.Write(oModels)
}
