package provider

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/obot-platform/nah/pkg/apply"
	"github.com/obot-platform/nah/pkg/name"
	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/logger"
	"github.com/obot-platform/obot/pkg/api/handlers/providers"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/gateway/server/dispatcher"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/license"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"go.yaml.in/yaml/v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	log           = logger.Package()
	jsonErrRegexp = regexp.MustCompile(`(?s)\{.*"error":.*}`)
)

var (
	openAIDefaultModelAliases = map[types.DefaultModelAliasType]string{
		types.DefaultModelAliasTypeLLM:             "gpt-5.4",
		types.DefaultModelAliasTypeLLMMini:         "gpt-5-mini",
		types.DefaultModelAliasTypeVision:          "gpt-5.4",
		types.DefaultModelAliasTypeImageGeneration: "dall-e-3",
		types.DefaultModelAliasTypeTextEmbedding:   "text-embedding-3-large",
	}
	anthropicDefaultModelAliases = map[types.DefaultModelAliasType]string{
		types.DefaultModelAliasTypeLLM:     "claude-sonnet-4-6",
		types.DefaultModelAliasTypeLLMMini: "claude-haiku-4-5",
		types.DefaultModelAliasTypeVision:  "claude-sonnet-4-6",
	}
)

func OpenAIDefaultModelAliases() map[types.DefaultModelAliasType]string {
	return maps.Clone(openAIDefaultModelAliases)
}

func AnthropicDefaultModelAliases() map[types.DefaultModelAliasType]string {
	return maps.Clone(anthropicDefaultModelAliases)
}

const (
	providerRegistryOwnerSubContext = "providers"
	modelProvidersRegistryDir       = "model-providers"
	authProvidersRegistryDir        = "auth-providers"
	providerRegistryMaxFiles        = 1000
)

var (
	providerNameInvalidChars = regexp.MustCompile(`[^a-z0-9-]+`)
	providerNameDashes       = regexp.MustCompile(`-{2,}`)
)

type Handler struct {
	gatewayClient   *gateway.Client
	dispatcher      *dispatcher.Dispatcher
	licenseProvider *license.Provider
	registryPaths   []string
}

func New(gatewayClient *gateway.Client, dispatcher *dispatcher.Dispatcher, licenseProvider *license.Provider, registryPaths []string) *Handler {
	return &Handler{
		gatewayClient:   gatewayClient,
		dispatcher:      dispatcher,
		licenseProvider: licenseProvider,
		registryPaths:   registryPaths,
	}
}

func providerResourceName(n string) string {
	n = strings.ToLower(n)
	n = providerNameInvalidChars.ReplaceAllString(n, "-")
	n = providerNameDashes.ReplaceAllString(n, "-")
	n = strings.Trim(n, "-")
	if len(n) <= 63 {
		return n
	}

	sum := fmt.Sprintf("%x", sha256.Sum256([]byte(n)))
	n = strings.Trim(n[:50], "-")
	return strings.Trim(fmt.Sprintf("%s-%s", n, sum[:12]), "-")
}

func providerName(path string) string {
	base := filepath.Base(path)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

func isYAMLFile(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".yaml", ".yml":
		return true
	default:
		return false
	}
}

type providerFromFile[T types.ModelProviderManifest | types.AuthProviderManifest] struct {
	Name     string `yaml:"-"`
	Manifest T      `yaml:",inline"`
}

func readProviderDirectory[T types.ModelProviderManifest | types.AuthProviderManifest](dir string) ([]providerFromFile[T], error) {
	fileInfo, err := os.Stat(dir)
	if err != nil {
		return nil, err
	}
	if !fileInfo.IsDir() {
		return nil, fmt.Errorf("provider registry path %s is not a directory", dir)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read provider registry directory %s: %w", dir, err)
	}

	var providers []providerFromFile[T]
	for _, entry := range entries {
		if entry.IsDir() || !isYAMLFile(entry.Name()) {
			continue
		}
		content, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("failed to read provider registry file %s: %w", entry.Name(), err)
		}

		var providerManifest providerFromFile[T]
		if err = yaml.Unmarshal(content, &providerManifest); err != nil {
			return nil, fmt.Errorf("failed to parse provider registry file %s: %w", entry.Name(), err)
		}

		providerManifest.Name = providerResourceName(providerName(entry.Name()))

		providers = append(providers, providerManifest)
		if len(providers) >= providerRegistryMaxFiles {
			log.Warnf("Reached maximum number of provider registry files (%d), skipping remaining files in directory %s", providerRegistryMaxFiles, dir)
			break
		}
	}

	return providers, nil
}

