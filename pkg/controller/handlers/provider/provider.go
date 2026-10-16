package provider

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	openai "github.com/obot-platform/chat-completion-client"
	"github.com/obot-platform/nah/pkg/apply"
	"github.com/obot-platform/nah/pkg/name"
	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api/handlers/providers"
	"github.com/obot-platform/obot/pkg/auth"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/gateway/server/dispatcher"
	"github.com/obot-platform/obot/pkg/license"
	"github.com/obot-platform/obot/pkg/llmengine"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"go.yaml.in/yaml/v3"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/fields"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	providerRegistryOwnerSubContext = "providers"
	modelProvidersRegistryDir       = "model-providers"
	authProvidersRegistryDir        = "auth-providers"
	providerRegistryMaxFiles        = 1000
)

var (
	jsonErrRegexp            = regexp.MustCompile(`(?s)\{.*"error":.*}`)
	providerNameInvalidChars = regexp.MustCompile(`[^a-z0-9-]+`)
	providerNameDashes       = regexp.MustCompile(`-{2,}`)
)

type Handler struct {
	gatewayClient   *gateway.Client
	dispatcher      *dispatcher.Dispatcher
	licenseProvider *license.Provider
	registryPaths   []string
}

type providerFromFile[T types.ModelProviderManifest | types.AuthProviderManifest] struct {
	Name     string `yaml:"-"`
	Manifest T      `yaml:",inline"`
}

// upstreamProviderLister is a built-in model provider that routes to several upstream
// providers, such as the LLM gateway.
type upstreamProviderLister interface {
	ConfiguredProviders(ctx context.Context) ([]string, error)
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
			slog.Warn("Reached maximum number of provider registry files, skipping remaining files in directory", "maxFiles", providerRegistryMaxFiles, "dir", dir)
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

	auths, err := readProviderDirectoryIgnoreMissing[types.AuthProviderManifest](filepath.Join(registryPath, authProvidersRegistryDir))
	if err != nil {
		return nil, fmt.Errorf("failed to read auth providers from registry %s: %w", registryPath, err)
	}

	return appendProviders(registryPath, auths), nil
}

// appendProviders converts a registry's auth providers to objects. Registries also ship model
// providers, which the LLM gateway replaced, so they are ignored.
func appendProviders(registryPath string, authProviderManifests []providerFromFile[types.AuthProviderManifest]) []kclient.Object {
	objs := make([]kclient.Object, 0, len(authProviderManifests))

	for _, a := range authProviderManifests {
		if a.Manifest.Command == "" {
			slog.Warn("Skipping auth provider with missing required fields", "name", a.Name, "command", a.Manifest.Command)
			continue
		}
		if err := auth.ValidateGroupIDPrefix(a.Manifest.GroupIDPrefix); err != nil {
			slog.Warn("Skipping auth provider with invalid group ID prefix", "name", a.Name, "groupIDPrefix", a.Manifest.GroupIDPrefix, "error", err)
			continue
		}

		a.Manifest.Command = path.Join(registryPath, a.Manifest.Command)

		objs = append(objs, &v1.AuthProvider{
			Name: a.Name,
			Spec: v1.AuthProviderSpec{
				AuthProviderManifest: a.Manifest,
			},
		})
	}

	return objs
}

func validateUniqueAuthProviderGroupIDPrefixes(objs []kclient.Object) error {
	providersByPrefix := make(map[string]string)
	for _, obj := range objs {
		provider, ok := obj.(*v1.AuthProvider)
		if !ok || provider.Spec.GroupIDPrefix == "" {
			continue
		}

		prefix := strings.ToLower(provider.Spec.GroupIDPrefix)
		if existingProvider, ok := providersByPrefix[prefix]; ok {
			return fmt.Errorf("auth providers %q and %q declare the same group ID prefix %q", existingProvider, provider.Name, provider.Spec.GroupIDPrefix)
		}
		providersByPrefix[prefix] = provider.Name
	}
	return nil
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
		slog.Info("Loaded provider registry", "registry", registryPath, "providers", len(objs))
		toAdd = append(toAdd, objs...)
	}

	if len(errs) > 0 {
		// Do not accidentally delete providers for registry URLs that failed to be read.
		slog.Info("Skipping provider registry apply due to registry read errors", "failedRegistries", len(errs))
		return errors.Join(errs...)
	}

	if len(toAdd) == 0 {
		// Do not accidentally delete all the providers.
		slog.Info("Skipping provider registry apply because no providers were resolved")
		return nil
	}
	if err := validateUniqueAuthProviderGroupIDPrefixes(toAdd); err != nil {
		return fmt.Errorf("validate provider registries: %w", err)
	}

	slog.Info("Applying resolved providers from registries", "providers", len(toAdd))
	// Only auth providers are pruned. Model providers the registry created before the LLM
	// gateway replaced them are deleted once the gateway has migrated them.
	return apply.New(c).WithOwnerSubContext(providerRegistryOwnerSubContext).WithPruneTypes(&v1.AuthProvider{}).Apply(ctx, nil, toAdd...)
}

