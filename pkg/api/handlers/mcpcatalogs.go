package handlers

import (
	"context"
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
			entries = append(entries, convertMCPServerCatalogEntryWithWorkspace(entry, workspaceID, powerUserID))
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
			entries = append(entries, convertMCPServerCatalogEntryWithWorkspace(entry, workspaceID, powerUserID))
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
		return req.Write(convertMCPServerCatalogEntryWithWorkspace(entry, workspaceID, workspace.Spec.UserID))
	}

	return req.Write(convertMCPServerCatalogEntry(entry))
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

	return req.Write(convertMCPServerCatalogEntry(entry))
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

	// Handle composite catalog entries
	if manifest.Runtime == types.RuntimeComposite && manifest.CompositeConfig != nil {
		if err := h.populateComponentManifests(req, &manifest, catalogName, workspaceID); err != nil {
			return err
		}
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

	return req.Write(convertMCPServerCatalogEntry(entry))
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

		slug, err := slugForMCPServer(req.Context(), req.Storage, server, server.Spec.UserID, catalogName, "")
		if err != nil {
			return fmt.Errorf("failed to generate slug: %w", err)
		}

		items = append(items, convertMCPServer(req, server, cred.Env, h.serverURL, slug))
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

		slug, err := slugForMCPServer(req.Context(), req.Storage, server, server.Spec.UserID, catalogName, "")
		if err != nil {
			return fmt.Errorf("failed to generate slug: %w", err)
		}

		items = append(items, convertMCPServer(req, server, cred.Env, h.serverURL, slug))
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

		slug, err := slugForMCPServer(req.Context(), req.Storage, server, server.Spec.UserID, catalogName, "")
		if err != nil {
			return fmt.Errorf("failed to generate slug: %w", err)
		}

		items = append(items, convertMCPServer(req, server, cred.Env, h.serverURL, slug))
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

	slug, err := slugForMCPServer(req.Context(), req.Storage, server, server.Spec.UserID, catalogName, "")
	if err != nil {
		return fmt.Errorf("failed to generate slug: %w", err)
	}

	return req.Write(convertMCPServer(req, server, cred.Env, h.serverURL, slug))
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

	server, serverConfig, err := tempServerAndConfig(entry.Spec.Manifest, configRequest.Config, configRequest.URL)
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
			_ = h.gatewayClient.DeleteMCPOAuthToken(context.Background(), "system", server.Name)
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
		return req.Write(convertMCPServerCatalogEntry(entry))
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
	return req.Write(convertMCPServerCatalogEntry(entry))
}

func (h *MCPCatalogHandler) generateCompositeToolPreviews(req api.Context, entry v1.MCPServerCatalogEntry, dryRun bool) error {
	// Read configuration from request body
	var configRequest struct {
		ComponentConfigs map[string]struct {
			Config map[string]string `json:"config"`
			URL    string            `json:"url"`
			Skip   bool              `json:"skip"`
		} `json:"componentConfigs"`
	}
	if err := req.Read(&configRequest); err != nil {
		return types.NewErrBadRequest("failed to read configuration: %v", err)
	}

	compositeConfig := entry.Spec.Manifest.CompositeConfig
	if compositeConfig == nil {
		return types.NewErrBadRequest("composite configuration is required")
	}

	compositeToolPreviews := make([]types.MCPServerTool, 0, len(compositeConfig.ComponentServers))
	for _, componentEntry := range compositeConfig.ComponentServers {
		config, ok := configRequest.ComponentConfigs[componentEntry.CatalogEntryID]
		if ok && config.Skip {
			// Skip configuring component if requested
			continue
		}

		server, serverConfig, err := tempServerAndConfig(componentEntry.Manifest, config.Config, config.URL)
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

			defer func() {
				_ = h.gatewayClient.DeleteMCPOAuthToken(context.Background(), "system", server.Name)
			}()
		}

		toolPreview, err := h.sessionManager.GenerateToolPreviews(req.Context(), server, serverConfig)
		if err != nil {
			return fmt.Errorf("failed to generate tool preview: %w", err)
		}

		compositeToolPreviews = append(compositeToolPreviews, toolPreview...)
	}

	// Set the tool preview on the catalog entry
	entry.Spec.Manifest.ToolPreview = compositeToolPreviews
	if dryRun {
		// Don't update the entry, just return the entry with the new tool set
		return req.Write(convertMCPServerCatalogEntry(entry))
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
	return req.Write(convertMCPServerCatalogEntry(entry))
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

	server, serverConfig, err := tempServerAndConfig(entry.Spec.Manifest, configRequest.Config, configRequest.URL)
	if err != nil {
		return types.NewErrBadRequest("failed to create temporary server and config: %v", err)
	}

	oauthURL, err := h.oauthChecker.CheckForMCPAuth(req, server, serverConfig, "system", server.Name, "")
	if err != nil {
		return types.NewErrBadRequest("failed to check for MCP auth: %v", err)
	}

	return req.Write(map[string]string{"oauthURL": oauthURL})
}

