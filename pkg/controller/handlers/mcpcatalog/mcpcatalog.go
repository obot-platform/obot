package mcpcatalog

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	gptscript "github.com/gptscript-ai/go-gptscript"
	"github.com/obot-platform/nah/pkg/apply"
	"github.com/obot-platform/nah/pkg/name"
	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/logger"
	"github.com/obot-platform/obot/pkg/accesscontrolrule"
	gclient "github.com/obot-platform/obot/pkg/gateway/client"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/obot-platform/obot/pkg/validation"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	kuser "k8s.io/apiserver/pkg/authentication/user"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

var log = logger.Package()

// CatalogCredentialToolName is the fixed tool name used for the single
// credential that stores all source-URL tokens for a catalog. Each URL's
// token is stored as a key in the credential's Env map.
const CatalogCredentialToolName = "catalog-source-tokens"

const (
	// These are used to force catalog sync on startup, used for times when changes are made to
	// catalogs, and they must be synced on the next start.
	forceSyncStartupAnnotation = "obot.ai/force-sync-startup"
	// Bump this any time this functionality is needed.
	startupSyncGeneration = "1"
)

type Handler struct {
	defaultCatalogPath       string
	defaultSystemCatalogPath string
	gptClient                *gptscript.GPTScript
	gatewayClient            *gclient.Client
	accessControlRuleHelper  *accesscontrolrule.Helper
}

// revealCatalogCredential retrieves a stored PAT for the given source URL.
// Returns an empty string if no credential is configured (not-found). Any other
// error is logged so credential-store failures are visible in the sync status.
func (h *Handler) revealCatalogCredential(ctx context.Context, catalogName, sourceURL string) string {
	cred, err := h.gptClient.RevealCredential(ctx,
		[]string{catalogName},
		CatalogCredentialToolName,
	)
	if err != nil {
		if !errors.As(err, &gptscript.ErrNotFound{}) {
			log.Errorf("failed to retrieve credential for catalog %s source %s: %v", catalogName, sourceURL, err)
		}
		return ""
	}
	return cred.Env[sourceURL]
}

func New(defaultCatalogPath, defaultSystemCatalogPath string, gptClient *gptscript.GPTScript, gatewayClient *gclient.Client, accessControlRuleHelper *accesscontrolrule.Helper) *Handler {
	return &Handler{
		defaultCatalogPath:       defaultCatalogPath,
		defaultSystemCatalogPath: defaultSystemCatalogPath,
		gptClient:                gptClient,
		gatewayClient:            gatewayClient,
		accessControlRuleHelper:  accessControlRuleHelper,
	}
}

func (h *Handler) Sync(req router.Request, resp router.Response) error {
	mcpCatalog := req.Object.(*v1.MCPCatalog)

	forceSync := mcpCatalog.Annotations[v1.MCPCatalogSyncAnnotation] == "true" || mcpCatalog.Annotations[forceSyncStartupAnnotation] != startupSyncGeneration
	if !forceSync && !mcpCatalog.Status.LastSyncTime.IsZero() {
		timeSinceLastSync := time.Since(mcpCatalog.Status.LastSyncTime.Time)
		if timeSinceLastSync < time.Hour {
			resp.RetryAfter(time.Hour - timeSinceLastSync)
			return nil
		}
	}

	mcpCatalog.Status.IsSyncing = true
	if err := req.Client.Status().Update(req.Ctx, mcpCatalog); err != nil {
		return fmt.Errorf("failed to update catalog status: %w", err)
	}

	defer func() {
		// Fetch the catalog again
		var catalog v1.MCPCatalog
		if err := req.Client.Get(req.Ctx, router.Key(system.DefaultNamespace, mcpCatalog.Name), &catalog); err != nil {
			log.Errorf("failed to get catalog: %v", err)
			return
		}

		catalog.Status.IsSyncing = false
		if err := req.Client.Status().Update(req.Ctx, &catalog); err != nil {
			log.Errorf("failed to update catalog status: %v", err)
		}
	}()

	toAdd := make([]client.Object, 0)
	mcpCatalog.Status.SyncErrors = make(map[string]string)

	for _, sourceURL := range mcpCatalog.Spec.SourceURLs {
		token := h.revealCatalogCredential(req.Ctx, mcpCatalog.Name, sourceURL)
		objs, err := h.readMCPCatalog(req.Ctx, mcpCatalog.Name, sourceURL, token)
		if err != nil {
			log.Errorf("failed to read catalog %s: %v", sourceURL, err)
			mcpCatalog.Status.SyncErrors[sourceURL] = err.Error()
		} else {
			log.Infof("Read MCP catalog source successfully: catalog=%s source=%s entries=%d", mcpCatalog.Name, sourceURL, len(objs))
			delete(mcpCatalog.Status.SyncErrors, sourceURL)
		}

		toAdd = append(toAdd, objs...)
	}

	mcpCatalog.Status.LastSyncTime = metav1.Now()
	if err := req.Client.Status().Update(req.Ctx, mcpCatalog); err != nil {
		return fmt.Errorf("failed to update catalog status: %w", err)
	}
	if forceSync {
		delete(mcpCatalog.Annotations, v1.MCPCatalogSyncAnnotation)
		if mcpCatalog.Annotations == nil {
			mcpCatalog.Annotations = make(map[string]string, 1)
		}
		mcpCatalog.Annotations[forceSyncStartupAnnotation] = startupSyncGeneration
		if err := req.Client.Update(req.Ctx, mcpCatalog); err != nil {
			return fmt.Errorf("failed to update catalog: %w", err)
		}
	}

	// We want to refresh this every hour.
	// TODO(g-linville): make this configurable.
	resp.RetryAfter(time.Hour)

	// I know we don't want to do apply anymore. But we were doing it before in a different place.
	// Now we're doing it here. It's not important enough to change right now.
	app := apply.New(req.Client).WithOwnerSubContext(fmt.Sprintf("catalog-%s", mcpCatalog.Name))

	// Don't run prune if there are sync errors
	if len(mcpCatalog.Status.SyncErrors) > 0 {
		log.Infof("Applying MCP catalog entries without prune due to source errors: catalog=%s entries=%d sourceErrors=%d", mcpCatalog.Name, len(toAdd), len(mcpCatalog.Status.SyncErrors))
		app = app.WithNoPrune()
	} else {
		log.Infof("Applying MCP catalog entries with prune enabled: catalog=%s entries=%d", mcpCatalog.Name, len(toAdd))
		app = app.WithPruneTypes(&v1.MCPServerCatalogEntry{})
	}

	return app.Apply(req.Ctx, mcpCatalog, toAdd...)
}