func (h *Handler) PollRegistries(ctx context.Context, c kclient.Client) {
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		if err := h.ReadFromRegistry(ctx, c); err != nil {
			slog.Error("Failed to read from registries", "error", err)
		} else {
			slog.Info("Completed periodic provider registry refresh")
		}

		select {
		case <-t.C:
		case <-ctx.Done():
			return
		}
	}
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
		cred, err := gatewayClient.RevealCredential(ctx, []string{authProvider.Name, system.GenericAuthProviderCredentialContext}, authProvider.Name)
		if err != nil && !errors.As(err, &gateway.CredentialNotFoundError{}) {
			return fmt.Errorf("failed to reveal credential for auth provider %q: %w", authProvider.Name, err)
		}

		if cred.Secrets == nil {
			// Don't pass a nil map so that the provider status functions can distinguish between "no credential found" and "credential found but has no secrets"
			cred.Secrets = make(map[string]string)
		}

		// A SCIM connection relaxes the parameters that only directory synchronization uses. The controller does not
		// watch the gateway database, so whatever creates or deletes a connection recomputes this status.
		conn, err := gatewayClient.SCIMConnectionForAuthProvider(ctx, authProvider.Namespace, authProvider.Name)
		if err != nil {
			return err
		}

		providerStatus, err := providers.AuthProviderStatus(ctx, *authProvider, cred.Secrets, conn, licenseProvider)
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

// SetModelProviderConfiguredStatus marks a built-in model provider, the LLM gateway,
// configured once it has an upstream provider. Legacy model providers are left as they are
// until the gateway migrates them.
func (h *Handler) SetModelProviderConfiguredStatus(req router.Request, _ router.Response) error {
	modelProvider := req.Object.(*v1.ModelProvider)
	builtin, ok := h.dispatcher.BuiltinModelProvider(modelProvider.Namespace, modelProvider.Name)
	if !ok {
		return nil
	}
	modelProvider.Status.Configured = builtin.HasUpstreamProviders(req.Ctx)
	modelProvider.Status.MissingConfigurationParameters = nil
	modelProvider.Status.ObservedGeneration = modelProvider.Generation
	return nil
}

func (h *Handler) BackPopulateModels(req router.Request, _ router.Response) error {
	modelProvider := req.Object.(*v1.ModelProvider)
	return BackPopulateModels(req.Ctx, req.Client, h.dispatcher, modelProvider)
}

func BackPopulateModels(ctx context.Context, client kclient.Client, dispatcher *dispatcher.Dispatcher, modelProvider *v1.ModelProvider) error {
	builtin, isBuiltin := dispatcher.BuiltinModelProvider(modelProvider.Namespace, modelProvider.Name)
	// Only built-in providers (the LLM gateway) list models. A built-in provider always lists,
	// even with no upstream providers, so that removing its last provider prunes its models.
	// While it is still starting, keep the current models until it can list them.
	if !isBuiltin || !builtin.Ready() {
		return nil
	}

	availableModels, err := dispatcher.FreshModelsForProvider(ctx, *modelProvider)
	if err != nil {
		// Don't error and retry because it will likely fail again. Log the error, and the user can re-sync manually.
		// Also, the modelProvider.Status.Error field will bubble up to the user in the UI.

		// Check if the model provider returned a properly formatted error message and set it as status
		modelProvider.Status.Error = err.Error()
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

		slog.Error("Failed to list models for model provider", "provider", modelProvider.Name, "error", err)
		return nil
	}
	modelProvider.Status.Error = ""

	// Administrators can rename, deactivate and re-categorize models, and override their
	// cost. Keep those edits: models are re-applied from the provider's list on every sync.
	var existingModels v1.ModelList
	if err := client.List(ctx, &existingModels, kclient.InNamespace(modelProvider.Namespace), kclient.MatchingFields{"spec.manifest.modelProvider": modelProvider.Name}); err != nil {
		return fmt.Errorf("failed to list models of model provider %q: %w", modelProvider.Name, err)
	}
	existing := make(map[string]types.ModelManifest, len(existingModels.Items))
	for _, m := range existingModels.Items {
		existing[m.Name] = m.Spec.Manifest
	}

	models := make([]kclient.Object, 0, len(availableModels.Models))
	for _, model := range availableModels.Models {
		displayName := model.Metadata["displayName"]
		if displayName == "" {
			displayName = model.ID
		}
		dialect := modelDialect(model.Metadata, modelProvider.Spec.Dialect)
		discovered := &v1.Model{
			Namespace: modelProvider.Namespace,
			Name:      modelName(modelProvider.Name, model.ID),
			Spec: v1.ModelSpec{
				Manifest: types.ModelManifest{
					Name:          strings.ReplaceAll(model.ID, "/", "-"),
					DisplayName:   displayName,
					TargetModel:   model.ID,
					ModelProvider: modelProvider.Name,
					Active:        true,
					Usage:         types.ModelUsage(model.Metadata["usage"]),
					Dialect:       dialect,
				},
			},
		}

		if prior, ok := existing[discovered.Name]; ok {
			keepModelEdits(&discovered.Spec.Manifest, prior)
		}

		models = append(models, discovered)

		// The embedded gateway reports its own pricing, so project it into ModelInfo for
		// the model controller to copy onto the model's status. It comes from no model info
		// source, so deleting one doesn't delete it; this provider's apply owns it.
		if modelProvider.Name == system.LLMGatewayModelProvider {
			if cost, ok := llmengine.CostFromMetadata(model.Metadata); ok {
				models = append(models, &v1.ModelInfo{
					Namespace: modelProvider.Namespace,
					Name:      v1.ModelInfoName(modelProvider.Name, model.ID),
					Spec: v1.ModelInfoSpec{
						Provider: modelProvider.Name,
						Model:    model.ID,
						Cost:     cost,
					},
				})
			}
		}
	}

	if lister, ok := builtin.(upstreamProviderLister); ok {
		kept, err := unlistedProviderModels(ctx, client, lister, availableModels, existingModels.Items)
		if err != nil {
			return err
		}
		models = append(models, kept...)
	}

	if err = apply.New(client).Apply(ctx, modelProvider, models...); err != nil {
		return fmt.Errorf("failed to create models for model provider %q: %w", modelProvider.Name, err)
	}
	slog.Info("Back-populated models for model provider", "provider", modelProvider.Name, "models", len(models))

	if modelProvider.Name == system.LLMGatewayModelProvider {
		// Move what refers to legacy providers' models over to the models just synced.
		if err := llmengine.MigrateLegacyModels(ctx, client, llmengine.LegacyImportDone(modelProvider)); err != nil {
			slog.Warn("failed to migrate legacy models to the LLM gateway", "error", err)
		}
		if err := llmengine.FillDefaultModelAliases(ctx, client); err != nil {
			slog.Warn("failed to set default model aliases", "error", err)
		}
	}

	return nil
}

