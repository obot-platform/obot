package handlers

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/api/handlers/providers"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/gateway/server/dispatcher"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/license"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/obot-platform/obot/pkg/wait"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type ModelProviderHandler struct {
	dispatcher *dispatcher.Dispatcher
	license    *license.KeygenProvider
}

func NewModelProviderHandler(dispatcher *dispatcher.Dispatcher, licenseProvider *license.KeygenProvider) *ModelProviderHandler {
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

	resp, err := mp.convertModelProvider(modelProvider)
	if err != nil {
		return err
	}

	return req.Write(resp)
}

func (mp *ModelProviderHandler) List(req api.Context) error {
	var modelProviders v1.ModelProviderList
	if err := req.List(&modelProviders, &kclient.ListOptions{
		Namespace: req.Namespace(),
	}); err != nil {
		return err
	}

	resp := make([]types.ModelProvider, 0, len(modelProviders.Items))
	for _, ref := range modelProviders.Items {
		modelProvider, err := mp.convertModelProvider(ref)
		if err != nil {
			log.Errorf("failed to convert model provider %q: %v", ref.Name, err)
			continue
		}
		resp = append(resp, modelProvider)
	}

	return req.Write(types.ModelProviderList{Items: resp})
}

func (mp *ModelProviderHandler) Validate(req api.Context) error {
	var modelProvider v1.ModelProvider
	if err := req.Get(&modelProvider, req.PathValue("model_provider_id")); err != nil {
		return err
	}

	if err := mp.license.RequireEntitlements(modelProvider.Spec.RequiredEntitlements); err != nil {
		return err
	}

	log.Debugf("Validating model provider %q", modelProvider.Name)

	var envVars map[string]string
	if err := req.Read(&envVars); err != nil {
		return err
	}

	u, err := mp.dispatcher.URLForModelProviderValidation(req.Context(), modelProvider.Namespace, modelProvider.Name, envVars)
	if err != nil {
		return err
	}
	defer mp.dispatcher.StopModelProviderValidation(req.Context(), modelProvider.Namespace, modelProvider.Name)

	validateReq, err := http.NewRequestWithContext(req.Context(), http.MethodGet, u.JoinPath("validate").String(), http.NoBody)
	if err != nil {
		return fmt.Errorf("failed to create model provider validation request: %w", err)
	}

	validateResp, err := http.DefaultClient.Do(validateReq)
	if err != nil {
		return fmt.Errorf("failed to validate model provider %q: %w", modelProvider.Name, err)
	}
	defer validateResp.Body.Close()

	validateRespBody, err := io.ReadAll(validateResp.Body)
	if err != nil {
		return fmt.Errorf("failed to read model provider validation response: %w", err)
	}

	switch validateResp.StatusCode {
	case http.StatusOK:
		return nil
	case http.StatusBadRequest:
		if len(validateRespBody) > 0 {
			return writeModelProviderValidationResponse(req, http.StatusBadRequest, validateResp.Header.Get("Content-Type"), validateRespBody)
		}
		return types.NewErrHTTP(http.StatusBadRequest, "model provider validation failed")
	default:
		message := fmt.Sprintf("model provider validation returned unexpected status code %d", validateResp.StatusCode)
		if len(validateRespBody) > 0 {
			message = fmt.Sprintf("%s: %s", message, string(validateRespBody))
		}
		return types.NewErrHTTP(http.StatusInternalServerError, message)
	}
}

func writeModelProviderValidationResponse(req api.Context, statusCode int, contentType string, body []byte) error {
	if contentType != "" {
		req.ResponseWriter.Header().Set("Content-Type", contentType)
	}
	req.WriteHeader(statusCode)
	_, err := req.ResponseWriter.Write(body)
	return err
}