func (h *Handler) SyncSystem(req router.Request, resp router.Response) error {
	systemCatalog := req.Object.(*v1.SystemMCPCatalog)

	forceSync := systemCatalog.Annotations[v1.SystemMCPCatalogSyncAnnotation] == "true" || systemCatalog.Annotations[forceSyncStartupAnnotation] != startupSyncGeneration
	if !forceSync && !systemCatalog.Status.LastSyncTime.IsZero() {
		timeSinceLastSync := time.Since(systemCatalog.Status.LastSyncTime.Time)
		if timeSinceLastSync < time.Hour {
			resp.RetryAfter(time.Hour - timeSinceLastSync)
			return nil
		}
	}

	systemCatalog.Status.IsSyncing = true
	if err := req.Client.Status().Update(req.Ctx, systemCatalog); err != nil {
		return fmt.Errorf("failed to update system catalog status: %w", err)
	}

	defer func() {
		var catalog v1.SystemMCPCatalog
		if err := req.Client.Get(req.Ctx, router.Key(system.DefaultNamespace, systemCatalog.Name), &catalog); err != nil {
			log.Errorf("failed to get system catalog: %v", err)
			return
		}

		catalog.Status.IsSyncing = false
		if err := req.Client.Status().Update(req.Ctx, &catalog); err != nil {
			log.Errorf("failed to update system catalog status: %v", err)
		}
	}()

	toAdd := make([]client.Object, 0)
	systemCatalog.Status.SyncErrors = make(map[string]string)

	for _, sourceURL := range systemCatalog.Spec.SourceURLs {
		token := h.revealCatalogCredential(req.Ctx, systemCatalog.Name, sourceURL)
		objs, err := h.readSystemMCPCatalog(req.Ctx, systemCatalog.Name, sourceURL, token)
		if err != nil {
			log.Errorf("failed to read system catalog %s: %v", sourceURL, err)
			systemCatalog.Status.SyncErrors[sourceURL] = err.Error()
		} else {
			log.Infof("Read system MCP catalog source successfully: catalog=%s source=%s entries=%d", systemCatalog.Name, sourceURL, len(objs))
			delete(systemCatalog.Status.SyncErrors, sourceURL)
		}

		toAdd = append(toAdd, objs...)
	}

	systemCatalog.Status.LastSyncTime = metav1.Now()
	if err := req.Client.Status().Update(req.Ctx, systemCatalog); err != nil {
		return fmt.Errorf("failed to update system catalog status: %w", err)
	}
	if forceSync {
		delete(systemCatalog.Annotations, v1.SystemMCPCatalogSyncAnnotation)
		if systemCatalog.Annotations == nil {
			systemCatalog.Annotations = make(map[string]string, 1)
		}
		systemCatalog.Annotations[forceSyncStartupAnnotation] = startupSyncGeneration
		if err := req.Client.Update(req.Ctx, systemCatalog); err != nil {
			return fmt.Errorf("failed to update system catalog: %w", err)
		}
	}

	resp.RetryAfter(time.Hour)

	app := apply.New(req.Client).WithOwnerSubContext(fmt.Sprintf("system-catalog-%s", systemCatalog.Name))
	if len(systemCatalog.Status.SyncErrors) > 0 {
		log.Infof("Applying system MCP catalog entries without prune due to source errors: catalog=%s entries=%d sourceErrors=%d", systemCatalog.Name, len(toAdd), len(systemCatalog.Status.SyncErrors))
		app = app.WithNoPrune()
	} else {
		log.Infof("Applying system MCP catalog entries with prune enabled: catalog=%s entries=%d", systemCatalog.Name, len(toAdd))
		app = app.WithPruneTypes(&v1.SystemMCPServerCatalogEntry{})
	}

	return app.Apply(req.Ctx, systemCatalog, toAdd...)
}

