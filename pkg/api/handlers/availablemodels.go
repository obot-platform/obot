package handlers

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/obot-platform/obot/pkg/api/handlers/providers"

	openai "github.com/gptscript-ai/chat-completion-client"
	"github.com/gptscript-ai/go-gptscript"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/gateway/server/dispatcher"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"k8s.io/apimachinery/pkg/fields"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type AvailableModelsHandler struct {
	dispatcher *dispatcher.Dispatcher
}

func NewAvailableModelsHandler(dispatcher *dispatcher.Dispatcher) *AvailableModelsHandler {
	return &AvailableModelsHandler{
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

	creds, err := req.GPTClient.ListCredentials(req.Context(), gptscript.ListCredentialsOptions{
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
		convertedModelProvider, err := providers.ConvertModelProviderToolRef(modelProvider, credMap[string(modelProvider.UID)+modelProvider.Name])
		if err != nil {
			log.Warnf("failed to convert model provider %q: %v", modelProvider.Name, err)
			continue
		}
		if !convertedModelProvider.Configured || modelProvider.Name == system.ModelProviderTool {
			continue
		}

		m, err := a.dispatcher.ModelsForProvider(req.Context(), modelProvider.Namespace, modelProvider.Name)
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
	assistantID := req.PathValue("assistant_id")
	projectID := req.PathValue("project_id")
	modelProviderID := req.PathValue("model_provider_id")
	if assistantID != "" {
		// Ensure that this agent allows this model provider.
		agent, err := getAssistant(req, assistantID)
		if err != nil {
			return fmt.Errorf("failed to get assistant: %w", err)
		}

		if !slices.Contains(agent.Spec.Manifest.AllowedModelProviders, modelProviderID) {
			return types.NewErrBadRequest("model provider %q is not allowed for assistant %q", modelProviderID, agent.Name)
		}
	}

	var modelProviderReference v1.ToolReference
	if err := req.Get(&modelProviderReference, modelProviderID); err != nil {
		return err
	}

	if modelProviderReference.Spec.Type != types.ToolReferenceTypeModelProvider {
		return types.NewErrBadRequest("%s is not a model provider", modelProviderReference.Name)
	}

	modelProvider, err := providers.ConvertModelProviderToolRef(modelProviderReference, nil)
	if err != nil {
		return err
	}

	credCtxs := []string{string(modelProviderReference.UID), system.GenericModelProviderCredentialContext}
	if projectID != "" {
		credCtxs = []string{fmt.Sprintf("%s-%s", projectID, modelProviderReference.Name)}
	}

	var credEnvVars map[string]string
	if modelProviderReference.Status.Tool != nil {
		if len(modelProvider.RequiredConfigurationParameters) > 0 {
			cred, err := req.GPTClient.RevealCredential(req.Context(), credCtxs, modelProviderReference.Name)
			if err != nil && !errors.As(err, &gptscript.ErrNotFound{}) {
				return fmt.Errorf("failed to reveal credential for model provider %q: %w", modelProviderReference.Name, err)
			} else if err == nil {
				credEnvVars = cred.Env
			}
		}
	}

	modelProvider, err = providers.ConvertModelProviderToolRef(modelProviderReference, credEnvVars)
	if err != nil {
		return err
	}
	if !modelProvider.Configured {
		return types.NewErrBadRequest("model provider %s is not configured, missing configuration parameters: %s", modelProviderReference.Name, strings.Join(modelProvider.MissingConfigurationParameters, ", "))
	}

	var oModels *openai.ModelsList
	if assistantID != "" {
		// If this is a request for obot-based models, then send the credential environment variables with the request so the model provider uses the correct credentials.
		oModels, err = a.dispatcher.ModelsForProviderWithEnv(req.Context(), modelProviderReference.Namespace, modelProviderReference.Name, credEnvVars)
	} else {
		oModels, err = a.dispatcher.ModelsForProvider(req.Context(), modelProviderReference.Namespace, modelProviderReference.Name)
	}
	if err != nil {
		return err
	}

	return req.Write(oModels)
}
