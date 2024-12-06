package toolreference

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/otto8-ai/nah/pkg/apply"
	"github.com/otto8-ai/nah/pkg/name"
	"github.com/otto8-ai/nah/pkg/router"
	"github.com/otto8-ai/otto8/apiclient/types"
	"github.com/otto8-ai/otto8/logger"
	"github.com/otto8-ai/otto8/pkg/availablemodels"
	"github.com/otto8-ai/otto8/pkg/gateway/server/dispatcher"
	v1 "github.com/otto8-ai/otto8/pkg/storage/apis/otto.otto8.ai/v1"
	"github.com/otto8-ai/otto8/pkg/system"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

var log = logger.Package()

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
}

type Handler struct {
	gptClient   *gptscript.GPTScript
	dispatcher  *dispatcher.Dispatcher
	registryURL string
}

func New(gptClient *gptscript.GPTScript, dispatcher *dispatcher.Dispatcher, registryURL string) *Handler {
	return &Handler{
		gptClient:   gptClient,
		dispatcher:  dispatcher,
		registryURL: registryURL,
	}
}

func isValidTool(tool gptscript.Tool) bool {
	if tool.MetaData["index"] == "false" {
		return false
	}
	return tool.Name != "" && (tool.Type == "" || tool.Type == "tool")
}