func (h *Handler) readSystemMCPCatalog(ctx context.Context, catalogName, sourceURL, token string) ([]client.Object, error) {
	entries, err := readCatalogManifests[types.SystemMCPServerCatalogEntryManifest](ctx, sourceURL, token)
	if err != nil {
		return nil, err
	}

	systemObjs := make([]client.Object, 0, len(entries))
	var errs []error
	for _, entry := range entries {
		if entry.Metadata["categories"] == "Official" {
			delete(entry.Metadata, "categories")
		}

		cleanName := strings.ToLower(strings.ReplaceAll(entry.Name, " ", "-"))
		mcpManifest := systemCatalogEntryManifestToMCP(entry)
		sanitizeCatalogEntryManifest(&mcpManifest)
		entry = mcpCatalogEntryManifestToSystem(mcpManifest, entry.SystemMCPServerType, entry.FilterConfig)
		if err := validation.ValidateSystemMCPServerCatalogEntryManifest(entry); err != nil {
			errs = append(errs, fmt.Errorf("failed to validate system catalog entry %s: %w", entry.Name, err))
			continue
		}

		systemObjs = append(systemObjs, &v1.SystemMCPServerCatalogEntry{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name.SafeHashConcatName(catalogName, cleanName),
				Namespace: system.DefaultNamespace,
			},
			Spec: v1.SystemMCPServerCatalogEntrySpec{
				SystemMCPCatalogName: catalogName,
				SourceURL:            sourceURL,
				Editable:             false,
				Manifest:             entry,
			},
		})
	}

	return systemObjs, errors.Join(errs...)
}

func mcpCatalogEntryManifestToSystem(manifest types.MCPServerCatalogEntryManifest, systemMCPServerType types.SystemMCPServerType, filterConfig *types.FilterConfig) types.SystemMCPServerCatalogEntryManifest {
	return types.SystemMCPServerCatalogEntryManifest{
		Metadata:            manifest.Metadata,
		Name:                manifest.Name,
		ShortDescription:    manifest.ShortDescription,
		Description:         manifest.Description,
		Icon:                manifest.Icon,
		RepoURL:             manifest.RepoURL,
		ToolPreview:         manifest.ToolPreview,
		SystemMCPServerType: systemMCPServerType,
		FilterConfig:        filterConfig,
		Runtime:             manifest.Runtime,
		UVXConfig:           manifest.UVXConfig,
		NPXConfig:           manifest.NPXConfig,
		ContainerizedConfig: manifest.ContainerizedConfig,
		RemoteConfig:        manifest.RemoteConfig,
		Env:                 manifest.Env,
	}
}

func systemCatalogEntryManifestToMCP(manifest types.SystemMCPServerCatalogEntryManifest) types.MCPServerCatalogEntryManifest {
	return types.MCPServerCatalogEntryManifest{
		Metadata:            manifest.Metadata,
		Name:                manifest.Name,
		ShortDescription:    manifest.ShortDescription,
		Description:         manifest.Description,
		Icon:                manifest.Icon,
		RepoURL:             manifest.RepoURL,
		ToolPreview:         manifest.ToolPreview,
		Runtime:             manifest.Runtime,
		UVXConfig:           manifest.UVXConfig,
		NPXConfig:           manifest.NPXConfig,
		ContainerizedConfig: manifest.ContainerizedConfig,
		RemoteConfig:        manifest.RemoteConfig,
		Env:                 manifest.Env,
	}
}

