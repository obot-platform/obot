package mcpcatalog

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/obot-platform/nah/pkg/apply"
	"github.com/obot-platform/nah/pkg/name"
	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/logger"
	"github.com/obot-platform/obot/pkg/accesscontrolrule"
	gclient "github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/git"
	"github.com/obot-platform/obot/pkg/gitcredential"
	"github.com/obot-platform/obot/pkg/mcp"
	catalogvalidation "github.com/obot-platform/obot/pkg/mcpcatalog"
	"github.com/obot-platform/obot/pkg/safehttp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	kuser "k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

var log = logger.Package()

// CatalogCredentialToolName is the fixed tool name used for the single
// credential that stores all source-URL tokens for a catalog. Each URL's
// token is stored as a key in the credential's Env map.
const CatalogCredentialToolName = "catalog-source-tokens"

const (
	catalogReferenceSeparator = "::"

	// These are used to force catalog sync on startup, used for times when changes are made to
	// catalogs, and they must be synced on the next start.
	forceSyncStartupAnnotation = "obot.ai/force-sync-startup"
	// Bump this any time this functionality is needed.
	startupSyncGeneration = "1"
)

type Handler struct {
	defaultCatalogPath        string
	defaultSystemCatalogPath  string
	httpClient                *http.Client
	gatewayClient             *gclient.Client
	accessControlRuleHelper   *accesscontrolrule.Helper
	remoteURLValidationConfig mcp.ValidationOptions
	mcpBackend                string
	mcpSessionManager         *mcp.SessionManager
}

func New(defaultCatalogPath, defaultSystemCatalogPath string, gatewayClient *gclient.Client, accessControlRuleHelper *accesscontrolrule.Helper, mcpSessionManager *mcp.SessionManager) *Handler {
	remoteURLValidationConfig := mcpSessionManager.RemoteMCPURLValidationConfig()
	validationOptions := mcp.ValidationOptions{
		RemoteMCPURLValidationConfig: remoteURLValidationConfig,
	}

	return &Handler{
		defaultCatalogPath:       defaultCatalogPath,
		defaultSystemCatalogPath: defaultSystemCatalogPath,
		gatewayClient:            gatewayClient,
		httpClient: safehttp.NewClient(safehttp.ClientOptions{
			BlockLoopback:  !remoteURLValidationConfig.AllowLocalhostMCP,
			BlockPrivateIP: !remoteURLValidationConfig.AllowPrivateIPMCP,
			BlockLinkLocal: !remoteURLValidationConfig.AllowLinkLocalMCP,
		}),
		accessControlRuleHelper:   accessControlRuleHelper,
		remoteURLValidationConfig: validationOptions,
		mcpBackend:                mcpSessionManager.MCPRuntimeBackend(),
		mcpSessionManager:         mcpSessionManager,
	}
}

