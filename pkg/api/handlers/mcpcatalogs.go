package handlers

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/obot-platform/nah/pkg/name"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/accesscontrolrule"
	"github.com/obot-platform/obot/pkg/api"
	mcpcataloghandler "github.com/obot-platform/obot/pkg/controller/handlers/mcpcatalog"
	gclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/obot-platform/obot/pkg/tunnel"
	"github.com/obot-platform/obot/pkg/utils"
	vmcpconfig "github.com/obot-platform/obot/pkg/vmcp"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	dnsLabelRegex = regexp.MustCompile("[^a-z0-9-]+")
)

type MCPCatalogHandler struct {
	defaultCatalogPath        string
	serverURL                 string
	mcpBackend                string
	sessionManager            *mcp.SessionManager
	capacityInfoProvider      capacityInfoProvider
	oauthChecker              MCPOAuthChecker
	gatewayClient             *gclient.Client
	acrHelper                 *accesscontrolrule.Helper
	secretBindingAllowedLabel string
}

type capacityInfoProvider interface {
	GetCapacityInfoForServers(context.Context, []string) (types.MCPCapacityInfo, error)
}

func NewMCPCatalogHandler(defaultCatalogPath string, serverURL string, mcpBackend string, sessionManager *mcp.SessionManager, oauthChecker MCPOAuthChecker, gatewayClient *gclient.Client, acrHelper *accesscontrolrule.Helper, secretBindingAllowedLabel string) *MCPCatalogHandler {
	return &MCPCatalogHandler{
		defaultCatalogPath:        defaultCatalogPath,
		serverURL:                 serverURL,
		mcpBackend:                mcpBackend,
		sessionManager:            sessionManager,
		capacityInfoProvider:      sessionManager,
		oauthChecker:              oauthChecker,
		gatewayClient:             gatewayClient,
		acrHelper:                 acrHelper,
		secretBindingAllowedLabel: secretBindingAllowedLabel,
	}
}

// List returns all catalogs.
func (*MCPCatalogHandler) List(req api.Context) error {
	var list v1.MCPCatalogList
	if err := req.List(&list); err != nil {
		return fmt.Errorf("failed to list catalogs: %w", err)
	}

	var items []types.MCPCatalog
	for _, item := range list.Items {
		tokenEnv, err := revealCatalogTokens(req, item.Name)
		if err != nil {
			return err
		}
		items = append(items, convertMCPCatalog(item, tokenEnv))
	}

	return req.Write(types.MCPCatalogList{
		Items: items,
	})
}

// validateEntryScope checks that a catalog entry strictly belongs to the scope indicated by the request path.
// Use this for mutating operations (update, delete) where cross-scope access should not be allowed.
func validateEntryScope(entry v1.MCPServerCatalogEntry, catalogName, workspaceID string) error {
	if catalogName != "" && entry.Spec.MCPCatalogName != catalogName {
		return types.NewErrBadRequest("entry does not belong to catalog")
	}
	if workspaceID != "" && entry.Spec.PowerUserWorkspaceID != workspaceID {
		return types.NewErrBadRequest("entry does not belong to workspace")
	}
	return nil
}

// validateEntryVisibleFromScope checks that a catalog entry is visible from the scope indicated by the request path.
// Unlike validateEntryScope, workspace-scoped routes also allow global catalog entries (MCPCatalogName set)
// so PowerUserPlus users can read and deploy servers from global catalog entries into their workspace.
func validateEntryVisibleFromScope(entry v1.MCPServerCatalogEntry, catalogName, workspaceID string) error {
	if catalogName != "" && entry.Spec.MCPCatalogName != catalogName {
		return types.NewErrBadRequest("entry does not belong to catalog")
	}
	if workspaceID != "" && entry.Spec.PowerUserWorkspaceID != workspaceID && entry.Spec.MCPCatalogName == "" {
		return types.NewErrBadRequest("entry does not belong to workspace")
	}
	return nil
}

// Get returns a specific catalog by ID.
func (*MCPCatalogHandler) Get(req api.Context) error {
	var catalog v1.MCPCatalog
	if err := req.Get(&catalog, req.PathValue("catalog_id")); err != nil {
		return fmt.Errorf("failed to get catalog: %w", err)
	}
	tokenEnv, err := revealCatalogTokens(req, catalog.Name)
	if err != nil {
		return err
	}
	return req.Write(convertMCPCatalog(catalog, tokenEnv))
}

// Refresh refreshes a catalog to sync its entries.
func (h *MCPCatalogHandler) Refresh(req api.Context) error {
	catalogName := req.PathValue("catalog_id")

	var catalog v1.MCPCatalog
	if err := req.Get(&catalog, catalogName); err != nil {
		return fmt.Errorf("failed to get catalog: %w", err)
	}

	if catalog.Annotations == nil {
		catalog.Annotations = make(map[string]string)
	}
	catalog.Annotations[v1.MCPCatalogSyncAnnotation] = "true"

	return req.Update(&catalog)
}

// Update updates a catalog (admin only, default catalog only).
func (h *MCPCatalogHandler) Update(req api.Context) error {
	var manifest types.MCPCatalogManifest
	if err := req.Read(&manifest); err != nil {
		return fmt.Errorf("failed to read catalog manifest: %w", err)
	}

	catalogID := req.PathValue("catalog_id")
	if catalogID != system.DefaultCatalog {
		return types.NewErrBadRequest("only the default catalog can be updated")
	}

	var catalog v1.MCPCatalog
	if err := req.Get(&catalog, catalogID); err != nil {
		return fmt.Errorf("failed to get catalog: %w", err)
	}

	originalSourceURLs := slices.Clone(manifest.SourceURLs)
	if err := normalizeAndValidateCatalogSourceURLs(manifest.SourceURLs, h.defaultCatalogPath); err != nil {
		return err
	}
	remapCatalogSourceValues(originalSourceURLs, manifest.SourceURLs, manifest.SourceURLCredentials)
	remapCatalogSourceValues(originalSourceURLs, manifest.SourceURLs, manifest.SourceURLGitCredentialIDs)
	if err := validateCatalogGitCredentials(req, manifest.SourceURLs, manifest.SourceURLGitCredentialIDs); err != nil {
		return err
	}

	// Reveal the existing single credential that holds all source-URL tokens.
	existingCred, err := req.GatewayClient.RevealCredential(req.Context(), []string{catalog.Name}, mcpcataloghandler.CatalogCredentialToolName)
	if err != nil && !errors.As(err, &gclient.CredentialNotFoundError{}) {
		return fmt.Errorf("failed to reveal catalog credentials: %w", err)
	}

	newTokens := mergeCatalogTokens(manifest.SourceURLs, manifest.SourceURLCredentials, existingCred.Secrets)
	removeSharedCredentialTokens(newTokens, manifest.SourceURLGitCredentialIDs)

	catalog.Spec.SourceURLs = manifest.SourceURLs
	catalog.Spec.SourceURLGitCredentialIDs = manifest.SourceURLGitCredentialIDs

	if err := req.Update(&catalog); err != nil {
		return fmt.Errorf("failed to update catalog: %w", err)
	}

	if err := storeCatalogTokens(req, catalog.Name, newTokens, existingCred.Secrets); err != nil {
		return err
	}

	return req.Write(convertMCPCatalog(catalog, newTokens))
}