func (h *Handler) readMCPCatalog(ctx context.Context, catalogName, sourceURL, token string) ([]client.Object, error) {
	var entries []types.MCPServerCatalogEntryManifest

	if strings.HasPrefix(sourceURL, "http://") || strings.HasPrefix(sourceURL, "https://") {
		if isGitRepoURL(sourceURL) {
			var err error
			entries, err = readGitCatalogEntries[types.MCPServerCatalogEntryManifest](ctx, sourceURL, token)
			if err != nil {
				return nil, fmt.Errorf("failed to read git catalog %s: %w", sourceURL, err)
			}
		} else {
			// If it wasn't a git repo, treat it as a raw file.
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, http.NoBody)
			if err != nil {
				return nil, fmt.Errorf("failed to create request for catalog %s: %w", sourceURL, err)
			}
			if token != "" {
				req.Header.Set("Authorization", "Bearer "+token)
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				return nil, fmt.Errorf("failed to read catalog %s: %w", sourceURL, err)
			}
			defer resp.Body.Close()

			contents, err := io.ReadAll(resp.Body)
			if err != nil {
				return nil, fmt.Errorf("failed to read catalog %s: %w", sourceURL, err)
			}

			if resp.StatusCode != http.StatusOK {
				return nil, fmt.Errorf("unexpected status when reading catalog %s: %s", sourceURL, string(contents))
			}

			if err = yaml.Unmarshal(contents, &entries); err != nil {
				return nil, fmt.Errorf("failed to decode catalog %s: %w", sourceURL, err)
			}
		}
	} else {
		fileInfo, err := os.Stat(sourceURL)
		if err != nil {
			return nil, fmt.Errorf("failed to stat catalog %s: %w", sourceURL, err)
		}

		if fileInfo.IsDir() {
			entries, err = readCatalogDirectory[types.MCPServerCatalogEntryManifest](sourceURL)
			if err != nil {
				return nil, fmt.Errorf("failed to read catalog %s: %w", sourceURL, err)
			}
		} else {
			contents, err := os.ReadFile(sourceURL)
			if err != nil {
				return nil, fmt.Errorf("failed to read catalog %s: %w", sourceURL, err)
			}

			if err = yaml.Unmarshal(contents, &entries); err != nil {
				return nil, fmt.Errorf("failed to decode catalog %s: %w", sourceURL, err)
			}
		}
	}

	objs := make([]client.Object, 0, len(entries))
	var errs []error
	for _, entry := range entries {
		if entry.Metadata["categories"] == "Official" {
			delete(entry.Metadata, "categories") // This shouldn't happen, but do this just in case.
			// We don't want to mark random MCP servers from the catalog as official.
		}

		cleanName := strings.ToLower(strings.ReplaceAll(entry.Name, " ", "-"))

		catalogEntry := v1.MCPServerCatalogEntry{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name.SafeHashConcatName(catalogName, cleanName),
				Namespace: system.DefaultNamespace,
			},
			Spec: v1.MCPServerCatalogEntrySpec{
				MCPCatalogName: catalogName,
				SourceURL:      sourceURL,
				Editable:       false, // entries from source URLs are not editable
			},
		}

		// Check the metadata for default disabled tools.
		if entry.Metadata["unsupportedTools"] != "" {
			catalogEntry.Spec.UnsupportedTools = strings.Split(entry.Metadata["unsupportedTools"], ",")
		}

		sanitizeCatalogEntryManifest(&entry)

		if err := validation.ValidateCatalogEntryManifest(entry); err != nil {
			errs = append(errs, fmt.Errorf("failed to validate catalog entry %s: %w", entry.Name, err))
			continue
		}
		// Catalog entries from a git source are git-managed by definition,
		// which is the gate for allowing secretBinding references.
		if err := validation.ValidateSecretBindingsCatalogEntry(entry, true); err != nil {
			errs = append(errs, fmt.Errorf("failed to validate catalog entry %s: %w", entry.Name, err))
			continue
		}
		if err := validation.ValidateTemplateReferencesCatalogEntry(entry); err != nil {
			errs = append(errs, fmt.Errorf("failed to validate catalog entry %s: %w", entry.Name, err))
			continue
		}
		catalogEntry.Spec.Manifest = entry

		objs = append(objs, &catalogEntry)
	}

	return objs, errors.Join(errs...)
}

func readCatalogManifests[T any](ctx context.Context, sourceURL, token string) ([]T, error) {
	if strings.HasPrefix(sourceURL, "http://") || strings.HasPrefix(sourceURL, "https://") {
		if isGitRepoURL(sourceURL) {
			entries, err := readGitCatalogEntries[T](ctx, sourceURL, token)
			if err != nil {
				return nil, fmt.Errorf("failed to read git catalog %s: %w", sourceURL, err)
			}
			return entries, nil
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, http.NoBody)
		if err != nil {
			return nil, fmt.Errorf("failed to create request for catalog %s: %w", sourceURL, err)
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("failed to read catalog %s: %w", sourceURL, err)
		}
		defer resp.Body.Close()

		contents, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to read catalog %s: %w", sourceURL, err)
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("unexpected status when reading catalog %s: %s", sourceURL, string(contents))
		}

		var entries []T
		if err = yaml.Unmarshal(contents, &entries); err != nil {
			return nil, fmt.Errorf("failed to decode catalog %s: %w", sourceURL, err)
		}
		return entries, nil
	}

	fileInfo, err := os.Stat(sourceURL)
	if err != nil {
		return nil, fmt.Errorf("failed to stat catalog %s: %w", sourceURL, err)
	}
	if fileInfo.IsDir() {
		entries, err := readCatalogDirectory[T](sourceURL)
		if err != nil {
			return nil, fmt.Errorf("failed to read catalog %s: %w", sourceURL, err)
		}
		return entries, nil
	}

	contents, err := os.ReadFile(sourceURL)
	if err != nil {
		return nil, fmt.Errorf("failed to read catalog %s: %w", sourceURL, err)
	}

	var entries []T
	if err = yaml.Unmarshal(contents, &entries); err != nil {
		return nil, fmt.Errorf("failed to decode catalog %s: %w", sourceURL, err)
	}
	return entries, nil
}

func sanitizeCatalogEntryManifest(entry *types.MCPServerCatalogEntryManifest) {
	for i, env := range entry.Env {
		if env.Key == "" {
			env.Key = env.Name
		}
		if filepath.Ext(env.Key) != "" {
			env.Key = strings.ReplaceAll(env.Key, ".", "_")
			env.File = true
		}
		env.Key = strings.ReplaceAll(strings.ToUpper(env.Key), "-", "_")
		entry.Env[i] = env
	}

	if entry.Runtime == types.RuntimeRemote && entry.RemoteConfig != nil {
		for i, header := range entry.RemoteConfig.Headers {
			if header.Key == "" {
				header.Key = header.Name
			}
			header.Key = strings.ReplaceAll(strings.ToUpper(header.Key), "_", "-")
			entry.RemoteConfig.Headers[i] = header
		}
	}
}

