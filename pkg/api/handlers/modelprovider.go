package handlers

import (
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/api/handlers/providers"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/gateway/server/dispatcher"
	"github.com/obot-platform/obot/pkg/license"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
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

	return req.Write(mp.convertModelProvider(modelProvider, *mps))
}

func (mp *ModelProviderHandler) List(req api.Context) error {
	var modelProviders v1.ModelProviderList
	if err := req.List(&modelProviders, &kclient.ListOptions{
		Namespace: req.Namespace(),
	}); err != nil {
		return err
	}

	resp := make([]types.ModelProvider, 0, len(modelProviders.Items))
	for _, modelProvider := range modelProviders.Items {
		mps, err := providers.ModelProviderStatus(req.Context(), modelProvider, nil, mp.license)
		if err != nil {
			return err
		}

		resp = append(resp, mp.convertModelProvider(modelProvider, *mps))
	}

	return req.Write(types.ModelProviderList{Items: resp})
}

func (mp *ModelProviderHandler) Validate(req api.Context) error {
	var modelProvider v1.ModelProvider
	if err := req.Get(&modelProvider, req.PathValue("model_provider_id")); err != nil {
		return err
	}

	if err := mp.license.RequireEntitlements(req.Context(), modelProvider.Spec.RequiredEntitlements); err != nil {
		return err
	}

	slog.Debug("Validating model provider", "modelProviderName", modelProvider.Name)

	var envVars map[string]string
	if err := req.Read(&envVars); err != nil {
		return err
	}

	if err := mp.dispatcher.ValidateModelProvider(req.Context(), modelProvider.Namespace, modelProvider.Name, envVars); err != nil {
		return types.NewErrBadRequest("failed to validate model provider %q: %v", modelProvider.Name, err)
	}

	return nil
}

func (mp *ModelProviderHandler) Configure(req api.Context) error {
	var modelProvider v1.ModelProvider
	if err := req.Get(&modelProvider, req.PathValue("model_provider_id")); err != nil {
		return err
	}

	if err := mp.license.RequireEntitlements(req.Context(), modelProvider.Spec.RequiredEntitlements); err != nil {
		return err
	}

	var envVars map[string]string
	if err := req.Read(&envVars); err != nil {
		return err
	} else if envVars == nil {
		envVars = make(map[string]string)
	}
	for key, value := range envVars {
		if value == "" {
			delete(envVars, key)
		}
	}

	stagedName, err := stageProviderCredential(req, envVars)
	if err != nil {
		return err
	}

	return submitProviderConfigurationChange(req, &v1.ProviderConfigurationChange{
		Name:      modelProviderConfigurationChangeName(modelProvider.Name),
		Namespace: modelProvider.Namespace,
		Spec: v1.ProviderConfigurationChangeSpec{
			ProviderType:         v1.ProviderTypeModel,
			ProviderName:         modelProvider.Name,
			DesiredState:         v1.ProviderDesiredStateConfigured,
			StagedCredentialName: stagedName,
		},
	})
}

func (mp *ModelProviderHandler) Deconfigure(req api.Context) error {
	var modelProvider v1.ModelProvider
	if err := req.Get(&modelProvider, req.PathValue("model_provider_id")); err != nil {
		return err
	}

	return submitProviderConfigurationChange(req, &v1.ProviderConfigurationChange{
		Name:      modelProviderConfigurationChangeName(modelProvider.Name),
		Namespace: modelProvider.Namespace,
		Spec: v1.ProviderConfigurationChangeSpec{
			ProviderType: v1.ProviderTypeModel,
			ProviderName: modelProvider.Name,
			DesiredState: v1.ProviderDesiredStateDeconfigured,
		},
	})
}

func (mp *ModelProviderHandler) Reveal(req api.Context) error {
	var modelProvider v1.ModelProvider
	if err := req.Get(&modelProvider, req.PathValue("model_provider_id")); err != nil {
		return err
	}

	// If this is a "global" model provider, then the generic model provider context is added to handle more git-ops-ian deployments.
	cred, err := req.GatewayClient.RevealCredential(req.Context(), []string{modelProvider.Name, system.GenericModelProviderCredentialContext}, modelProvider.Name)
	if err != nil && !errors.As(err, &gateway.CredentialNotFoundError{}) {
		return fmt.Errorf("failed to reveal credential: %w", err)
	} else if err == nil {
		return req.Write(cred.Secrets)
	}

	return types.NewErrNotFound("no credential found for %q", modelProvider.Name)
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

	resp := mp.convertModelProvider(modelProvider, *mps)
	if !resp.Configured {
		return types.NewErrBadRequest("model provider %s is not configured, missing configuration parameters: %s", resp.Name, strings.Join(resp.MissingConfigurationParameters, ", "))
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

	return req.Write(resp)
}

func (mp *ModelProviderHandler) convertModelProvider(modelProvider v1.ModelProvider, modelProviderStatus types.ModelProviderStatus) types.ModelProvider {
	return types.ModelProvider{
		Metadata:              MetadataFrom(&modelProvider),
		ModelProviderManifest: modelProvider.Spec.ModelProviderManifest,
		ModelProviderStatus:   modelProviderStatus,
	}
}
