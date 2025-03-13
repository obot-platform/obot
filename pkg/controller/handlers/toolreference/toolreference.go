package toolreference

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/obot-platform/nah/pkg/apply"
	"github.com/obot-platform/nah/pkg/name"
	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/logger"
	"github.com/obot-platform/obot/pkg/api/handlers/providers"
	"github.com/obot-platform/obot/pkg/availablemodels"
	"github.com/obot-platform/obot/pkg/controller/creds"
	"github.com/obot-platform/obot/pkg/gateway/server/dispatcher"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/obot-platform/obot/pkg/tools"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

var (
	log           = logger.Package()
	jsonErrRegexp = regexp.MustCompile(`(?s)\{.*"error":.*}`)
)

const toolRecheckPeriod = time.Hour

type indexEntry struct {
	Reference string `json:"reference,omitempty"`
}

type index struct {
	Tools                    map[string]indexEntry `json:"tools,omitempty"`
	KnowledgeDataSources     map[string]indexEntry `json:"knowledgeDataSources,omitempty"`
	KnowledgeDocumentLoaders map[string]indexEntry `json:"knowledgeDocumentLoaders,omitempty"`
	System                   map[string]indexEntry `json:"system,omitempty"`
	ModelProviders           map[string]indexEntry `json:"modelProviders,omitempty"`
	AuthProviders            map[string]indexEntry `json:"authProviders,omitempty"`
}

type Handler struct {
	gptClient      *gptscript.GPTScript
	dispatcher     *dispatcher.Dispatcher
	supportDocker  bool
	registryURLs   []string
	lastChecksLock *sync.RWMutex
	lastChecks     map[string]time.Time
}

func New(gptClient *gptscript.GPTScript,
	dispatcher *dispatcher.Dispatcher,
	registryURLs []string,
	supportDocker bool,
) *Handler {
	return &Handler{
		gptClient:      gptClient,
		dispatcher:     dispatcher,
		registryURLs:   registryURLs,
		supportDocker:  supportDocker,
		lastChecks:     make(map[string]time.Time),
		lastChecksLock: new(sync.RWMutex),
	}
}

func (h *Handler) toolsToToolReferences(ctx context.Context, toolType types.ToolReferenceType, registryURL string, entries map[string]indexEntry) (result []client.Object) {
	for name, entry := range entries {
		if ref, ok := strings.CutPrefix(entry.Reference, "./"); ok {
			entry.Reference = registryURL + "/" + ref
		}
		if !h.supportDocker && name == system.ShellTool {
			continue
		}

		toolRefs, err := tools.ResolveToolReferences(ctx, h.gptClient, name, entry.Reference, true, toolType)
		if err != nil {
			log.Errorf("Failed to resolve tool references for %s: %v", entry.Reference, err)
			continue
		}

		for _, toolRef := range toolRefs {
			result = append(result, toolRef)
		}
	}

	return
}

func (h *Handler) readRegistry(ctx context.Context, registryURL string) (index, error) {
	run, err := h.gptClient.Run(ctx, registryURL, gptscript.Options{})
	if err != nil {
		return index{}, err
	}

	out, err := run.Text()
	if err != nil {
		return index{}, err
	}

	var index index
	if err := yaml.Unmarshal([]byte(out), &index); err != nil {
		log.Errorf("Failed to decode index: %v", err)
		return index, err
	}

	return index, nil
}

func (h *Handler) readFromRegistry(ctx context.Context, c client.Client) error {
	var (
		toAdd []client.Object
		errs  []error
	)
	for _, registryURL := range h.registryURLs {
		index, err := h.readRegistry(ctx, registryURL)
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to read registry %s: %w", registryURL, err))
			continue
		}

		toAdd = append(toAdd, h.toolsToToolReferences(ctx, types.ToolReferenceTypeSystem, registryURL, index.System)...)
		toAdd = append(toAdd, h.toolsToToolReferences(ctx, types.ToolReferenceTypeModelProvider, registryURL, index.ModelProviders)...)
		toAdd = append(toAdd, h.toolsToToolReferences(ctx, types.ToolReferenceTypeAuthProvider, registryURL, index.AuthProviders)...)
		toAdd = append(toAdd, h.toolsToToolReferences(ctx, types.ToolReferenceTypeTool, registryURL, index.Tools)...)
		toAdd = append(toAdd, h.toolsToToolReferences(ctx, types.ToolReferenceTypeKnowledgeDataSource, registryURL, index.KnowledgeDataSources)...)
		toAdd = append(toAdd, h.toolsToToolReferences(ctx, types.ToolReferenceTypeKnowledgeDocumentLoader, registryURL, index.KnowledgeDocumentLoaders)...)
	}

	if len(errs) > 0 {
		// Don't accidentally delete tool references for registry URLs that failed to be read.
		return errors.Join(errs...)
	}

	if len(toAdd) < 1 {
		// Don't accidentally delete all the tool references
		return nil
	}

	return apply.New(c).WithOwnerSubContext("toolreferences").Apply(ctx, nil, toAdd...)
}

