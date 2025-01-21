package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

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
	gptscript  *gptscript.GPTScript
	dispatcher *dispatcher.Dispatcher
	invoker    *invoke.Invoker
}

func NewModelProviderHandler(gClient *gptscript.GPTScript, dispatcher *dispatcher.Dispatcher, invoker *invoke.Invoker) *ModelProviderHandler {
	return &ModelProviderHandler{
		gptscript:  gClient,
		dispatcher: dispatcher,
		invoker:    invoker,
	}
}

func (mp *ModelProviderHandler) ByID(req api.Context) error {
	var ref v1.ToolReference
	if err := req.Get(&ref, req.PathValue("id")); err != nil {
		return err
	}

	if ref.Spec.Type != types.ToolReferenceTypeModelProvider {
		return types.NewErrNotFound(
			"model provider %q not found",
			ref.Name,
		)
	}

	var credEnvVars map[string]string
	if ref.Status.Tool != nil {
		if envVars := ref.Status.Tool.Metadata["envVars"]; envVars != "" {
			cred, err := mp.gptscript.RevealCredential(req.Context(), []string{string(ref.UID), system.GenericModelProviderCredentialContext}, ref.Name)
			if err != nil && !errors.As(err, &gptscript.ErrNotFound{}) {
				return fmt.Errorf("failed to reveal credential for model provider %q: %w", ref.Name, err)
			} else if err == nil {
				credEnvVars = cred.Env
			}
		}
	}

	return req.Write(convertToolReferenceToModelProvider(ref, credEnvVars))
}

func (mp *ModelProviderHandler) List(req api.Context) error {
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
		credCtxs = append(credCtxs, string(ref.UID))
	}
	credCtxs = append(credCtxs, system.GenericModelProviderCredentialContext)

	creds, err := mp.gptscript.ListCredentials(req.Context(), gptscript.ListCredentialsOptions{
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
	for _, ref := range refList.Items {
		env, ok := credMap[string(ref.UID)+ref.Name]
		if !ok {
			env = credMap[system.GenericModelProviderCredentialContext+ref.Name]
		}
		resp = append(resp, convertToolReferenceToModelProvider(ref, env))
	}

	return req.Write(types.ModelProviderList{Items: resp})
}

type ValidationError struct {
	Err string `json:"error"`
}

func (ve *ValidationError) Error() string {
	return fmt.Sprintf("model-provider credentials validation failed: {\"error\": \"%s\"}", ve.Err)
}

func (mp *ModelProviderHandler) Validate(req api.Context) error {
	var ref v1.ToolReference
	if err := req.Get(&ref, req.PathValue("id")); err != nil {
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
			GenerateName: system.ThreadPrefix + "-" + ref.Name + "-validate-",
			Namespace:    ref.Namespace,
		},
		Spec: v1.ThreadSpec{
			SystemTask: true,
		},
	}

	if err := req.Create(thread); err != nil {
		return fmt.Errorf("failed to create thread: %w", err)
	}

	defer func() { _ = req.Delete(thread) }()

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
		return types.NewErrHttp(http.StatusProxyAuthRequired, strings.Trim(err.Error(), "\"'"))
	}

	var validationError ValidationError
	if json.Unmarshal([]byte(res.Output), &validationError) == nil && validationError.Err != "" {
		return types.NewErrHttp(http.StatusProxyAuthRequired, validationError.Error())
	}

	return nil
}

func (mp *ModelProviderHandler) Configure(req api.Context) error {
	var ref v1.ToolReference
	if err := req.Get(&ref, req.PathValue("id")); err != nil {
		return err
	}

	if ref.Spec.Type != types.ToolReferenceTypeModelProvider {
		return types.NewErrBadRequest("%q is not a model provider", ref.Name)
	}

	var envVars map[string]string
	if err := req.Read(&envVars); err != nil {
		return err
	}

	// Allow for updating credentials. The only way to update a credential is to delete the existing one and recreate it.
	cred, err := mp.gptscript.RevealCredential(req.Context(), []string{string(ref.UID), system.GenericModelProviderCredentialContext}, ref.Name)
	if err != nil {
		if !errors.As(err, &gptscript.ErrNotFound{}) {
			return fmt.Errorf("failed to find credential: %w", err)
		}
	} else if err = mp.gptscript.DeleteCredential(req.Context(), cred.Context, ref.Name); err != nil {
		return fmt.Errorf("failed to remove existing credential: %w", err)
	}

	for key, val := range envVars {
		if val == "" {
			delete(envVars, key)
		}
	}

	if err := mp.gptscript.CreateCredential(req.Context(), gptscript.Credential{
		Context:  string(ref.UID),
		ToolName: ref.Name,
		Type:     gptscript.CredentialTypeModelProvider,
		Env:      envVars,
	}); err != nil {
		return fmt.Errorf("failed to create credential: %w", err)
	}

	mp.dispatcher.StopModelProvider(ref.Namespace, ref.Name)

	if ref.Annotations[v1.ModelProviderSyncAnnotation] == "" {
		if ref.Annotations == nil {
			ref.Annotations = make(map[string]string, 1)
		}
		ref.Annotations[v1.ModelProviderSyncAnnotation] = "true"
	} else {
		delete(ref.Annotations, v1.ModelProviderSyncAnnotation)
	}

	return req.Update(&ref)
}

