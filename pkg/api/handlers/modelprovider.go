package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/obot-platform/obot/pkg/api/handlers/providers"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/gateway/server/dispatcher"
	"github.com/obot-platform/obot/pkg/invoke"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type ModelProviderHandler struct {
	dispatcher *dispatcher.Dispatcher
	invoker    *invoke.Invoker
}

func NewModelProviderHandler(dispatcher *dispatcher.Dispatcher, invoker *invoke.Invoker) *ModelProviderHandler {
	return &ModelProviderHandler{
		dispatcher: dispatcher,
		invoker:    invoker,
	}
}

func (mp *ModelProviderHandler) ByID(req api.Context) error {
	var ref v1.ToolReference
	if err := req.Get(&ref, req.PathValue("model_provider_id")); err != nil {
		return err
	}

	if ref.Spec.Type != types.ToolReferenceTypeModelProvider {
		return types.NewErrNotFound(
			"model provider %q not found",
			ref.Name,
		)
	}

	mps, err := providers.ConvertModelProviderToolRef(ref, nil)
	if err != nil {
		return err
	}

	var credEnvVars map[string]string
	if ref.Status.Tool != nil {
		if len(mps.RequiredConfigurationParameters) > 0 {
			cred, err := req.GPTClient.RevealCredential(req.Context(), []string{string(ref.UID), system.GenericModelProviderCredentialContext}, ref.Name)
			if err != nil && !errors.As(err, &gptscript.ErrNotFound{}) {
				return fmt.Errorf("failed to reveal credential for model provider %q: %w", ref.Name, err)
			} else if err == nil {
				credEnvVars = cred.Env
			}
		}
	}

	modelProvider, err := convertToolReferenceToModelProvider(ref, credEnvVars)
	if err != nil {
		return err
	}

	return req.Write(modelProvider)
}