// ListEntries lists all entries for a catalog or workspace.
func (h *MCPCatalogHandler) ListEntries(req api.Context) error {
	catalogName := req.PathValue("catalog_id")
	workspaceID := req.PathValue("workspace_id")
	minimal, _ := strconv.ParseBool(req.URL.Query().Get("minimal"))
	var powerUserID string

	// Verify the scope exists
	if catalogName != "" {
		if err := req.Get(&v1.MCPCatalog{}, catalogName); err != nil {
			return fmt.Errorf("failed to get catalog: %w", err)
		}
	} else if workspaceID != "" {
		var workspace v1.PowerUserWorkspace
		if err := req.Get(&workspace, workspaceID); err != nil {
			return fmt.Errorf("failed to get workspace: %w", err)
		}
		powerUserID = workspace.Spec.UserID
	} else {
		return types.NewErrBadRequest("either catalog_id or workspace_id is required")
	}

	var fieldSelector kclient.MatchingFields
	if catalogName != "" {
		fieldSelector = kclient.MatchingFields{"spec.mcpCatalogName": catalogName}
	} else if workspaceID != "" {
		fieldSelector = kclient.MatchingFields{"spec.powerUserWorkspaceID": workspaceID}
	} else {
		return types.NewErrBadRequest("either catalog_id or workspace_id is required")
	}

	var list v1.MCPServerCatalogEntryList
	if err := req.List(&list, fieldSelector); err != nil {
		return fmt.Errorf("failed to list entries: %w", err)
	}

	// Allow admins/auditors to bypass ACR filtering with ?all=true
	if (req.UserIsAdmin() || req.UserIsAuditor()) && req.URL.Query().Get("all") == "true" {
		entries := make([]types.MCPServerCatalogEntry, 0, len(list.Items))
		for _, entry := range list.Items {
			entries = append(entries, convertMCPServerCatalogEntryForList(entry, workspaceID, powerUserID, h.serverURL, minimal))
		}
		return req.Write(types.MCPServerCatalogEntryList{Items: entries})
	}

	// Apply ACR filtering for regular users and for admins without ?all=true
	entries := make([]types.MCPServerCatalogEntry, 0, len(list.Items))
	for _, entry := range list.Items {
		var (
			err       error
			hasAccess bool
		)

		// Check default catalog entries
		if entry.Spec.MCPCatalogName != "" {
			hasAccess, err = h.acrHelper.UserHasAccessToMCPServerCatalogEntryInCatalog(req.User, entry.Name, entry.Spec.MCPCatalogName)
		} else if entry.Spec.PowerUserWorkspaceID != "" {
			// Check workspace-scoped entries
			hasAccess, err = h.acrHelper.UserHasAccessToMCPServerCatalogEntryInWorkspace(req.Context(), req.User, entry.Name, entry.Spec.PowerUserWorkspaceID)
		}
		if err != nil {
			return err
		}

		if hasAccess {
			// Hide catalog entries that require OAuth credentials that haven't been configured (non-admins only).
			if !req.UserIsAdmin() && entryRequiresStaticOAuthCreds(entry) {
				continue
			}
			entries = append(entries, convertMCPServerCatalogEntryForList(entry, workspaceID, powerUserID, h.serverURL, minimal))
		}
	}

	return req.Write(types.MCPServerCatalogEntryList{Items: entries})
}

// GetEntry returns a specific entry from a catalog or workspace.
func (h *MCPCatalogHandler) GetEntry(req api.Context) error {
	catalogName := req.PathValue("catalog_id")
	workspaceID := req.PathValue("workspace_id")
	entryName := req.PathValue("entry_id")

	// Verify the scope exists
	if catalogName != "" {
		if err := req.Get(&v1.MCPCatalog{}, catalogName); err != nil {
			return fmt.Errorf("failed to get catalog: %w", err)
		}
	} else if workspaceID != "" {
		if err := req.Get(&v1.PowerUserWorkspace{}, workspaceID); err != nil {
			return fmt.Errorf("failed to get workspace: %w", err)
		}
	} else {
		return types.NewErrBadRequest("either catalog_id or workspace_id is required")
	}

	var entry v1.MCPServerCatalogEntry
	if err := req.Get(&entry, entryName); err != nil {
		return fmt.Errorf("failed to get entry: %w", err)
	}

	if err := validateEntryVisibleFromScope(entry, catalogName, workspaceID); err != nil {
		return err
	}

	// For workspace entries, include powerUserId in the response
	if workspaceID != "" {
		var workspace v1.PowerUserWorkspace
		if err := req.Get(&workspace, workspaceID); err != nil {
			return fmt.Errorf("failed to get workspace for powerUserId: %w", err)
		}
		return req.Write(ConvertMCPServerCatalogEntryWithWorkspace(entry, workspaceID, workspace.Spec.UserID, h.serverURL))
	}

	return req.Write(ConvertMCPServerCatalogEntry(entry, h.serverURL))
}

// CreateEntry creates a new entry for a catalog or workspace.
func (h *MCPCatalogHandler) CreateEntry(req api.Context) error {
	catalogName := req.PathValue("catalog_id")
	workspaceID := req.PathValue("workspace_id")

	// Verify the scope exists
	if catalogName != "" {
		if err := req.Get(&v1.MCPCatalog{}, catalogName); err != nil {
			return fmt.Errorf("failed to get catalog: %w", err)
		}
	} else if workspaceID != "" {
		if err := req.Get(&v1.PowerUserWorkspace{}, workspaceID); err != nil {
			return fmt.Errorf("failed to get workspace: %w", err)
		}
	} else {
		return types.NewErrBadRequest("either catalog_id or workspace_id is required")
	}

	var manifest types.MCPServerCatalogEntryManifest
	if err := req.Read(&manifest); err != nil {
		return types.NewErrBadRequest("failed to read entry manifest: %v", err)
	}
	if err := validateCatalogEntryManifestWithResourceMaximums(req, manifest, false, h.sessionManager); err != nil {
		return types.NewErrBadRequest("failed to validate entry manifest: %v", err)
	}
	if err := tunnel.ValidateCatalogEntryTunnelReferences(req.Context(), req.Storage, manifest); err != nil {
		return types.NewErrBadRequest("failed to validate entry manifest: %v", err)
	}
	// UI-created catalog entries are never git-managed, but multi-user catalog
	// entries may still define secretBinding as part of their shared template.
	if err := mcp.ValidateSecretBindingsCatalogEntry(manifest, false, req.UserIsAdmin(), h.mcpBackend); err != nil {
		return types.NewErrBadRequest("failed to validate entry manifest: %v", err)
	}
	if err := mcp.ValidateTemplateReferencesCatalogEntry(manifest); err != nil {
		return types.NewErrBadRequest("failed to validate entry manifest: %v", err)
	}

	cleanName := normalizeMCPCatalogEntryName(manifest.Name)

	entry := v1.MCPServerCatalogEntry{
		Namespace: req.Namespace(),
		Spec: v1.MCPServerCatalogEntrySpec{
			Editable: true,
			Manifest: manifest,
			// TODO(g-linville): add support for unsupportedTools field?
		},
	}

	// Set scope-specific fields
	if catalogName != "" {
		entry.GenerateName = name.SafeHashConcatName(catalogName, cleanName)
		entry.Spec.MCPCatalogName = catalogName
	} else {
		entry.GenerateName = name.SafeHashConcatName(workspaceID, cleanName)
		entry.Spec.PowerUserWorkspaceID = workspaceID
	}

	if err := req.Create(&entry); err != nil {
		return fmt.Errorf("failed to create entry: %w", err)
	}

	return req.Write(ConvertMCPServerCatalogEntry(entry, h.serverURL))
}