func (h *Handler) Sync(req router.Request, resp router.Response) error {
	mcpCatalog := req.Object.(*v1.MCPCatalog)
	maximums, err := h.mcpSessionManager.EffectiveKubernetesResourceMaximums(req.Ctx, req.Client)
	if err != nil {
		return fmt.Errorf("failed to get effective resource maximums: %w", err)
	}
	validationOptions := h.remoteURLValidationConfig
	validationOptions.ResourceMaximums = maximums

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
		credentialID := mcpCatalog.Spec.SourceURLGitCredentialIDs[sourceURL]
		token, err := gitcredential.ResolveOrReveal(req.Ctx, req.Client, h.gatewayClient, mcpCatalog.Namespace, credentialID, sourceURL, mcpCatalog.Name, CatalogCredentialToolName)
		if errors.Is(err, gitcredential.ErrLegacyCredential) {
			log.Errorf("failed to retrieve legacy credential for catalog %s source %s, continuing without authentication: %v", mcpCatalog.Name, sourceURL, err)
			err = nil
		} else if err != nil {
			log.Errorf("failed to resolve credential for catalog %s source %s: %v", mcpCatalog.Name, sourceURL, err)
			mcpCatalog.Status.SyncErrors[sourceURL] = err.Error()
			continue
		}
		objs, err := h.readMCPCatalog(req.Ctx, mcpCatalog.Name, sourceURL, token, validationOptions)
		if err != nil {
			log.Errorf("failed to read catalog %s: %v", sourceURL, err)
			mcpCatalog.Status.SyncErrors[sourceURL] = err.Error()
		} else {
			log.Infof("Read MCP catalog source successfully: catalog=%s source=%s entries=%d", mcpCatalog.Name, sourceURL, len(objs))
			delete(mcpCatalog.Status.SyncErrors, sourceURL)
		}

		toAdd = append(toAdd, objs...)
	}

	toAdd, conflictErrors, err := filterConflictingCatalogEntries(req.Ctx, req.Client, mcpCatalog.Namespace, toAdd)
	if err != nil {
		return fmt.Errorf("failed to check catalog entry conflicts: %w", err)
	}
	for sourceURL, errMsg := range conflictErrors {
		addSyncError(mcpCatalog.Status.SyncErrors, sourceURL, errMsg)
	}

	toAdd, compositeRefErrors := h.resolveCompositeSourceRefs(req.Ctx, req.Client, mcpCatalog.Namespace, mcpCatalog.Name, toAdd, validationOptions)
	for sourceURL, errMsg := range compositeRefErrors {
		addSyncError(mcpCatalog.Status.SyncErrors, sourceURL, errMsg)
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
	// Apply must not prune because its informer may still observe stale ownership metadata and
	// delete a freshly detached entry
	app := apply.New(req.Client).WithOwnerSubContext(fmt.Sprintf("catalog-%s", mcpCatalog.Name)).WithNoPrune()

	// Missing entries cannot be reconciled safely from a partial desired set.
	if len(mcpCatalog.Status.SyncErrors) > 0 {
		log.Infof("Applying MCP catalog entries without reconciling missing entries due to source errors: catalog=%s entries=%d sourceErrors=%d", mcpCatalog.Name, len(toAdd), len(mcpCatalog.Status.SyncErrors))
		return app.Apply(req.Ctx, mcpCatalog, toAdd...)
	}

	if err := reconcileRemovedEntries(req.Ctx, req.Client, mcpCatalog, toAdd); err != nil {
		return err
	}

	log.Infof("Applying MCP catalog entries without prune: catalog=%s entries=%d", mcpCatalog.Name, len(toAdd))
	return app.Apply(req.Ctx, mcpCatalog, toAdd...)
}

func addSyncError(syncErrors map[string]string, sourceURL, errMsg string) {
	if existing := syncErrors[sourceURL]; existing != "" {
		syncErrors[sourceURL] = existing + "; " + errMsg
	} else {
		syncErrors[sourceURL] = errMsg
	}
}

func filterConflictingCatalogEntries(ctx context.Context, c client.Client, namespace string, objs []client.Object) ([]client.Object, map[string]string, error) {
	result := make([]client.Object, 0, len(objs))
	errsBySourceURL := make(map[string]string)
	var existingEntries v1.MCPServerCatalogEntryList
	if err := c.List(ctx, &existingEntries, client.InNamespace(namespace)); err != nil {
		return nil, nil, err
	}
	existingByName := make(map[client.ObjectKey]v1.MCPServerCatalogEntry, len(existingEntries.Items))
	for i := range existingEntries.Items {
		entry := &existingEntries.Items[i]
		existingByName[client.ObjectKeyFromObject(entry)] = *entry
	}

	for _, obj := range objs {
		entry, ok := obj.(*v1.MCPServerCatalogEntry)
		if !ok {
			result = append(result, obj)
			continue
		}

		key := client.ObjectKeyFromObject(entry)
		existing, found := existingByName[key]
		if !found {
			if err := c.Get(ctx, key, &existing); err == nil {
				found = true
			} else if !apierrors.IsNotFound(err) {
				return nil, nil, err
			}
		}
		if !found {
			result = append(result, obj)
			continue
		}
		if existing.Spec.Detached {
			result = append(result, obj)
			continue
		}
		if !existing.IsGitManaged() {
			addSyncError(errsBySourceURL, entry.Spec.SourceURL, fmt.Sprintf("catalog entry %q conflicts with an Obot-managed entry of the same identity", entry.Spec.Manifest.Name))
			continue
		}
		result = append(result, obj)
	}

	return result, errsBySourceURL, nil
}

