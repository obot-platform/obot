package toolreference

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/obot-platform/nah/pkg/apply"
	"github.com/obot-platform/nah/pkg/name"
	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/logger"
	"github.com/obot-platform/obot/pkg/api/handlers/providers"
	"github.com/obot-platform/obot/pkg/controller/creds"
	"github.com/obot-platform/obot/pkg/gateway/server/dispatcher"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/obot-platform/obot/pkg/tools"
	"k8s.io/apimachinery/pkg/api/equality"
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
	FileScanners             map[string]indexEntry `json:"fileScanners,omitempty"`
	MCPServers               map[string]indexEntry `json:"mcpServers,omitempty"`
}

type Handler struct {
	gptClient               *gptscript.GPTScript
	dispatcher              *dispatcher.Dispatcher
	supportDockerTools      bool
	registryURLs            []string
	mcpCatalogs             []string
	lastChecksLock          *sync.RWMutex
	lastChecks              map[string]time.Time
	allowedDockerImageRepos []string
}

func New(gptClient *gptscript.GPTScript,
	dispatcher *dispatcher.Dispatcher,
	registryURLs []string,
	supportDocker bool,
	mcpCatalogs []string,
	allowedDockerImageRepos []string,
) *Handler {
	return &Handler{
		gptClient:               gptClient,
		dispatcher:              dispatcher,
		registryURLs:            registryURLs,
		mcpCatalogs:             mcpCatalogs,
		allowedDockerImageRepos: allowedDockerImageRepos,
		supportDockerTools:      supportDocker,
		lastChecks:              make(map[string]time.Time),
		lastChecksLock:          new(sync.RWMutex),
	}
}

func (h *Handler) mcpServers(ctx context.Context, registryURL string, entries map[string]indexEntry) (result []client.Object) {
	for _, entry := range entries {
		if ref, ok := strings.CutPrefix(entry.Reference, "./"); ok {
			entry.Reference = registryURL + "/" + ref
		}

		run, err := h.gptClient.Run(ctx, entry.Reference, gptscript.Options{})
		if err != nil {
			log.Errorf("Failed to run %s: %v", entry.Reference, err)
		}
		out, err := run.Text()
		if err != nil {
			log.Errorf("Failed to get text for run %s: %v", entry.Reference, err)
			continue
		}

		for _, filename := range strings.Split(strings.TrimSpace(out), "\n") {
			filename = strings.TrimSpace(filename)
			if filename == "" || !strings.HasSuffix(filename, ".yaml") {
				continue
			}

			input, _ := json.Marshal(map[string]interface{}{
				"name": filename,
			})

			out, err := h.gptClient.Run(ctx, "read from "+entry.Reference, gptscript.Options{
				Input: string(input),
			})
			if err != nil {
				log.Errorf("Failed to get contents of %s: %v", filename, err)
				continue
			}

			text, err := out.Text()
			if err != nil {
				log.Errorf("Failed to get server text for %s: %v", filename, err)
				continue
			}

			var manifest types.MCPServerManifest
			if err := yaml.Unmarshal([]byte(text), &manifest); err != nil {
				log.Errorf("Failed to decode manifest for %s: %v", filename, err)
				continue
			}
			catalogEntry := &v1.MCPServerCatalogEntry{
				ObjectMeta: metav1.ObjectMeta{
					Name:      strings.TrimSuffix(filename, ".yaml"),
					Namespace: system.DefaultNamespace,
				},
			}

			if manifest.Command != "" {
				catalogEntry.Spec.CommandManifest.Server = manifest
			} else if manifest.URL != "" {
				catalogEntry.Spec.URLManifest.Server = manifest
			} else {
				continue
			}
			result = append(result, catalogEntry)
		}
	}

	return
}

