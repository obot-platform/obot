package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	openai "github.com/gptscript-ai/chat-completion-client"
	"github.com/gptscript-ai/go-gptscript"
	"github.com/otto8-ai/otto8/apiclient/types"
	"github.com/otto8-ai/otto8/pkg/api"
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

		m, err := a.getAvailableModelsForProvider(req.Context(), modelProvider.Namespace, modelProvider.Name)
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
		return types.NewErrBadRequest("model provider %s is not configured, missing env vars: %s", modelProviderReference.Name, strings.Join(modelProvider.MissingConfigurationParameters, ", "))
	}

	oModels, err := a.getAvailableModelsForProvider(req.Context(), modelProviderReference.Namespace, modelProviderReference.Name)
	if err != nil {
		return err
	}

	return req.Write(oModels)
}

func (a *AvailableModelsHandler) getAvailableModelsForProvider(ctx context.Context, modelProviderNamespace, modelProviderName string) (*openai.ModelsList, error) {
	u, err := a.dispatcher.URLForModelProvider(ctx, modelProviderNamespace, modelProviderName)
	if err != nil {
		return nil, fmt.Errorf("failed to get URL for model provider %q: %w", modelProviderName, err)
	}

	r, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String()+"/v1/models", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request to model provider %s: %w", modelProviderName, err)
	}

	resp, err := http.DefaultClient.Do(r)
	if err != nil {
		return nil, fmt.Errorf("failed to make request to model provider %s: %w", modelProviderName, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		message, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to get model list from model provider %s: %s", modelProviderName, message)
	}

	var oModels openai.ModelsList
	if err = json.NewDecoder(resp.Body).Decode(&oModels); err != nil {
		return nil, fmt.Errorf("failed to decode model list from model provider %s: %w", modelProviderName, err)
	}

	return &oModels, nil
}