func reconcileRemovedEntries(ctx context.Context, c client.Client, catalog *v1.MCPCatalog, desired []client.Object) error {
	desiredNames := make(map[string]struct{}, len(desired))
	for _, obj := range desired {
		if entry, ok := obj.(*v1.MCPServerCatalogEntry); ok {
			desiredNames[entry.Name] = struct{}{}
		}
	}
	configuredSources := make(map[string]struct{}, len(catalog.Spec.SourceURLs))
	for _, sourceURL := range catalog.Spec.SourceURLs {
		configuredSources[mcp.SourceIDForURL(sourceURL)] = struct{}{}
	}

	var entries v1.MCPServerCatalogEntryList
	if err := c.List(ctx, &entries, client.InNamespace(catalog.Namespace), client.MatchingFields{"spec.mcpCatalogName": catalog.Name}); err != nil {
		return fmt.Errorf("failed to list catalog entries: %w", err)
	}

	missingNames := make(map[string]struct{})
	for i := range entries.Items {
		entry := &entries.Items[i]
		if _, ok := desiredNames[entry.Name]; ok {
			continue
		}
		if entry.Spec.SourceURL == "" {
			continue
		}

		if _, configured := configuredSources[mcp.SourceIDForURL(entry.Spec.SourceURL)]; !configured {
			if err := c.Delete(ctx, entry); err != nil && !apierrors.IsNotFound(err) {
				return fmt.Errorf("failed to delete catalog entry %q from removed source: %w", entry.Name, err)
			}
			log.Infof("Deleted MCP catalog entry from removed source: catalog=%s entry=%s source=%s", catalog.Name, entry.Name, entry.Spec.SourceURL)
			continue
		}

		missingNames[entry.Name] = struct{}{}
	}

	if len(missingNames) == 0 {
		return nil
	}

	var servers v1.MCPServerList
	if err := c.List(ctx, &servers, client.InNamespace(catalog.Namespace)); err != nil {
		return fmt.Errorf("failed to list servers for removed catalog entries: %w", err)
	}
	referencedNames := make([]string, 0, len(missingNames))
	for _, server := range servers.Items {
		entryName := server.Spec.MCPServerCatalogEntryName
		if _, missing := missingNames[entryName]; missing {
			referencedNames = append(referencedNames, entryName)
			delete(missingNames, entryName)
		}
	}

	for entryName := range missingNames {
		entry := &v1.MCPServerCatalogEntry{ObjectMeta: metav1.ObjectMeta{Name: entryName, Namespace: catalog.Namespace}}
		if err := c.Delete(ctx, entry); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to delete unused catalog entry %q: %w", entryName, err)
		}
		log.Infof("Deleted unused removed MCP catalog entry: catalog=%s entry=%s", catalog.Name, entryName)
	}

	for _, entryName := range referencedNames {
		if err := detachCatalogEntry(ctx, c, catalog, entryName); err != nil {
			return fmt.Errorf("failed to detach catalog entry %q: %w", entryName, err)
		}
		log.Infof("Detached removed MCP catalog entry with active servers: catalog=%s entry=%s", catalog.Name, entryName)
	}

	return nil
}

func detachCatalogEntry(ctx context.Context, c client.Client, catalog *v1.MCPCatalog, entryName string) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		var entry v1.MCPServerCatalogEntry
		if err := c.Get(ctx, client.ObjectKey{Namespace: catalog.Namespace, Name: entryName}, &entry); err != nil {
			if apierrors.IsNotFound(err) {
				return nil
			}
			return err
		}

		entry.Spec.Editable = false
		entry.Spec.Detached = true
		for key := range entry.Annotations {
			if strings.HasPrefix(key, apply.LabelPrefix) {
				delete(entry.Annotations, key)
			}
		}
		for key := range entry.Labels {
			if strings.HasPrefix(key, apply.LabelPrefix) {
				delete(entry.Labels, key)
			}
		}
		entry.OwnerReferences = slices.DeleteFunc(entry.OwnerReferences, func(ref metav1.OwnerReference) bool {
			return ref.APIVersion == v1.SchemeGroupVersion.String() && ref.Kind == "MCPCatalog" && ref.Name == catalog.Name
		})

		return c.Update(ctx, &entry)
	})
}