func readCatalogDirectory[T any](catalog string) ([]T, error) {
	var (
		catalogPatterns       = []string{"*.json", "*.yaml", "*.yml"} // Default to all JSON and YAML files
		ignorePatterns        []string
		usingObotCatalogsFile bool
	)

	// First try to get .obotcatalogs file
	obotCatalogsPath := filepath.Join(catalog, ".obotcatalogs")
	if content, err := os.ReadFile(obotCatalogsPath); err == nil {
		usingObotCatalogsFile = true
		scanner := bufio.NewScanner(strings.NewReader(string(content)))
		var patterns []string
		for scanner.Scan() {
			line := scanner.Text()
			line = strings.TrimSpace(line)
			if line != "" && !strings.HasPrefix(line, "#") {
				patterns = append(patterns, line)
			}
		}
		if scanner.Err() != nil && scanner.Err() != io.EOF {
			log.Warnf("Failed to read .obotcatalogs file: %v", scanner.Err())
		} else if len(patterns) > 0 {
			catalogPatterns = patterns
		}
	}

	obotIgnoreCatalogsPath := filepath.Join(catalog, ".ignoreobotcatalogs")
	if content, err := os.ReadFile(obotIgnoreCatalogsPath); err == nil {
		scanner := bufio.NewScanner(strings.NewReader(string(content)))
		var patterns []string
		for scanner.Scan() {
			line := scanner.Text()
			line = strings.TrimSpace(line)
			if line != "" && !strings.HasPrefix(line, "#") {
				patterns = append(patterns, line)
			}
		}
		if scanner.Err() != nil && scanner.Err() != io.EOF {
			log.Warnf("Failed to read .ignoreobotcatalogs file: %v", scanner.Err())
		} else if len(patterns) > 0 {
			ignorePatterns = patterns
		}
	}

	// Walk through the cloned repository to find matching files
	var (
		entries   []T
		fileCount int
	)
	const maxFiles = 1000 // Limit the number of files processed to prevent resource exhaustion

	err := filepath.WalkDir(catalog, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Get relative path from repository root
		relPath, err := filepath.Rel(catalog, path)
		if err != nil {
			return err
		}

		// Skip the .git directory specifically
		if d.IsDir() && (relPath == ".git" || strings.HasPrefix(relPath, ".git/")) {
			return filepath.SkipDir
		}

		// Skip directories (but continue walking into them)
		if d.IsDir() {
			for _, pattern := range ignorePatterns {
				if matched, _ := filepath.Match(pattern, relPath); matched {
					return filepath.SkipDir
				}
			}
			return nil
		}

		// Check file count limit
		fileCount++
		if fileCount > maxFiles {
			return fmt.Errorf("too many files to process (limit: %d)", maxFiles)
		}

		// Check if file matches any pattern
		var matches bool
		for _, pattern := range catalogPatterns {
			if matched, _ := filepath.Match(pattern, filepath.Base(relPath)); matched {
				matches = true
				break
			}
		}
		if !matches {
			return nil
		}

		// Check if file matches any ignore pattern
		for _, pattern := range ignorePatterns {
			if matched, _ := filepath.Match(pattern, relPath); matched {
				return nil
			}
		}

		// Security check: ensure the file is safe to read
		if err := isPathSafe(path, catalog); err != nil {
			log.Warnf("Skipping unsafe file %s: %v", relPath, err)
			return nil
		}

		// Read file contents
		content, err := os.ReadFile(path)
		if err != nil {
			log.Warnf("Failed to read contents of %s: %v", relPath, err)
			return nil
		}

		// Try to unmarshal as array first
		var fileEntries []T
		if err := yaml.Unmarshal(content, &fileEntries); err != nil {
			// If that fails, try single object with YAML
			var entry T
			if err := yaml.Unmarshal(content, &entry); err != nil {
				if usingObotCatalogsFile {
					log.Warnf("Failed to parse %s as catalog entry: %v", relPath, err)
				} else {
					log.Debugf("Failed to parse %s as catalog entry: %v", relPath, err)
				}
				return nil
			}
			fileEntries = []T{entry}
		}

		entries = append(entries, fileEntries...)
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to walk repository files: %w", err)
	}

	return entries, nil
}

func (h *Handler) SetUpDefaultMCPCatalog(ctx context.Context, c client.Client) error {
	var existing v1.MCPCatalog
	if err := c.Get(ctx, router.Key(system.DefaultNamespace, system.DefaultCatalog), &existing); err == nil {
		// TODO: Remove this migration logic once we've migrated all Obot deployments to the new catalog path.
		if i := slices.IndexFunc(existing.Spec.SourceURLs, func(url string) bool {
			matched, _ := regexp.MatchString(`^(\./)?/?catalog$`, url)
			return matched
		}); i >= 0 {
			existing.Spec.SourceURLs[i] = h.defaultCatalogPath
			if err := c.Update(ctx, &existing); err != nil {
				return fmt.Errorf("failed to migrate default catalog: %w", err)
			}
			log.Infof("Migrated default MCP catalog source URL: catalog=%s source=%s", existing.Name, h.defaultCatalogPath)
		}

		return nil
	} else if !apierrors.IsNotFound(err) {
		return err
	}

	var sourceURLs []string
	if h.defaultCatalogPath != "" {
		sourceURLs = append(sourceURLs, h.defaultCatalogPath)
	}

	if err := c.Create(ctx, &v1.MCPCatalog{
		ObjectMeta: metav1.ObjectMeta{
			Name:      system.DefaultCatalog,
			Namespace: system.DefaultNamespace,
		},
		Spec: v1.MCPCatalogSpec{
			DisplayName: "Default",
			SourceURLs:  sourceURLs,
		},
	}); err != nil {
		return fmt.Errorf("failed to create default catalog: %w", err)
	}
	log.Infof("Created default MCP catalog: catalog=%s sources=%d", system.DefaultCatalog, len(sourceURLs))

	return nil
}