func (h *MCPCatalogHandler) UpdateEntry(req api.Context) error {
	catalogName := req.PathValue("catalog_id")
	workspaceID := req.PathValue("workspace_id")
	entryName := req.PathValue("entry_id")

	// Verify the scope exists
	if catalogName != "" {
		if err := req.Get(&v1.MCPCatalog{}, catalogName); err != nil {
			return fmt.Errorf("failed to get catalog: %w", err)
		}
	} else if workspaceID != "" {
		if err := req.Get(&v1.PowerUserWorkspace{}, workspaceID); err != nil {
			return fmt.Errorf("failed to get workspace: %w", err)
		}
	} else {
		return types.NewErrBadRequest("either catalog_id or workspace_id is required")
	}

	var entry v1.MCPServerCatalogEntry
	if err := req.Get(&entry, entryName); err != nil {
		return fmt.Errorf("failed to get entry: %w", err)
	}

	if err := validateEntryScope(entry, catalogName, workspaceID); err != nil {
		return err
	}

	if !entry.Spec.Editable {
		return types.NewErrBadRequest("entry is not editable")
	}

	var manifest types.MCPServerCatalogEntryManifest
	if err := req.Read(&manifest); err != nil {
		return types.NewErrBadRequest("failed to read entry manifest: %v", err)
	}

	if err := validateCatalogEntryManifestWithResourceMaximums(req, manifest, false, h.sessionManager); err != nil {
		return types.NewErrBadRequest("failed to validate entry manifest: %v", err)
	}
	if err := tunnel.ValidateCatalogEntryTunnelReferences(req.Context(), req.Storage, manifest); err != nil {
		return types.NewErrBadRequest("failed to validate entry manifest: %v", err)
	}
	// UI-updated catalog entries are never git-managed at this call site. The
	// git-sync controller reconciles git-managed entries through a separate path.
	// Multi-user catalog entries may still define secretBinding as part of their
	// shared template.
	if err := mcp.ValidateSecretBindingsCatalogEntry(manifest, false, req.UserIsAdmin(), h.mcpBackend); err != nil {
		return types.NewErrBadRequest("failed to validate entry manifest: %v", err)
	}
	if err := mcp.ValidateTemplateReferencesCatalogEntry(manifest); err != nil {
		return types.NewErrBadRequest("failed to validate entry manifest: %v", err)
	}

	// Copy the tool previews over so that they don't get wiped out when updating the manifest
	manifest.ToolPreview = entry.Spec.Manifest.ToolPreview

	// Update the manifest
	entry.Spec.Manifest = manifest

	if err := req.Update(&entry); err != nil {
		return fmt.Errorf("failed to update entry: %w", err)
	}

	return req.Write(ConvertMCPServerCatalogEntry(entry, h.serverURL))
}

func (h *MCPCatalogHandler) AcceptEntryOwnership(req api.Context) error {
	catalogName := req.PathValue("catalog_id")
	entryName := req.PathValue("entry_id")

	if err := req.Get(&v1.MCPCatalog{}, catalogName); err != nil {
		return fmt.Errorf("failed to get catalog: %w", err)
	}

	var entry v1.MCPServerCatalogEntry
	if err := req.Get(&entry, entryName); err != nil {
		return fmt.Errorf("failed to get entry: %w", err)
	}
	if err := validateEntryScope(entry, catalogName, ""); err != nil {
		return err
	}
	if !entry.Spec.Detached {
		return types.NewErrBadRequest("entry is not detached")
	}

	acceptCatalogEntryOwnership(&entry)
	if err := req.Update(&entry); err != nil {
		return fmt.Errorf("failed to accept ownership of entry: %w", err)
	}

	return req.Write(ConvertMCPServerCatalogEntry(entry, h.serverURL))
}

func acceptCatalogEntryOwnership(entry *v1.MCPServerCatalogEntry) {
	entry.Spec.Editable = true
	entry.Spec.Detached = false
	entry.Spec.SourceURL = ""
	entry.Spec.Manifest.EntryKey = ""
	entry.Spec.Manifest.UpgradeNote = ""
}

func (h *MCPCatalogHandler) DeleteEntry(req api.Context) error {
	catalogName := req.PathValue("catalog_id")
	workspaceID := req.PathValue("workspace_id")
	entryName := req.PathValue("entry_id")

	// Verify the scope exists
	if catalogName != "" {
		if err := req.Get(&v1.MCPCatalog{}, catalogName); err != nil {
			return fmt.Errorf("failed to get catalog: %w", err)
		}
	} else if workspaceID != "" {
		if err := req.Get(&v1.PowerUserWorkspace{}, workspaceID); err != nil {
			return fmt.Errorf("failed to get workspace: %w", err)
		}
	} else {
		return types.NewErrBadRequest("either catalog_id or workspace_id is required")
	}

	var entry v1.MCPServerCatalogEntry
	if err := req.Get(&entry, entryName); err != nil {
		return fmt.Errorf("failed to get entry: %w", err)
	}

	if err := validateEntryScope(entry, catalogName, workspaceID); err != nil {
		return err
	}

	if !entry.Spec.Editable {
		return types.NewErrBadRequest("entry is not editable and cannot be manually deleted")
	}

	if err := req.Delete(&entry); err != nil {
		return fmt.Errorf("failed to delete entry: %w", err)
	}

	return nil
}

func (h *MCPCatalogHandler) AdminListServersForEntryInCatalog(req api.Context) error {
	catalogName := req.PathValue("catalog_id")
	entryName := req.PathValue("entry_id")

	var catalog v1.MCPCatalog
	if err := req.Get(&catalog, catalogName); err != nil {
		return fmt.Errorf("failed to get catalog: %w", err)
	}

	var entry v1.MCPServerCatalogEntry
	if err := req.Get(&entry, entryName); err != nil {
		return fmt.Errorf("failed to get entry: %w", err)
	}

	if entry.Spec.MCPCatalogName != catalogName {
		return types.NewErrBadRequest("entry does not belong to catalog")
	}

	var list v1.MCPServerList
	if err := req.List(&list, kclient.MatchingFields{
		"spec.mcpServerCatalogEntryName": entryName,
	}); err != nil {
		return fmt.Errorf("failed to list servers: %w", err)
	}

	var items []types.MCPServer
	for _, server := range list.Items {
		if server.Spec.Template {
			// Hide template servers
			continue
		}

		cred, err := req.GatewayClient.RevealCredential(req.Context(), []string{server.CredentialContext(server.Spec.UserID)}, server.Name)
		if err != nil && !errors.As(err, &gclient.CredentialNotFoundError{}) {
			return fmt.Errorf("failed to find credential: %w", err)
		}

		mergedEnv, err := mcp.MergeBoundCreds(req.Context(), req.LocalK8sClient, req.ObotNamespace, server.Spec.Manifest.Config, cred.Secrets, h.secretBindingAllowedLabel)
		if err != nil {
			return fmt.Errorf("failed to resolve secret bindings: %w", err)
		}

		slug, err := SlugForMCPServer(req.Context(), req.Storage, server, server.Spec.UserID, catalogName, "")
		if err != nil {
			return fmt.Errorf("failed to generate slug: %w", err)
		}

		items = append(items, ConvertMCPServer(server, mergedEnv, h.serverURL, slug))
	}

	return req.Write(types.MCPServerList{Items: items})
}

