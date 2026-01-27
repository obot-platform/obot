package handlers

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/gptscript-ai/gptscript/pkg/hash"
	"github.com/obot-platform/nah/pkg/name"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/accesscontrolrule"
	"github.com/obot-platform/obot/pkg/api"
	gclient "github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/obot-platform/obot/pkg/validation"
	"golang.org/x/crypto/bcrypt"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var dnsLabelRegex = regexp.MustCompile("[^a-z0-9-]+")

type MCPCatalogHandler struct {
	defaultCatalogPath string
	serverURL          string
	sessionManager     *mcp.SessionManager
	oauthChecker       MCPOAuthChecker
	gatewayClient      *gclient.Client
	acrHelper          *accesscontrolrule.Helper
}

func NewMCPCatalogHandler(defaultCatalogPath string, serverURL string, sessionManager *mcp.SessionManager, oauthChecker MCPOAuthChecker, gatewayClient *gclient.Client, acrHelper *accesscontrolrule.Helper) *MCPCatalogHandler {
	return &MCPCatalogHandler{
		defaultCatalogPath: defaultCatalogPath,
		serverURL:          serverURL,
		sessionManager:     sessionManager,
		oauthChecker:       oauthChecker,
		gatewayClient:      gatewayClient,
		acrHelper:          acrHelper,
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
		items = append(items, convertMCPCatalog(item))
	}

	return req.Write(types.MCPCatalogList{
		Items: items,
	})
}

// Get returns a specific catalog by ID.
func (*MCPCatalogHandler) Get(req api.Context) error {
	var catalog v1.MCPCatalog
	if err := req.Get(&catalog, req.PathValue("catalog_id")); err != nil {
		return fmt.Errorf("failed to get catalog: %w", err)
	}
	return req.Write(convertMCPCatalog(catalog))
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

	// The only field that can be updated is the source URLs.
	for _, urlStr := range manifest.SourceURLs {
		if urlStr != "" && urlStr != h.defaultCatalogPath {
			u, err := url.Parse(urlStr)
			if err != nil {
				return types.NewErrBadRequest("invalid URL: %v", err)
			}

			if u.Scheme != "https" {
				return types.NewErrBadRequest("only HTTPS URLs are supported")
			}
		}
	}

	// Check for duplicate URLs
	seen := make(map[string]struct{}, len(manifest.SourceURLs))
	for _, urlStr := range manifest.SourceURLs {
		if urlStr != "" {
			if _, ok := seen[urlStr]; ok {
				return types.NewErrBadRequest("duplicate URL found: %s", urlStr)
			}
			seen[urlStr] = struct{}{}
		}
	}

	catalog.Spec.SourceURLs = manifest.SourceURLs

	if err := req.Update(&catalog); err != nil {
		return fmt.Errorf("failed to update catalog: %w", err)
	}

	return req.Write(convertMCPCatalog(catalog))
}