func readProviderDirectoryIgnoreMissing[T types.ModelProviderManifest | types.AuthProviderManifest](dir string) ([]providerFromFile[T], error) {
	entries, err := readProviderDirectory[T](dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	return entries, err
}

func readRegistry(registryPath string) ([]kclient.Object, error) {
	fileInfo, err := os.Stat(registryPath)
	if err != nil {
		return nil, fmt.Errorf("failed to stat provider registry %s: %w", registryPath, err)
	}
	if !fileInfo.IsDir() {
		return nil, fmt.Errorf("provider registry path %s is not a directory", registryPath)
	}

	models, err := readProviderDirectoryIgnoreMissing[types.ModelProviderManifest](filepath.Join(registryPath, modelProvidersRegistryDir))
	if err != nil {
		return nil, fmt.Errorf("failed to read model providers from registry %s: %w", registryPath, err)
	}

	auths, err := readProviderDirectoryIgnoreMissing[types.AuthProviderManifest](filepath.Join(registryPath, authProvidersRegistryDir))
	if err != nil {
		return nil, fmt.Errorf("failed to read auth providers from registry %s: %w", registryPath, err)
	}

	return appendProviders(registryPath, auths, models), nil
}

func appendProviders(registryPath string, authProviderManifests []providerFromFile[types.AuthProviderManifest], modelProviderManifests []providerFromFile[types.ModelProviderManifest]) []kclient.Object {
	objs := make([]kclient.Object, 0, len(authProviderManifests)+len(modelProviderManifests))

	for _, m := range modelProviderManifests {
		if m.Manifest.Command == "" {
			log.Warnf("Skipping model provider with missing required fields: name=%s command=%s", m.Name, m.Manifest.Command)
			continue
		}

		m.Manifest.Command = path.Join(registryPath, m.Manifest.Command)

		objs = append(objs, &v1.ModelProvider{
			ObjectMeta: metav1.ObjectMeta{
				Name: m.Name,
			},
			Spec: v1.ModelProviderSpec{
				ModelProviderManifest: m.Manifest,
			},
		})
	}

	for _, a := range authProviderManifests {
		if a.Manifest.Command == "" {
			log.Warnf("Skipping auth provider with missing required fields: name=%s command=%s", a.Name, a.Manifest.Command)
			continue
		}

		a.Manifest.Command = path.Join(registryPath, a.Manifest.Command)

		objs = append(objs, &v1.AuthProvider{
			ObjectMeta: metav1.ObjectMeta{
				Name: a.Name,
			},
			Spec: v1.AuthProviderSpec{
				AuthProviderManifest: a.Manifest,
			},
		})
	}

	return objs
}

func (h *Handler) ReadFromRegistry(ctx context.Context, c kclient.Client) error {
	var (
		toAdd []kclient.Object
		errs  []error
	)
	for _, registryPath := range h.registryPaths {
		objs, err := readRegistry(registryPath)
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to read provider registry %s: %w", registryPath, err))
			continue
		}
		log.Infof("Loaded provider registry: registry=%s providers=%d", registryPath, len(objs))
		toAdd = append(toAdd, objs...)
	}

	if len(errs) > 0 {
		// Do not accidentally delete providers for registry URLs that failed to be read.
		log.Infof("Skipping provider registry apply due to registry read errors: failedRegistries=%d", len(errs))
		return errors.Join(errs...)
	}

	if len(toAdd) == 0 {
		// Do not accidentally delete all the providers.
		log.Infof("Skipping provider registry apply because no providers were resolved")
		return nil
	}

	log.Infof("Applying resolved providers from registries: providers=%d", len(toAdd))
	return apply.New(c).WithOwnerSubContext(providerRegistryOwnerSubContext).WithPruneTypes(&v1.ModelProvider{}, &v1.AuthProvider{}).Apply(ctx, nil, toAdd...)
}

func (h *Handler) PollRegistries(ctx context.Context, c kclient.Client) {
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		if err := h.ReadFromRegistry(ctx, c); err != nil {
			log.Errorf("Failed to read from registries: %v", err)
		} else {
			log.Infof("Completed periodic provider registry refresh")
		}

		select {
		case <-t.C:
		case <-ctx.Done():
			return
		}
	}
}