func (h *Handler) SetUpDefaultSystemMCPCatalog(ctx context.Context, c client.Client) error {
	var existing v1.SystemMCPCatalog
	if err := c.Get(ctx, router.Key(system.DefaultNamespace, system.DefaultCatalog), &existing); !apierrors.IsNotFound(err) {
		return err
	}

	var sourceURLs []string
	if h.defaultSystemCatalogPath != "" {
		sourceURLs = append(sourceURLs, h.defaultSystemCatalogPath)
	}

	if err := c.Create(ctx, &v1.SystemMCPCatalog{
		ObjectMeta: metav1.ObjectMeta{
			Name:      system.DefaultCatalog,
			Namespace: system.DefaultNamespace,
		},
		Spec: v1.SystemMCPCatalogSpec{
			DisplayName: "Default",
			SourceURLs:  sourceURLs,
		},
	}); err != nil {
		return fmt.Errorf("failed to create default system MCP catalog: %w", err)
	}
	log.Infof("Created default system MCP catalog: catalog=%s sources=%d", system.DefaultCatalog, len(sourceURLs))

	return nil
}

// DeleteUnauthorizedMCPServersForCatalog is a handler that deletes MCP servers that are no longer authorized to exist
// for the given catalog. This can happen whenever AccessControlRules change.
// It does not delete MCPServerInstances, since those have a delete ref to their MCPServer, and will be deleted automatically.
func (h *Handler) DeleteUnauthorizedMCPServersForCatalog(req router.Request, _ router.Response) error {
	// List AccessControlRules so that this handler gets triggered any time one of them changes.
	if err := req.List(&v1.AccessControlRuleList{}, &client.ListOptions{
		Namespace:     req.Object.GetNamespace(),
		FieldSelector: fields.OneTermEqualSelector("spec.mcpCatalogID", req.Object.GetName()),
	}); err != nil {
		return fmt.Errorf("failed to list access control rules: %w", err)
	}

	var mcpCatalogEntries v1.MCPServerCatalogEntryList
	if err := req.List(&mcpCatalogEntries, &client.ListOptions{
		Namespace:     req.Object.GetNamespace(),
		FieldSelector: fields.OneTermEqualSelector("spec.mcpCatalogName", req.Object.GetName()),
	}); err != nil {
		return fmt.Errorf("failed to list MCP catalog entries: %w", err)
	}

	usersCache := map[string]*userInfo{}
	for _, entry := range mcpCatalogEntries.Items {
		var mcpServers v1.MCPServerList
		err := req.List(&mcpServers, &client.ListOptions{
			Namespace:     req.Object.GetNamespace(),
			FieldSelector: fields.OneTermEqualSelector("spec.mcpServerCatalogEntryName", entry.Name),
		})
		if err != nil {
			return fmt.Errorf("failed to list MCP servers: %w", err)
		}
		// Iterate through each MCPServer and make sure it is still allowed to exist.
		for _, server := range mcpServers.Items {
			if !server.DeletionTimestamp.IsZero() || server.Spec.ThreadName != "" || server.Spec.MCPCatalogID != "" {
				// For legacy project-scoped servers and multi-user servers created by the admin, we don't need to check them.
				continue
			}

			user := usersCache[server.Spec.UserID]
			if user == nil {
				user, err = h.getUserInfoForAccessControl(req.Ctx, server.Spec.UserID)
				if err != nil {
					return fmt.Errorf("failed to get user info for %s: %w", server.Spec.UserID, err)
				}

				usersCache[server.Spec.UserID] = user
			}

			hasAccess, err := h.accessControlRuleHelper.UserHasAccessToMCPServerCatalogEntryInCatalog(user, server.Spec.MCPServerCatalogEntryName, entry.Spec.MCPCatalogName)
			if err != nil {
				return fmt.Errorf("failed to check if user %s has access to catalog entry %s: %w", server.Spec.UserID, server.Spec.MCPServerCatalogEntryName, err)
			}

			if !hasAccess && server.Spec.CompositeName == "" {
				log.Infof("Deleting MCP server %q because it is no longer authorized to exist", server.Name)
				if err := req.Delete(&server); err != nil {
					return fmt.Errorf("failed to delete MCP server %s: %w", server.Name, err)
				}
			}
		}
	}

	return nil
}