func tempServerAndConfig(entryManifest types.MCPServerCatalogEntryManifest, config map[string]string, url string) (v1.MCPServer, mcp.ServerConfig, error) {
	// Convert catalog entry to server manifest
	serverManifest, err := types.MapCatalogEntryToServer(entryManifest, url)
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

	serverConfig, missingFields, err := mcp.ServerToServerConfig(tempMCPServer, "temp", config)
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
			categoryList := strings.Split(categories, ",")
			for _, category := range categoryList {
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
		} else {
			// Catalog entry component
			var entry v1.MCPServerCatalogEntry
			if err := req.Get(&entry, component.CatalogEntryID); err != nil {
				return types.NewErrBadRequest("failed to get component catalog entry %s: %v", component.CatalogEntryID, err)
			}

			// Verify the component entry belongs to the same scope
			if catalogName != "" && entry.Spec.MCPCatalogName != catalogName {
				return types.NewErrBadRequest("component entry %s does not belong to catalog %s", component.CatalogEntryID, catalogName)
			}
			if workspaceID != "" && entry.Spec.PowerUserWorkspaceID != workspaceID {
				return types.NewErrBadRequest("component entry %s does not belong to workspace %s", component.CatalogEntryID, workspaceID)
			}

			// Populate the manifest
			component.Manifest = entry.Spec.Manifest
		}
	}

	return nil
}

// convertServerManifestToCatalogManifest converts an MCPServerManifest to MCPServerCatalogEntryManifest
func convertServerManifestToCatalogManifest(serverManifest types.MCPServerManifest) types.MCPServerCatalogEntryManifest {
	catalogManifest := types.MCPServerCatalogEntryManifest{
		Metadata:    serverManifest.Metadata,
		Name:        serverManifest.Name,
		Description: serverManifest.Description,
		Icon:        serverManifest.Icon,
		Runtime:     serverManifest.Runtime,
		Env:         serverManifest.Env,
		ToolPreview: serverManifest.ToolPreview,
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
			// Convert CompositeRuntimeConfig to CompositeCatalogConfig
			componentServers := make([]types.CatalogComponentServer, len(serverManifest.CompositeConfig.ComponentServers))
			for i, comp := range serverManifest.CompositeConfig.ComponentServers {
				componentServers[i] = types.CatalogComponentServer{
					CatalogEntryID: comp.CatalogEntryID,
					MCPServerID:    comp.MCPServerID,
					Manifest:       convertServerManifestToCatalogManifest(comp.Manifest),
					ToolOverrides:  comp.ToolOverrides,
					Disabled:       comp.Disabled,
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

	if entry.Spec.Manifest.Runtime != types.RuntimeComposite {
		return types.NewErrBadRequest("entry is not a composite catalog entry")
	}

	if entry.Spec.Manifest.CompositeConfig == nil {
		return types.NewErrBadRequest("composite entry has no component configuration")
	}

	// Store old manifests to compare for changes
	oldManifests := make(map[int]string)
	for i, component := range entry.Spec.Manifest.CompositeConfig.ComponentServers {
		oldManifests[i] = hash.Digest(component.Manifest)
	}

	// Refresh component manifests from their current sources
	if err := h.populateComponentManifests(req, &entry.Spec.Manifest, catalogName, ""); err != nil {
		return err
	}

	// Clear toolOverrides for components whose manifests changed
	for i := range entry.Spec.Manifest.CompositeConfig.ComponentServers {
		component := &entry.Spec.Manifest.CompositeConfig.ComponentServers[i]
		oldHash := oldManifests[i]
		newHash := hash.Digest(component.Manifest)

		if oldHash != newHash {
			component.ToolOverrides = nil
		}
	}

	// Update the entry
	if err := req.Update(&entry); err != nil {
		return fmt.Errorf("failed to update entry: %w", err)
	}

	return req.Write(convertMCPServerCatalogEntry(entry))
}