func (h *Handler) EnsureOpenAIEnvCredentialAndDefaults(ctx context.Context, c kclient.Client) error {
	return h.ensureModelProviderCredAndDefaults(ctx, c, OpenAIDefaultModelAliases(), system.OpenAIModelProvider, system.OpenAIAPIKeyEnvVar)
}

func (h *Handler) EnsureAnthropicCredentialAndDefaults(ctx context.Context, c kclient.Client) error {
	return h.ensureModelProviderCredAndDefaults(ctx, c, AnthropicDefaultModelAliases(), system.AnthropicModelProvider, system.AnthropicAPIKeyEnvVar)
}

func (h *Handler) ensureModelProviderCredAndDefaults(ctx context.Context, c kclient.Client, defaultModelAliasMapping map[types.DefaultModelAliasType]string, modelProviderName, envVarName string) error {
	apiKey := os.Getenv(envVarName)
	if apiKey == "" {
		return nil
	}

	credentialEnvVarName := fmt.Sprintf("OBOT_%s_API_KEY", strings.ToUpper(strings.ReplaceAll(modelProviderName, "-", "_")))

	// If the model provider exists and the environment variable is set, then ensure the credential exists.
	var modelProvider v1.ModelProvider
	for {
		select {
		case <-time.After(2 * time.Second):
		case <-ctx.Done():
			return ctx.Err()
		}

		if err := c.Get(ctx, kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: modelProviderName}, &modelProvider); err == nil {
			break
		}
	}

	if cred, err := h.gatewayClient.RevealCredential(ctx, []string{modelProvider.Name, system.GenericModelProviderCredentialContext}, modelProviderName); err != nil {
		if !errors.As(err, &gateway.CredentialNotFoundError{}) {
			return fmt.Errorf("failed to check OpenAI credential: %w", err)
		}

		// The credential doesn't exist, so create it.
		if err = h.gatewayClient.UpsertCredential(ctx, gatewaytypes.Credential{
			Context: modelProvider.Name,
			Name:    modelProviderName,
			Secrets: map[string]string{
				credentialEnvVarName: apiKey,
			},
		}); err != nil {
			return err
		}
		log.Infof("Created model provider credential from environment configuration: provider=%s envVar=%s", modelProviderName, envVarName)
	} else if cred.Secrets[credentialEnvVarName] != apiKey {
		// If the credential exists, but has a different value, then update it.
		if err = h.gatewayClient.UpsertCredential(ctx, gatewaytypes.Credential{
			Context: modelProvider.Name,
			Name:    modelProviderName,
			Secrets: map[string]string{
				credentialEnvVarName: apiKey,
			},
		}); err != nil {
			return fmt.Errorf("failed to update model provider credential: %w", err)
		}
		log.Infof("Updated model provider credential from environment configuration: provider=%s envVar=%s", modelProviderName, envVarName)

		// Stop the model provider if it was started while we were updating the credential.
		h.dispatcher.StopModelProvider(modelProvider.Namespace, modelProvider.Name)
		log.Infof("Stopped model provider to force restart after credential update: provider=%s", modelProvider.Name)
	}

	var modelAliases v1.DefaultModelAliasList
	if err := c.List(ctx, &modelAliases); err != nil {
		return fmt.Errorf("failed to list model aliases: %w", err)
	}

	updatedAliases := 0
	for _, alias := range modelAliases.Items {
		if alias.Spec.Manifest.Model != "" {
			continue
		}

		targetModel := defaultModelAliasMapping[types.DefaultModelAliasType(alias.Spec.Manifest.Alias)]
		if targetModel == "" {
			continue
		}

		alias.Spec.Manifest.Model = modelName(modelProvider.Name, targetModel)
		if err := c.Update(ctx, &alias); err != nil {
			return fmt.Errorf("failed to update model alias %q for model provider %q: %w", alias.Name, modelProviderName, err)
		}
		updatedAliases++
	}
	log.Infof("Populated default model aliases for provider: provider=%s aliases=%d", modelProviderName, updatedAliases)

	// Lastly, ensure that the models are populated from the model provider
	if err := c.Get(ctx, kclient.ObjectKey{Namespace: modelProvider.Namespace, Name: modelProvider.Name}, &modelProvider); err != nil {
		return nil
	}

	if modelProvider.Annotations[v1.ModelProviderSyncAnnotation] != "" {
		delete(modelProvider.Annotations, v1.ModelProviderSyncAnnotation)
	} else {
		if modelProvider.Annotations == nil {
			modelProvider.Annotations = make(map[string]string, 1)
		}
		modelProvider.Annotations[v1.ModelProviderSyncAnnotation] = "true"
	}
	log.Infof("Toggled model provider sync annotation to refresh models: provider=%s", modelProvider.Name)

	return c.Update(ctx, &modelProvider)
}