// DeleteUnauthorizedMCPServersForWorkspace is a handler that deletes MCP servers that are no longer authorized to exist
// for the given workspace. This can happen whenever AccessControlRules change.
// It does not delete MCPServerInstances, since those have a delete ref to their MCPServer, and will be deleted automatically.
func (h *Handler) DeleteUnauthorizedMCPServersForWorkspace(req router.Request, _ router.Response) error {
	// List AccessControlRules so that this handler gets triggered any time one of them changes.
	if err := req.List(&v1.AccessControlRuleList{}, &client.ListOptions{
		Namespace:     req.Object.GetNamespace(),
		FieldSelector: fields.OneTermEqualSelector("spec.powerUserWorkspaceID", req.Object.GetName()),
	}); err != nil {
		return fmt.Errorf("failed to list access control rules: %w", err)
	}

	var mcpCatalogEntries v1.MCPServerCatalogEntryList
	if err := req.List(&mcpCatalogEntries, &client.ListOptions{
		Namespace:     req.Object.GetNamespace(),
		FieldSelector: fields.OneTermEqualSelector("spec.powerUserWorkspaceID", req.Object.GetName()),
	}); err != nil {
		return fmt.Errorf("failed to list MCP catalog entries: %w", err)
	}

	usersCache := map[string]*userInfo{}
	for _, entry := range mcpCatalogEntries.Items {
		var mcpServers v1.MCPServerList
		err := req.List(&mcpServers, &client.ListOptions{
			Namespace:     req.Object.GetNamespace(),
			FieldSelector: fields.OneTermEqualSelector("spec.mcpServerCatalogEntryName", entry.Name),
		})
		if err != nil {
			return fmt.Errorf("failed to list MCP servers: %w", err)
		}

		// Iterate through each MCPServer and make sure it is still allowed to exist.
		for _, server := range mcpServers.Items {
			if !server.DeletionTimestamp.IsZero() {
				continue
			}

			user := usersCache[server.Spec.UserID]
			if user == nil {
				user, err = h.getUserInfoForAccessControl(req.Ctx, server.Spec.UserID)
				if err != nil {
					return fmt.Errorf("failed to get user info for %s: %w", server.Spec.UserID, err)
				}

				usersCache[server.Spec.UserID] = user
			}

			if server.Spec.PowerUserWorkspaceID != "" {
				// For multi-user servers in a PowerUserWorkspace, make sure that the user on that workspace is a PowerUserPlus, and not a normal PowerUser
				if !user.role.HasRole(types.RolePowerUserPlus) {
					log.Infof("Deleting multi-user MCP server %q because its owner is no longer a PowerUserPlus", server.Name)
					if err := req.Delete(&server); err != nil {
						return fmt.Errorf("failed to delete MCP server %s: %w", server.Name, err)
					}
				}

				continue
			}

			hasAccess, err := h.accessControlRuleHelper.UserHasAccessToMCPServerCatalogEntryInWorkspace(req.Ctx, user, server.Spec.MCPServerCatalogEntryName, entry.Spec.PowerUserWorkspaceID)
			if err != nil {
				return fmt.Errorf("failed to check if user %s has access to catalog entry %s in workspace %s: %w", server.Spec.UserID, server.Spec.MCPServerCatalogEntryName, entry.Spec.PowerUserWorkspaceID, err)
			}

			if !hasAccess {
				log.Infof("Deleting MCP server %q because it is no longer authorized to exist", server.Name)
				if err := req.Delete(&server); err != nil {
					return fmt.Errorf("failed to delete MCP server %s: %w", server.Name, err)
				}
			}
		}
	}

	return nil
}

// DeleteUnauthorizedMCPServerInstancesForCatalog is a handler that deletes MCPServerInstances that point to multi-user MCPServers created by the admin,
// where the user who owns the MCPServerInstance is no longer authorized to use the MCPServer.
// This can happen whenever AccessControlRules change.
func (h *Handler) DeleteUnauthorizedMCPServerInstancesForCatalog(req router.Request, _ router.Response) error {
	// List AccessControlRules so that this handler gets triggered any time one of them changes.
	if err := req.List(&v1.AccessControlRuleList{}, &client.ListOptions{
		Namespace:     req.Object.GetNamespace(),
		FieldSelector: fields.OneTermEqualSelector("spec.mcpCatalogID", req.Object.GetName()),
	}); err != nil {
		return fmt.Errorf("failed to list access control rules: %w", err)
	}

	var mcpServers v1.MCPServerList
	err := req.List(&mcpServers, &client.ListOptions{
		Namespace:     req.Object.GetNamespace(),
		FieldSelector: fields.OneTermEqualSelector("spec.mcpCatalogID", req.Object.GetName()),
	})
	if err != nil {
		return fmt.Errorf("failed to list MCP servers: %w", err)
	}

	userCache := map[string]*userInfo{}
	for _, server := range mcpServers.Items {
		var mcpServerInstances v1.MCPServerInstanceList
		err = req.List(&mcpServerInstances, &client.ListOptions{
			Namespace:     req.Object.GetNamespace(),
			FieldSelector: fields.OneTermEqualSelector("spec.mcpServerName", server.Name),
		})
		if err != nil {
			return fmt.Errorf("failed to list MCP server instances: %w", err)
		}

		// Iterate through each MCPServerInstance and make sure it is still allowed to exist.
		for _, instance := range mcpServerInstances.Items {
			if !instance.DeletionTimestamp.IsZero() {
				continue
			}

			user := userCache[instance.Spec.UserID]
			if user == nil {
				user, err = h.getUserInfoForAccessControl(req.Ctx, instance.Spec.UserID)
				if err != nil {
					return fmt.Errorf("failed to get user %s: %w", instance.Spec.UserID, err)
				}

				userCache[instance.Spec.UserID] = user
			}

			hasAccess, err := h.accessControlRuleHelper.UserHasAccessToMCPServerInCatalog(user, instance.Spec.MCPServerName, server.Spec.MCPCatalogID)
			if err != nil {
				return fmt.Errorf("failed to check if user %s has access to MCP server %s: %w", instance.Spec.UserID, instance.Spec.MCPServerName, err)
			}

			if !hasAccess && instance.Spec.CompositeName == "" {
				log.Infof("Deleting MCPServerInstance %q because it is no longer authorized to exist", instance.Name)
				if err := req.Delete(&instance); err != nil {
					return fmt.Errorf("failed to delete MCPServerInstance %s: %w", instance.Name, err)
				}
			}
		}
	}

	return nil
}