// GetEntryCapacity returns MCP capacity info for deployments tied to a catalog entry.
func (h *MCPCatalogHandler) GetEntryCapacity(req api.Context) error {
	catalogName := req.PathValue("catalog_id")
	entryName := req.PathValue("entry_id")

	var catalog v1.MCPCatalog
	if err := req.Get(&catalog, catalogName); err != nil {
		return fmt.Errorf("failed to get catalog: %w", err)
	}

	var entry v1.MCPServerCatalogEntry
	if err := req.Get(&entry, entryName); err != nil {
		return fmt.Errorf("failed to get entry: %w", err)
	}

	if entry.Spec.MCPCatalogName != catalogName {
		return types.NewErrBadRequest("entry does not belong to catalog")
	}
	if entry.Spec.Manifest.Runtime == types.RuntimeRemote || entry.Spec.Manifest.Runtime == types.RuntimeComposite {
		return types.NewErrBadRequest("capacity is only supported for hosted catalog entries")
	}

	var list v1.MCPServerList
	if err := req.List(&list, kclient.MatchingFields{
		"spec.mcpServerCatalogEntryName": entryName,
	}); err != nil {
		return fmt.Errorf("failed to list servers: %w", err)
	}

	serverNames := make([]string, 0, len(list.Items))
	for _, server := range list.Items {
		if server.Spec.Template {
			continue
		}
		serverNames = append(serverNames, server.Name)
	}

	info, err := h.capacityInfoProvider.GetCapacityInfoForServers(req.Context(), serverNames)
	if err != nil {
		if nse, ok := errors.AsType[*mcp.ErrNotSupportedByBackend](err); ok {
			return types.NewErrBadRequest("%s", nse.Error())
		}
		return err
	}

	return req.Write(info)
}

// AdminListServersForAllEntriesInCatalog returns all servers for all entries in a catalog.
func (h *MCPCatalogHandler) AdminListServersForAllEntriesInCatalog(req api.Context) error {
	catalogName := req.PathValue("catalog_id")

	var catalog v1.MCPCatalog
	if err := req.Get(&catalog, catalogName); err != nil {
		return fmt.Errorf("failed to get catalog: %w", err)
	}

	// Get all entries in the catalog using field selector
	var entriesList v1.MCPServerCatalogEntryList
	if err := req.List(&entriesList, kclient.MatchingFields{
		"spec.mcpCatalogName": catalogName,
	}); err != nil {
		return fmt.Errorf("failed to list entries: %w", err)
	}

	catalogEntries := entriesList.Items

	// For each entry, get its servers using the same approach as AdminListServersForEntryInCatalog
	var allServers []v1.MCPServer
	for _, entry := range catalogEntries {
		var serverList v1.MCPServerList
		if err := req.List(&serverList, kclient.MatchingFields{
			"spec.mcpServerCatalogEntryName": entry.Name,
		}); err != nil {
			return fmt.Errorf("failed to list servers for entry %s: %w", entry.Name, err)
		}
		allServers = append(allServers, serverList.Items...)
	}

	// Filter out template servers and servers in workspaces
	var filteredServers []v1.MCPServer
	for _, server := range allServers {
		if server.Spec.Template || server.Spec.PowerUserWorkspaceID != "" {
			// Hide template servers and servers in workspaces.
			// Servers in workspaces should not be possible,
			// unless somehow someone (like an admin) created one from
			// an entry in the default catalog.
			// Though the UI does not expose the ability to do this,
			// nor would ordinary users have the authz rules to allow them to.
			continue
		}

		filteredServers = append(filteredServers, server)
	}

	var items []types.MCPServer
	for _, server := range filteredServers {
		cred, err := req.GatewayClient.RevealCredential(req.Context(), []string{server.CredentialContext(server.Spec.UserID)}, server.Name)
		if err != nil && !errors.As(err, &gclient.CredentialNotFoundError{}) {
			return fmt.Errorf("failed to find credential: %w", err)
		}

		mergedEnv, err := mcp.MergeBoundCreds(req.Context(), req.LocalK8sClient, req.ObotNamespace, server.Spec.Manifest.Config, cred.Secrets, h.secretBindingAllowedLabel)
		if err != nil {
			return fmt.Errorf("failed to resolve secret bindings: %w", err)
		}

		slug, err := SlugForMCPServer(req.Context(), req.Storage, server, server.Spec.UserID, catalogName, "")
		if err != nil {
			return fmt.Errorf("failed to generate slug: %w", err)
		}

		items = append(items, ConvertMCPServer(server, mergedEnv, h.serverURL, slug))
	}

	return req.Write(types.MCPServerList{Items: items})
}

// ListServersForEntry returns a specific entry from a catalog or workspace.
func (h *MCPCatalogHandler) ListServersForEntry(req api.Context) error {
	catalogName := req.PathValue("catalog_id")
	workspaceID := req.PathValue("workspace_id")
	entryName := req.PathValue("entry_id")

	// Verify the scope exists
	if catalogName != "" {
		if err := req.Get(&v1.MCPCatalog{}, catalogName); err != nil {
			return fmt.Errorf("failed to get catalog: %w", err)
		}
	} else if workspaceID != "" {
		if err := req.Get(&v1.PowerUserWorkspace{}, workspaceID); err != nil {
			return fmt.Errorf("failed to get workspace: %w", err)
		}
	} else {
		return types.NewErrBadRequest("either catalog_id or workspace_id is required")
	}

	var entry v1.MCPServerCatalogEntry
	if err := req.Get(&entry, entryName); err != nil {
		return fmt.Errorf("failed to get entry: %w", err)
	}

	if err := validateEntryVisibleFromScope(entry, catalogName, workspaceID); err != nil {
		return err
	}

	var list v1.MCPServerList
	if err := req.List(&list, kclient.MatchingFields{
		"spec.mcpServerCatalogEntryName": entryName,
	}); err != nil {
		return fmt.Errorf("failed to list servers: %w", err)
	}

	var items []types.MCPServer
	for _, server := range list.Items {
		if server.Spec.Template {
			// Hide template servers
			continue
		}

		cred, err := req.GatewayClient.RevealCredential(req.Context(), []string{server.CredentialContext(server.Spec.UserID)}, server.Name)
		if err != nil && !errors.As(err, &gclient.CredentialNotFoundError{}) {
			return fmt.Errorf("failed to find credential: %w", err)
		}

		mergedEnv, err := mcp.MergeBoundCreds(req.Context(), req.LocalK8sClient, req.ObotNamespace, server.Spec.Manifest.Config, cred.Secrets, h.secretBindingAllowedLabel)
		if err != nil {
			return fmt.Errorf("failed to resolve secret bindings: %w", err)
		}

		slug, err := SlugForMCPServer(req.Context(), req.Storage, server, server.Spec.UserID, catalogName, "")
		if err != nil {
			return fmt.Errorf("failed to generate slug: %w", err)
		}

		items = append(items, ConvertMCPServer(server, mergedEnv, h.serverURL, slug))
	}

	return req.Write(types.MCPServerList{Items: items})
}

// GetServerFromEntry returns a specific entry from a catalog or workspace.
func (h *MCPCatalogHandler) GetServerFromEntry(req api.Context) error {
	catalogName := req.PathValue("catalog_id")
	workspaceID := req.PathValue("workspace_id")
	entryName := req.PathValue("entry_id")

	// Verify the scope exists
	if catalogName != "" {
		if err := req.Get(&v1.MCPCatalog{}, catalogName); err != nil {
			return fmt.Errorf("failed to get catalog: %w", err)
		}
	} else if workspaceID != "" {
		if err := req.Get(&v1.PowerUserWorkspace{}, workspaceID); err != nil {
			return fmt.Errorf("failed to get workspace: %w", err)
		}
	} else {
		return types.NewErrBadRequest("either catalog_id or workspace_id is required")
	}

	var entry v1.MCPServerCatalogEntry
	if err := req.Get(&entry, entryName); err != nil {
		return fmt.Errorf("failed to get entry: %w", err)
	}

	if err := validateEntryVisibleFromScope(entry, catalogName, workspaceID); err != nil {
		return err
	}

	var server v1.MCPServer
	if err := req.Get(&server, req.PathValue("mcp_server_id")); err != nil {
		return fmt.Errorf("failed to list servers: %w", err)
	}

	cred, err := req.GatewayClient.RevealCredential(req.Context(), []string{server.CredentialContext(server.Spec.UserID)}, server.Name)
	if err != nil && !errors.As(err, &gclient.CredentialNotFoundError{}) {
		return fmt.Errorf("failed to find credential: %w", err)
	}

	mergedEnv, err := mcp.MergeBoundCreds(req.Context(), req.LocalK8sClient, req.ObotNamespace, server.Spec.Manifest.Config, cred.Secrets, h.secretBindingAllowedLabel)
	if err != nil {
		return fmt.Errorf("failed to resolve secret bindings: %w", err)
	}

	slug, err := SlugForMCPServer(req.Context(), req.Storage, server, server.Spec.UserID, catalogName, "")
	if err != nil {
		return fmt.Errorf("failed to generate slug: %w", err)
	}

	return req.Write(ConvertMCPServer(server, mergedEnv, h.serverURL, slug))
}