func (h *Handler) SetAuthProviderConfiguredStatus(req router.Request, _ router.Response) error {
	authProvider := req.Object.(*v1.AuthProvider)
	return SetAuthProviderConfiguredStatus(req.Ctx, h.gatewayClient, h.licenseProvider, authProvider)
}

func SetAuthProviderConfiguredStatus(ctx context.Context, gatewayClient *gateway.Client, licenseProvider *license.Provider, authProvider *v1.AuthProvider) error {
	var (
		configured          = true
		missingConfigParams []string
	)
	if len(authProvider.Spec.RequiredConfigurationParameters) > 0 {
		cred, err := gatewayClient.RevealCredential(ctx, []string{authProvider.Name, system.GenericModelProviderCredentialContext}, authProvider.Name)
		if err != nil && !errors.As(err, &gateway.CredentialNotFoundError{}) {
			return fmt.Errorf("failed to reveal credential for auth provider %q: %w", authProvider.Name, err)
		}

		if cred.Secrets == nil {
			// Don't pass a nil map so that the provider status functions can distinguish between "no credential found" and "credential found but has no secrets"
			cred.Secrets = make(map[string]string)
		}

		providerStatus, err := providers.AuthProviderStatus(*authProvider, cred.Secrets, licenseProvider)
		if err != nil {
			return err
		}

		configured = providerStatus.Configured
		missingConfigParams = providerStatus.MissingConfigurationParameters
	}

	authProvider.Status.MissingConfigurationParameters = missingConfigParams
	authProvider.Status.Configured = configured
	authProvider.Status.ObservedGeneration = authProvider.Generation

	return nil
}

func (h *Handler) SetModelProviderConfiguredStatus(req router.Request, _ router.Response) error {
	modelProvider := req.Object.(*v1.ModelProvider)
	return SetModelProviderConfiguredStatus(req.Ctx, h.gatewayClient, h.licenseProvider, modelProvider)
}

func SetModelProviderConfiguredStatus(ctx context.Context, gatewayClient *gateway.Client, licenseProvider *license.Provider, modelProvider *v1.ModelProvider) error {
	var (
		configured          = true
		missingConfigParams []string
	)
	if len(modelProvider.Spec.RequiredConfigurationParameters) > 0 {
		cred, err := gatewayClient.RevealCredential(ctx, []string{modelProvider.Name, system.GenericModelProviderCredentialContext}, modelProvider.Name)
		if err != nil && !errors.As(err, &gateway.CredentialNotFoundError{}) {
			return fmt.Errorf("failed to reveal credential for model provider %q: %w", modelProvider.Name, err)
		}
		if cred.Secrets == nil {
			// Don't pass a nil map so that the provider status functions can distinguish between "no credential found" and "credential found but has no secrets"
			cred.Secrets = make(map[string]string)
		}

		providerStatus, err := providers.ModelProviderStatus(*modelProvider, cred.Secrets, licenseProvider)
		if err != nil {
			return err
		}

		configured = providerStatus.Configured
		missingConfigParams = providerStatus.MissingConfigurationParameters
	}

	modelProvider.Status.MissingConfigurationParameters = missingConfigParams
	modelProvider.Status.ObservedGeneration = modelProvider.Generation
	modelProvider.Status.Configured = configured

	return nil
}

func (h *Handler) BackPopulateModels(req router.Request, _ router.Response) error {
	modelProvider := req.Object.(*v1.ModelProvider)
	return BackPopulateModels(req.Ctx, req.Client, h.dispatcher, modelProvider)
}