// DeleteUnauthorizedMCPServerInstancesForWorkspace is a handler that deletes MCPServerInstances that point to multi-user MCPServers created by the admin,
// where the user who owns the MCPServerInstance is no longer authorized to use the MCPServer.
// This can happen whenever AccessControlRules change.
func (h *Handler) DeleteUnauthorizedMCPServerInstancesForWorkspace(req router.Request, _ router.Response) error {
	// List AccessControlRules so that this handler gets triggered any time one of them changes.
	if err := req.List(&v1.AccessControlRuleList{}, &client.ListOptions{
		Namespace:     req.Object.GetNamespace(),
		FieldSelector: fields.OneTermEqualSelector("spec.powerUserWorkspaceID", req.Object.GetName()),
	}); err != nil {
		return fmt.Errorf("failed to list access control rules: %w", err)
	}

	var mcpServers v1.MCPServerList
	err := req.List(&mcpServers, &client.ListOptions{
		Namespace:     req.Object.GetNamespace(),
		FieldSelector: fields.OneTermEqualSelector("spec.powerUserWorkspaceID", req.Object.GetName()),
	})
	if err != nil {
		return fmt.Errorf("failed to list MCP servers: %w", err)
	}

	userCache := map[string]*userInfo{}
	for _, server := range mcpServers.Items {
		var mcpServerInstances v1.MCPServerInstanceList
		err = req.List(&mcpServerInstances, &client.ListOptions{
			Namespace:     req.Object.GetNamespace(),
			FieldSelector: fields.OneTermEqualSelector("spec.mcpServerName", server.Name),
		})
		if err != nil {
			return fmt.Errorf("failed to list MCP server instances: %w", err)
		}

		// Iterate through each MCPServerInstance and make sure it is still allowed to exist.
		for _, instance := range mcpServerInstances.Items {
			if !instance.DeletionTimestamp.IsZero() {
				continue
			}

			user := userCache[instance.Spec.UserID]
			if user == nil {
				user, err = h.getUserInfoForAccessControl(req.Ctx, instance.Spec.UserID)
				if err != nil {
					return fmt.Errorf("failed to get user %s: %w", instance.Spec.UserID, err)
				}

				userCache[instance.Spec.UserID] = user
			}

			hasAccess, err := h.accessControlRuleHelper.UserHasAccessToMCPServerInWorkspace(user, instance.Spec.MCPServerName, server.Spec.PowerUserWorkspaceID, server.Spec.UserID)
			if err != nil {
				return fmt.Errorf("failed to check if user %s has access to MCP server %s: %w", instance.Spec.UserID, instance.Spec.MCPServerName, err)
			}

			if !hasAccess && instance.Spec.CompositeName == "" {
				log.Infof("Deleting MCPServerInstance %q because it is no longer authorized to exist", instance.Name)
				if err := req.Delete(&instance); err != nil {
					return fmt.Errorf("failed to delete MCPServerInstance %s: %w", instance.Name, err)
				}
			}
		}
	}

	return nil
}

// userInfo is a wrapper around kuser.Info that includes the user's role.
type userInfo struct {
	kuser.Info
	role types.Role
}

// getUserInfoForAccessControl gets user info needed for access control checks
func (h *Handler) getUserInfoForAccessControl(ctx context.Context, userID string) (*userInfo, error) {
	gatewayUser, err := h.gatewayClient.UserByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user %s: %w", userID, err)
	}

	// Get all provider auth groups for the user.
	groupIDs, err := h.gatewayClient.ListGroupIDsForUser(ctx, gatewayUser.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to list user group IDs: %w", err)
	}

	return &userInfo{
		Info: &kuser.DefaultInfo{
			Name:   gatewayUser.Username,
			UID:    fmt.Sprintf("%d", gatewayUser.ID),
			Groups: []string{},
			Extra: map[string][]string{
				// Omit the auth provider namespace and name since groupIDs may include groups from multiple auth providers.
				"auth_provider_groups": groupIDs,
			},
		},
		role: gatewayUser.Role,
	}, nil
}