// GenerateToolPreviews launches a temporary instance of an MCP server from a catalog entry
// to generate tool preview data, then cleans up the instance.
func (h *MCPCatalogHandler) GenerateToolPreviews(req api.Context) error {
	var (
		catalogName = req.PathValue("catalog_id")
		workspaceID = req.PathValue("workspace_id")
		entryName   = req.PathValue("entry_id")
		// "dryRun" lets us get the previews for an MCP server without updating its CatalogEntry.
		// This is used when we populate the tools for individual MCP servers when configuring a vMCP
		// (configuring tool overrides).
		dryRun = req.Request.URL.Query().Get("dryRun") == "true"
	)

	// Verify the scope exists
	if catalogName != "" {
		if err := req.Get(&v1.MCPCatalog{}, catalogName); err != nil {
			return fmt.Errorf("failed to get catalog: %w", err)
		}
	} else if workspaceID != "" {
		if err := req.Get(&v1.PowerUserWorkspace{}, workspaceID); err != nil {
			return fmt.Errorf("failed to get workspace: %w", err)
		}
	} else {
		return types.NewErrBadRequest("either catalog_id or workspace_id is required")
	}

	// Get the catalog entry
	var entry v1.MCPServerCatalogEntry
	if err := req.Get(&entry, entryName); err != nil {
		return fmt.Errorf("failed to get catalog entry: %w", err)
	}

	if err := validateEntryVisibleFromScope(entry, catalogName, workspaceID); err != nil {
		return err
	}
	if !dryRun && !entry.Spec.Editable {
		return types.NewErrBadRequest("entry is not editable")
	}

	if entry.Spec.Manifest.Runtime == types.RuntimeComposite {
		return types.NewErrBadRequest("composite catalog entries are no longer supported")
	}

	// Read configuration from request body
	var configRequest struct {
		Config map[string]string `json:"config"`
		URL    string            `json:"url"`
	}
	if err := req.Read(&configRequest); err != nil {
		return types.NewErrBadRequest("failed to read configuration: %v", err)
	}

	catalogName = entry.Spec.MCPCatalogName
	if catalogName == "" {
		catalogName = entry.Spec.PowerUserWorkspaceID
	}
	validationOptions, err := ValidationOptionsWithResourceMaximums(req, h.sessionManager)
	if err != nil {
		return err
	}
	server, serverConfig, err := tempServerAndConfig(
		req.Context(),
		req.Storage,
		req.LocalK8sClient,
		req.ObotNamespace,
		h.secretBindingAllowedLabel,
		entry.Name,
		catalogName,
		entry.Spec.Manifest,
		configRequest.Config,
		configRequest.URL,
		h.serverURL,
		validationOptions,
	)
	if err != nil {
		return types.NewErrBadRequest("failed to create temporary server and config: %v", err)
	}

	if serverConfig.Runtime == types.RuntimeRemote {
		oauthURL, err := h.oauthChecker.CheckForMCPAuth(req, server, serverConfig, "system", server.Name, "")
		if err != nil {
			return fmt.Errorf("failed to check for MCP auth: %w", err)
		}

		if oauthURL != "" {
			return types.NewErrBadRequest("MCP server requires OAuth authentication")
		}

		defer func() {
			_ = h.gatewayClient.DeleteMCPOAuthTokens(context.Background(), "system", server.Name)
		}()
	}

	// Launch temporary instance and get tools
	toolPreviews, err := h.sessionManager.GenerateToolPreviews(req.Context(), server, serverConfig)
	if err != nil {
		return fmt.Errorf("failed to launch temporary instance: %w", err)
	}

	// Set the tool preview on the catalog entry
	entry.Spec.Manifest.ToolPreview = toolPreviews
	if dryRun {
		// Don't update the entry, just return the entry with the new tool set
		return req.Write(ConvertMCPServerCatalogEntry(entry, h.serverURL))
	}

	if err := req.Update(&entry); err != nil {
		return fmt.Errorf("failed to update catalog entry: %w", err)
	}

	now := metav1.Now()
	entry.Status.ToolPreviewsLastGenerated = &now
	if err := req.Storage.Status().Update(req.Context(), &entry); err != nil {
		return fmt.Errorf("failed to update catalog entry: %w", err)
	}

	// Return the updated catalog entry
	return req.Write(ConvertMCPServerCatalogEntry(entry, h.serverURL))
}

func (h *MCPCatalogHandler) GenerateToolPreviewsOAuthURL(req api.Context) error {
	var (
		catalogName = req.PathValue("catalog_id")
		workspaceID = req.PathValue("workspace_id")
		entryName   = req.PathValue("entry_id")
		// "dryRun" lets us get the previews for an MCP server without updating its CatalogEntry.
		// This is used when we populate the tools for individual MCP servers when configuring a vMCP
		// (configuring tool overrides).
		dryRun = req.Request.URL.Query().Get("dryRun") == "true"
	)

	// Verify the scope exists
	if catalogName != "" {
		if err := req.Get(&v1.MCPCatalog{}, catalogName); err != nil {
			return fmt.Errorf("failed to get catalog: %w", err)
		}
	} else if workspaceID != "" {
		if err := req.Get(&v1.PowerUserWorkspace{}, workspaceID); err != nil {
			return fmt.Errorf("failed to get workspace: %w", err)
		}
	} else {
		return types.NewErrBadRequest("either catalog_id or workspace_id is required")
	}

	// Get the catalog entry
	var entry v1.MCPServerCatalogEntry
	if err := req.Get(&entry, entryName); err != nil {
		return fmt.Errorf("failed to get catalog entry: %w", err)
	}

	if err := validateEntryVisibleFromScope(entry, catalogName, workspaceID); err != nil {
		return err
	}

	if !entry.Spec.Editable && !dryRun {
		return types.NewErrBadRequest("entry is not editable")
	}

	if entry.Spec.Manifest.Runtime == types.RuntimeComposite {
		return types.NewErrBadRequest("composite catalog entries are no longer supported")
	}

	if entry.Spec.Manifest.Runtime != types.RuntimeRemote {
		return req.Write(map[string]string{"oauthURL": ""})
	}

	// Read configuration from request body
	var configRequest struct {
		Config map[string]string `json:"config"`
		URL    string            `json:"url"`
	}
	if err := req.Read(&configRequest); err != nil {
		return types.NewErrBadRequest("failed to read configuration: %v", err)
	}

	catalogName = entry.Spec.MCPCatalogName
	if catalogName == "" {
		catalogName = entry.Spec.PowerUserWorkspaceID
	}
	validationOptions, err := ValidationOptionsWithResourceMaximums(req, h.sessionManager)
	if err != nil {
		return err
	}
	server, serverConfig, err := tempServerAndConfig(req.Context(), req.Storage, req.LocalK8sClient, req.ObotNamespace, h.secretBindingAllowedLabel, entry.Name, catalogName, entry.Spec.Manifest, configRequest.Config, configRequest.URL, h.serverURL, validationOptions)
	if err != nil {
		return types.NewErrBadRequest("failed to create temporary server and config: %v", err)
	}

	oauthURL, err := h.oauthChecker.CheckForMCPAuth(req, server, serverConfig, "system", server.Name, "")
	if err != nil {
		return types.NewErrBadRequest("failed to check for MCP auth: %v", err)
	}

	return req.Write(map[string]string{"oauthURL": oauthURL})
}

