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
	"sort"
	"strings"
	"time"

	"github.com/obot-platform/nah/pkg/apply"
	"github.com/obot-platform/nah/pkg/name"
	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/nanobot/pkg/safehttp"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/logger"
	"github.com/obot-platform/obot/pkg/accesscontrolrule"
	gclient "github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/git"
	"github.com/obot-platform/obot/pkg/gitcredential"
	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	kvalidation "k8s.io/apimachinery/pkg/util/validation"
	kuser "k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

var (
	log              = logger.Package()
	invalidNameChars = regexp.MustCompile(`[^a-z0-9-]+`)
	multipleDashes   = regexp.MustCompile(`-{2,}`)
)

// sanitizeName lowercases the input, replaces any characters that are invalid
// for RFC 1123 subdomain names with dashes, collapses consecutive dashes, and
// trims leading/trailing dashes.
func sanitizeName(n string) string {
	n = strings.ToLower(n)
	n = invalidNameChars.ReplaceAllString(n, "-")
	n = multipleDashes.ReplaceAllString(n, "-")
	return strings.Trim(n, "-")
}

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
}

func New(defaultCatalogPath, defaultSystemCatalogPath string, gatewayClient *gclient.Client, accessControlRuleHelper *accesscontrolrule.Helper, mcpSessionManager *mcp.SessionManager) *Handler {
	remoteURLValidationConfig := mcpSessionManager.RemoteMCPURLValidationConfig()
	validationOptions := mcp.ValidationOptions{
		RemoteMCPURLValidationConfig: remoteURLValidationConfig,
	}
	validationOptions.ResourceMaximums = mcpSessionManager.KubernetesResourceMaximums()

	return &Handler{
		defaultCatalogPath:        defaultCatalogPath,
		defaultSystemCatalogPath:  defaultSystemCatalogPath,
		gatewayClient:             gatewayClient,
		httpClient:                safehttp.NewClient(!remoteURLValidationConfig.AllowLocalhostMCP, !remoteURLValidationConfig.AllowPrivateIPMCP, !remoteURLValidationConfig.AllowLinkLocalMCP),
		accessControlRuleHelper:   accessControlRuleHelper,
		remoteURLValidationConfig: validationOptions,
		mcpBackend:                mcpSessionManager.MCPRuntimeBackend(),
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

	failedSources := make(map[string]bool, len(mcpCatalog.Status.SyncErrors))
	for sourceURL := range mcpCatalog.Status.SyncErrors {
		failedSources[sourceURL] = true
	}
	toAdd, err := h.reconcileMCPCatalogVersions(req.Ctx, req.Client, mcpCatalog.Spec.SourceURLs, failedSources, toAdd)
	if err != nil {
		return fmt.Errorf("failed to reconcile MCP catalog versions: %w", err)
	}

	toAdd, compositeRefErrors := h.resolveCompositeSourceRefs(req.Ctx, req.Client, mcpCatalog.Namespace, mcpCatalog.Name, toAdd)
	for sourceURL, errMsg := range compositeRefErrors {
		addSyncError(mcpCatalog.Status.SyncErrors, sourceURL, errMsg)
	}
	if len(compositeRefErrors) > 0 && len(failedSources) == 0 {
		// Reconcile once more in no-prune mode so versions removed from otherwise
		// healthy sources are explicitly made inactive despite composite errors.
		toAdd, err = h.reconcileMCPCatalogVersions(req.Ctx, req.Client, mcpCatalog.Spec.SourceURLs, map[string]bool{"": true}, toAdd)
		if err != nil {
			return fmt.Errorf("failed to reconcile MCP catalog versions after composite errors: %w", err)
		}
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
	// delete a freshly converted entry
	app := apply.New(req.Client).WithOwnerSubContext(fmt.Sprintf("catalog-%s", mcpCatalog.Name)).WithNoPrune()

	// Missing entries cannot be reconciled safely from a partial desired set.
	if len(mcpCatalog.Status.SyncErrors) > 0 {
		log.Infof("Applying MCP catalog entries without reconciling missing entries due to source errors: catalog=%s entries=%d sourceErrors=%d", mcpCatalog.Name, len(toAdd), len(mcpCatalog.Status.SyncErrors))
		if err := app.Apply(req.Ctx, mcpCatalog, toAdd...); err != nil {
			return err
		}
		return updateCatalogLatestVersions(req.Ctx, req.Client, toAdd)
	}

	if err := reconcileRemovedEntries(req.Ctx, req.Client, mcpCatalog, toAdd); err != nil {
		return err
	}

	log.Infof("Applying MCP catalog entries without prune: catalog=%s entries=%d", mcpCatalog.Name, len(toAdd))
	if err := app.Apply(req.Ctx, mcpCatalog, toAdd...); err != nil {
		return err
	}
	return updateCatalogLatestVersions(req.Ctx, req.Client, toAdd)
}

func addSyncError(syncErrors map[string]string, sourceURL, errMsg string) {
	if existing := syncErrors[sourceURL]; existing != "" {
		syncErrors[sourceURL] = existing + "; " + errMsg
	} else {
		syncErrors[sourceURL] = errMsg
	}
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
		if err := convertCatalogEntryToEditable(ctx, c, catalog, entryName); err != nil {
			return fmt.Errorf("failed to convert catalog entry %q to editable: %w", entryName, err)
		}
		log.Infof("Converted removed MCP catalog entry with active servers to editable: catalog=%s entry=%s", catalog.Name, entryName)
	}

	return nil
}

func convertCatalogEntryToEditable(ctx context.Context, c client.Client, catalog *v1.MCPCatalog, entryName string) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		var entry v1.MCPServerCatalogEntry
		if err := c.Get(ctx, client.ObjectKey{Namespace: catalog.Namespace, Name: entryName}, &entry); err != nil {
			if apierrors.IsNotFound(err) {
				return nil
			}
			return err
		}

		entry.Spec.Editable = true
		entry.Spec.SourceURL = ""
		entry.Spec.Manifest.EntryKey = ""
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
func (h *Handler) resolveCompositeSourceRefs(ctx context.Context, c client.Client, namespace, catalogName string, objs []client.Object) ([]client.Object, map[string]string) {
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
		var (
			manifest   *types.MCPServerCatalogEntryManifest
			sourceURL  string
			objectRef  string
			gitManaged bool
		)
		switch entry := obj.(type) {
		case *v1.MCPServerCatalogEntry:
			manifest = &entry.Spec.Manifest
			sourceURL = entry.Spec.SourceURL
			objectRef = fmt.Sprintf("catalog entry %q", entry.Name)
			gitManaged = entry.IsGitManaged()
		case *v1.MCPServerCatalogEntryVersion:
			manifest = &entry.Spec.Manifest
			sourceURL = entry.Spec.SourceURL
			objectRef = fmt.Sprintf("catalog entry version %q", entry.Name)
			gitManaged = true
		default:
			result = append(result, obj)
			continue
		}
		if manifest.Runtime != types.RuntimeComposite || manifest.CompositeConfig == nil {
			result = append(result, obj)
			continue
		}

		if err := h.resolveCompositeManifest(ctx, c, namespace, catalogName, refs, entriesByName, sourceURL, manifest, gitManaged); err != nil {
			addSyncError(errsBySourceURL, sourceURL, fmt.Sprintf("failed to resolve composite %s: %v", objectRef, err))
			continue
		}
		result = append(result, obj)
	}

	return result, errsBySourceURL
}