func (h *Handler) PollRegistries(ctx context.Context, c client.Client) {
	if len(h.registryURLs) < 1 {
		return
	}

	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		if err := h.readFromRegistry(ctx, c); err != nil {
			log.Errorf("Failed to read from registries: %v", err)
		}

		select {
		case <-t.C:
		case <-ctx.Done():
			return
		}
	}
}

func (h *Handler) Populate(req router.Request, resp router.Response) error {
	toolRef := req.Object.(*v1.ToolReference)
	h.lastChecksLock.RLock()
	lastCheck, ok := h.lastChecks[toolRef.Name]
	h.lastChecksLock.RUnlock()
	if !ok {
		// Tracking these times this way is not HA. However, we don't run this in an HA way right now.
		// When we are ready to start exploring HA as an option, this will have to be changed.
		lastCheck = time.Now()
		h.lastChecksLock.Lock()
		h.lastChecks[toolRef.Name] = time.Now()
		h.lastChecksLock.Unlock()
	}

	var retry time.Duration
	defer func() {
		resp.RetryAfter(toolRecheckPeriod + retry)
	}()

	if retry = time.Until(lastCheck); ok && toolRef.Generation == toolRef.Status.ObservedGeneration && retry > -toolRecheckPeriod {
		return nil
	}
	retry = 0

	// Reset status
	toolRef.Status.ObservedGeneration = toolRef.Generation
	toolRef.Status.Reference = toolRef.Spec.Reference
	toolRef.Status.Commit = ""
	toolRef.Status.Tool = nil
	toolRef.Status.Error = ""

	h.lastChecksLock.Lock()
	h.lastChecks[toolRef.Name] = time.Now()
	h.lastChecksLock.Unlock()

	prg, err := h.gptClient.LoadFile(req.Ctx, toolRef.Spec.Reference, gptscript.LoadOptions{
		DisableCache: toolRef.Spec.ForceRefresh.After(lastCheck),
	})
	if err != nil {
		toolRef.Status.Error = err.Error()
		return nil
	}

	tool := prg.ToolSet[prg.EntryToolID]
	toolRef.Status.Tool = &v1.ToolShortDescription{
		Name:        tool.Name,
		Description: tool.Description,
		Metadata:    tool.MetaData,
		Params:      map[string]string{},
	}
	if tool.Source.Repo != nil {
		toolRef.Status.Commit = tool.Source.Repo.Revision
	}
	if tool.Arguments != nil {
		for name, param := range tool.Arguments.Properties {
			if param.Value != nil {
				toolRef.Status.Tool.Params[name] = param.Value.Description
			}
		}
	}

	toolRef.Status.Tool.Credentials, toolRef.Status.Tool.CredentialNames, err = creds.DetermineCredsAndCredNames(prg, tool, toolRef.Spec.Reference)
	if err != nil {
		toolRef.Status.Error = err.Error()
	}

	return nil
}