// resolveCompositeSourceRefs rewrites GitOps portable component refs to stored
// catalog entry names and snapshots the target manifests. Entries with invalid
// portable refs are skipped so bad composites do not get applied.
func (h *Handler) resolveCompositeSourceRefs(ctx context.Context, c client.Client, namespace, catalogName string, objs []client.Object, options ...mcp.ValidationOptions) ([]client.Object, map[string]string) {
	validationOptions := h.remoteURLValidationConfig
	if len(options) > 0 {
		validationOptions = options[0]
	}
	refs := make(map[string]*v1.MCPServerCatalogEntry)
	entriesByName := make(map[string]*v1.MCPServerCatalogEntry)
	for _, obj := range objs {
		entry, ok := obj.(*v1.MCPServerCatalogEntry)
		if !ok {
			continue
		}
		entriesByName[entry.Name] = entry
		if entry.Spec.SourceURL != "" && entry.Spec.Manifest.EntryKey != "" {
			refs[sourceRef(mcp.SourceIDForURL(entry.Spec.SourceURL), entry.Spec.Manifest.EntryKey)] = entry
		}
	}

	result := make([]client.Object, 0, len(objs))
	errsBySourceURL := make(map[string]string)
	for _, obj := range objs {
		entry, ok := obj.(*v1.MCPServerCatalogEntry)
		if !ok || entry.Spec.Manifest.Runtime != types.RuntimeComposite || entry.Spec.Manifest.CompositeConfig == nil {
			result = append(result, obj)
			continue
		}

		changed := false
		var errs []error
		for i := range entry.Spec.Manifest.CompositeConfig.ComponentServers {
			component := &entry.Spec.Manifest.CompositeConfig.ComponentServers[i]
			if component.MCPServerID != "" {
				var server v1.MCPServer
				if err := c.Get(ctx, client.ObjectKey{Namespace: namespace, Name: component.MCPServerID}, &server); err != nil {
					errs = append(errs, fmt.Errorf("failed to get multi-user server %q: %w", component.MCPServerID, err))
					continue
				}
				if server.Spec.IsSingleUser() {
					errs = append(errs, fmt.Errorf("server %q is not a multi-user server", component.MCPServerID))
					continue
				}
				if catalogName != "" && server.Spec.MCPCatalogID != catalogName {
					errs = append(errs, fmt.Errorf("multi-user server %q not found in catalog %q", component.MCPServerID, catalogName))
					continue
				}

				component.Manifest = server.Spec.Manifest.ConvertToCatalogEntry()
				changed = true
				continue
			}
			if component.CatalogEntryID == "" {
				continue
			}

			target, err := resolveComponentSourceRef(refs, mcp.SourceIDForURL(entry.Spec.SourceURL), component.CatalogEntryID)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			if target == nil {
				target = entriesByName[component.CatalogEntryID]
			}
			if target == nil && c != nil {
				var storedEntry v1.MCPServerCatalogEntry
				if err := c.Get(ctx, client.ObjectKey{Namespace: namespace, Name: component.CatalogEntryID}, &storedEntry); err != nil && !apierrors.IsNotFound(err) {
					errs = append(errs, fmt.Errorf("failed to get component catalog entry %q: %w", component.CatalogEntryID, err))
					continue
				} else if err == nil {
					if catalogName != "" && storedEntry.Spec.MCPCatalogName != catalogName {
						errs = append(errs, fmt.Errorf("component catalog entry %q not found in catalog %q", component.CatalogEntryID, catalogName))
						continue
					}
					target = &storedEntry
				}
			}
			if target == nil {
				continue
			}

			component.CatalogEntryID = target.Name
			component.Manifest = target.Spec.Manifest
			changed = true
		}

		if len(errs) > 0 {
			addSyncError(errsBySourceURL, entry.Spec.SourceURL, fmt.Sprintf("failed to resolve composite catalog entry %q: %v", entry.Name, errors.Join(errs...)))
			continue
		}

		if changed {
			if err := catalogvalidation.ValidateManifest(ctx, entry.Spec.Manifest, catalogvalidation.ValidationOptions{
				MCP:        validationOptions,
				MCPBackend: h.mcpBackend,
				GitManaged: entry.IsGitManaged(),
			}); err != nil {
				addSyncError(errsBySourceURL, entry.Spec.SourceURL, fmt.Sprintf("failed to validate resolved composite catalog entry %q: %v", entry.Name, err))
				continue
			}
		}

		result = append(result, obj)
	}

	return result, errsBySourceURL
}