func (mp *ModelProviderHandler) List(req api.Context) error {
	assistantID := req.PathValue("assistant_id")
	projectID := req.PathValue("project_id")

	var allowedModelProviders []string
	if assistantID != "" {
		agent, err := getAssistant(req, assistantID)
		if err != nil {
			return fmt.Errorf("failed to get assistant: %w", err)
		}

		allowedModelProviders = agent.Spec.Manifest.AllowedModelProviders

		project, err := getProjectThread(req)
		if err != nil {
			return fmt.Errorf("failed to get project: %w", err)
		}

		// Add any model providers on the project to the allowed list if they aren't already on there.
		// This protects against the agent changing allowed model providers, but the project is already configured with one that is removed.
		if project.Spec.DefaultModelProvider != "" && !slices.Contains(allowedModelProviders, project.Spec.DefaultModelProvider) {
			allowedModelProviders = append(allowedModelProviders, project.Spec.DefaultModelProvider)
		}

		for modelProvider := range project.Spec.Models {
			if modelProvider != "" && !slices.Contains(allowedModelProviders, modelProvider) {
				allowedModelProviders = append(allowedModelProviders, modelProvider)
			}
		}

		if len(allowedModelProviders) == 0 {
			return req.Write(types.ModelProviderList{})
		}
	}

	var refList v1.ToolReferenceList
	if err := req.List(&refList, &kclient.ListOptions{
		Namespace: req.Namespace(),
		FieldSelector: fields.SelectorFromSet(map[string]string{
			"spec.type": string(types.ToolReferenceTypeModelProvider),
		}),
	}); err != nil {
		return err
	}

	credCtxs := make([]string, 0, len(refList.Items)+1)
	for _, ref := range refList.Items {
		if projectID == "" {
			credCtxs = append(credCtxs, string(ref.UID))
		} else if slices.Contains(allowedModelProviders, ref.Name) {
			// When listing for a specific agent/projects, only list the model providers allowed by the agent.
			credCtxs = append(credCtxs, fmt.Sprintf("%s-%s", projectID, ref.Name))
		}
	}

	if projectID == "" {
		credCtxs = append(credCtxs, system.GenericModelProviderCredentialContext)
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

	resp := make([]types.ModelProvider, 0, len(refList.Items))
	var env map[string]string
	for _, ref := range refList.Items {
		if projectID == "" {
			env = credMap[string(ref.UID)+ref.Name]
			if env == nil {
				env = credMap[system.GenericModelProviderCredentialContext+ref.Name]
			}
		} else if slices.Contains(allowedModelProviders, ref.Name) {
			env = credMap[fmt.Sprintf("%s-%s", projectID, ref.Name)+ref.Name]
		} else {
			continue
		}

		modelProvider, err := convertToolReferenceToModelProvider(ref, env)
		if err != nil {
			log.Errorf("failed to convert model provider %q: %v", ref.Name, err)
			continue
		}
		resp = append(resp, modelProvider)
	}

	return req.Write(types.ModelProviderList{Items: resp})
}

type modelProviderValidationError struct {
	Err string `json:"error"`
}

func (ve *modelProviderValidationError) Error() string {
	return fmt.Sprintf("model-provider credentials validation failed: {\"error\": \"%s\"}", ve.Err)
}

func (mp *ModelProviderHandler) Validate(req api.Context) error {
	assistantID := req.PathValue("assistant_id")
	projectID := req.PathValue("project_id")
	modelProvider := req.PathValue("model_provider_id")

	if assistantID != "" {
		// Ensure that this agent allows this model provider.
		agent, err := getAssistant(req, assistantID)
		if err != nil {
			return fmt.Errorf("failed to get assistant: %w", err)
		}

		if !slices.Contains(agent.Spec.Manifest.AllowedModelProviders, modelProvider) {
			return types.NewErrBadRequest("model provider %q is not allowed for assistant %q", modelProvider, agent.Name)
		}
	}

	var ref v1.ToolReference
	if err := req.Get(&ref, modelProvider); err != nil {
		return err
	}

	if ref.Spec.Type != types.ToolReferenceTypeModelProvider {
		return types.NewErrBadRequest("%q is not a model provider", ref.Name)
	}

	log.Debugf("Validating model provider %q", ref.Name)

	var envVars map[string]string
	if err := req.Read(&envVars); err != nil {
		return err
	}

	envs := make([]string, 0, len(envVars))
	for key, val := range envVars {
		envs = append(envs, key+"="+val)
	}

	thread := &v1.Thread{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: fmt.Sprintf("%s-%s-validate-%s", system.ThreadPrefix, ref.Name, projectID),
			Namespace:    ref.Namespace,
		},
		Spec: v1.ThreadSpec{
			SystemTask: true,
			Ephemeral:  true,
		},
	}

	if err := req.Create(thread); err != nil {
		return fmt.Errorf("failed to create thread: %w", err)
	}

	task, err := mp.invoker.SystemTask(req.Context(), thread, "validate from "+ref.Spec.Reference, "", invoke.SystemTaskOptions{Env: envs})
	if err != nil {
		return err
	}
	defer task.Close()

	res, err := task.Result(req.Context())
	if err != nil {
		if strings.Contains(err.Error(), "tool not found: validate from "+ref.Spec.Reference) { // there's no simple way to do errors.As/.Is at this point unfortunately
			log.Errorf("Model provider %q does not provide a validate tool. Looking for 'validate from %s'", ref.Name, ref.Spec.Reference)
			return types.NewErrNotFound(
				fmt.Sprintf("`validate from %s` tool not found", ref.Spec.Reference),
				ref.Name,
			)
		}
		return types.NewErrHTTP(http.StatusUnprocessableEntity, strings.Trim(err.Error(), "\"'"))
	}

	var validationError modelProviderValidationError
	if json.Unmarshal([]byte(res.Output), &validationError) == nil && validationError.Err != "" {
		return types.NewErrHTTP(http.StatusUnprocessableEntity, validationError.Error())
	}

	return nil
}