// GenerateVMCPComponentToolPreviews generates tool previews for a vMCP
// component using the manifest snapshot stored on the vMCP. The source
// catalog entry is intentionally not consulted: a vMCP component is a
// deployed snapshot and preview generation must use the same definition.
func (h *MCPCatalogHandler) GenerateVMCPComponentToolPreviews(req api.Context) error {
	vmcp, component, server, serverConfig, err := h.vmcpComponentToolPreviewConfig(req)
	if err != nil {
		return err
	}

	if serverConfig.Runtime == types.RuntimeRemote {
		oauthURL, err := h.oauthChecker.CheckForMCPAuth(req, server, serverConfig, "system", server.Name, "")
		if err != nil {
			return fmt.Errorf("failed to check for MCP auth: %w", err)
		}
		if oauthURL != "" {
			return types.NewErrBadRequest("MCP server requires OAuth authentication")
		}
		if h.gatewayClient != nil {
			defer func() {
				_ = h.gatewayClient.DeleteMCPOAuthTokens(context.Background(), "system", server.Name)
			}()
		}
	}

	toolPreviews, err := h.sessionManager.GenerateToolPreviews(req.Context(), server, serverConfig)
	if err != nil {
		return fmt.Errorf("failed to generate tool preview: %w", err)
	}

	return h.writeVMCPComponentToolPreview(req, vmcp, component, toolPreviews)
}

// GenerateVMCPComponentToolPreviewsOAuthURL returns the OAuth URL, if any,
// needed to generate previews for a vMCP component.
func (h *MCPCatalogHandler) GenerateVMCPComponentToolPreviewsOAuthURL(req api.Context) error {
	_, _, server, serverConfig, err := h.vmcpComponentToolPreviewConfig(req)
	if err != nil {
		return err
	}
	if serverConfig.Runtime != types.RuntimeRemote {
		return req.Write(map[string]string{"oauthURL": ""})
	}

	oauthURL, err := h.oauthChecker.CheckForMCPAuth(req, server, serverConfig, "system", server.Name, "")
	if err != nil {
		return types.NewErrBadRequest("failed to check for MCP auth: %v", err)
	}
	return req.Write(map[string]string{"oauthURL": oauthURL})
}

// vmcpComponentToolPreviewConfig builds the temporary server used by both
// vMCP component preview endpoints. Only fixed configuration from the vMCP's
// VMCP-scoped credential is used; callers cannot override vMCP policy by
// posting arbitrary configuration to this endpoint.
func (h *MCPCatalogHandler) vmcpComponentToolPreviewConfig(req api.Context) (v1.VMCP, types.VMCPComponent, v1.MCPServer, mcp.ServerConfig, error) {
	var vmcp v1.VMCP
	if err := req.Get(&vmcp, req.PathValue("vmcp_id")); err != nil {
		return vmcp, types.VMCPComponent{}, v1.MCPServer{}, mcp.ServerConfig{}, fmt.Errorf("failed to get vMCP: %w", err)
	}

	componentID := req.PathValue("component_id")
	var component *types.VMCPComponent
	for index := range vmcp.Spec.Manifest.Components {
		candidate := &vmcp.Spec.Manifest.Components[index]
		if candidate.ID == componentID || (candidate.ID == "" && candidate.MCPServerCatalogEntryID == componentID) {
			component = candidate
			break
		}
	}
	if component == nil {
		return vmcp, types.VMCPComponent{}, v1.MCPServer{}, mcp.ServerConfig{}, types.NewErrNotFound("vMCP component not found")
	}

	manifest := component.CatalogEntry.Manifest.DeepCopy()
	if manifest == nil {
		return vmcp, *component, v1.MCPServer{}, mcp.ServerConfig{}, types.NewErrBadRequest("vMCP component has no catalog-entry snapshot")
	}

	staticConfiguration, err := h.vmcpStaticConfiguration(req, vmcp.Name, *component)
	if err != nil {
		return vmcp, *component, v1.MCPServer{}, mcp.ServerConfig{}, err
	}
	validationOptions, err := ValidationOptionsWithResourceMaximums(req, h.sessionManager)
	if err != nil {
		return vmcp, *component, v1.MCPServer{}, mcp.ServerConfig{}, err
	}
	catalogName := component.MCPCatalogID
	server, serverConfig, err := tempServerAndConfig(
		req.Context(),
		req.Storage,
		req.LocalK8sClient,
		req.ObotNamespace,
		h.secretBindingAllowedLabel,
		component.MCPServerCatalogEntryID,
		catalogName,
		*manifest,
		staticConfiguration,
		"",
		h.serverURL,
		validationOptions,
	)
	if err != nil {
		return vmcp, *component, v1.MCPServer{}, mcp.ServerConfig{}, types.NewErrBadRequest("failed to create temporary server and config: %v", err)
	}

	// GenerateToolPreviews uses the system OAuth identity. Include the
	// requester and vMCP component in the temporary server name so concurrent
	// preview requests never share OAuth state or a runtime.
	tempName := "vmcp-tool-preview-" + utils.Digest(struct {
		VMCPID              string
		ComponentID         string
		UserID              string
		SnapshotDigest      string
		ConfigurationDigest string
	}{
		VMCPID:              vmcp.Name,
		ComponentID:         component.ID,
		UserID:              req.User.GetUID(),
		SnapshotDigest:      utils.Digest(*manifest),
		ConfigurationDigest: utils.Digest(staticConfiguration),
	})[:16]
	server.Name = tempName
	serverConfig.MCPServerName = tempName

	return vmcp, *component, server, serverConfig, nil
}

func (h *MCPCatalogHandler) vmcpStaticConfiguration(req api.Context, vmcpID string, component types.VMCPComponent) (map[string]string, error) {
	configuration := map[string]string{}
	if h.gatewayClient == nil {
		return nil, fmt.Errorf("gateway client is not configured")
	}
	credential, err := h.gatewayClient.RevealCredential(
		req.Context(),
		[]string{vmcpconfig.StaticConfigurationCredentialContext(vmcpID)},
		vmcpconfig.ConfigurationCredentialName(),
	)
	if err != nil {
		if errors.As(err, &gclient.CredentialNotFoundError{}) {
			return configuration, nil
		}
		return nil, fmt.Errorf("failed to reveal vMCP static configuration: %w", err)
	}
	fixedKeys := make(map[string]struct{}, len(component.Configuration))
	for _, policy := range component.Configuration {
		if policy.Policy == types.VMCPConfigurationPolicyFixed {
			fixedKeys[policy.Key] = struct{}{}
		}
	}
	for key, value := range credential.Secrets {
		keyComponentID, configurationKey, ok := vmcpconfig.ParseConfigurationKey(key)
		if ok && keyComponentID == component.ID {
			if _, isFixed := fixedKeys[configurationKey]; !isFixed {
				continue
			}
			configuration[configurationKey] = value
		}
	}
	return configuration, nil
}

func (h *MCPCatalogHandler) writeVMCPComponentToolPreview(req api.Context, vmcp v1.VMCP, component types.VMCPComponent, toolPreviews []types.MCPServerTool) error {
	manifest := component.CatalogEntry.Manifest.DeepCopy()
	if manifest == nil {
		return types.NewErrBadRequest("vMCP component has no catalog-entry snapshot")
	}
	manifest.ToolPreview = toolPreviews
	entry := v1.MCPServerCatalogEntry{
		Name:      component.MCPServerCatalogEntryID,
		Namespace: vmcp.Namespace,
		Spec: v1.MCPServerCatalogEntrySpec{
			MCPCatalogName: component.MCPCatalogID,
			Manifest:       *manifest,
		},
	}
	return req.Write(ConvertMCPServerCatalogEntry(entry, h.serverURL))
}