func (h *Handler) resolveCompositeManifest(ctx context.Context, c client.Client, namespace, catalogName string, refs, entriesByName map[string]*v1.MCPServerCatalogEntry, sourceURL string, manifest *types.MCPServerCatalogEntryManifest, gitManaged bool) error {
	changed := false
	var errs []error
	for i := range manifest.CompositeConfig.ComponentServers {
		component := &manifest.CompositeConfig.ComponentServers[i]
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

		target, err := resolveComponentSourceRef(refs, mcp.SourceIDForURL(sourceURL), component.CatalogEntryID)
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
		return errors.Join(errs...)
	}
	if !changed {
		return nil
	}
	if err := mcp.ValidateCatalogEntryManifest(ctx, *manifest, gitManaged, h.remoteURLValidationConfig); err != nil {
		return fmt.Errorf("failed to validate resolved manifest: %w", err)
	}
	if err := mcp.ValidateSecretBindingsCatalogEntry(*manifest, gitManaged, false, h.mcpBackend); err != nil {
		return fmt.Errorf("failed to validate resolved manifest: %w", err)
	}
	if err := mcp.ValidateTemplateReferencesCatalogEntry(*manifest); err != nil {
		return fmt.Errorf("failed to validate resolved manifest: %w", err)
	}
	return nil
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

		cleanName := sanitizeName(entry.Name)
		if cleanName == "" {
			err := fmt.Errorf("invalid system catalog entry name after sanitization: original=%q sanitized=%q", entry.Name, cleanName)
			errs = append(errs, err)
			continue
		}

		mcpManifest := systemCatalogEntryManifestToMCP(entry)
		sanitizeCatalogEntryManifest(&mcpManifest)
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

type versionedCatalogManifest struct {
	types.MCPServerCatalogEntryManifest `json:",inline"`
	Version                             *int `json:"version,omitempty"`
}

type catalogManifestFamily struct {
	name       string
	cleanName  string
	entryKey   string
	versions   map[int]types.MCPServerCatalogEntryManifest
	invalid    bool
	firstIndex int
}

func (h *Handler) readMCPCatalog(ctx context.Context, catalogName, sourceURL, token string) ([]client.Object, error) {
	var entries []versionedCatalogManifest

	if strings.HasPrefix(sourceURL, "http://") || strings.HasPrefix(sourceURL, "https://") {
		if git.IsGitRepoURL(sourceURL) {
			var err error
			entries, err = readGitCatalogEntries[versionedCatalogManifest](ctx, sourceURL, token)
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
			resp, err := h.httpClient.Do(req)
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
			entries, err = readCatalogDirectory[versionedCatalogManifest](sourceURL)
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

	var errs []error
	families := make(map[string]*catalogManifestFamily)
	entryKeyFamilies := make(map[string]string)
	for i, decoded := range entries {
		entry := decoded.MCPServerCatalogEntryManifest
		if entry.Metadata["categories"] == "Official" {
			delete(entry.Metadata, "categories") // This shouldn't happen, but do this just in case.
			// We don't want to mark random MCP servers from the catalog as official.
		}

		cleanName := sanitizeName(entry.Name)
		if cleanName == "" {
			err := fmt.Errorf("invalid catalog entry name after sanitization: original=%q sanitized=%q", entry.Name, cleanName)
			errs = append(errs, err)
			continue
		}
		catalogEntryName := name.SafeHashConcatName(catalogName, cleanName)

		if entry.EntryKey != "" {
			if strings.Contains(entry.EntryKey, catalogReferenceSeparator) {
				errs = append(errs, fmt.Errorf("source entry key %q cannot contain %s; skipping catalog entry %q", entry.EntryKey, catalogReferenceSeparator, catalogEntryName))
				continue
			}
			if dnsErrs := kvalidation.IsDNS1123Subdomain(entry.EntryKey); len(dnsErrs) > 0 {
				errs = append(errs, fmt.Errorf("source entry key %q must be DNS-friendly: %s; skipping catalog entry %q", entry.EntryKey, strings.Join(dnsErrs, "; "), catalogEntryName))
				continue
			}
		}

		version := 0
		if decoded.Version != nil {
			if *decoded.Version <= 0 {
				errs = append(errs, fmt.Errorf("catalog entry %q explicit version must be positive, got %d", entry.Name, *decoded.Version))
				continue
			}
			version = *decoded.Version
		}

		family := families[cleanName]
		if family == nil {
			family = &catalogManifestFamily{
				name:       entry.Name,
				cleanName:  cleanName,
				entryKey:   entry.EntryKey,
				versions:   make(map[int]types.MCPServerCatalogEntryManifest),
				firstIndex: i,
			}
			families[cleanName] = family
			if entry.EntryKey != "" {
				if otherName, ok := entryKeyFamilies[entry.EntryKey]; ok && otherName != cleanName {
					errs = append(errs, fmt.Errorf("duplicate source entry key %q also used by catalog entry %q", entry.EntryKey, catalogEntryName))
					family.invalid = true
				} else {
					entryKeyFamilies[entry.EntryKey] = cleanName
				}
			}
		} else {
			if family.name != entry.Name {
				errs = append(errs, fmt.Errorf("versions in catalog entry family %q must use exact name %q, got %q", cleanName, family.name, entry.Name))
				family.invalid = true
			}
			if family.entryKey != entry.EntryKey {
				errs = append(errs, fmt.Errorf("versions in catalog entry family %q must use entry key %q, got %q", cleanName, family.entryKey, entry.EntryKey))
				family.invalid = true
			}
		}
		if _, ok := family.versions[version]; ok {
			errs = append(errs, fmt.Errorf("duplicate version %d in catalog entry family %q", version, family.name))
			family.invalid = true
			continue
		}

		sanitizeCatalogEntryManifest(&entry)
		if err := mcp.ValidateCatalogEntryManifest(ctx, entry, true, h.remoteURLValidationConfig); err != nil {
			errs = append(errs, fmt.Errorf("failed to validate catalog entry %s: %w", entry.Name, err))
			continue
		}
		// secretBinding references are only allowed for git-managed entries.
		if err := mcp.ValidateSecretBindingsCatalogEntry(entry, true, false, h.mcpBackend); err != nil {
			errs = append(errs, fmt.Errorf("failed to validate catalog entry %s: %w", entry.Name, err))
			continue
		}
		if err := mcp.ValidateTemplateReferencesCatalogEntry(entry); err != nil {
			errs = append(errs, fmt.Errorf("failed to validate catalog entry %s: %w", entry.Name, err))
			continue
		}
		family.versions[version] = entry
	}

	orderedFamilies := make([]*catalogManifestFamily, 0, len(families))
	for _, family := range families {
		if !family.invalid && len(family.versions) > 0 {
			orderedFamilies = append(orderedFamilies, family)
		}
	}
	sort.Slice(orderedFamilies, func(i, j int) bool { return orderedFamilies[i].firstIndex < orderedFamilies[j].firstIndex })

	objs := make([]client.Object, 0, len(entries)+len(orderedFamilies))
	for _, family := range orderedFamilies {
		versions := make([]int, 0, len(family.versions))
		for version := range family.versions {
			versions = append(versions, version)
		}
		sort.Ints(versions)
		defaultVersion := versions[len(versions)-1]
		catalogEntryName := name.SafeHashConcatName(catalogName, family.cleanName)
		selectedManifest := family.versions[defaultVersion]
		catalogEntry := &v1.MCPServerCatalogEntry{
			ObjectMeta: metav1.ObjectMeta{Name: catalogEntryName, Namespace: system.DefaultNamespace},
			Spec: v1.MCPServerCatalogEntrySpec{
				Manifest:         selectedManifest,
				UnsupportedTools: unsupportedTools(selectedManifest),
				DefaultVersion:   defaultVersion,
				MCPCatalogName:   catalogName,
				SourceURL:        sourceURL,
				Editable:         false,
			},
			Status: v1.MCPServerCatalogEntryStatus{LatestVersion: defaultVersion},
		}
		objs = append(objs, catalogEntry)
		for _, version := range versions {
			manifest := family.versions[version]
			objs = append(objs, &v1.MCPServerCatalogEntryVersion{
				ObjectMeta: metav1.ObjectMeta{
					Name:      v1.MCPServerCatalogEntryVersionName(catalogEntryName, version),
					Namespace: system.DefaultNamespace,
				},
				Spec: v1.MCPServerCatalogEntryVersionSpec{
					MCPServerCatalogEntryName: catalogEntryName,
					Version:                   version,
					Manifest:                  manifest,
					UnsupportedTools:          unsupportedTools(manifest),
					SourceURL:                 sourceURL,
					Active:                    true,
				},
			})
		}
	}
	return objs, errors.Join(errs...)
}

func unsupportedTools(manifest types.MCPServerCatalogEntryManifest) []string {
	if manifest.Metadata["unsupportedTools"] == "" {
		return nil
	}
	return strings.Split(manifest.Metadata["unsupportedTools"], ",")
}

func (h *Handler) reconcileMCPCatalogVersions(ctx context.Context, c client.Client, sourceURLs []string, failedSources map[string]bool, objs []client.Object) ([]client.Object, error) {
	type family struct {
		parent   *v1.MCPServerCatalogEntry
		versions []*v1.MCPServerCatalogEntryVersion
	}

	families := make(map[string]*family)
	var order []string
	var others []client.Object
	for _, obj := range objs {
		switch obj := obj.(type) {
		case *v1.MCPServerCatalogEntry:
			if previous := slices.Index(order, obj.Name); previous >= 0 {
				order = slices.Delete(order, previous, previous+1)
			}
			order = append(order, obj.Name)
			families[obj.Name] = &family{parent: obj}
		case *v1.MCPServerCatalogEntryVersion:
			if current := families[obj.Spec.MCPServerCatalogEntryName]; current != nil && current.parent.Spec.SourceURL == obj.Spec.SourceURL {
				current.versions = append(current.versions, obj)
			}
		default:
			others = append(others, obj)
		}
	}

	var existingVersions v1.MCPServerCatalogEntryVersionList
	var servers v1.MCPServerList
	if c != nil {
		if err := c.List(ctx, &existingVersions, client.InNamespace(system.DefaultNamespace)); err != nil {
			return nil, fmt.Errorf("failed to list existing catalog entry versions: %w", err)
		}
		if err := c.List(ctx, &servers, client.InNamespace(system.DefaultNamespace)); err != nil {
			return nil, fmt.Errorf("failed to list MCP servers for catalog versions: %w", err)
		}
	}
	referenced := make(map[string]map[int]bool)
	for _, server := range servers.Items {
		if referenced[server.Spec.MCPServerCatalogEntryName] == nil {
			referenced[server.Spec.MCPServerCatalogEntryName] = make(map[int]bool)
		}
		referenced[server.Spec.MCPServerCatalogEntryName][server.Spec.MCPServerCatalogEntryVersion] = true
	}

	result := append([]client.Object(nil), others...)
	sourcePriority := make(map[string]int, len(sourceURLs))
	for i, sourceURL := range sourceURLs {
		sourcePriority[sourceURL] = i
	}
	for _, entryName := range order {
		family := families[entryName]
		active := make(map[int]*v1.MCPServerCatalogEntryVersion, len(family.versions))
		for _, version := range family.versions {
			active[version.Spec.Version] = version
		}

		var existingParent *v1.MCPServerCatalogEntry
		if c != nil {
			var existing v1.MCPServerCatalogEntry
			if err := c.Get(ctx, client.ObjectKey{Namespace: family.parent.Namespace, Name: family.parent.Name}, &existing); err == nil {
				existingParent = &existing
				currentPriority, currentKnown := sourcePriority[family.parent.Spec.SourceURL]
				existingPriority, existingKnown := sourcePriority[existing.Spec.SourceURL]
				if existing.Spec.SourceURL != family.parent.Spec.SourceURL && failedSources[existing.Spec.SourceURL] &&
					existingKnown && (!currentKnown || existingPriority > currentPriority) {
					result = append(result, existing.DeepCopy())
					for i := range existingVersions.Items {
						if existingVersions.Items[i].Spec.MCPServerCatalogEntryName == existing.Name {
							result = append(result, existingVersions.Items[i].DeepCopy())
						}
					}
					continue
				}
				if active[existing.Spec.DefaultVersion] != nil {
					family.parent.Spec.DefaultVersion = existing.Spec.DefaultVersion
				}
			} else if !apierrors.IsNotFound(err) {
				return nil, fmt.Errorf("failed to get existing catalog entry %q: %w", family.parent.Name, err)
			}
		}

		for i := range existingVersions.Items {
			existing := &existingVersions.Items[i]
			if existing.Spec.MCPServerCatalogEntryName != family.parent.Name || active[existing.Spec.Version] != nil {
				continue
			}
			retained := existing.DeepCopy()
			if existingParent == nil || !failedSources[family.parent.Spec.SourceURL] {
				retained.Spec.Active = false
			}
			if !referenced[family.parent.Name][existing.Spec.Version] && len(failedSources) == 0 {
				continue
			}
			family.versions = append(family.versions, retained)
		}
		sort.Slice(family.versions, func(i, j int) bool { return family.versions[i].Spec.Version < family.versions[j].Spec.Version })

		selected := active[family.parent.Spec.DefaultVersion]
		if selected == nil {
			return nil, fmt.Errorf("catalog entry %q default version %d is not active", family.parent.Name, family.parent.Spec.DefaultVersion)
		}
		family.parent.Spec.Manifest = selected.Spec.Manifest
		family.parent.Spec.UnsupportedTools = slices.Clone(selected.Spec.UnsupportedTools)
		latest := 0
		for version := range active {
			latest = max(latest, version)
		}
		family.parent.Status.LatestVersion = latest
		result = append(result, family.parent)
		for _, version := range family.versions {
			result = append(result, version)
		}
	}
	return result, nil
}

func updateCatalogLatestVersions(ctx context.Context, c client.Client, objs []client.Object) error {
	for _, obj := range objs {
		desired, ok := obj.(*v1.MCPServerCatalogEntry)
		if !ok {
			continue
		}
		var stored v1.MCPServerCatalogEntry
		if err := c.Get(ctx, client.ObjectKeyFromObject(desired), &stored); err != nil {
			return fmt.Errorf("failed to get catalog entry %q after apply: %w", desired.Name, err)
		}
		if stored.Status.LatestVersion == desired.Status.LatestVersion {
			continue
		}
		stored.Status.LatestVersion = desired.Status.LatestVersion
		if err := c.Status().Update(ctx, &stored); err != nil {
			return fmt.Errorf("failed to update latest version for catalog entry %q: %w", desired.Name, err)
		}
	}
	return nil
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

	if entry.ServerUserType == "" {
		entry.ServerUserType = types.ServerUserTypeSingleUser
	}
}

// isPathSafe checks if a file path is safe to read (not a symlink and within bounds).
func isPathSafe(path, baseDir string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("failed to get file info: %w", err)
	}

	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("symbolic links are not allowed for security reasons")
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %w", err)
	}

	absBaseDir, err := filepath.Abs(baseDir)
	if err != nil {
		return fmt.Errorf("failed to get absolute base directory: %w", err)
	}

	if !strings.HasPrefix(absPath, absBaseDir+string(filepath.Separator)) {
		return fmt.Errorf("file path is outside the allowed directory")
	}

	return nil
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

		// Check file count limit
		fileCount++
		if fileCount > maxFiles {
			return fmt.Errorf("too many files to process (limit: %d)", maxFiles)
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