func (h *Handler) toolsToToolReferences(ctx context.Context, toolType types.ToolReferenceType, registryURL string, entries map[string]indexEntry) (result []client.Object) {
	for name, entry := range entries {
		if ref, ok := strings.CutPrefix(entry.Reference, "./"); ok {
			entry.Reference = registryURL + "/" + ref
		}
		if !h.supportDockerTools && name == system.ShellTool {
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
		toAdd = append(toAdd, h.toolsToToolReferences(ctx, types.ToolReferenceTypeFileScannerProvider, registryURL, index.FileScanners)...)
		toAdd = append(toAdd, h.toolsToToolReferences(ctx, types.ToolReferenceTypeTool, registryURL, index.Tools)...)
		toAdd = append(toAdd, h.toolsToToolReferences(ctx, types.ToolReferenceTypeKnowledgeDataSource, registryURL, index.KnowledgeDataSources)...)
		toAdd = append(toAdd, h.toolsToToolReferences(ctx, types.ToolReferenceTypeKnowledgeDocumentLoader, registryURL, index.KnowledgeDocumentLoaders)...)
		toAdd = append(toAdd, h.mcpServers(ctx, registryURL, index.MCPServers)...)
	}

	if len(errs) > 0 {
		// Don't accidentally delete tool references for registry URLs that failed to be read.
		return errors.Join(errs...)
	}

	if len(toAdd) == 0 {
		// Don't accidentally delete all the tool references
		return nil
	}

	return apply.New(c).WithOwnerSubContext("toolreferences").Apply(ctx, nil, toAdd...)
}

func (h *Handler) readMCPCatalog(catalog string) ([]client.Object, error) {
	var (
		contents []byte
		err      error
	)
	if strings.HasPrefix(catalog, "http://") || strings.HasPrefix(catalog, "https://") {
		var resp *http.Response
		resp, err = http.Get(catalog)
		if err != nil {
			return nil, fmt.Errorf("failed to read catalog %s: %w", catalog, err)
		}
		defer resp.Body.Close()

		contents, err = io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("unexpected status when reading catalog %s: %s", catalog, string(contents))
		}
	} else {
		// Assume it is a local file.
		contents, err = os.ReadFile(catalog)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read catalog %s: %w", catalog, err)
	}

	var entries []catalogEntryInfo
	if err = json.Unmarshal(contents, &entries); err != nil {
		return nil, fmt.Errorf("failed to decode catalog %s: %w", catalog, err)
	}

	objs := make([]client.Object, 0, len(entries))

	for _, entry := range entries {
		entry.FullName = string(slices.DeleteFunc(bytes.ToLower([]byte(entry.FullName)), func(r byte) bool {
			return r != '/' && !unicode.IsLetter(rune(r)) && !unicode.IsNumber(rune(r))
		}))

		var m map[string]string
		// Best effort to parse metadata
		_ = json.Unmarshal([]byte(entry.Metadata), &m)

		if m["categories"] == "Official" {
			delete(m, "categories") // This shouldn't happen, but do this just in case.
			// We don't want to mark random MCP servers from the catalog as official.
		}

		catalogEntry := v1.MCPServerCatalogEntry{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name.SafeHashConcatName(strings.Split(entry.FullName, "/")...),
				Namespace: system.DefaultNamespace,
			},
		}

		var manifests []mcpServerConfig
		if err = json.Unmarshal([]byte(entry.Manifest), &manifests); err != nil {
			// It wasn't an array, see if it is a single object
			var manifest mcpServerManifest
			if err = json.Unmarshal([]byte(entry.Manifest), &manifest); err != nil {
				return nil, fmt.Errorf("failed to decode manifest for %s: %w", entry.DisplayName, err)
			}
			manifests = append(manifests, manifest.Configs...)
		}

		var preferredFound, addEntry bool
		for _, c := range manifests {
			if c.Command != "" {
				if preferredFound {
					continue
				}
				switch c.Command {
				case "npx", "uvx":
				case "docker":
					// Only allow docker commands if the image name starts with one of the allowed repos.
					if len(c.Args) == 0 || len(h.allowedDockerImageRepos) > 0 && !slices.ContainsFunc(h.allowedDockerImageRepos, func(s string) bool {
						return strings.HasPrefix(c.Args[len(c.Args)-1], s)
					}) {
						continue
					}
				default:
					log.Debugf("Ignoring MCP catalog entry %s: unsupported command %s", entry.DisplayName, c.Command)
					continue
				}

				preferredFound = c.Preferred
				if !preferredFound && !isCommandPreferred(catalogEntry.Spec.CommandManifest.Server.Command, c.Command) {
					continue
				}

				// Sanitize the environment variables
				for i, env := range c.Env {
					if env.Key == "" {
						env.Key = env.Name
					}

					if filepath.Ext(env.Key) != "" {
						env.Key = strings.ReplaceAll(env.Key, ".", "_")
						env.File = true
					}

					env.Key = strings.ReplaceAll(strings.ToUpper(env.Key), "-", "_")

					c.Env[i] = env
				}

				addEntry = true
				catalogEntry.Spec.CommandManifest = types.MCPServerCatalogEntryManifest{
					URL:         entry.URL,
					GitHubStars: entry.Stars,
					Metadata:    m,
					Server: types.MCPServerManifest{
						Name:        entry.DisplayName,
						Description: entry.Description,
						Icon:        entry.Icon,
						Env:         c.Env,
						Command:     c.Command,
						Args:        c.Args,
						URL:         c.URL,
						Headers:     c.HTTPHeaders,
					},
				}
			} else if c.URL != "" || c.Remote {
				if c.URL != "" {
					if u, err := url.Parse(c.URL); err != nil || u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" {
						continue
					}
				}

				// Sanitize the headers
				for i, header := range c.HTTPHeaders {
					if header.Key == "" {
						header.Key = header.Name
					}

					header.Key = strings.ReplaceAll(strings.ToUpper(header.Key), "_", "-")

					c.HTTPHeaders[i] = header
				}

				addEntry = true
				catalogEntry.Spec.URLManifest = types.MCPServerCatalogEntryManifest{
					URL:         entry.URL,
					GitHubStars: entry.Stars,
					Metadata:    m,
					Server: types.MCPServerManifest{
						Name:        entry.DisplayName,
						Description: entry.Description,
						Icon:        entry.Icon,
						URL:         c.URL,
						Headers:     c.HTTPHeaders,
					},
				}
			}
		}

		if addEntry {
			objs = append(objs, &catalogEntry)
		}
	}

	return objs, nil
}

