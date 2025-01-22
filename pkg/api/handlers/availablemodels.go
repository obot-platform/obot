package handlers

import (
	"errors"
	"fmt"
	"strings"

	openai "github.com/gptscript-ai/chat-completion-client"
	"github.com/gptscript-ai/go-gptscript"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/availablemodels"
	"github.com/obot-platform/obot/pkg/gateway/server/dispatcher"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
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
	var modelProviderReferences v1.ToolReferenceList
	if err := req.List(&modelProviderReferences, &kclient.ListOptions{
		Namespace: req.Namespace(),
		FieldSelector: fields.SelectorFromSet(map[string]string{
			"spec.type": string(types.ToolReferenceTypeModelProvider),
		}),
	}); err != nil {
		return err
	}

	credCtxs := make([]string, 0, len(modelProviderReferences.Items))
	for _, ref := range modelProviderReferences.Items {
		credCtxs = append(credCtxs, string(ref.UID))
	}

	creds, err := a.gptscript.ListCredentials(req.Context(), gptscript.ListCredentialsOptions{
		CredentialContexts: credCtxs,
	})
	if err != nil {
		return fmt.Errorf("failed to list model provider credentials: %w", err)
	}

	credMap := make(map[string]map[string]string, len(creds))
	for _, cred := range creds {
		credMap[cred.Context+cred.ToolName] = cred.Env
	}

	var oModels openai.ModelsList
	for _, modelProvider := range modelProviderReferences.Items {
		convertedModelProvider := convertModelProviderToolRef(modelProvider, credMap[string(modelProvider.UID)+modelProvider.Name])
		if !convertedModelProvider.Configured || modelProvider.Name == system.ModelProviderTool {
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

	var credEnvVars map[string]string
	if modelProviderReference.Status.Tool != nil {
		if envVars := modelProviderReference.Status.Tool.Metadata["envVars"]; envVars != "" {
			cred, err := a.gptscript.RevealCredential(req.Context(), []string{string(modelProviderReference.UID), system.GenericModelProviderCredentialContext}, modelProviderReference.Name)
			if err != nil && !errors.As(err, &gptscript.ErrNotFound{}) {
				return fmt.Errorf("failed to reveal credential for model provider %q: %w", modelProviderReference.Name, err)
			} else if err == nil {
				credEnvVars = cred.Env
			}
		}
	}

	if modelProvider := convertModelProviderToolRef(modelProviderReference, credEnvVars); !modelProvider.Configured {
		return types.NewErrBadRequest("model provider %s is not configured, missing configuration parameters: %s", modelProviderReference.Name, strings.Join(modelProvider.MissingConfigurationParameters, ", "))
	}

	oModels, err := availablemodels.ForProvider(req.Context(), a.dispatcher, modelProviderReference.Namespace, modelProviderReference.Name)
	if err != nil {
		return err
	}

	return req.Write(oModels)
}
