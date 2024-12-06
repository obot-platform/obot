package handlers

import (
	"context"
	"fmt"
	"strings"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/otto8-ai/otto8/apiclient/types"
	"github.com/otto8-ai/otto8/pkg/api"
	"github.com/otto8-ai/otto8/pkg/gateway/server/dispatcher"
	v1 "github.com/otto8-ai/otto8/pkg/storage/apis/otto.otto8.ai/v1"
	"k8s.io/apimachinery/pkg/fields"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type ModelProviderHandler struct {
	gptscript  *gptscript.GPTScript
	dispatcher *dispatcher.Dispatcher
}

func NewModelProviderHandler(gClient *gptscript.GPTScript, dispatcher *dispatcher.Dispatcher) *ModelProviderHandler {
	return &ModelProviderHandler{
		gptscript:  gClient,
		dispatcher: dispatcher,
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

	modelProvider, err := convertToolReferenceToModelProvider(req.Context(), mp.gptscript, ref)
	if err != nil {
		return err
	}

	return req.Write(modelProvider)
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

	resp := make([]types.ModelProvider, 0, len(refList.Items))
	for _, ref := range refList.Items {
		modelProvider, err := convertToolReferenceToModelProvider(req.Context(), mp.gptscript, ref)
		if err != nil {
			return fmt.Errorf("failed to determine model provider status: %w", err)
		}

		resp = append(resp, modelProvider)
	}

	return req.Write(types.ModelProviderList{Items: resp})
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
	if err := mp.gptscript.DeleteCredential(req.Context(), string(ref.UID), ref.Name); err != nil && !strings.HasSuffix(err.Error(), "credential not found") {
		return fmt.Errorf("failed to update credential: %w", err)
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

func (mp *ModelProviderHandler) Reveal(req api.Context) error {
	var ref v1.ToolReference
	if err := req.Get(&ref, req.PathValue("id")); err != nil {
		return err
	}

	cred, err := mp.gptscript.RevealCredential(req.Context(), []string{string(ref.UID)}, ref.Name)
	if err != nil && !strings.HasSuffix(err.Error(), "credential not found") {
		return fmt.Errorf("failed to reveal credential: %w", err)
	}

	return req.Write(cred.Env)
}

func (mp *ModelProviderHandler) RefreshModels(req api.Context) error {
	var ref v1.ToolReference
	if err := req.Get(&ref, req.PathValue("id")); err != nil {
		return err
	}

	if ref.Spec.Type != types.ToolReferenceTypeModelProvider {
		return types.NewErrBadRequest("%q is not a model provider", ref.Name)
	}

	modelProvider, err := convertToolReferenceToModelProvider(req.Context(), mp.gptscript, ref)
	if err != nil {
		return err
	}

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

	if err = req.Update(&ref); err != nil {
		return fmt.Errorf("failed to sync models for model provider %q: %w", ref.Name, err)
	}

	return req.Write(modelProvider)
}

func convertToolReferenceToModelProvider(ctx context.Context, gClient *gptscript.GPTScript, ref v1.ToolReference) (types.ModelProvider, error) {
	status, err := convertModelProviderToolRef(ctx, gClient, ref)
	if err != nil {
		return types.ModelProvider{}, err
	}

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
		ModelProviderStatus: *status,
	}

	mp.Type = "modelprovider"

	return mp, nil
}

func convertModelProviderToolRef(ctx context.Context, gptscript *gptscript.GPTScript, toolRef v1.ToolReference) (*types.ModelProviderStatus, error) {
	var (
		requiredEnvVars, missingEnvVars []string
		icon                            string
	)
	if toolRef.Status.Tool != nil {
		if toolRef.Status.Tool.Metadata["envVars"] != "" {
			cred, err := gptscript.RevealCredential(ctx, []string{string(toolRef.UID)}, toolRef.Name)
			if err != nil && !strings.HasSuffix(err.Error(), "credential not found") {
				return nil, fmt.Errorf("failed to reveal credential for model provider %q: %w", toolRef.Name, err)
			}

			if toolRef.Status.Tool.Metadata["envVars"] != "" {
				requiredEnvVars = strings.Split(toolRef.Status.Tool.Metadata["envVars"], ",")
			}

			for _, envVar := range requiredEnvVars {
				if cred.Env[envVar] == "" {
					missingEnvVars = append(missingEnvVars, envVar)
				}
			}
		}

		icon = toolRef.Status.Tool.Metadata["icon"]
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
	}, nil
}