// resolveComponentSourceRef resolves GitOps portable refs. A bare entry key is
// scoped to the current source; source::entryKey targets another source. If the
// ref has no separator and no same-source match, callers can treat it as a
// normal internal catalog entry ID.
func resolveComponentSourceRef(refs map[string]*v1.MCPServerCatalogEntry, sourceID, catalogEntryID string) (*v1.MCPServerCatalogEntry, error) {
	refSourceID, entryKey, hasSep, valid := parseSourceRef(sourceID, catalogEntryID)
	if !valid {
		return nil, fmt.Errorf("invalid catalogEntryID source ref %q", catalogEntryID)
	}
	if refSourceID == "" {
		return nil, nil
	}

	target := refs[sourceRef(refSourceID, entryKey)]
	if hasSep && target == nil {
		return nil, fmt.Errorf("unresolved catalogEntryID source ref %q", catalogEntryID)
	}
	return target, nil
}

// parseSourceRef returns the source/key pair for either an explicit
// source::entryKey reference or a same-source shorthand entryKey.
func parseSourceRef(sourceID, catalogEntryID string) (refSourceID, entryKey string, hasSep, valid bool) {
	refSourceID, entryKey, hasSep = strings.Cut(catalogEntryID, catalogReferenceSeparator)
	if !hasSep {
		return sourceID, catalogEntryID, false, true
	}
	if strings.Contains(entryKey, catalogReferenceSeparator) {
		return refSourceID, entryKey, true, false
	}
	return refSourceID, entryKey, true, refSourceID != "" && entryKey != ""
}