func tempServerAndConfig(ctx context.Context, client kclient.Client, localK8sClient kclient.Client, obotNamespace, secretBindingAllowedLabel, entryName, catalogName string, entryManifest types.MCPServerCatalogEntryManifest, config map[string]string, url, baseURL string, validationOptions mcp.ValidationOptions) (v1.MCPServer, mcp.ServerConfig, error) {
	// Convert catalog entry to server manifest
	serverManifest, err := types.MapCatalogEntryToServer(entryManifest, url, false)
	if err != nil {
		return v1.MCPServer{}, mcp.ServerConfig{}, fmt.Errorf("failed to convert catalog entry to server config: %w", err)
	}

	config, err = prepareTempServerConfig(ctx, localK8sClient, obotNamespace, secretBindingAllowedLabel, &serverManifest, config, false, validationOptions)
	if err != nil {
		return v1.MCPServer{}, mcp.ServerConfig{}, err
	}
	if err := tunnel.ValidateServerTunnelReferences(ctx, client, serverManifest); err != nil {
		return v1.MCPServer{}, mcp.ServerConfig{}, types.NewErrBadRequest("validation failed: %v", err)
	}

	// Create temporary MCPServer object to use existing conversion logic
	tempName := "tool-preview-" + utils.Digest(serverManifest)[:16]
	tempMCPServer := v1.MCPServer{
		Name: tempName,
		Spec: v1.MCPServerSpec{
			Manifest:                  serverManifest,
			MCPServerCatalogEntryName: entryName,
		},
	}

	serverConfig, missingFields, err := mcp.ServerToServerConfig(tempMCPServer, tempMCPServer.ValidConnectURLs(baseURL), "temp", "temp", catalogName, config)
	if err != nil {
		return v1.MCPServer{}, mcp.ServerConfig{}, fmt.Errorf("failed to create server config: %w", err)
	}

	if len(missingFields) > 0 {
		return v1.MCPServer{}, mcp.ServerConfig{}, types.NewErrBadRequest("missing required configuration fields: %v", missingFields)
	}
	if serverConfig.AuditLogMetadata == nil {
		serverConfig.AuditLogMetadata = map[string]string{}
	}
	serverConfig.AuditLogMetadata[mcp.AuditLogIgnore] = "true"

	return tempMCPServer, serverConfig, nil
}

func prepareTempServerConfig(ctx context.Context, localK8sClient kclient.Client, obotNamespace, secretBindingAllowedLabel string, serverManifest *types.MCPServerManifest, config map[string]string, isMultiUser bool, validationOptions mcp.ValidationOptions) (map[string]string, error) {
	if err := validateConfiguredOptions(serverManifest.Config, config); err != nil {
		return nil, types.NewErrBadRequest("invalid configuration: %v", err)
	}
	// Render templates before resolving bindings so Secret values can only be
	// used by runtime fields such as headers, never embedded in the URL.
	if err := applyRemoteURLTemplate(ctx, serverManifest, config, isMultiUser, validationOptions); err != nil {
		if configErr, ok := errors.AsType[*urlTemplateConfigurationError](err); ok {
			return nil, types.NewErrBadRequest("invalid configuration: %v", configErr)
		}
		return nil, err
	}

	mergedConfig, err := mcp.MergeBoundCreds(ctx, localK8sClient, obotNamespace, serverManifest.Config, config, secretBindingAllowedLabel)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve secret bindings: %w", err)
	}
	return mergedConfig, nil
}

// ListCategoriesForCatalog returns all unique categories from entries in a catalog
func (h *MCPCatalogHandler) ListCategoriesForCatalog(req api.Context) error {
	catalogName := req.PathValue("catalog_id")

	var list v1.MCPServerCatalogEntryList
	if err := req.List(&list, kclient.MatchingFields{
		"spec.mcpCatalogName": catalogName,
	}); err != nil {
		return fmt.Errorf("failed to list entries: %w", err)
	}

	// Collect unique categories
	categoriesSet := make(map[string]struct{})
	for _, entry := range list.Items {
		if categories := entry.Spec.Manifest.Metadata["categories"]; categories != "" {
			// Handle both comma-separated and single categories
			for category := range strings.SplitSeq(categories, ",") {
				trimmed := strings.TrimSpace(category)
				if trimmed != "" {
					categoriesSet[trimmed] = struct{}{}
				}
			}
		}
	}

	// Convert to sorted slice
	categories := make([]string, 0, len(categoriesSet))
	for category := range categoriesSet {
		categories = append(categories, category)
	}
	sort.Strings(categories)

	return req.Write(categories)
}

// revealCatalogTokens returns the Env map from the single credential that stores
// all source-URL tokens for a catalog. Returns an empty map if no credential exists.
func revealCatalogTokens(req api.Context, catalogName string) (map[string]string, error) {
	cred, err := req.GatewayClient.RevealCredential(req.Context(), []string{catalogName}, mcpcataloghandler.CatalogCredentialToolName)
	if err != nil {
		if errors.As(err, &gclient.CredentialNotFoundError{}) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to reveal credentials for catalog %s: %w", catalogName, err)
	}
	return cred.Secrets, nil
}

func convertMCPCatalog(catalog v1.MCPCatalog, tokenEnv map[string]string) types.MCPCatalog {
	return types.MCPCatalog{
		Metadata:                  MetadataFrom(&catalog),
		DisplayName:               catalog.Spec.DisplayName,
		SourceURLs:                catalog.Spec.SourceURLs,
		SourceURLCredentials:      maskCatalogCredentials(catalog.Spec.SourceURLs, tokenEnv),
		SourceURLGitCredentialIDs: catalog.Spec.SourceURLGitCredentialIDs,
		LastSynced:                *types.NewTime(catalog.Status.LastSyncTime.Time),
		SyncErrors:                catalog.Status.SyncErrors,
		IsSyncing:                 catalog.Status.IsSyncing || catalog.Annotations[v1.MCPCatalogSyncAnnotation] == "true",
	}
}

func normalizeMCPCatalogEntryName(name string) string {
	// lowercase
	name = strings.ToLower(name)
	// replace invalid chars with '-'
	name = dnsLabelRegex.ReplaceAllString(name, "-")
	// collapse multiple consecutive '-' into single '-'
	for strings.Contains(name, "--") {
		name = strings.ReplaceAll(name, "--", "-")
	}
	// trim leading/trailing '-'
	name = strings.Trim(name, "-")
	// max length 63
	if len(name) > 63 {
		name = name[:63]
		// ensure we don't end with '-' after truncation
		name = strings.TrimRight(name, "-")
	}
	return name
}

// entryRequiresStaticOAuthCreds checks if a catalog entry requires OAuth credentials
// that haven't been configured yet. Returns true if the entry should be hidden from non-admin users.
func entryRequiresStaticOAuthCreds(entry v1.MCPServerCatalogEntry) bool {
	// Check if the entry requires static OAuth
	if entry.Spec.Manifest.RemoteConfig == nil || !entry.Spec.Manifest.RemoteConfig.StaticOAuthRequired {
		return false
	}

	// Use the cached status field instead of doing a credential lookup
	return !entry.Status.OAuthCredentialConfigured
}