// unlistedProviderModels keeps the models, and their pricing, of upstream providers that are
// still configured but are missing from a listing: a provider whose list call fails (an
// outage, an expired key) lists nothing, and syncing without its models would delete them,
// and with them the default model aliases, access policies and edits that refer to them.
// Only removing a provider removes its models.
func unlistedProviderModels(ctx context.Context, client kclient.Client, lister upstreamProviderLister, listed *openai.ModelsList, existing []v1.Model) ([]kclient.Object, error) {
	configured, err := lister.ConfiguredProviders(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list upstream providers: %w", err)
	}
	listedProviders := map[string]bool{}
	for _, m := range listed.Models {
		provider, _, _ := strings.Cut(m.ID, "/")
		listedProviders[provider] = true
	}
	var kept []kclient.Object
	for _, m := range existing {
		provider, _, _ := strings.Cut(m.Spec.Manifest.TargetModel, "/")
		if listedProviders[provider] || !slices.Contains(configured, provider) {
			continue
		}
		kept = append(kept, &v1.Model{Namespace: m.Namespace, Name: m.Name, Spec: m.Spec})
		var info v1.ModelInfo
		if err := client.Get(ctx, kclient.ObjectKey{Namespace: m.Namespace, Name: v1.ModelInfoName(m.Spec.Manifest.ModelProvider, m.Spec.Manifest.TargetModel)}, &info); err == nil {
			kept = append(kept, &v1.ModelInfo{Namespace: info.Namespace, Name: info.Name, Spec: info.Spec})
		} else if !apierrors.IsNotFound(err) {
			return nil, err
		}
	}
	return kept, nil
}

// keepModelEdits keeps the fields administrators edit from a model's current manifest.
func keepModelEdits(discovered *types.ModelManifest, prior types.ModelManifest) {
	discovered.DisplayName = prior.DisplayName
	discovered.Active = prior.Active
	discovered.Alias = prior.Alias
	discovered.OverrideCost = prior.OverrideCost
	if prior.Usage != "" {
		discovered.Usage = prior.Usage
	}
}

func modelDialect(metadata map[string]string, fallback string) string {
	if dialect := metadata["dialect"]; dialect != "" {
		return dialect
	}
	return fallback
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
		slog.Info("Removed stale models for provider", "provider", name, "models", deleted)
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

	// A legacy model provider's credential is kept: the LLM gateway only imports some of them,
	// and an administrator may still need the others to configure the gateway.
	return removeModelsForProvider(req.Ctx, req.Client, req.Namespace, req.Name)
}

func modelName(modelProviderName, modelName string) string {
	return name.SafeConcatName(system.ModelPrefix, modelProviderName, fmt.Sprintf("%x", sha256.Sum256([]byte(modelName))))
}