func (mp *ModelProviderHandler) Configure(req api.Context) error {
	assistantID := req.PathValue("assistant_id")
	projectID := req.PathValue("project_id")
	modelProvider := req.PathValue("model_provider_id")
	if assistantID != "" {
		// Ensure that this agent allows this model provider.
		agent, err := getAssistant(req, assistantID)
		if err != nil {
			return fmt.Errorf("failed to get assistant: %w", err)
		}

		if !slices.Contains(agent.Spec.Manifest.AllowedModelProviders, modelProvider) {
			return types.NewErrBadRequest("model provider %q is not allowed for assistant %q", modelProvider, agent.Name)
		}
	}

	var ref v1.ToolReference
	if err := req.Get(&ref, modelProvider); err != nil {
		return err
	}

	if ref.Spec.Type != types.ToolReferenceTypeModelProvider {
		return types.NewErrBadRequest("%q is not a model provider", ref.Name)
	}

	var envVars map[string]string
	if err := req.Read(&envVars); err != nil {
		return err
	}

	// If this is a "global" model provider, then the generic model provider context is added to handle more git-ops-ian deployments.
	credCtxs := []string{string(ref.UID), system.GenericModelProviderCredentialContext}
	if projectID != "" {
		// If this is project-based, then only use the one context.
		credCtxs = []string{fmt.Sprintf("%s-%s", projectID, ref.Name)}
	}

	// Allow for updating credentials. The only way to update a credential is to delete the existing one and recreate it.
	cred, err := req.GPTClient.RevealCredential(req.Context(), credCtxs, ref.Name)
	if err != nil {
		if !errors.As(err, &gptscript.ErrNotFound{}) {
			return fmt.Errorf("failed to find credential: %w", err)
		}
	} else if err = req.GPTClient.DeleteCredential(req.Context(), cred.Context, ref.Name); err != nil {
		return fmt.Errorf("failed to remove existing credential: %w", err)
	}

	for key, val := range envVars {
		if val == "" {
			delete(envVars, key)
		}
	}

	if err := req.GPTClient.CreateCredential(req.Context(), gptscript.Credential{
		Context:  credCtxs[0],
		ToolName: ref.Name,
		Type:     gptscript.CredentialTypeModelProvider,
		Env:      envVars,
	}); err != nil {
		return fmt.Errorf("failed to create credential: %w", err)
	}

	if assistantID == "" {
		// We only need to reprocess the model provider if this is the "global" model provider.
		mp.dispatcher.StopModelProvider(ref.Namespace, ref.Name)

		if ref.Annotations[v1.ModelProviderSyncAnnotation] == "" {
			if ref.Annotations == nil {
				ref.Annotations = make(map[string]string, 1)
			}
			ref.Annotations[v1.ModelProviderSyncAnnotation] = "true"
		} else {
			delete(ref.Annotations, v1.ModelProviderSyncAnnotation)
		}
	}

	return req.Update(&ref)
}

func (mp *ModelProviderHandler) Deconfigure(req api.Context) error {
	assistantID := req.PathValue("assistant_id")
	projectID := req.PathValue("project_id")
	modelProvider := req.PathValue("model_provider_id")

	var ref v1.ToolReference
	if err := req.Get(&ref, modelProvider); err != nil {
		return err
	}

	if ref.Spec.Type != types.ToolReferenceTypeModelProvider {
		return types.NewErrBadRequest("%q is not a model provider", ref.Name)
	}

	// If this is a "global" model provider, then the generic model provider context is added to handle more git-ops-ian deployments.
	credCtxs := []string{string(ref.UID), system.GenericModelProviderCredentialContext}
	if projectID != "" {
		// If this is project-based, then only use the one context.
		credCtxs = []string{fmt.Sprintf("%s-%s", projectID, ref.Name)}
	}

	cred, err := req.GPTClient.RevealCredential(req.Context(), credCtxs, ref.Name)
	if err != nil {
		if !errors.As(err, &gptscript.ErrNotFound{}) {
			return fmt.Errorf("failed to find credential: %w", err)
		}
	} else if err = req.GPTClient.DeleteCredential(req.Context(), cred.Context, ref.Name); err != nil {
		return fmt.Errorf("failed to remove existing credential: %w", err)
	}

	if assistantID == "" {
		// We only need to reprocess the model provider if this is the "global" model provider.
		// Stop the model provider so that the credential is completely removed from the system.
		mp.dispatcher.StopModelProvider(ref.Namespace, ref.Name)

		if ref.Annotations[v1.ModelProviderSyncAnnotation] == "" {
			if ref.Annotations == nil {
				ref.Annotations = make(map[string]string, 1)
			}
			ref.Annotations[v1.ModelProviderSyncAnnotation] = "true"
		} else {
			delete(ref.Annotations, v1.ModelProviderSyncAnnotation)
		}
	}

	return req.Update(&ref)
}