func (h *Handler) toolsToToolReferences(ctx context.Context, toolType types.ToolReferenceType, entries map[string]indexEntry) (result []client.Object) {
	for name, entry := range entries {
		if ref, ok := strings.CutPrefix(entry.Reference, "./"); ok {
			entry.Reference = h.registryURL + "/" + ref
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
						Name:       normalize(name, toolName),
						Namespace:  system.DefaultNamespace,
						Finalizers: []string{v1.ToolReferenceFinalizer},
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
							Name:       normalize(name, toolName),
							Namespace:  system.DefaultNamespace,
							Finalizers: []string{v1.ToolReferenceFinalizer},
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
			result = append(result, &v1.ToolReference{
				ObjectMeta: metav1.ObjectMeta{
					Name:       name,
					Namespace:  system.DefaultNamespace,
					Finalizers: []string{v1.ToolReferenceFinalizer},
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

func (h *Handler) readRegistry(ctx context.Context) (index, error) {
	run, err := h.gptClient.Run(ctx, h.registryURL, gptscript.Options{})
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
	index, err := h.readRegistry(ctx)
	if err != nil {
		return err
	}

	var toAdd []client.Object

	toAdd = append(toAdd, h.toolsToToolReferences(ctx, types.ToolReferenceTypeSystem, index.System)...)
	toAdd = append(toAdd, h.toolsToToolReferences(ctx, types.ToolReferenceTypeModelProvider, index.ModelProviders)...)
	toAdd = append(toAdd, h.toolsToToolReferences(ctx, types.ToolReferenceTypeTool, index.Tools)...)
	toAdd = append(toAdd, h.toolsToToolReferences(ctx, types.ToolReferenceTypeStepTemplate, index.StepTemplates)...)
	toAdd = append(toAdd, h.toolsToToolReferences(ctx, types.ToolReferenceTypeKnowledgeDataSource, index.KnowledgeDataSources)...)
	toAdd = append(toAdd, h.toolsToToolReferences(ctx, types.ToolReferenceTypeKnowledgeDocumentLoader, index.KnowledgeDocumentLoaders)...)

	if len(toAdd) == 0 {
		// Don't accidentally delete all the tool references
		return nil
	}

	return apply.New(c).WithOwnerSubContext("toolreferences").Apply(ctx, nil, toAdd...)
}

func normalize(names ...string) string {
	return strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(strings.Join(names, "-"), " ", "-"), "_", "-"))
}

func (h *Handler) PollRegistry(ctx context.Context, c client.Client) {
	if h.registryURL == "" {
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
			log.Errorf("Failed to read from registry: %v", err)
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
	toolRef.Status.LastReferenceCheck = metav1.Now()
	toolRef.Status.ObservedGeneration = toolRef.Generation
	toolRef.Status.Reference = toolRef.Spec.Reference
	toolRef.Status.Tool = nil
	toolRef.Status.Error = ""

	prg, err := h.gptClient.LoadFile(req.Ctx, toolRef.Spec.Reference)
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
	if len(tool.Credentials) == 1 {
		if strings.HasPrefix(tool.Credentials[0], ".") {
			refURL, err := url.Parse(toolRef.Spec.Reference)
			if err == nil {
				refURL.Path = path.Join(refURL.Path, tool.Credentials[0])
				toolRef.Status.Tool.Credential = refURL.String()
			}
		} else {
			toolRef.Status.Tool.Credential = tool.Credentials[0]
		}
	}

	return nil
}

func (h *Handler) EnsureOpenAIEnvCredential(ctx context.Context, c client.Client) error {
	if os.Getenv("OPENAI_API_KEY") == "" {
		return nil
	}

	for {
		select {
		case <-time.After(2 * time.Second):
		case <-ctx.Done():
			return ctx.Err()
		}

		// If the openai-model-provider exists and the OPENAI_API_KEY environment variable is set, then ensure the credential exists.
		var openAIModelProvider v1.ToolReference
		if err := c.Get(ctx, client.ObjectKey{Namespace: system.DefaultNamespace, Name: "openai-model-provider"}, &openAIModelProvider); err != nil {
			continue
		}

		if cred, err := h.gptClient.RevealCredential(ctx, []string{string(openAIModelProvider.UID)}, "openai-model-provider"); err != nil {
			if strings.HasSuffix(err.Error(), "credential not found") {
				// The credential doesn't exist, so create it.
				return h.gptClient.CreateCredential(ctx, gptscript.Credential{
					Context:  string(openAIModelProvider.UID),
					ToolName: "openai-model-provider",
					Type:     gptscript.CredentialTypeModelProvider,
					Env: map[string]string{
						"OTTO8_OPENAI_MODEL_PROVIDER_API_KEY": os.Getenv("OPENAI_API_KEY"),
					},
				})
			}

			return fmt.Errorf("failed to check OpenAI credential: %w", err)
		} else if cred.Env["OTTO8_OPENAI_MODEL_PROVIDER_API_KEY"] != os.Getenv("OPENAI_API_KEY") {
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
					"OTTO8_OPENAI_MODEL_PROVIDER_API_KEY": os.Getenv("OPENAI_API_KEY"),
				},
			})
		}

		return nil
	}
}

func (h *Handler) BackPopulateModels(req router.Request, _ router.Response) error {
	toolRef := req.Object.(*v1.ToolReference)
	if toolRef.Spec.Type != types.ToolReferenceTypeModelProvider || toolRef.Status.Tool == nil {
		return nil
	}

	if toolRef.Status.Tool.Metadata["envVars"] != "" {
		cred, err := h.gptClient.RevealCredential(req.Ctx, []string{string(toolRef.UID)}, toolRef.Name)
		if err != nil {
			if strings.Contains(err.Error(), "credential not found") {
				// Model provider is not configured, don't error
				return nil
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
		log.Errorf("Failed to get available models for model provider %q: %v", toolRef.Name, err)
		return nil
	}

	models := make([]client.Object, 0, len(availableModels.Models))
	for _, model := range availableModels.Models {
		models = append(models, &v1.Model{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: req.Namespace,
				Name:      name.SafeConcatName(system.ModelPrefix, toolRef.Name, fmt.Sprintf("%x", sha256.Sum256([]byte(model.ID)))),
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

func (h *Handler) CleanupModelProvider(req router.Request, _ router.Response) error {
	toolRef := req.Object.(*v1.ToolReference)
	if toolRef.Spec.Type != types.ToolReferenceTypeModelProvider || toolRef.Status.Tool == nil {
		return nil
	}

	if toolRef.Status.Tool.Metadata["envVars"] != "" {
		if err := h.gptClient.DeleteCredential(req.Ctx, string(toolRef.UID), toolRef.Name); err != nil && !strings.Contains(err.Error(), "credential not found") {
			return err
		}
	}

	var models v1.ModelList
	if err := req.List(&models, &client.ListOptions{
		Namespace: req.Namespace,
		FieldSelector: fields.SelectorFromSet(fields.Set{
			"spec.manifest.modelProvider": toolRef.Name,
		}),
	}); err != nil {
		return fmt.Errorf("failed to list models for model provider %q for cleanup: %w", toolRef.Name, err)
	}

	for _, model := range models.Items {
		if err := client.IgnoreNotFound(req.Delete(&model)); err != nil {
			return fmt.Errorf("failed to delete model %q for cleanup: %w", model.Name, err)
		}
	}

	return nil
}