func (mp *ModelProviderHandler) Configure(req api.Context) error {
	var modelProvider v1.ModelProvider
	if err := req.Get(&modelProvider, req.PathValue("model_provider_id")); err != nil {
		return err
	}

	if err := mp.license.RequireEntitlements(modelProvider.Spec.RequiredEntitlements); err != nil {
		return err
	}

	var envVars map[string]string
	if err := req.Read(&envVars); err != nil {
		return err
	}

	if err := req.GatewayClient.UpsertCredential(req.Context(), gatewaytypes.Credential{
		Context: modelProvider.Name,
		Name:    modelProvider.Name,
		Secrets: envVars,
	}); err != nil {
		return fmt.Errorf("failed to create credential: %w", err)
	}

	// We only need to reprocess the model provider if this is the "global" model provider.
	mp.dispatcher.StopModelProvider(req.Context(), modelProvider.Namespace, modelProvider.Name)

	if modelProvider.Annotations[v1.ModelProviderSyncAnnotation] == "" {
		if modelProvider.Annotations == nil {
			modelProvider.Annotations = make(map[string]string, 1)
		}
		modelProvider.Annotations[v1.ModelProviderSyncAnnotation] = "true"
	} else {
		delete(modelProvider.Annotations, v1.ModelProviderSyncAnnotation)
	}

	if err := req.Update(&modelProvider); err != nil {
		return fmt.Errorf("failed to update model provider: %w", err)
	}

	// Wait for the controllers to process to ensure the API will return correct configuration status.
	if _, err := wait.For(req.Context(), req.Storage, &modelProvider, func(m *v1.ModelProvider) (bool, error) {
		return m.Status.ObservedGeneration == m.Generation, nil
	}, wait.Option{
		Timeout: 10 * time.Second,
	}); err != nil {
		return fmt.Errorf("failed to wait for model provider: %w", err)
	}

	return nil
}

func (mp *ModelProviderHandler) Deconfigure(req api.Context) error {
	var modelProvider v1.ModelProvider
	if err := req.Get(&modelProvider, req.PathValue("model_provider_id")); err != nil {
		return err
	}

	// If this is a "global" model provider, then the generic model provider context is added to handle more git-ops-ian deployments.
	cred, err := req.GatewayClient.RevealCredential(req.Context(), []string{modelProvider.Name, system.GenericModelProviderCredentialContext}, modelProvider.Name)
	if err != nil {
		if !errors.As(err, &gateway.CredentialNotFoundError{}) {
			return fmt.Errorf("failed to find credential: %w", err)
		}
	} else if _, err = req.GatewayClient.DeleteCredential(req.Context(), cred.Context, modelProvider.Name); err != nil {
		return fmt.Errorf("failed to remove existing credential: %w", err)
	}

	// We only need to reprocess the model provider if this is the "global" model provider.
	// Stop the model provider so that the credential is completely removed from the system.
	mp.dispatcher.StopModelProvider(req.Context(), modelProvider.Namespace, modelProvider.Name)

	if modelProvider.Annotations[v1.ModelProviderSyncAnnotation] == "" {
		if modelProvider.Annotations == nil {
			modelProvider.Annotations = make(map[string]string, 1)
		}
		modelProvider.Annotations[v1.ModelProviderSyncAnnotation] = "true"
	} else {
		delete(modelProvider.Annotations, v1.ModelProviderSyncAnnotation)
	}

	if err := req.Update(&modelProvider); err != nil {
		return fmt.Errorf("failed to update model provider: %w", err)
	}

	// Wait for the controllers to process to ensure the API will return correct configuration status.
	if _, err := wait.For(req.Context(), req.Storage, &modelProvider, func(m *v1.ModelProvider) (bool, error) {
		return m.Status.ObservedGeneration == m.Generation, nil
	}, wait.Option{
		Timeout: 10 * time.Second,
	}); err != nil {
		return fmt.Errorf("failed to wait for model provider: %w", err)
	}

	return nil
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

	resp, err := mp.convertModelProvider(modelProvider)
	if err != nil {
		return err
	}
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

func (mp *ModelProviderHandler) convertModelProvider(modelProvider v1.ModelProvider) (types.ModelProvider, error) {
	mps, err := providers.ModelProviderStatus(modelProvider, nil, mp.license)
	if err != nil {
		return types.ModelProvider{}, err
	}

	return types.ModelProvider{
		Metadata:              MetadataFrom(&modelProvider),
		ModelProviderManifest: modelProvider.Spec.ModelProviderManifest,
		ModelProviderStatus:   *mps,
	}, nil
}