// verifyOAuthCredentialAccess verifies that:
// 1. The scope (catalog or workspace) exists
// 2. The entry exists and belongs to that scope
// 3. The entry requires static OAuth configuration
// Returns the entry on success, or an error.
func verifyOAuthCredentialAccess(req api.Context, catalogName, workspaceID, entryName string) (*v1.MCPServerCatalogEntry, error) {
	// Verify the scope exists
	if catalogName != "" {
		if err := req.Get(&v1.MCPCatalog{}, catalogName); err != nil {
			return nil, fmt.Errorf("failed to get catalog: %w", err)
		}
	} else if workspaceID != "" {
		if err := req.Get(&v1.PowerUserWorkspace{}, workspaceID); err != nil {
			return nil, fmt.Errorf("failed to get workspace: %w", err)
		}
	} else {
		return nil, types.NewErrBadRequest("either catalog_id or workspace_id is required")
	}

	var entry v1.MCPServerCatalogEntry
	if err := req.Get(&entry, entryName); err != nil {
		return nil, fmt.Errorf("failed to get entry: %w", err)
	}

	if err := validateEntryScope(entry, catalogName, workspaceID); err != nil {
		return nil, err
	}

	// Check if the entry requires static OAuth
	if entry.Spec.Manifest.RemoteConfig == nil || !entry.Spec.Manifest.RemoteConfig.StaticOAuthRequired {
		return nil, types.NewErrBadRequest("entry does not require OAuth configuration")
	}

	return &entry, nil
}

// GetOAuthCredentials returns the OAuth credential status for a catalog entry.
// GET /api/mcp-catalogs/{catalog_id}/entries/{entry_id}/oauth-credentials
// GET /api/workspaces/{workspace_id}/entries/{entry_id}/oauth-credentials
func (h *MCPCatalogHandler) GetOAuthCredentials(req api.Context) error {
	catalogName := req.PathValue("catalog_id")
	workspaceID := req.PathValue("workspace_id")
	entryName := req.PathValue("entry_id")

	entry, err := verifyOAuthCredentialAccess(req, catalogName, workspaceID, entryName)
	if err != nil {
		return err
	}

	// Check if credentials exist
	credName := system.MCPOAuthCredentialName(entry.Name)
	cred, err := req.GatewayClient.RevealCredential(req.Context(), []string{credName}, system.StaticOAuthCredentialName)
	configured := err == nil

	var clientID string
	if configured {
		clientID = cred.Secrets["CLIENT_ID"]
	}

	return req.Write(types.MCPServerOAuthCredentialStatus{
		Configured: configured,
		ClientID:   clientID,
	})
}

// SetOAuthCredentials sets OAuth credentials for a catalog entry.
// POST /api/mcp-catalogs/{catalog_id}/entries/{entry_id}/oauth-credentials
// POST /api/workspaces/{workspace_id}/entries/{entry_id}/oauth-credentials
func (h *MCPCatalogHandler) SetOAuthCredentials(req api.Context) error {
	catalogName := req.PathValue("catalog_id")
	workspaceID := req.PathValue("workspace_id")
	entryName := req.PathValue("entry_id")

	entry, err := verifyOAuthCredentialAccess(req, catalogName, workspaceID, entryName)
	if err != nil {
		return err
	}

	var credReq types.MCPServerOAuthCredentialRequest
	if err := req.Read(&credReq); err != nil {
		return err
	}

	// Check if credentials already exist
	credName := system.MCPOAuthCredentialName(entry.Name)
	_, err = req.GatewayClient.RevealCredential(req.Context(), []string{credName}, system.StaticOAuthCredentialName)
	credentialsExist := err == nil

	if credentialsExist {
		// Credentials already exist - must delete and recreate to change them
		return types.NewErrBadRequest("credentials already exist; delete and recreate credentials to change them")
	}

	secrets, err := staticOAuthCredentialSecrets(credReq)
	if err != nil {
		return err
	}

	// Store new credential
	cred := gatewaytypes.Credential{
		Context: credName,
		Name:    system.StaticOAuthCredentialName,
		Secrets: secrets,
	}
	if err := req.GatewayClient.UpsertCredential(req.Context(), cred); err != nil {
		return fmt.Errorf("failed to create OAuth credential: %w", err)
	}

	// Trigger reconciliation to update the status
	if entry.Annotations == nil {
		entry.Annotations = make(map[string]string, 1)
	}
	entry.Annotations[v1.MCPServerCatalogEntrySyncAnnotation] = "true"
	if err := req.Update(entry); err != nil {
		return fmt.Errorf("failed to trigger reconciliation: %w", err)
	}

	return req.Write(types.MCPServerOAuthCredentialStatus{
		Configured: true,
		ClientID:   secrets["CLIENT_ID"],
	})
}

func staticOAuthCredentialSecrets(credReq types.MCPServerOAuthCredentialRequest) (map[string]string, error) {
	clientID := strings.TrimSpace(credReq.ClientID)
	if clientID == "" {
		return nil, types.NewErrBadRequest("clientID is required")
	}

	secrets := map[string]string{"CLIENT_ID": clientID}
	if clientSecret := strings.TrimSpace(credReq.ClientSecret); clientSecret != "" {
		secrets["CLIENT_SECRET"] = clientSecret
	}
	return secrets, nil
}

// DeleteOAuthCredentials removes OAuth credentials for a catalog entry.
// DELETE /api/mcp-catalogs/{catalog_id}/entries/{entry_id}/oauth-credentials
// DELETE /api/workspaces/{workspace_id}/entries/{entry_id}/oauth-credentials
func (h *MCPCatalogHandler) DeleteOAuthCredentials(req api.Context) error {
	catalogName := req.PathValue("catalog_id")
	workspaceID := req.PathValue("workspace_id")
	entryName := req.PathValue("entry_id")

	entry, err := verifyOAuthCredentialAccess(req, catalogName, workspaceID, entryName)
	if err != nil {
		return err
	}

	// Written before the credential is deleted. The controller decides whether an entry is worth a
	// credential query from its status and this annotation, so a delete that lands without it
	// leaves the status reading configured with nothing left to correct it.
	if entry.Annotations == nil {
		entry.Annotations = make(map[string]string, 1)
	}
	entry.Annotations[v1.MCPServerCatalogEntrySyncAnnotation] = "true"
	if err := req.Update(entry); err != nil {
		return fmt.Errorf("failed to trigger reconciliation: %w", err)
	}

	credName := system.MCPOAuthCredentialName(entry.Name)
	deleted, err := req.GatewayClient.DeleteCredential(req.Context(), credName, system.StaticOAuthCredentialName)
	if err != nil {
		return err
	}

	// Publish the completed deletion even if reconciliation consumed the initial
	// sync request while the credential was still present.
	before := entry.DeepCopy()
	entry.Annotations[v1.MCPServerCatalogEntrySyncAnnotation] = "true"
	// Include the sync annotation in the patch even when it was already set locally.
	delete(before.Annotations, v1.MCPServerCatalogEntrySyncAnnotation)
	if err := req.Storage.Patch(req.Context(), entry, kclient.MergeFrom(before)); err != nil {
		return fmt.Errorf("failed to trigger reconciliation after credential deletion: %w", err)
	}

	// Best-effort cleanup of per-user OAuth tokens associated with this catalog entry.
	var mcpServers v1.MCPServerList
	if err := req.List(&mcpServers, kclient.MatchingFields{"spec.mcpServerCatalogEntryName": entry.Name}); err != nil {
		slog.Warn("failed to list MCP servers for token cleanup of catalog entry", "catalogEntryName", entry.Name, "error", err)
	} else {
		for _, server := range mcpServers.Items {
			if err := h.gatewayClient.DeleteMCPOAuthTokenForAllUsers(req.Context(), server.Name); err != nil {
				slog.Warn("failed to delete OAuth tokens for MCP server", "serverName", server.Name, "catalogEntryName", entry.Name, "error", err)
			}
		}
	}

	return req.Write(map[string]bool{"deleted": deleted})
}