func (h *Handler) readFromMCPCatalogs(ctx context.Context, c client.Client) error {
	var (
		toAdd []client.Object
		errs  []error
	)
	for _, catalog := range h.mcpCatalogs {
		entries, err := h.readMCPCatalog(catalog)
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to read MCP catalog %s: %w", catalog, err))
			continue
		}

		toAdd = append(toAdd, entries...)
	}

	if len(errs) > 0 {
		// Don't accidentally delete tool entries for catalogs that failed to be read.
		return errors.Join(errs...)
	}

	if len(toAdd) == 0 {
		// Don't accidentally delete all the catalog entries
		return nil
	}

	return apply.New(c).WithOwnerSubContext("mcpcatalogentries").Apply(ctx, nil, toAdd...)
}

func isCommandPreferred(existing, newer string) bool {
	if existing == "" {
		return true
	}
	if newer == "" || existing == "npx" {
		return false
	}

	if existing == "uvx" {
		return newer == "npx"
	}

	// This would mean that existing is docker and newer is either npx or uvx.
	return true
}

type catalogEntryInfo struct {
	ID              int    `json:"id"`
	Path            string `json:"path"`
	DisplayName     string `json:"displayName"`
	FullName        string `json:"fullName"`
	URL             string `json:"url"`
	Description     string `json:"description"`
	Stars           int    `json:"stars"`
	ReadmeContent   string `json:"readmeContent"`
	Language        string `json:"language"`
	Metadata        string `json:"metadata"`
	License         string `json:"license"`
	Icon            string `json:"icon"`
	Manifest        string `json:"manifest"`
	ToolDefinitions string `json:"toolDefinitions"`
}

type mcpServerManifest struct {
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Category    string            `json:"category"`
	Configs     []mcpServerConfig `json:"configs"`
}

type mcpServerConfig struct {
	Env            []types.MCPEnv    `json:"env"`
	Command        string            `json:"command,omitempty"`
	Args           []string          `json:"args,omitempty"`
	HTTPHeaders    []types.MCPHeader `json:"httpHeaders,omitempty"`
	URL            string            `json:"url,omitempty"`
	Remote         bool              `json:"remote,omitempty"`
	URLDescription string            `json:"urlDescription,omitempty"`
	Preferred      bool              `json:"preferred,omitempty"`
}

