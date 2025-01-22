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
	"time"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/obot-platform/nah/pkg/apply"
	"github.com/obot-platform/nah/pkg/name"
	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/logger"
	"github.com/obot-platform/obot/pkg/availablemodels"
	"github.com/obot-platform/obot/pkg/controller/creds"
	"github.com/obot-platform/obot/pkg/gateway/server/dispatcher"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

var log = logger.Package()

var jsonErrRegexp = regexp.MustCompile(`\{.*"error":.*}`)

type indexEntry struct {
	Reference string `json:"reference,omitempty"`
	All       bool   `json:"all,omitempty"`
}

type index struct {
	Tools                    map[string]indexEntry `json:"tools,omitempty"`
	StepTemplates            map[string]indexEntry `json:"stepTemplates,omitempty"`
	KnowledgeDataSources     map[string]indexEntry `json:"knowledgeDataSources,omitempty"`
	KnowledgeDocumentLoaders map[string]indexEntry `json:"knowledgeDocumentLoaders,omitempty"`
	System                   map[string]indexEntry `json:"system,omitempty"`
	ModelProviders           map[string]indexEntry `json:"modelProviders,omitempty"`
	AuthProviders            map[string]indexEntry `json:"authProviders,omitempty"`
}

type Handler struct {
	gptClient     *gptscript.GPTScript
	dispatcher    *dispatcher.Dispatcher
	supportDocker bool
	registryURLs  []string
}

func New(gptClient *gptscript.GPTScript,
	dispatcher *dispatcher.Dispatcher,
	registryURLs []string,
	supportDocker bool,
) *Handler {
	return &Handler{
		gptClient:     gptClient,
		dispatcher:    dispatcher,
		registryURLs:  registryURLs,
		supportDocker: supportDocker,
	}
}

func isValidTool(tool gptscript.Tool) bool {
	if tool.MetaData["index"] == "false" {
		return false
	}
	return tool.Name != "" && (tool.Type == "" || tool.Type == "tool")
}

func (h *Handler) toolsToToolReferences(ctx context.Context, toolType types.ToolReferenceType, registryURL string, entries map[string]indexEntry) (result []client.Object) {
	annotations := map[string]string{
		"obot.obot.ai/timestamp": time.Now().String(),
	}
	for name, entry := range entries {
		if ref, ok := strings.CutPrefix(entry.Reference, "./"); ok {
			entry.Reference = registryURL + "/" + ref
		}

		if entry.All {
			prg, err := h.gptClient.LoadFile(ctx, "* from "+entry.Reference)
			if err != nil {
				log.Errorf("Failed to load tool %s: %v", entry.Reference, err)
				continue
			}

			tool := prg.ToolSet[prg.EntryToolID]
			if isValidTool(tool) {
				toolName := tool.Name
				if tool.MetaData["bundle"] == "true" {
					toolName = "bundle"
				}
				result = append(result, &v1.ToolReference{
					ObjectMeta: metav1.ObjectMeta{
						Name:        normalize(name, toolName),
						Namespace:   system.DefaultNamespace,
						Finalizers:  []string{v1.ToolReferenceFinalizer},
						Annotations: annotations,
					},
					Spec: v1.ToolReferenceSpec{
						Type:      toolType,
						Reference: entry.Reference,
						Builtin:   true,
					},
				})
			}
			for _, peerToolID := range tool.LocalTools {
				// If this is the entry tool, then we already added it or skipped it above.
				if peerToolID == prg.EntryToolID {
					continue
				}

				peerTool := prg.ToolSet[peerToolID]
				if isValidTool(peerTool) {
					toolName := peerTool.Name
					if peerTool.MetaData["bundle"] == "true" {
						toolName += "-bundle"
					}
					result = append(result, &v1.ToolReference{
						ObjectMeta: metav1.ObjectMeta{
							Name:        normalize(name, toolName),
							Namespace:   system.DefaultNamespace,
							Finalizers:  []string{v1.ToolReferenceFinalizer},
							Annotations: annotations,
						},
						Spec: v1.ToolReferenceSpec{
							Type:      toolType,
							Reference: fmt.Sprintf("%s from %s", peerTool.Name, entry.Reference),
							Builtin:   true,
						},
					})
				}
			}
		} else {
			if !h.supportDocker && name == system.ShellTool {
				continue
			}
			result = append(result, &v1.ToolReference{
				ObjectMeta: metav1.ObjectMeta{
					Name:        name,
					Namespace:   system.DefaultNamespace,
					Finalizers:  []string{v1.ToolReferenceFinalizer},
					Annotations: annotations,
				},
				Spec: v1.ToolReferenceSpec{
					Type:      toolType,
					Reference: entry.Reference,
					Builtin:   true,
				},
			})
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
		toAdd = append(toAdd, h.toolsToToolReferences(ctx, types.ToolReferenceTypeStepTemplate, registryURL, index.StepTemplates)...)
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

func normalize(names ...string) string {
	return strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(strings.Join(names, "-"), " ", "-"), "_", "-"))
}

func (h *Handler) PollRegistries(ctx context.Context, c client.Client) {
	if len(h.registryURLs) < 1 {
		return
	}

	for {
		if err := c.List(ctx, &v1.ToolReferenceList{}, client.InNamespace(system.DefaultNamespace)); err != nil {
			time.Sleep(time.Second)
			continue
		}
		break
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
	if retry := time.Until(toolRef.Status.LastReferenceCheck.Time); toolRef.Generation == toolRef.Status.ObservedGeneration && retry > -time.Hour {
		resp.RetryAfter(time.Hour + retry)
		return nil
	}

	// Reset status
	lastCheck := toolRef.Status.LastReferenceCheck
	toolRef.Status.LastReferenceCheck = metav1.Now()
	toolRef.Status.ObservedGeneration = toolRef.Generation
	toolRef.Status.Reference = toolRef.Spec.Reference
	toolRef.Status.Tool = nil
	toolRef.Status.Error = ""

	prg, err := h.gptClient.LoadFile(req.Ctx, toolRef.Spec.Reference, gptscript.LoadOptions{
		DisableCache: toolRef.Spec.ForceRefresh.After(lastCheck.Time),
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

	if toolRef.Status.Tool.Metadata["envVars"] != "" {
		cred, err := h.gptClient.RevealCredential(req.Ctx, []string{string(toolRef.UID), system.GenericModelProviderCredentialContext}, toolRef.Name)
		if err != nil {
			if errors.As(err, &gptscript.ErrNotFound{}) {
				// Unable to find credential, ensure all models remove for this model provider
				return removeModelsForProvider(req.Ctx, req.Client, req.Namespace, req.Name)
			}
			return err
		}

		for _, envVar := range strings.Split(toolRef.Status.Tool.Metadata["envVars"], ",") {
			if _, ok := cred.Env[envVar]; !ok {
				// Model provider is not configured, don't error
				return nil
			}
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
			var eR errorResponse
			if err := json.Unmarshal([]byte(match), &eR); err == nil {
				toolRef.Status.Error = eR.Error
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

	if toolRef.Status.Tool.Metadata["envVars"] != "" {
		if err := h.gptClient.DeleteCredential(req.Ctx, string(toolRef.UID), toolRef.Name); err != nil && !errors.As(err, &gptscript.ErrNotFound{}) {
			return err
		}
	}

	return removeModelsForProvider(req.Ctx, req.Client, req.Namespace, req.Name)
}

func modelName(modelProviderName, modelName string) string {
	return name.SafeConcatName(system.ModelPrefix, modelProviderName, fmt.Sprintf("%x", sha256.Sum256([]byte(modelName))))
}