func (h *Handler) EnsureOpenAIEnvCredentialAndDefaults(ctx context.Context, c client.Client) error {
	if os.Getenv("OPENAI_API_KEY") == "" {
		return nil
	}

	// If the openai-model-provider exists and the OPENAI_API_KEY environment variable is set, then ensure the credential exists.
	var openAIModelProvider v1.ToolReference
	for {
		select {
		case <-time.After(2 * time.Second):
		case <-ctx.Done():
			return ctx.Err()
		}

		if err := c.Get(ctx, client.ObjectKey{Namespace: system.DefaultNamespace, Name: "openai-model-provider"}, &openAIModelProvider); err == nil {
			break
		}
	}

	if cred, err := h.gptClient.RevealCredential(ctx, []string{string(openAIModelProvider.UID), system.GenericModelProviderCredentialContext}, "openai-model-provider"); err != nil {
		if !errors.As(err, &gptscript.ErrNotFound{}) {
			return fmt.Errorf("failed to check OpenAI credential: %w", err)
		}

		// The credential doesn't exist, so create it.
		if err = h.gptClient.CreateCredential(ctx, gptscript.Credential{
			Context:  string(openAIModelProvider.UID),
			ToolName: "openai-model-provider",
			Type:     gptscript.CredentialTypeModelProvider,
			Env: map[string]string{
				"OBOT_OPENAI_MODEL_PROVIDER_API_KEY": os.Getenv("OPENAI_API_KEY"),
			},
		}); err != nil {
			return err
		}
	} else if cred.Env["OBOT_OPENAI_MODEL_PROVIDER_API_KEY"] != os.Getenv("OPENAI_API_KEY") {
		// If the credential exists, but has a different value, then update it.
		// The only way to update it is to delete the existing credential and recreate it.
		if err = h.gptClient.DeleteCredential(ctx, string(openAIModelProvider.UID), "openai-model-provider"); err != nil {
			return fmt.Errorf("failed to delete credential: %w", err)
		}
		return h.gptClient.CreateCredential(ctx, gptscript.Credential{
			Context:  string(openAIModelProvider.UID),
			ToolName: "openai-model-provider",
			Type:     gptscript.CredentialTypeModelProvider,
			Env: map[string]string{
				"OBOT_OPENAI_MODEL_PROVIDER_API_KEY": os.Getenv("OPENAI_API_KEY"),
			},
		})
	}

	// Since the user is setting up the OpenAI model provider with an environment variable, we should set the default model aliases to something reasonable.
	openAIDefaultModelAliasMapping := map[types.DefaultModelAliasType]string{
		types.DefaultModelAliasTypeLLM:             "gpt-4o",
		types.DefaultModelAliasTypeLLMMini:         "gpt-4o-mini",
		types.DefaultModelAliasTypeVision:          "gpt-4o",
		types.DefaultModelAliasTypeImageGeneration: "dall-e-3",
		types.DefaultModelAliasTypeTextEmbedding:   "text-embedding-3-large",
	}

	var modelAliases v1.DefaultModelAliasList
	if err := c.List(ctx, &modelAliases); err != nil {
		return fmt.Errorf("failed to list model aliases: %w", err)
	}

	for _, alias := range modelAliases.Items {
		if alias.Spec.Manifest.Model != "" {
			continue
		}

		alias.Spec.Manifest.Model = modelName(openAIModelProvider.Name, openAIDefaultModelAliasMapping[types.DefaultModelAliasType(alias.Spec.Manifest.Alias)])
		if err := c.Update(ctx, &alias); err != nil {
			return fmt.Errorf("failed to update model alias %q: %w", alias.Name, err)
		}
	}

	// Lastly, ensure that the models are populated from the model provider
	if err := c.Get(ctx, client.ObjectKey{Namespace: openAIModelProvider.Namespace, Name: openAIModelProvider.Name}, &openAIModelProvider); err != nil {
		return nil
	}

	if openAIModelProvider.Annotations[v1.ModelProviderSyncAnnotation] != "" {
		delete(openAIModelProvider.Annotations, v1.ModelProviderSyncAnnotation)
	} else {
		if openAIModelProvider.Annotations == nil {
			openAIModelProvider.Annotations = make(map[string]string)
		}
		openAIModelProvider.Annotations[v1.ModelProviderSyncAnnotation] = "true"
	}

	return c.Update(ctx, &openAIModelProvider)
}