func (mp *ModelProviderHandler) Reveal(req api.Context) error {
	assistantID := req.PathValue("assistant_id")
	projectID := req.PathValue("project_id")
	modelProvider := req.PathValue("model_provider_id")
	if assistantID != "" {
		// Ensure that this agent allows this model provider.
		agent, err := getAssistant(req, assistantID)
		if err != nil {
			return fmt.Errorf("failed to get assistant: %w", err)
		}

		if !slices.Contains(agent.Spec.Manifest.AllowedModelProviders, modelProvider) {
			return types.NewErrBadRequest("model provider %q is not allowed for assistant %q", modelProvider, agent.Name)
		}
	}

	var ref v1.ToolReference
	if err := req.Get(&ref, modelProvider); err != nil {
		return err
	}

	if ref.Spec.Type != types.ToolReferenceTypeModelProvider {
		return types.NewErrBadRequest("%q is not a model provider", ref.Name)
	}

	// If this is a "global" model provider, then the generic model provider context is added to handle more git-ops-ian deployments.
	credCtxs := []string{string(ref.UID), system.GenericModelProviderCredentialContext}
	if projectID != "" {
		// If this is project-based, then only use the one context.
		credCtxs = []string{fmt.Sprintf("%s-%s", projectID, ref.Name)}
	}

	cred, err := req.GPTClient.RevealCredential(req.Context(), credCtxs, ref.Name)
	if err != nil && !errors.As(err, &gptscript.ErrNotFound{}) {
		return fmt.Errorf("failed to reveal credential: %w", err)
	} else if err == nil {
		return req.Write(cred.Env)
	}

	return types.NewErrNotFound("no credential found for %q", ref.Name)
}

func (mp *ModelProviderHandler) RefreshModels(req api.Context) error {
	var ref v1.ToolReference
	if err := req.Get(&ref, req.PathValue("model_provider_id")); err != nil {
		return err
	}

	if ref.Spec.Type != types.ToolReferenceTypeModelProvider {
		return types.NewErrBadRequest("%q is not a model provider", ref.Name)
	}

	mps, err := providers.ConvertModelProviderToolRef(ref, nil)
	if err != nil {
		return err
	}

	var credEnvVars map[string]string
	if ref.Status.Tool != nil {
		if len(mps.RequiredConfigurationParameters) > 0 {
			cred, err := req.GPTClient.RevealCredential(req.Context(), []string{string(ref.UID), system.GenericModelProviderCredentialContext}, ref.Name)
			if err != nil && !errors.As(err, &gptscript.ErrNotFound{}) {
				return fmt.Errorf("failed to reveal credential for model provider %q: %w", ref.Name, err)
			} else if err == nil {
				credEnvVars = cred.Env
			}
		}
	}

	modelProvider, err := convertToolReferenceToModelProvider(ref, credEnvVars)
	if err != nil {
		return err
	}
	if !modelProvider.Configured {
		return types.NewErrBadRequest("model provider %s is not configured, missing configuration parameters: %s", modelProvider.Name, strings.Join(modelProvider.MissingConfigurationParameters, ", "))
	}

	if ref.Annotations[v1.ModelProviderSyncAnnotation] == "" {
		if ref.Annotations == nil {
			ref.Annotations = make(map[string]string, 1)
		}
		ref.Annotations[v1.ModelProviderSyncAnnotation] = "true"
	} else {
		delete(ref.Annotations, v1.ModelProviderSyncAnnotation)
	}

	if err := req.Update(&ref); err != nil {
		return fmt.Errorf("failed to sync models for model provider %q: %w", ref.Name, err)
	}

	return req.Write(modelProvider)
}

func convertToolReferenceToModelProvider(ref v1.ToolReference, credEnvVars map[string]string) (types.ModelProvider, error) {
	name := ref.Name
	if ref.Status.Tool != nil {
		name = ref.Status.Tool.Name
	}

	mps, err := providers.ConvertModelProviderToolRef(ref, credEnvVars)
	if err != nil {
		return types.ModelProvider{}, err
	}
	mp := types.ModelProvider{
		Metadata: MetadataFrom(&ref),
		ModelProviderManifest: types.ModelProviderManifest{
			Name:          name,
			ToolReference: ref.Spec.Reference,
		},
		ModelProviderStatus: *mps,
	}

	mp.Type = "modelprovider"

	return mp, nil
}