// ListEntries lists all entries for a catalog or workspace.
func (h *MCPCatalogHandler) ListEntries(req api.Context) error {
	catalogName := req.PathValue("catalog_id")
	workspaceID := req.PathValue("workspace_id")
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

	var fieldSelector client.MatchingFields
	if catalogName != "" {
		fieldSelector = client.MatchingFields{"spec.mcpCatalogName": catalogName}
	} else if workspaceID != "" {
		fieldSelector = client.MatchingFields{"spec.powerUserWorkspaceID": workspaceID}
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
			entries = append(entries, ConvertMCPServerCatalogEntryWithWorkspace(entry, workspaceID, powerUserID))
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
			entries = append(entries, ConvertMCPServerCatalogEntryWithWorkspace(entry, workspaceID, powerUserID))
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

	// Verify entry belongs to the requested scope
	if catalogName != "" && entry.Spec.MCPCatalogName != catalogName {
		return types.NewErrBadRequest("entry does not belong to catalog")
	} else if workspaceID != "" && entry.Spec.PowerUserWorkspaceID != workspaceID {
		return types.NewErrBadRequest("entry does not belong to workspace")
	}

	// For workspace entries, include powerUserId in the response
	if workspaceID != "" {
		var workspace v1.PowerUserWorkspace
		if err := req.Get(&workspace, workspaceID); err != nil {
			return fmt.Errorf("failed to get workspace for powerUserId: %w", err)
		}
		return req.Write(ConvertMCPServerCatalogEntryWithWorkspace(entry, workspaceID, workspace.Spec.UserID))
	}

	return req.Write(ConvertMCPServerCatalogEntry(entry))
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

	// Handle composite catalog entries
	if manifest.Runtime == types.RuntimeComposite && manifest.CompositeConfig != nil {
		if err := h.populateComponentManifests(req, &manifest, catalogName, workspaceID); err != nil {
			return err
		}
	}

	// Auto-set StaticOAuthRequired if AuthorizationServerURL is provided
	if manifest.RemoteConfig != nil && manifest.RemoteConfig.AuthorizationServerURL != "" {
		manifest.RemoteConfig.StaticOAuthRequired = true
	}

	if err := validation.ValidateCatalogEntryManifest(manifest); err != nil {
		return types.NewErrBadRequest("failed to validate entry manifest: %v", err)
	}

	cleanName := normalizeMCPCatalogEntryName(manifest.Name)

	entry := v1.MCPServerCatalogEntry{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: req.Namespace(),
		},
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

	return req.Write(ConvertMCPServerCatalogEntry(entry))
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

	// Verify entry belongs to the requested scope
	if catalogName != "" && entry.Spec.MCPCatalogName != catalogName {
		return types.NewErrBadRequest("entry does not belong to catalog")
	} else if workspaceID != "" && entry.Spec.PowerUserWorkspaceID != workspaceID {
		return types.NewErrBadRequest("entry does not belong to workspace")
	}

	if !entry.Spec.Editable {
		return types.NewErrBadRequest("entry is not editable")
	}

	var manifest types.MCPServerCatalogEntryManifest
	if err := req.Read(&manifest); err != nil {
		return types.NewErrBadRequest("failed to read entry manifest: %v", err)
	}

	// Auto-set StaticOAuthRequired if AuthorizationServerURL is provided
	if manifest.RemoteConfig != nil && manifest.RemoteConfig.AuthorizationServerURL != "" {
		manifest.RemoteConfig.StaticOAuthRequired = true
	}

	if err := validation.ValidateCatalogEntryManifest(manifest); err != nil {
		return types.NewErrBadRequest("failed to validate entry manifest: %v", err)
	}

	// Copy the tool previews over so that they don't get wiped out when updating the manifest
	manifest.ToolPreview = entry.Spec.Manifest.ToolPreview

	// Update the manifest
	entry.Spec.Manifest = manifest

	if err := req.Update(&entry); err != nil {
		return fmt.Errorf("failed to update entry: %w", err)
	}

	return req.Write(ConvertMCPServerCatalogEntry(entry))
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

	// Verify entry belongs to the requested scope
	if catalogName != "" && entry.Spec.MCPCatalogName != catalogName {
		return types.NewErrBadRequest("entry does not belong to catalog")
	} else if workspaceID != "" && entry.Spec.PowerUserWorkspaceID != workspaceID {
		return types.NewErrBadRequest("entry does not belong to workspace")
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
	if err := req.List(&list, client.MatchingFields{
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

		var credCtx string
		if server.Spec.MCPCatalogID != "" {
			credCtx = fmt.Sprintf("%s-%s", server.Spec.MCPCatalogID, server.Name)
		} else {
			credCtx = fmt.Sprintf("%s-%s", server.Spec.UserID, server.Name)
		}

		cred, err := req.GPTClient.RevealCredential(req.Context(), []string{credCtx}, server.Name)
		if err != nil && !errors.As(err, &gptscript.ErrNotFound{}) {
			return fmt.Errorf("failed to find credential: %w", err)
		}

		slug, err := SlugForMCPServer(req.Context(), req.Storage, server, server.Spec.UserID, catalogName, "")
		if err != nil {
			return fmt.Errorf("failed to generate slug: %w", err)
		}

		var components []types.MCPServer
		if server.Spec.Manifest.Runtime == types.RuntimeComposite {
			components, err = resolveCompositeComponents(req, server)
			if err != nil {
				return err
			}
		}
		items = append(items, ConvertMCPServer(server, cred.Env, h.serverURL, slug, components...))
	}

	return req.Write(types.MCPServerList{Items: items})
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
	if err := req.List(&entriesList, client.MatchingFields{
		"spec.mcpCatalogName": catalogName,
	}); err != nil {
		return fmt.Errorf("failed to list entries: %w", err)
	}

	catalogEntries := entriesList.Items

	// For each entry, get its servers using the same approach as AdminListServersForEntryInCatalog
	var allServers []v1.MCPServer
	for _, entry := range catalogEntries {
		var serverList v1.MCPServerList
		if err := req.List(&serverList, client.MatchingFields{
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
		var credCtx string
		if server.Spec.MCPCatalogID != "" {
			credCtx = fmt.Sprintf("%s-%s", server.Spec.MCPCatalogID, server.Name)
		} else {
			credCtx = fmt.Sprintf("%s-%s", server.Spec.UserID, server.Name)
		}

		cred, err := req.GPTClient.RevealCredential(req.Context(), []string{credCtx}, server.Name)
		if err != nil && !errors.As(err, &gptscript.ErrNotFound{}) {
			return fmt.Errorf("failed to find credential: %w", err)
		}

		slug, err := SlugForMCPServer(req.Context(), req.Storage, server, server.Spec.UserID, catalogName, "")
		if err != nil {
			return fmt.Errorf("failed to generate slug: %w", err)
		}

		var components []types.MCPServer
		if server.Spec.Manifest.Runtime == types.RuntimeComposite {
			components, err = resolveCompositeComponents(req, server)
			if err != nil {
				return err
			}
		}
		items = append(items, ConvertMCPServer(server, cred.Env, h.serverURL, slug, components...))
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

	// Verify entry belongs to the requested scope
	if catalogName != "" && entry.Spec.MCPCatalogName != catalogName {
		return types.NewErrBadRequest("entry does not belong to catalog")
	} else if workspaceID != "" && entry.Spec.PowerUserWorkspaceID != workspaceID {
		return types.NewErrBadRequest("entry does not belong to workspace")
	}

	var list v1.MCPServerList
	if err := req.List(&list, client.MatchingFields{
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

		var credCtx string
		if server.Spec.MCPCatalogID != "" {
			credCtx = fmt.Sprintf("%s-%s", server.Spec.MCPCatalogID, server.Name)
		} else if server.Spec.PowerUserWorkspaceID != "" {
			credCtx = fmt.Sprintf("%s-%s", server.Spec.PowerUserWorkspaceID, server.Name)
		} else {
			credCtx = fmt.Sprintf("%s-%s", server.Spec.UserID, server.Name)
		}
		cred, err := req.GPTClient.RevealCredential(req.Context(), []string{credCtx}, server.Name)
		if err != nil && !errors.As(err, &gptscript.ErrNotFound{}) {
			return fmt.Errorf("failed to find credential: %w", err)
		}

		slug, err := SlugForMCPServer(req.Context(), req.Storage, server, server.Spec.UserID, catalogName, "")
		if err != nil {
			return fmt.Errorf("failed to generate slug: %w", err)
		}

		var components []types.MCPServer
		if server.Spec.Manifest.Runtime == types.RuntimeComposite {
			components, err = resolveCompositeComponents(req, server)
			if err != nil {
				return fmt.Errorf("failed to resolve composite components: %w", err)
			}
		}
		items = append(items, ConvertMCPServer(server, cred.Env, h.serverURL, slug, components...))
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

	// Verify entry belongs to the requested scope
	if catalogName != "" && entry.Spec.MCPCatalogName != catalogName {
		return types.NewErrBadRequest("entry does not belong to catalog")
	} else if workspaceID != "" && entry.Spec.PowerUserWorkspaceID != workspaceID {
		return types.NewErrBadRequest("entry does not belong to workspace")
	}

	var server v1.MCPServer
	if err := req.Get(&server, req.PathValue("mcp_server_id")); err != nil {
		return fmt.Errorf("failed to list servers: %w", err)
	}

	var credCtx string
	if server.Spec.MCPCatalogID != "" {
		credCtx = fmt.Sprintf("%s-%s", server.Spec.MCPCatalogID, server.Name)
	} else if server.Spec.PowerUserWorkspaceID != "" {
		credCtx = fmt.Sprintf("%s-%s", server.Spec.PowerUserWorkspaceID, server.Name)
	} else {
		credCtx = fmt.Sprintf("%s-%s", server.Spec.UserID, server.Name)
	}

	cred, err := req.GPTClient.RevealCredential(req.Context(), []string{credCtx}, server.Name)
	if err != nil && !errors.As(err, &gptscript.ErrNotFound{}) {
		return fmt.Errorf("failed to find credential: %w", err)
	}

	slug, err := SlugForMCPServer(req.Context(), req.Storage, server, server.Spec.UserID, catalogName, "")
	if err != nil {
		return fmt.Errorf("failed to generate slug: %w", err)
	}

	var components []types.MCPServer
	if server.Spec.Manifest.Runtime == types.RuntimeComposite {
		components, err = resolveCompositeComponents(req, server)
		if err != nil {
			log.Warnf("failed to resolve composite components for catalog server %s: %v", server.Name, err)
			return err
		}
	}
	return req.Write(ConvertMCPServer(server, cred.Env, h.serverURL, slug, components...))
}

// GenerateToolPreviews launches a temporary instance of an MCP server from a catalog entry
// to generate tool preview data, then cleans up the instance.
func (h *MCPCatalogHandler) GenerateToolPreviews(req api.Context) error {
	var (
		catalogName = req.PathValue("catalog_id")
		workspaceID = req.PathValue("workspace_id")
		entryName   = req.PathValue("entry_id")
		// "dryRun" lets us get the previews for an MCP server without updating its CatalogEntry.
		// This is used when we populate the tools for individual MCP servers when creating a composite CatalogEntry
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

	// Verify entry belongs to the requested scope
	if catalogName != "" && entry.Spec.MCPCatalogName != catalogName {
		return types.NewErrBadRequest("entry does not belong to catalog")
	} else if workspaceID != "" && entry.Spec.PowerUserWorkspaceID != workspaceID {
		return types.NewErrBadRequest("entry does not belong to workspace")
	}

	if !dryRun && !entry.Spec.Editable {
		return types.NewErrBadRequest("entry is not editable")
	}

	if entry.Spec.Manifest.Runtime == types.RuntimeComposite {
		return h.generateCompositeToolPreviews(req, entry, dryRun)
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
	server, serverConfig, err := tempServerAndConfig(
		req.Context(),
		req.GPTClient,
		req.Storage,
		entry.Namespace,
		catalogName,
		entry.Spec.Manifest,
		configRequest.Config,
		configRequest.URL,
		h.serverURL,
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
		return req.Write(ConvertMCPServerCatalogEntry(entry))
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
	return req.Write(ConvertMCPServerCatalogEntry(entry))
}

func (h *MCPCatalogHandler) generateCompositeToolPreviews(req api.Context, entry v1.MCPServerCatalogEntry, dryRun bool) error {
	// Read configuration from request body
	var configRequest struct {
		ComponentConfigs map[string]struct {
			Config   map[string]string `json:"config"`
			URL      string            `json:"url"`
			Disabled bool              `json:"disabled"`
		} `json:"componentConfigs"`
	}
	if err := req.Read(&configRequest); err != nil {
		return types.NewErrBadRequest("failed to read configuration: %v", err)
	}

	compositeConfig := entry.Spec.Manifest.CompositeConfig
	if compositeConfig == nil {
		return types.NewErrBadRequest("composite configuration is required")
	}

	catalogName := entry.Spec.MCPCatalogName
	if catalogName == "" {
		catalogName = entry.Spec.PowerUserWorkspaceID
	}

	compositeToolPreviews := make([]types.MCPServerTool, 0, len(compositeConfig.ComponentServers))
	for _, componentEntry := range compositeConfig.ComponentServers {
		// If this component references an existing MCPServer, list its tools directly
		// (as we do for multi-user servers) instead of creating a temporary server.
		if componentEntry.MCPServerID != "" {
			var mcpServer v1.MCPServer
			if err := req.Get(&mcpServer, componentEntry.MCPServerID); err != nil {
				return fmt.Errorf("failed to get MCP server %q: %w", componentEntry.MCPServerID, err)
			}

			serverConfig, err := serverConfigForAction(req, mcpServer)
			if err != nil {
				return fmt.Errorf("failed to build server configuration for MCP server %q: %w", mcpServer.Name, err)
			}

			tools, err := toolsForServer(req.Context(), h.sessionManager, mcpServer, serverConfig, nil)
			if err != nil {
				return fmt.Errorf("failed to list tools for MCP server %q: %w", mcpServer.Name, err)
			}

			transformedTools := mcp.ApplyToolOverrides(tools, componentEntry.ToolOverrides)
			compositeToolPreviews = append(compositeToolPreviews, transformedTools...)
			continue
		}

		// CatalogEntry-based components still use a temporary server with the supplied config.
		componentID := componentEntry.CatalogEntryID
		if componentID == "" {
			componentID = componentEntry.MCPServerID
		}

		config, ok := configRequest.ComponentConfigs[componentID]
		if !ok {
			// No config provided for this component, skip it
			continue
		}

		// Skip disabled components when generating composite tool previews
		if config.Disabled {
			continue
		}

		server, serverConfig, err := tempServerAndConfig(
			req.Context(),
			req.GPTClient,
			req.Storage,
			entry.Namespace,
			catalogName,
			componentEntry.Manifest,
			config.Config,
			config.URL,
			h.serverURL,
		)
		if err != nil {
			return err
		}

		if serverConfig.Runtime == types.RuntimeRemote {
			oauthURL, err := h.oauthChecker.CheckForMCPAuth(
				req,
				server,
				serverConfig,
				"system",
				server.Name,
				"",
			)
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

		toolPreview, err := h.sessionManager.GenerateToolPreviews(req.Context(), server, serverConfig)
		if err != nil {
			return fmt.Errorf("failed to generate tool preview: %w", err)
		}

		// Apply tool overrides before aggregating
		transformedTools := mcp.ApplyToolOverrides(toolPreview, componentEntry.ToolOverrides)

		compositeToolPreviews = append(compositeToolPreviews, transformedTools...)
	}

	// Set the tool preview on the catalog entry
	entry.Spec.Manifest.ToolPreview = compositeToolPreviews
	if dryRun {
		// Don't update the entry, just return the entry with the new tool set
		return req.Write(ConvertMCPServerCatalogEntry(entry))
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
	return req.Write(ConvertMCPServerCatalogEntry(entry))
}

func (h *MCPCatalogHandler) GenerateToolPreviewsOAuthURL(req api.Context) error {
	var (
		catalogName = req.PathValue("catalog_id")
		workspaceID = req.PathValue("workspace_id")
		entryName   = req.PathValue("entry_id")
		// "dryRun" lets us get the previews for an MCP server without updating its CatalogEntry.
		// This is used when we populate the tools for individual MCP servers when creating a composite CatalogEntry
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

	// Verify entry belongs to the requested scope
	if catalogName != "" && entry.Spec.MCPCatalogName != catalogName {
		return types.NewErrBadRequest("entry does not belong to catalog")
	} else if workspaceID != "" && entry.Spec.PowerUserWorkspaceID != workspaceID {
		return types.NewErrBadRequest("entry does not belong to workspace")
	}

	if !entry.Spec.Editable && !dryRun {
		return types.NewErrBadRequest("entry is not editable")
	}

	// Handle composite servers
	if entry.Spec.Manifest.Runtime == types.RuntimeComposite {
		return h.generateCompositeOAuthURLs(req, entry)
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
	server, serverConfig, err := tempServerAndConfig(req.Context(), req.GPTClient, req.Storage, entry.Namespace, catalogName, entry.Spec.Manifest, configRequest.Config, configRequest.URL, h.serverURL)
	if err != nil {
		return types.NewErrBadRequest("failed to create temporary server and config: %v", err)
	}

	oauthURL, err := h.oauthChecker.CheckForMCPAuth(req, server, serverConfig, "system", server.Name, "")
	if err != nil {
		return types.NewErrBadRequest("failed to check for MCP auth: %v", err)
	}

	return req.Write(map[string]string{"oauthURL": oauthURL})
}

// GenerateComponentToolPreviews generates tool previews for a single component of a composite
// catalog entry using the manifest snapshot embedded in the composite entry. This is used by
// the composite \"Configure Tools\" flow so that previews are based on the composite's
// stored manifest, not on any newer version of the standalone MCP catalog entry.
func (h *MCPCatalogHandler) GenerateComponentToolPreviews(req api.Context) error {
	var (
		catalogName = req.PathValue("catalog_id")
		compositeID = req.PathValue("entry_id")
		componentID = req.PathValue("component_id")
	)

	if catalogName == "" {
		return types.NewErrBadRequest("catalog_id is required")
	}

	// Verify catalog exists
	if err := req.Get(&v1.MCPCatalog{}, catalogName); err != nil {
		return fmt.Errorf("failed to get catalog: %w", err)
	}

	// Load the composite catalog entry
	var composite v1.MCPServerCatalogEntry
	if err := req.Get(&composite, compositeID); err != nil {
		return fmt.Errorf("failed to get composite catalog entry: %w", err)
	}

	if composite.Spec.MCPCatalogName != catalogName {
		return types.NewErrBadRequest("entry does not belong to catalog")
	}

	if composite.Spec.Manifest.Runtime != types.RuntimeComposite {
		return types.NewErrBadRequest("entry is not a composite catalog entry")
	}

	if composite.Spec.Manifest.CompositeConfig == nil {
		return types.NewErrBadRequest("composite entry has no component configuration")
	}

	// Find the referenced component in the composite's configuration.
	var component *types.CatalogComponentServer
	for i := range composite.Spec.Manifest.CompositeConfig.ComponentServers {
		c := &composite.Spec.Manifest.CompositeConfig.ComponentServers[i]
		if c.CatalogEntryID == componentID {
			component = c
			break
		}
	}

	if component == nil {
		return types.NewErrBadRequest("component not found in composite entry")
	}

	// Multi-user components use the multi-user tools API and should not call this endpoint.
	if component.MCPServerID != "" {
		return types.NewErrBadRequest("multi-user server components are not supported by this endpoint")
	}

	// Read configuration from request body
	var configRequest struct {
		Config map[string]string `json:"config"`
		URL    string            `json:"url"`
	}
	if err := req.Read(&configRequest); err != nil {
		return types.NewErrBadRequest("failed to read configuration: %v", err)
	}

	catalogName = composite.Spec.MCPCatalogName
	if catalogName == "" {
		catalogName = composite.Spec.PowerUserWorkspaceID
	}

	// Use the manifest snapshot embedded in the composite entry for this component.
	server, serverConfig, err := tempServerAndConfig(
		req.Context(),
		req.GPTClient,
		req.Storage,
		composite.Namespace,
		catalogName,
		component.Manifest,
		configRequest.Config,
		configRequest.URL,
		h.serverURL,
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

	toolPreviews, err := h.sessionManager.GenerateToolPreviews(req.Context(), server, serverConfig)
	if err != nil {
		return fmt.Errorf("failed to generate tool preview: %w", err)
	}

	// Return the tool previews on a skeleton entry
	// We don't bother adding these to the real entry because:
	// - it may no longer exist
	// - we already have enough information to generate composite tool overrides for the component
	entry := v1.MCPServerCatalogEntry{
		ObjectMeta: metav1.ObjectMeta{
			Name:      component.CatalogEntryID,
			Namespace: composite.Namespace,
		},
		Spec: v1.MCPServerCatalogEntrySpec{
			MCPCatalogName: composite.Spec.MCPCatalogName,
			Manifest:       component.Manifest,
		},
	}
	entry.Spec.Manifest.ToolPreview = toolPreviews

	return req.Write(ConvertMCPServerCatalogEntry(entry))
}

// GenerateComponentToolPreviewsOAuthURL returns an OAuth URL for a single component of a
// composite catalog entry, using the component manifest snapshot embedded in the composite.
func (h *MCPCatalogHandler) GenerateComponentToolPreviewsOAuthURL(req api.Context) error {
	var (
		catalogName = req.PathValue("catalog_id")
		compositeID = req.PathValue("entry_id")
		componentID = req.PathValue("component_id")
	)

	if catalogName == "" {
		return types.NewErrBadRequest("catalog_id is required")
	}

	// Verify catalog exists
	if err := req.Get(&v1.MCPCatalog{}, catalogName); err != nil {
		return fmt.Errorf("failed to get catalog: %w", err)
	}

	// Load the composite catalog entry
	var composite v1.MCPServerCatalogEntry
	if err := req.Get(&composite, compositeID); err != nil {
		return fmt.Errorf("failed to get composite catalog entry: %w", err)
	}

	if composite.Spec.MCPCatalogName != catalogName {
		return types.NewErrBadRequest("entry does not belong to catalog")
	}

	if composite.Spec.Manifest.Runtime != types.RuntimeComposite {
		return types.NewErrBadRequest("entry is not a composite catalog entry")
	}

	if composite.Spec.Manifest.CompositeConfig == nil {
		return types.NewErrBadRequest("composite entry has no component configuration")
	}

	// Find the referenced component in the composite's configuration.
	var component *types.CatalogComponentServer
	for i := range composite.Spec.Manifest.CompositeConfig.ComponentServers {
		c := &composite.Spec.Manifest.CompositeConfig.ComponentServers[i]
		if c.CatalogEntryID == componentID {
			component = c
			break
		}
	}

	if component == nil {
		return types.NewErrBadRequest("component not found in composite entry")
	}

	if component.MCPServerID != "" {
		return types.NewErrBadRequest("multi-user server components are not supported by this endpoint")
	}

	// Read configuration from request body
	var configRequest struct {
		Config map[string]string `json:"config"`
		URL    string            `json:"url"`
	}
	if err := req.Read(&configRequest); err != nil {
		return types.NewErrBadRequest("failed to read configuration: %v", err)
	}

	catalogName = composite.Spec.MCPCatalogName
	if catalogName == "" {
		catalogName = composite.Spec.PowerUserWorkspaceID
	}

	server, serverConfig, err := tempServerAndConfig(
		req.Context(),
		req.GPTClient,
		req.Storage,
		composite.Namespace,
		catalogName,
		component.Manifest,
		configRequest.Config,
		configRequest.URL,
		h.serverURL,
	)
	if err != nil {
		return types.NewErrBadRequest("failed to create temporary server and config: %v", err)
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

func (h *MCPCatalogHandler) generateCompositeOAuthURLs(req api.Context, entry v1.MCPServerCatalogEntry) error {
	// Read configuration from request body (same as generateCompositeToolPreviews)
	var configRequest struct {
		ComponentConfigs map[string]struct {
			Config   map[string]string `json:"config"`
			URL      string            `json:"url"`
			Disabled bool              `json:"disabled"`
		} `json:"componentConfigs"`
	}
	if err := req.Read(&configRequest); err != nil {
		return types.NewErrBadRequest("failed to read configuration: %v", err)
	}

	compositeConfig := entry.Spec.Manifest.CompositeConfig
	if compositeConfig == nil {
		return types.NewErrBadRequest("composite configuration is required")
	}

	// Collect OAuth URLs for each component
	oauthURLs := make(map[string]string)

	catalogName := entry.Spec.MCPCatalogName
	if catalogName == "" {
		catalogName = entry.Spec.PowerUserWorkspaceID
	}

	for _, componentEntry := range compositeConfig.ComponentServers {
		if componentEntry.MCPServerID != "" {
			// Skip multi-user server components when checking for OAuth URLs
			continue
		}

		componentID := componentEntry.CatalogEntryID
		if componentID == "" {
			componentID = componentEntry.MCPServerID
		}

		config, ok := configRequest.ComponentConfigs[componentID]
		if !ok {
			// No config provided for this component, skip
			continue
		}

		// Skip disabled components when generating OAuth URLs
		if config.Disabled {
			continue
		}

		// Only check OAuth for remote components
		if componentEntry.Manifest.Runtime != types.RuntimeRemote {
			delete(oauthURLs, componentID)
			continue
		}

		server, serverConfig, err := tempServerAndConfig(
			req.Context(),
			req.GPTClient,
			req.Storage,
			entry.Namespace,
			catalogName,
			componentEntry.Manifest,
			config.Config,
			config.URL,
			h.serverURL,
		)
		if err != nil {
			// If we can't create server config, skip this component
			delete(oauthURLs, componentID)
			continue
		}

		// Check if OAuth is required
		oauthURL, err := h.oauthChecker.CheckForMCPAuth(req, server, serverConfig, "system", server.Name, "")
		if err != nil || oauthURL == "" {
			// On error, assume no OAuth needed
			delete(oauthURLs, componentID)
			continue
		}

		oauthURLs[componentID] = oauthURL
	}

	// Return map of component IDs to OAuth URLs
	return req.Write(oauthURLs)
}

func tempServerAndConfig(ctx context.Context, gptClient *gptscript.GPTScript, client client.Client, namespace, catalogName string, entryManifest types.MCPServerCatalogEntryManifest, config map[string]string, url, baseURL string) (v1.MCPServer, mcp.ServerConfig, error) {
	// Convert catalog entry to server manifest
	serverManifest, err := types.MapCatalogEntryToServer(entryManifest, url, false)
	if err != nil {
		return v1.MCPServer{}, mcp.ServerConfig{}, fmt.Errorf("failed to convert catalog entry to server config: %w", err)
	}

	// Create temporary MCPServer object to use existing conversion logic
	tempName := "temp-preview-" + hash.Digest(serverManifest)[:32]
	tempMCPServer := v1.MCPServer{
		ObjectMeta: metav1.ObjectMeta{
			Name: tempName,
		},
		Spec: v1.MCPServerSpec{
			Manifest: serverManifest,
		},
	}

	// Generate a temporary OAuth Client
	clientID := system.OAuthClientPrefix + strings.ToLower(rand.Text())
	clientSecret := strings.ToLower(rand.Text() + rand.Text())
	hashedClientSecretHash, err := bcrypt.GenerateFromPassword([]byte(clientSecret), bcrypt.DefaultCost)
	if err != nil {
		return v1.MCPServer{}, mcp.ServerConfig{}, fmt.Errorf("failed to hash client secret: %w", err)
	}

	tokenExchangeEnv := map[string]string{
		"TOKEN_EXCHANGE_CLIENT_ID":     fmt.Sprintf("%s:%s", namespace, clientID),
		"TOKEN_EXCHANGE_CLIENT_SECRET": clientSecret,
	}
	if err := gptClient.CreateCredential(ctx, gptscript.Credential{
		Context:  tempMCPServer.Name,
		ToolName: tempMCPServer.Name,
		Type:     gptscript.CredentialTypeTool,
		Env:      tokenExchangeEnv,
	}); err != nil {
		return v1.MCPServer{}, mcp.ServerConfig{}, fmt.Errorf("failed to create credential: %w", err)
	}

	oauthClient := v1.OAuthClient{
		ObjectMeta: metav1.ObjectMeta{
			Name:       clientID,
			Namespace:  namespace,
			Finalizers: []string{v1.OAuthClientFinalizer},
		},
		Spec: v1.OAuthClientSpec{
			Manifest: types.OAuthClientManifest{
				GrantTypes: []string{"urn:ietf:params:oauth:grant-type:token-exchange"},
			},
			ClientSecretHash: hashedClientSecretHash,
			Ephemeral:        true,
		},
	}

	if err := client.Create(ctx, &oauthClient); err != nil {
		return v1.MCPServer{}, mcp.ServerConfig{}, fmt.Errorf("failed to create OAuth client: %w", err)
	}

	serverConfig, missingFields, err := mcp.ServerToServerConfig(tempMCPServer, tempMCPServer.ValidConnectURLs(baseURL), baseURL, "temp", "temp", catalogName, config, tokenExchangeEnv)
	if err != nil {
		return v1.MCPServer{}, mcp.ServerConfig{}, fmt.Errorf("failed to create server config: %w", err)
	}

	if len(missingFields) > 0 {
		return v1.MCPServer{}, mcp.ServerConfig{}, types.NewErrBadRequest("missing required configuration fields: %v", missingFields)
	}

	return tempMCPServer, serverConfig, nil
}

// ListCategoriesForCatalog returns all unique categories from entries in a catalog
func (h *MCPCatalogHandler) ListCategoriesForCatalog(req api.Context) error {
	catalogName := req.PathValue("catalog_id")

	var list v1.MCPServerCatalogEntryList
	if err := req.List(&list, client.MatchingFields{
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

func convertMCPCatalog(catalog v1.MCPCatalog) types.MCPCatalog {
	return types.MCPCatalog{
		Metadata: MetadataFrom(&catalog),
		MCPCatalogManifest: types.MCPCatalogManifest{
			DisplayName: catalog.Spec.DisplayName,
			SourceURLs:  catalog.Spec.SourceURLs,
		},
		LastSynced: *types.NewTime(catalog.Status.LastSyncTime.Time),
		SyncErrors: catalog.Status.SyncErrors,
		IsSyncing:  catalog.Status.IsSyncing || catalog.Annotations[v1.MCPCatalogSyncAnnotation] == "true",
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

func (h *MCPCatalogHandler) populateComponentManifests(req api.Context, manifest *types.MCPServerCatalogEntryManifest, catalogName, workspaceID string) error {
	// For each component server, fetch its catalog entry and populate the manifest
	var componentServers []types.CatalogComponentServer
	for i := range manifest.CompositeConfig.ComponentServers {
		var (
			component                    = &manifest.CompositeConfig.ComponentServers[i]
			hasCatalogEntry, hasServerID = component.CatalogEntryID != "", component.MCPServerID != ""
		)
		// Validate that exactly one of CatalogEntryID or MCPServerID is set
		if hasCatalogEntry && hasServerID {
			return types.NewErrBadRequest("component cannot have both catalogEntryID and mcpServerID set")
		}
		if !hasCatalogEntry && !hasServerID {
			return types.NewErrBadRequest("component must have either catalogEntryID or mcpServerID set")
		}

		if component.MCPServerID != "" {
			// Multi-user server component
			var server v1.MCPServer
			if err := req.Get(&server, component.MCPServerID); err != nil {
				if apierrors.IsNotFound(err) {
					// Skip components referencing servers that no longer exist
					continue
				}
				return types.NewErrBadRequest("failed to get multi-user server %s: %v", component.MCPServerID, err)
			}

			// Verify this is actually a multi-user server
			if server.Spec.MCPCatalogID == "" && server.Spec.PowerUserWorkspaceID == "" {
				return types.NewErrBadRequest("server %s is not a multi-user server", component.MCPServerID)
			}

			// Verify the server belongs to the same catalog
			if catalogName != "" && server.Spec.MCPCatalogID != catalogName {
				return types.NewErrBadRequest("multi-user server %s belongs to catalog %s, not %s", component.MCPServerID, server.Spec.MCPCatalogID, catalogName)
			}

			// Populate the manifest snapshot from the multi-user server
			component.Manifest = convertServerManifestToCatalogManifest(server.Spec.Manifest)
			// Keep this component
			componentServers = append(componentServers, *component)
		} else {
			// Catalog entry component
			var entry v1.MCPServerCatalogEntry
			if err := req.Get(&entry, component.CatalogEntryID); err != nil {
				if apierrors.IsNotFound(err) {
					// Skip components referencing catalog entries that no longer exist
					continue
				}
				return types.NewErrBadRequest("failed to get component catalog entry %s: %v", component.CatalogEntryID, err)
			}

			// Verify the component entry belongs to the same scope
			if catalogName != "" && entry.Spec.MCPCatalogName != catalogName {
				return types.NewErrBadRequest("component entry %s does not belong to catalog %s", component.CatalogEntryID, catalogName)
			}
			if workspaceID != "" && entry.Spec.PowerUserWorkspaceID != workspaceID {
				return types.NewErrBadRequest("component entry %s does not belong to workspace %s", component.CatalogEntryID, workspaceID)
			}

			// Reject remote entries with static OAuth from being included in composites
			if entry.Spec.Manifest.Runtime == types.RuntimeRemote &&
				entry.Spec.Manifest.RemoteConfig != nil &&
				entry.Spec.Manifest.RemoteConfig.StaticOAuthRequired {
				return types.NewErrBadRequest("remote catalog entry %s with static OAuth cannot be included in a composite server", component.CatalogEntryID)
			}

			// Populate the manifest
			component.Manifest = entry.Spec.Manifest
			// Keep this component
			componentServers = append(componentServers, *component)
		}
	}

	// Replace with filtered component list
	manifest.CompositeConfig.ComponentServers = componentServers

	return nil
}

// convertServerManifestToCatalogManifest converts an MCPServerManifest to MCPServerCatalogEntryManifest
func convertServerManifestToCatalogManifest(serverManifest types.MCPServerManifest) types.MCPServerCatalogEntryManifest {
	catalogManifest := types.MCPServerCatalogEntryManifest{
		Metadata:         serverManifest.Metadata,
		Name:             serverManifest.Name,
		ShortDescription: serverManifest.ShortDescription,
		Description:      serverManifest.Description,
		Icon:             serverManifest.Icon,
		Runtime:          serverManifest.Runtime,
		Env:              serverManifest.Env,
		ToolPreview:      serverManifest.ToolPreview,
	}

	// Convert runtime-specific configs
	switch serverManifest.Runtime {
	case types.RuntimeUVX:
		catalogManifest.UVXConfig = serverManifest.UVXConfig
	case types.RuntimeNPX:
		catalogManifest.NPXConfig = serverManifest.NPXConfig
	case types.RuntimeContainerized:
		catalogManifest.ContainerizedConfig = serverManifest.ContainerizedConfig
	case types.RuntimeRemote:
		if serverManifest.RemoteConfig != nil {
			catalogManifest.RemoteConfig = &types.RemoteCatalogConfig{
				FixedURL: serverManifest.RemoteConfig.URL,
				Headers:  serverManifest.RemoteConfig.Headers,
			}
		}
	case types.RuntimeComposite:
		if serverManifest.CompositeConfig != nil {
			// TODO(njhale): Figure out if this can be removed
			// Convert CompositeRuntimeConfig to CompositeCatalogConfig
			componentServers := make([]types.CatalogComponentServer, len(serverManifest.CompositeConfig.ComponentServers))
			for i, comp := range serverManifest.CompositeConfig.ComponentServers {
				componentServers[i] = types.CatalogComponentServer{
					CatalogEntryID: comp.CatalogEntryID,
					MCPServerID:    comp.MCPServerID,
					Manifest:       convertServerManifestToCatalogManifest(comp.Manifest),
					ToolOverrides:  comp.ToolOverrides,
				}
			}
			catalogManifest.CompositeConfig = &types.CompositeCatalogConfig{
				ComponentServers: componentServers,
			}
		}
	}

	return catalogManifest
}

// RefreshCompositeComponents refreshes the component snapshots in a composite catalog entry
func (h *MCPCatalogHandler) RefreshCompositeComponents(req api.Context) error {
	catalogName := req.PathValue("catalog_id")
	entryName := req.PathValue("entry_id")

	// Verify the catalog exists
	if err := req.Get(&v1.MCPCatalog{}, catalogName); err != nil {
		return fmt.Errorf("failed to get catalog: %w", err)
	}

	var entry v1.MCPServerCatalogEntry
	if err := req.Get(&entry, entryName); err != nil {
		return fmt.Errorf("failed to get entry: %w", err)
	}

	// Verify entry belongs to the catalog
	if entry.Spec.MCPCatalogName != catalogName {
		return types.NewErrBadRequest("entry does not belong to catalog")
	}

	// Composites are not supported in power user workspaces yet; ensure this entry is not workspace-scoped
	if entry.Spec.PowerUserWorkspaceID != "" {
		return types.NewErrBadRequest("composite entries in power user workspaces are not supported")
	}

	if entry.Spec.Manifest.Runtime != types.RuntimeComposite {
		return types.NewErrBadRequest("entry is not a composite catalog entry")
	}

	if entry.Spec.Manifest.CompositeConfig == nil {
		return types.NewErrBadRequest("composite entry has no component configuration")
	}

	// Refresh component manifests from their current sources
	// This will populate the entry.Spec.Manifest.CompositeConfig.ComponentServers with the current manifests
	// and remove components that no longer exist
	if err := h.populateComponentManifests(req, &entry.Spec.Manifest, catalogName, ""); err != nil {
		return err
	}

	// Validate the refreshed manifest to ensure it's still valid
	if err := validation.ValidateCatalogEntryManifest(entry.Spec.Manifest); err != nil {
		return types.NewErrBadRequest("failed to validate entry manifest: %v", err)
	}

	// Update the entry
	if err := req.Update(&entry); err != nil {
		return fmt.Errorf("failed to update entry: %w", err)
	}

	return req.Write(ConvertMCPServerCatalogEntry(entry))
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

// validateAuthorizationServerURL validates that the URL is a valid HTTPS URL.
func validateAuthorizationServerURL(rawURL string) error {
	if rawURL == "" {
		return nil
	}

	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return types.NewErrBadRequest("invalid authorization server URL format: %v", err)
	}

	if parsedURL.Scheme != "https" {
		return types.NewErrBadRequest("authorization server URL must use HTTPS scheme")
	}

	if parsedURL.Host == "" {
		return types.NewErrBadRequest("authorization server URL must have a host")
	}

	return nil
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

	// Verify entry belongs to the requested scope
	if catalogName != "" && entry.Spec.MCPCatalogName != catalogName {
		return nil, types.NewErrBadRequest("entry does not belong to catalog")
	} else if workspaceID != "" && entry.Spec.PowerUserWorkspaceID != workspaceID {
		return nil, types.NewErrBadRequest("entry does not belong to workspace")
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
	cred, err := req.GPTClient.RevealCredential(req.Context(), []string{credName}, "oauth")
	configured := err == nil

	var (
		clientID      string
		authServerURL string
	)
	if configured {
		clientID = cred.Env["CLIENT_ID"]
		// Use the stored authorization server URL if present
		if url := cred.Env["AUTHORIZATION_SERVER_URL"]; url != "" {
			authServerURL = url
		}
	}
	// Fall back to catalog entry default if no override was stored
	if authServerURL == "" {
		authServerURL = entry.Spec.Manifest.RemoteConfig.AuthorizationServerURL
	}

	return req.Write(types.MCPServerOAuthCredentialStatus{
		Configured:             configured,
		ClientID:               clientID,
		AuthorizationServerURL: authServerURL,
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
	existingCred, err := req.GPTClient.RevealCredential(req.Context(), []string{credName}, "oauth")
	credentialsExist := err == nil

	var clientID, clientSecret, authServerURL string

	if credentialsExist {
		// Update mode: Only authorization server URL can be updated
		// Return error if client tries to update credentials
		if credReq.ClientID != "" || credReq.ClientSecret != "" {
			return types.NewErrBadRequest("cannot update clientID or clientSecret; delete and recreate credentials to change them")
		}

		// Preserve existing client ID and secret
		clientID = existingCred.Env["CLIENT_ID"]
		clientSecret = existingCred.Env["CLIENT_SECRET"]

		// Determine the authorization server URL
		authServerURL = credReq.AuthorizationServerURL
		if authServerURL == "" {
			// If not provided in request, keep existing value or use entry default
			authServerURL = existingCred.Env["AUTHORIZATION_SERVER_URL"]
			if authServerURL == "" {
				authServerURL = entry.Spec.Manifest.RemoteConfig.AuthorizationServerURL
			}
		}
	} else {
		// Initial setup mode: All fields are required
		// Trim whitespace before validation
		trimmedClientID := strings.TrimSpace(credReq.ClientID)
		trimmedClientSecret := strings.TrimSpace(credReq.ClientSecret)
		if trimmedClientID == "" || trimmedClientSecret == "" {
			return types.NewErrBadRequest("clientID and clientSecret are required")
		}

		clientID = trimmedClientID
		clientSecret = trimmedClientSecret

		// Determine the effective authorization server URL
		authServerURL = credReq.AuthorizationServerURL
		if authServerURL == "" {
			authServerURL = entry.Spec.Manifest.RemoteConfig.AuthorizationServerURL
		}
		if authServerURL == "" {
			return types.NewErrBadRequest("authorizationServerURL is required because the catalog entry does not have a default")
		}
	}

	// Validate the authorization server URL
	if err := validateAuthorizationServerURL(authServerURL); err != nil {
		return err
	}

	// Store new credential
	cred := gptscript.Credential{
		Context:  credName,
		ToolName: "oauth",
		Type:     gptscript.CredentialTypeTool,
		Env: map[string]string{
			"CLIENT_ID":                clientID,
			"CLIENT_SECRET":            clientSecret,
			"AUTHORIZATION_SERVER_URL": authServerURL,
		},
	}
	if err := req.GPTClient.CreateCredential(req.Context(), cred); err != nil {
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
		Configured:             true,
		ClientID:               clientID,
		AuthorizationServerURL: authServerURL,
	})
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

	credName := system.MCPOAuthCredentialName(entry.Name)
	if err := req.GPTClient.DeleteCredential(req.Context(), credName, "oauth"); err != nil {
		var notFound gptscript.ErrNotFound
		if !errors.As(err, &notFound) {
			return err
		}
	}

	// Trigger reconciliation to update the status
	if entry.Annotations == nil {
		entry.Annotations = make(map[string]string, 1)
	}
	entry.Annotations[v1.MCPServerCatalogEntrySyncAnnotation] = "true"
	if err := req.Update(entry); err != nil {
		return fmt.Errorf("failed to trigger reconciliation: %w", err)
	}

	return req.Write(map[string]bool{"deleted": true})
}