func (h *Handler) BackPopulateModels(req router.Request, _ router.Response) error {
	toolRef := req.Object.(*v1.ToolReference)
	if toolRef.Spec.Type != types.ToolReferenceTypeModelProvider || toolRef.Status.Tool == nil {
		return nil
	}

	mps, err := providers.ConvertModelProviderToolRef(*toolRef, nil)
	if err != nil {
		return err
	}
	if len(mps.RequiredConfigurationParameters) > 0 {
		cred, err := h.gptClient.RevealCredential(req.Ctx, []string{string(toolRef.UID), system.GenericModelProviderCredentialContext}, toolRef.Name)
		if err != nil {
			if errors.As(err, &gptscript.ErrNotFound{}) {
				// Unable to find credential, ensure all models remove for this model provider
				return removeModelsForProvider(req.Ctx, req.Client, req.Namespace, req.Name)
			}
			return err
		}
		mps, err = providers.ConvertModelProviderToolRef(*toolRef, cred.Env)
		if err != nil {
			return err
		}

		if !mps.Configured {
			return nil
		}
	}

	availableModels, err := availablemodels.ForProvider(req.Ctx, h.dispatcher, req.Namespace, req.Name)
	if err != nil {
		// Don't error and retry because it will likely fail again. Log the error, and the user can re-sync manually.
		// Also, the toolRef.Status.Error field will bubble up to the user in the UI.

		// Check if the model provider returned a properly formatted error message and set it as status
		match := jsonErrRegexp.FindString(err.Error())
		if match != "" {
			toolRef.Status.Error = match
			type errorResponse struct {
				Error string `json:"error"`
			}

			// custom response from model-provider implementation
			var eR errorResponse
			if err := json.Unmarshal([]byte(match), &eR); err == nil {
				toolRef.Status.Error = eR.Error
			} else {
				type openAIErrResponse struct {
					Error struct {
						Message string `json:"message"`
					} `json:"error"`
				}

				// OpenAI API style response
				var eR openAIErrResponse
				if err := json.Unmarshal([]byte(match), &eR); err == nil {
					toolRef.Status.Error = eR.Error.Message
				}
			}
		}

		log.Errorf("%v", err)
		return nil
	}

	models := make([]client.Object, 0, len(availableModels.Models))
	for _, model := range availableModels.Models {
		models = append(models, &v1.Model{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: req.Namespace,
				Name:      modelName(toolRef.Name, model.ID),
				Annotations: map[string]string{
					apply.AnnotationUpdate: "false",
				},
			},
			Spec: v1.ModelSpec{
				Manifest: types.ModelManifest{
					Name:          model.ID,
					TargetModel:   model.ID,
					ModelProvider: toolRef.Name,
					Active:        true,
					Usage:         types.ModelUsage(model.Metadata["usage"]),
				},
			},
		})
	}

	if err = apply.New(req.Client).Apply(req.Ctx, toolRef, models...); err != nil {
		return fmt.Errorf("failed to create models for model provider %q: %w", toolRef.Name, err)
	}

	return nil
}

func removeModelsForProvider(ctx context.Context, c client.Client, namespace, name string) error {
	var models v1.ModelList
	if err := c.List(ctx, &models, &client.ListOptions{
		Namespace: namespace,
		FieldSelector: fields.SelectorFromSet(fields.Set{
			"spec.manifest.modelProvider": name,
		}),
	}); err != nil {
		return fmt.Errorf("failed to list models for model provider %q for cleanup: %w", name, err)
	}

	var errs []error
	for _, model := range models.Items {
		if err := client.IgnoreNotFound(c.Delete(ctx, &model)); err != nil {
			errs = append(errs, fmt.Errorf("failed to delete model %q for cleanup: %w", model.Name, err))
		}
	}

	return errors.Join(errs...)
}

func (h *Handler) CleanupModelProvider(req router.Request, _ router.Response) error {
	toolRef := req.Object.(*v1.ToolReference)
	if toolRef.Spec.Type != types.ToolReferenceTypeModelProvider || toolRef.Status.Tool == nil {
		return nil
	}

	mps, err := providers.ConvertModelProviderToolRef(*toolRef, nil)
	if err != nil {
		return err
	}

	if len(mps.RequiredConfigurationParameters) > 0 {
		if err := h.gptClient.DeleteCredential(req.Ctx, string(toolRef.UID), toolRef.Name); err != nil && !errors.As(err, &gptscript.ErrNotFound{}) {
			return err
		}
	}

	return removeModelsForProvider(req.Ctx, req.Client, req.Namespace, req.Name)
}

func modelName(modelProviderName, modelName string) string {
	return name.SafeConcatName(system.ModelPrefix, modelProviderName, fmt.Sprintf("%x", sha256.Sum256([]byte(modelName))))
}