func (mp *ModelProviderHandler) Deconfigure(req api.Context) error {
	var ref v1.ToolReference
	if err := req.Get(&ref, req.PathValue("id")); err != nil {
		return err
	}

	if ref.Spec.Type != types.ToolReferenceTypeModelProvider {
		return types.NewErrBadRequest("%q is not a model provider", ref.Name)
	}

	cred, err := mp.gptscript.RevealCredential(req.Context(), []string{string(ref.UID), system.GenericModelProviderCredentialContext}, ref.Name)
	if err != nil {
		if !errors.As(err, &gptscript.ErrNotFound{}) {
			return fmt.Errorf("failed to find credential: %w", err)
		}
	} else if err = mp.gptscript.DeleteCredential(req.Context(), cred.Context, ref.Name); err != nil {
		return fmt.Errorf("failed to remove existing credential: %w", err)
	}

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

	return req.Update(&ref)
}

func (mp *ModelProviderHandler) Reveal(req api.Context) error {
	var ref v1.ToolReference
	if err := req.Get(&ref, req.PathValue("id")); err != nil {
		return err
	}

	if ref.Spec.Type != types.ToolReferenceTypeModelProvider {
		return types.NewErrBadRequest("%q is not a model provider", ref.Name)
	}

	cred, err := mp.gptscript.RevealCredential(req.Context(), []string{string(ref.UID), system.GenericModelProviderCredentialContext}, ref.Name)
	if err != nil && !errors.As(err, &gptscript.ErrNotFound{}) {
		return fmt.Errorf("failed to reveal credential: %w", err)
	} else if err == nil {
		return req.Write(cred.Env)
	}

	return types.NewErrNotFound("no credential found for %q", ref.Name)
}

func (mp *ModelProviderHandler) RefreshModels(req api.Context) error {
	var ref v1.ToolReference
	if err := req.Get(&ref, req.PathValue("id")); err != nil {
		return err
	}

	if ref.Spec.Type != types.ToolReferenceTypeModelProvider {
		return types.NewErrBadRequest("%q is not a model provider", ref.Name)
	}

	var credEnvVars map[string]string
	if ref.Status.Tool != nil {
		if envVars := ref.Status.Tool.Metadata["envVars"]; envVars != "" {
			cred, err := mp.gptscript.RevealCredential(req.Context(), []string{string(ref.UID), system.GenericModelProviderCredentialContext}, ref.Name)
			if err != nil && !errors.As(err, &gptscript.ErrNotFound{}) {
				return fmt.Errorf("failed to reveal credential for model provider %q: %w", ref.Name, err)
			} else if err == nil {
				credEnvVars = cred.Env
			}
		}
	}

	modelProvider := convertToolReferenceToModelProvider(ref, credEnvVars)
	if !modelProvider.Configured {
		return types.NewErrBadRequest("model provider %s is not configured, missing configuration parameters: %s", modelProvider.ModelProviderManifest.Name, strings.Join(modelProvider.MissingConfigurationParameters, ", "))
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

func convertToolReferenceToModelProvider(ref v1.ToolReference, credEnvVars map[string]string) types.ModelProvider {
	name := ref.Name
	if ref.Status.Tool != nil {
		name = ref.Status.Tool.Name
	}

	mp := types.ModelProvider{
		Metadata: MetadataFrom(&ref),
		ModelProviderManifest: types.ModelProviderManifest{
			Name:          name,
			ToolReference: ref.Spec.Reference,
		},
		ModelProviderStatus: *convertModelProviderToolRef(ref, credEnvVars),
	}

	mp.Type = "modelprovider"

	return mp
}

func convertModelProviderToolRef(toolRef v1.ToolReference, cred map[string]string) *types.ModelProviderStatus {
	var (
		requiredEnvVars, missingEnvVars, optionalEnvVars []string
		icon                                             string
	)
	if toolRef.Status.Tool != nil {
		if toolRef.Status.Tool.Metadata["envVars"] != "" {
			requiredEnvVars = strings.Split(toolRef.Status.Tool.Metadata["envVars"], ",")
		}

		for _, envVar := range requiredEnvVars {
			if _, ok := cred[envVar]; !ok {
				missingEnvVars = append(missingEnvVars, envVar)
			}
		}

		icon = toolRef.Status.Tool.Metadata["icon"]

		if optionalEnvVarMetadata := toolRef.Status.Tool.Metadata["optionalEnvVars"]; optionalEnvVarMetadata != "" {
			optionalEnvVars = strings.Split(optionalEnvVarMetadata, ",")
		}
	}

	var modelsPopulated *bool
	configured := toolRef.Status.Tool != nil && len(missingEnvVars) == 0
	if configured {
		modelsPopulated = new(bool)
		*modelsPopulated = toolRef.Status.ObservedGeneration == toolRef.Generation
	}

	return &types.ModelProviderStatus{
		Icon:                            icon,
		Configured:                      configured,
		ModelsBackPopulated:             modelsPopulated,
		RequiredConfigurationParameters: requiredEnvVars,
		MissingConfigurationParameters:  missingEnvVars,
		OptionalConfigurationParameters: optionalEnvVars,
	}
}
