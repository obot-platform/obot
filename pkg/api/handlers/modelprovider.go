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

	return nil
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

func convertToolReferenceToModelProvider(ctx context.Context, gClient *gptscript.GPTScript, ref v1.ToolReference) (types.ModelProvider, error) {
	status, err := convertModelProviderToolRef(ctx, gClient, ref)
	if err != nil {
		return types.ModelProvider{}, err
	}

	mp := types.ModelProvider{
		Metadata: MetadataFrom(&ref),
		ModelProviderManifest: types.ModelProviderManifest{
			Name:          ref.Name,
			ToolReference: ref.Spec.Reference,
		},
		ModelProviderStatus: *status,
	}

	mp.Type = "modelprovider"

	return mp, nil
}
