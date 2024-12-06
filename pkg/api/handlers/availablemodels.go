package handlers

import (
	"fmt"
	"strings"

	openai "github.com/gptscript-ai/chat-completion-client"
	"github.com/gptscript-ai/go-gptscript"
	"github.com/otto8-ai/otto8/apiclient/types"
	"github.com/otto8-ai/otto8/pkg/api"
	"github.com/otto8-ai/otto8/pkg/availablemodels"
	"github.com/otto8-ai/otto8/pkg/gateway/server/dispatcher"
	"github.com/otto8-ai/otto8/pkg/storage/apis/otto.otto8.ai/v1"
	"github.com/otto8-ai/otto8/pkg/system"
	"k8s.io/apimachinery/pkg/fields"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type AvailableModelsHandler struct {
	dispatcher *dispatcher.Dispatcher
	gptscript  *gptscript.GPTScript
}

func NewAvailableModelsHandler(gClient *gptscript.GPTScript, dispatcher *dispatcher.Dispatcher) *AvailableModelsHandler {
	return &AvailableModelsHandler{
		gptscript:  gClient,
		dispatcher: dispatcher,
	}
}

func (a *AvailableModelsHandler) List(req api.Context) error {
	var modelProviderReference v1.ToolReferenceList
	if err := req.List(&modelProviderReference, &kclient.ListOptions{
		Namespace: req.Namespace(),
		FieldSelector: fields.SelectorFromSet(map[string]string{
			"spec.type": string(types.ToolReferenceTypeModelProvider),
		}),
	}); err != nil {
		return err
	}

	var oModels openai.ModelsList
	for _, modelProvider := range modelProviderReference.Items {
		if convertedModelProvider, err := convertModelProviderToolRef(req.Context(), a.gptscript, modelProvider); err != nil {
			return fmt.Errorf("failed to determine if model provider %q is configured: %w", modelProvider.Name, err)
		} else if !convertedModelProvider.Configured || modelProvider.Name == system.ModelProviderTool {
			continue
		}

		m, err := availablemodels.ForProvider(req.Context(), a.dispatcher, modelProvider.Namespace, modelProvider.Name)
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
	var modelProviderReference v1.ToolReference
	if err := req.Get(&modelProviderReference, req.PathValue("model_provider")); err != nil {
		return err
	}

	if modelProviderReference.Spec.Type != types.ToolReferenceTypeModelProvider {
		return types.NewErrBadRequest("%s is not a model provider", modelProviderReference.Name)
	}

	if modelProvider, err := convertModelProviderToolRef(req.Context(), a.gptscript, modelProviderReference); err != nil {
		return fmt.Errorf("failed to determine if model provider is configured: %w", err)
	} else if !modelProvider.Configured {
		return types.NewErrBadRequest("model provider %s is not configured, missing configuration parameters: %s", modelProviderReference.Name, strings.Join(modelProvider.MissingConfigurationParameters, ", "))
	}

	oModels, err := availablemodels.ForProvider(req.Context(), a.dispatcher, modelProviderReference.Namespace, modelProviderReference.Name)
	if err != nil {
		return err
	}

	return req.Write(oModels)
}