func (h *Handler) PollRegistriesAndCatalogs(ctx context.Context, c client.Client) {
	if len(h.registryURLs) == 0 && len(h.mcpCatalogs) == 0 {
		return
	}

	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		if err := h.readFromRegistry(ctx, c); err != nil {
			log.Errorf("Failed to read from registries: %v", err)
		}
		if err := h.readFromMCPCatalogs(ctx, c); err != nil {
			log.Errorf("Failed to read from MCP catalogs: %v", err)
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

	if err := h.createMCPServerCatalog(req, toolRef); err != nil {
		toolRef.Status.Error = err.Error()
		return nil
	}

	toolRef.Status.Tool.Credentials, toolRef.Status.Tool.CredentialNames, err = creds.DetermineCredsAndCredNames(prg, tool, toolRef.Spec.Reference)
	if err != nil {
		toolRef.Status.Error = err.Error()
	}

	return nil
}

func (h *Handler) createMCPServerCatalog(req router.Request, toolRef *v1.ToolReference) error {
	if toolRef.Spec.Type != types.ToolReferenceTypeTool || toolRef.Spec.BundleToolName != "" {
		return nil
	}

	// MIGRATION: delete catalog entries for existing non-mcp tools.
	if toolRef.Spec.ToolMetadata["mcp"] != "true" {
		return client.IgnoreNotFound(req.Client.Delete(req.Ctx, &v1.MCPServerCatalogEntry{
			ObjectMeta: metav1.ObjectMeta{
				Name:      toolRef.Name,
				Namespace: system.DefaultNamespace,
			},
		}))
	}

	if toolRef.Status.Tool == nil {
		return nil
	}

	if toolRef.Spec.Active != nil && !*toolRef.Spec.Active {
		err := req.Client.Delete(req.Ctx, &v1.MCPServerCatalogEntry{
			ObjectMeta: metav1.ObjectMeta{
				Name:      toolRef.Name,
				Namespace: system.DefaultNamespace,
			},
		})
		return client.IgnoreNotFound(err)
	}

	serverManifest := types.MCPServerManifest{
		Name:        toolRef.Status.Tool.Name,
		Description: toolRef.Status.Tool.Description,
		Icon:        toolRef.Status.Tool.Metadata["icon"],
	}

	var mcpCatalogEntry v1.MCPServerCatalogEntry
	if err := req.Client.Get(req.Ctx, router.Key(system.DefaultNamespace, toolRef.Name), &mcpCatalogEntry); client.IgnoreNotFound(err) != nil {
		return err
	} else if err == nil {
		// Migration: add the obot-official metadata key to the command manifest if it isn't there.
		var shouldUpdate bool
		if mcpCatalogEntry.Spec.CommandManifest.Metadata["categories"] == "" {
			if mcpCatalogEntry.Spec.CommandManifest.Metadata == nil {
				mcpCatalogEntry.Spec.CommandManifest.Metadata = make(map[string]string)
			}
			mcpCatalogEntry.Spec.CommandManifest.Metadata["categories"] = "Official"
			shouldUpdate = true
		}

		if !equality.Semantic.DeepEqual(mcpCatalogEntry.Spec.CommandManifest.Server, serverManifest) &&
			mcpCatalogEntry.Spec.ToolReferenceName == toolRef.Name {
			shouldUpdate = true
		}

		if shouldUpdate {
			mcpCatalogEntry.Spec.CommandManifest.Server = serverManifest
			mcpCatalogEntry.Spec.ToolReferenceName = toolRef.Name
			return req.Client.Update(req.Ctx, &mcpCatalogEntry)
		}
		return nil
	}

	return req.Client.Create(req.Ctx, &v1.MCPServerCatalogEntry{
		ObjectMeta: metav1.ObjectMeta{
			Name:      toolRef.Name,
			Namespace: system.DefaultNamespace,
		},
		Spec: v1.MCPServerCatalogEntrySpec{
			CommandManifest: types.MCPServerCatalogEntryManifest{
				Server:   serverManifest,
				Metadata: map[string]string{"categories": "Official"},
			},
			ToolReferenceName: toolRef.Name,
		},
	})
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
		types.DefaultModelAliasTypeLLM:             "gpt-4.1",
		types.DefaultModelAliasTypeLLMMini:         "gpt-4.1-mini",
		types.DefaultModelAliasTypeVision:          "gpt-4.1",
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

	availableModels, err := h.dispatcher.ModelsForProvider(req.Ctx, req.Namespace, req.Name)
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