func sourceRef(sourceID, entryKey string) string {
	return fmt.Sprintf("%s%s%s", sourceID, catalogReferenceSeparator, entryKey)
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
		credentialID := systemCatalog.Spec.SourceURLGitCredentialIDs[sourceURL]
		token, err := gitcredential.ResolveOrReveal(req.Ctx, req.Client, h.gatewayClient, systemCatalog.Namespace, credentialID, sourceURL, systemCatalog.Name, CatalogCredentialToolName)
		if errors.Is(err, gitcredential.ErrLegacyCredential) {
			log.Errorf("failed to retrieve legacy credential for system catalog %s source %s, continuing without authentication: %v", systemCatalog.Name, sourceURL, err)
			err = nil
		} else if err != nil {
			log.Errorf("failed to resolve credential for system catalog %s source %s: %v", systemCatalog.Name, sourceURL, err)
			systemCatalog.Status.SyncErrors[sourceURL] = err.Error()
			continue
		}
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
	entries, err := readCatalogManifests[types.SystemMCPServerCatalogEntryManifest](ctx, h.httpClient, sourceURL, token)
	if err != nil {
		return nil, err
	}

	systemObjs := make([]client.Object, 0, len(entries))
	var errs []error
	for _, entry := range entries {
		if entry.Metadata["categories"] == "Official" {
			delete(entry.Metadata, "categories")
		}

		cleanName := catalogvalidation.SanitizeName(entry.Name)
		if cleanName == "" {
			err := fmt.Errorf("invalid system catalog entry name after sanitization: original=%q sanitized=%q", entry.Name, cleanName)
			errs = append(errs, err)
			continue
		}

		mcpManifest := systemCatalogEntryManifestToMCP(entry)
		catalogvalidation.NormalizeManifest(&mcpManifest)
		entry = mcpCatalogEntryManifestToSystem(mcpManifest, entry.SystemMCPServerType, entry.FilterConfig)
		if err := mcp.ValidateSystemMCPServerCatalogEntryManifest(ctx, entry, mcp.ValidationOptions{}); err != nil {
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
		ServerUserType:      manifest.ServerUserType,
		FilterConfig:        filterConfig,
		Runtime:             manifest.Runtime,
		UVXConfig:           manifest.UVXConfig,
		NPXConfig:           manifest.NPXConfig,
		ContainerizedConfig: manifest.ContainerizedConfig,
		RemoteConfig:        manifest.RemoteConfig,
		Env:                 manifest.Env,
		Resources:           manifest.Resources,
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
		Resources:           manifest.Resources,
	}
}

func (h *Handler) readMCPCatalog(ctx context.Context, catalogName, sourceURL, token string, options ...mcp.ValidationOptions) ([]client.Object, error) {
	validationOptions := h.remoteURLValidationConfig
	if len(options) > 0 {
		validationOptions = options[0]
	}
	entries, err := readCatalogManifests[types.MCPServerCatalogEntryManifest](ctx, h.httpClient, sourceURL, token)
	if err != nil {
		return nil, err
	}

	objs := make([]client.Object, 0, len(entries))
	var errs []error
	uniqueEntryKeys := make(map[string]struct{})
	for _, entry := range entries {
		if entry.Metadata["categories"] == "Official" {
			delete(entry.Metadata, "categories") // This shouldn't happen, but do this just in case.
			// We don't want to mark random MCP servers from the catalog as official.
		}

		if err := catalogvalidation.ValidateSourceFields(entry); err != nil {
			errs = append(errs, err)
			continue
		}
		cleanName := catalogvalidation.SanitizeName(entry.Name)
		catalogEntryName := name.SafeHashConcatName(catalogName, cleanName)

		if entry.EntryKey != "" {
			if _, ok := uniqueEntryKeys[entry.EntryKey]; ok {
				errs = append(errs, fmt.Errorf("duplicate source entry key %q also used by catalog entry %q", entry.EntryKey, catalogEntryName))
				continue
			}
			uniqueEntryKeys[entry.EntryKey] = struct{}{}
		}

		catalogEntry := v1.MCPServerCatalogEntry{
			ObjectMeta: metav1.ObjectMeta{
				Name:      catalogEntryName,
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

		catalogvalidation.NormalizeManifest(&entry)
		if err := catalogvalidation.ValidateManifest(ctx, entry, catalogvalidation.ValidationOptions{
			MCP:        validationOptions,
			MCPBackend: h.mcpBackend,
			GitManaged: catalogEntry.IsGitManaged(),
		}); err != nil {
			errs = append(errs, fmt.Errorf("failed to validate catalog entry %s: %w", entry.Name, err))
			continue
		}
		catalogEntry.Spec.Manifest = entry

		objs = append(objs, &catalogEntry)
	}

	return objs, errors.Join(errs...)
}

func readCatalogManifests[T any](ctx context.Context, httpClient *http.Client, sourceURL, token string) ([]T, error) {
	if strings.HasPrefix(sourceURL, "http://") || strings.HasPrefix(sourceURL, "https://") {
		if git.IsGitRepoURL(sourceURL) {
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
		resp, err := httpClient.Do(req)
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

func readCatalogDirectory[T any](catalog string) ([]T, error) {
	files, usingObotCatalogsFile, err := catalogvalidation.WalkCatalogFiles(catalog)
	if err != nil {
		return nil, fmt.Errorf("failed to walk repository files: %w", err)
	}

	var entries []T
	for path, walkErr := range files {
		if walkErr != nil {
			return nil, fmt.Errorf("failed to walk repository files: %w", walkErr)
		}
		fileEntries, _, err := catalogvalidation.DecodeCatalogFile[T](path, false)
		if err == nil {
			entries = append(entries, fileEntries...)
			continue
		}
		if usingObotCatalogsFile {
			log.Warnf("Failed to parse %s as catalog entry: %v", path, err)
		} else {
			log.Debugf("Failed to parse %s as catalog entry: %v", path, err)
		}
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
			if !server.DeletionTimestamp.IsZero() || !server.Spec.IsSingleUser() {
				// For multi-user servers, we don't need to check them.
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