func BackPopulateModels(ctx context.Context, client kclient.Client, dispatcher *dispatcher.Dispatcher, modelProvider *v1.ModelProvider) error {
	if !modelProvider.Status.Configured {
		return nil
	}

	availableModels, err := dispatcher.ModelsForProvider(ctx, *modelProvider)
	if err != nil {
		// Don't error and retry because it will likely fail again. Log the error, and the user can re-sync manually.
		// Also, the modelProvider.Status.Error field will bubble up to the user in the UI.

		// Check if the model provider returned a properly formatted error message and set it as status
		match := jsonErrRegexp.FindString(err.Error())
		if match != "" {
			modelProvider.Status.Error = match
			type errorResponse struct {
				Error string `json:"error"`
			}

			// custom response from model-provider implementation
			var eR errorResponse
			if err := json.Unmarshal([]byte(match), &eR); err == nil {
				modelProvider.Status.Error = eR.Error
			} else {
				type openAIErrResponse struct {
					Error struct {
						Message string `json:"message"`
					} `json:"error"`
				}

				// OpenAI API style response
				var eR openAIErrResponse
				if err := json.Unmarshal([]byte(match), &eR); err == nil {
					modelProvider.Status.Error = eR.Error.Message
				}
			}
		}

		log.Errorf("%v", err)
		return nil
	}

	models := make([]kclient.Object, 0, len(availableModels.Models))
	for _, model := range availableModels.Models {
		displayName := model.Metadata["displayName"]
		if displayName == "" {
			displayName = model.ID
		}
		models = append(models, &v1.Model{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: modelProvider.Namespace,
				Name:      modelName(modelProvider.Name, model.ID),
				Annotations: map[string]string{
					apply.AnnotationUpdate: "false",
				},
			},
			Spec: v1.ModelSpec{
				Manifest: types.ModelManifest{
					Name:          strings.ReplaceAll(model.ID, "/", "-"),
					DisplayName:   displayName,
					TargetModel:   model.ID,
					ModelProvider: modelProvider.Name,
					Active:        true,
					Usage:         types.ModelUsage(model.Metadata["usage"]),
					Dialect:       modelProvider.Spec.Dialect,
				},
			},
		})
	}

	if err = apply.New(client).Apply(ctx, modelProvider, models...); err != nil {
		return fmt.Errorf("failed to create models for model provider %q: %w", modelProvider.Name, err)
	}
	log.Infof("Back-populated models for model provider: provider=%s models=%d", modelProvider.Name, len(models))

	return nil
}

func removeModelsForProvider(ctx context.Context, c kclient.Client, namespace, name string) error {
	var models v1.ModelList
	if err := c.List(ctx, &models, &kclient.ListOptions{
		Namespace: namespace,
		FieldSelector: fields.SelectorFromSet(fields.Set{
			"spec.manifest.modelProvider": name,
		}),
	}); err != nil {
		return fmt.Errorf("failed to list models for model provider %q for cleanup: %w", name, err)
	}

	var (
		errs    []error
		deleted int
	)
	for _, model := range models.Items {
		if err := kclient.IgnoreNotFound(c.Delete(ctx, &model)); err != nil {
			errs = append(errs, fmt.Errorf("failed to delete model %q for cleanup: %w", model.Name, err))
		} else {
			deleted++
		}
	}
	if deleted > 0 {
		log.Infof("Removed stale models for provider: provider=%s models=%d", name, deleted)
	}

	return errors.Join(errs...)
}

func (h *Handler) CleanupModelProvider(req router.Request, _ router.Response) error {
	modelProvider := req.Object.(*v1.ModelProvider)
	if idx := slices.Index(modelProvider.Finalizers, "obot.obot.ai/tool-reference"); idx != -1 {
		// Remove the old finalizer.
		modelProvider.Finalizers = slices.Delete(modelProvider.Finalizers, idx, idx+1)
		if err := req.Client.Update(req.Ctx, modelProvider); err != nil {
			return fmt.Errorf("failed to remove old finalizer from model provider during cleanup: %w", err)
		}

		// Return, we'll be called again without the old finalizer.
		return nil
	}

	if len(modelProvider.Spec.RequiredConfigurationParameters) > 0 {
		deleted, err := h.gatewayClient.DeleteCredential(req.Ctx, modelProvider.Name, modelProvider.Name)
		if err != nil {
			return err
		}
		if deleted {
			log.Infof("Removed model provider credential during cleanup: provider=%s", modelProvider.Name)
		}
	}

	return removeModelsForProvider(req.Ctx, req.Client, req.Namespace, req.Name)
}

func modelName(modelProviderName, modelName string) string {
	return name.SafeConcatName(system.ModelPrefix, modelProviderName, fmt.Sprintf("%x", sha256.Sum256([]byte(modelName))))
}
