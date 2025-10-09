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
			entries = append(entries, convertMCPServerCatalogEntry(entry))
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

		items = append(items, convertMCPServer(server, cred.Env, h.serverURL, slug))
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

		items = append(items, convertMCPServer(server, cred.Env, h.serverURL, slug))
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

	return req.Write(convertMCPServer(server, cred.Env, h.serverURL, slug))
}

// GenerateToolPreviews launches a temporary instance of an MCP server from a catalog entry
// to generate tool preview data, then cleans up the instance.
func (h *MCPCatalogHandler) GenerateToolPreviews(req api.Context) error {
	var (
		catalogName = req.PathValue("catalog_id")
		workspaceID = req.PathValue("workspace_id")
		entryName   = req.PathValue("entry_id")
		preview     = req.Request.URL.Query().Get("preview") == "true"
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

	if !preview && !entry.Spec.Editable {
		return types.NewErrBadRequest("entry is not editable")
	}

	// Read configuration from request body
	var configRequest struct {
		Config map[string]string `json:"config"`
		URL    string            `json:"url"`
	}
	if err := req.Read(&configRequest); err != nil {
		return types.NewErrBadRequest("failed to read configuration: %v", err)
	}

	var toolPreviews []types.MCPServerTool

	// Handle composite servers differently - they need to aggregate tools from components
	if entry.Spec.Manifest.Runtime == types.RuntimeComposite {
		// Fetch all component catalog entries
		if entry.Spec.Manifest.CompositeConfig == nil || len(entry.Spec.Manifest.CompositeConfig.ComponentCatalogEntries) == 0 {
			return types.NewErrBadRequest("composite server has no component catalog entries configured")
		}

		// Build tool mapping index for quick lookup
		compositeConfig := entry.Spec.Manifest.CompositeConfig
		toolMap := make(map[string]types.CompositeToolMapping)
		hasAnyMappings := len(compositeConfig.ToolMappings) > 0
		for _, tm := range compositeConfig.ToolMappings {
			key := tm.ComponentEntryName + "\x00" + tm.ComponentTool
			toolMap[key] = tm
		}

		// Process each component catalog entry
		for _, componentEntryID := range compositeConfig.ComponentCatalogEntries {
			var componentEntry v1.MCPServerCatalogEntry
			if err := req.Get(&componentEntry, componentEntryID); err != nil {
				return fmt.Errorf("failed to get component catalog entry %s: %w", componentEntryID, err)
			}

			// Skip nested composite servers (not yet supported)
			if componentEntry.Spec.Manifest.Runtime == types.RuntimeComposite {
				continue
			}

			// Extract component-specific config from prefixed keys
			var (
				componentConfig = make(map[string]string)
				prefix          = fmt.Sprintf("%s_", componentEntry.Name)
				componentURL    string
			)
			for key, value := range configRequest.Config {
				if strings.HasPrefix(key, prefix) {
					// Remove prefix to get original env var name
					originalKey := strings.TrimPrefix(key, prefix)
					componentConfig[originalKey] = value
				}
			}
			// Check for component-specific URL (prefixed)
			if urlValue, ok := configRequest.Config[prefix+"URL"]; ok {
				componentURL = urlValue
			}

			// Generate tool previews for this component using existing logic
			componentServer, componentServerConfig, err := tempServerAndConfig(componentEntry, componentConfig, componentURL)
			if err != nil {
				// Skip components that can't be configured without user input
				continue
			}

			// Check for OAuth requirement for remote components
			if componentServerConfig.Runtime == types.RuntimeRemote {
				oauthURL, err := h.oauthChecker.CheckForMCPAuth(req.Context(), componentServer, componentServerConfig, "system", componentServer.Name, "")
				if err != nil {
					return fmt.Errorf("failed to check for MCP auth on component %s: %w", componentEntry.Spec.Manifest.Name, err)
				}

				if oauthURL != "" {
					return types.NewErrBadRequest("Component '%s' requires OAuth authentication", componentEntry.Spec.Manifest.Name)
				}

				defer func() {
					_ = h.gatewayClient.DeleteMCPOAuthToken(context.Background(), "system", componentServer.Name)
				}()
			}

			componentTools, err := h.sessionManager.GenerateToolPreviews(req.Context(), componentServer, componentServerConfig)
			if err != nil {
				// Log but continue with other components
				continue
			}

			// Apply mappings and prefixing to component tools
			componentPrefix := sanitizeToolPrefix(componentEntry.Spec.Manifest.Name)
			entryKey := componentEntry.Name

			for _, tool := range componentTools {
				lookupKey := entryKey + "\x00" + tool.Name
				if tm, ok := toolMap[lookupKey]; ok {
					// Tool has explicit mapping
					if !tm.Enabled {
						continue // Skip disabled tools
					}
					// Use exposed name if provided, otherwise use original tool name
					exposedName := tm.ExposedTool
					if exposedName == "" {
						exposedName = tool.Name
					}
					// Always prefix with component prefix
					tool.Name = buildCompositedToolName(componentPrefix, sanitizeToolPrefix(exposedName))
					if tm.ExposedDescription != "" {
						tool.Description = tm.ExposedDescription
					}
					toolPreviews = append(toolPreviews, tool)
				} else if !hasAnyMappings {
					// No mappings configured - include everything with prefix to avoid collisions
					tool.Name = buildCompositedToolName(componentPrefix, sanitizeToolPrefix(tool.Name))
					toolPreviews = append(toolPreviews, tool)
				}
				// If mappings exist but this tool isn't mapped - exclude it (allowlist behavior)
			}
		}
	} else {
		server, serverConfig, err := tempServerAndConfig(entry, configRequest.Config, configRequest.URL)
		if err != nil {
			return types.NewErrBadRequest("failed to create temporary server and config: %v", err)
		}

		if serverConfig.Runtime == types.RuntimeRemote {
			oauthURL, err := h.oauthChecker.CheckForMCPAuth(req.Context(), server, serverConfig, "system", server.Name, "")
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
		toolPreviews, err = h.sessionManager.GenerateToolPreviews(req.Context(), server, serverConfig)
		if err != nil {
			return fmt.Errorf("failed to launch temporary instance: %w", err)
		}
	}

	entry.Spec.Manifest.ToolPreview = toolPreviews
	if preview {
		// Return the entry with updated tool previews without persisting it
		entry.Spec.Manifest.ToolPreview = toolPreviews
		return req.Write(convertMCPServerCatalogEntry(entry))
	}

	// Persist tool previews on the catalog entry (legacy behavior)
	if err := req.Update(&entry); err != nil {
		return fmt.Errorf("failed to update catalog entry: %w", err)
	}

	now := metav1.Now()
	entry.Status.ToolPreviewsLastGenerated = &now
	if err := req.Storage.Status().Update(req.Context(), &entry); err != nil {
		return fmt.Errorf("failed to update catalog entry: %w", err)
	}

	return req.Write(convertMCPServerCatalogEntry(entry))
}

func (h *MCPCatalogHandler) GenerateToolPreviewsOAuthURL(req api.Context) error {
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

	if !entry.Spec.Editable {
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

	server, serverConfig, err := tempServerAndConfig(entry, configRequest.Config, configRequest.URL)
	if err != nil {
		return types.NewErrBadRequest("failed to create temporary server and config: %v", err)
	}

	oauthURL, err := h.oauthChecker.CheckForMCPAuth(req.Context(), server, serverConfig, "system", server.Name, "")
	if err != nil {
		return types.NewErrBadRequest("failed to check for MCP auth: %v", err)
	}

	return req.Write(map[string]string{"oauthURL": oauthURL})
}

func tempServerAndConfig(entry v1.MCPServerCatalogEntry, config map[string]string, url string) (v1.MCPServer, mcp.ServerConfig, error) {
	// Convert catalog entry to server manifest
	serverManifest, err := types.MapCatalogEntryToServer(entry.Spec.Manifest, url)
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
		IsSyncing:  catalog.Status.IsSyncing,
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

// sanitizeToolPrefix converts a server manifest name to a safe tool prefix
// e.g., "Component 1" -> "component_1"
func sanitizeToolPrefix(name string) string {
	prefix := strings.ToLower(name)
	prefix = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			return r
		}
		return '_'
	}, prefix)
	for strings.Contains(prefix, "__") {
		prefix = strings.ReplaceAll(prefix, "__", "_")
	}
	prefix = strings.Trim(prefix, "_")
	return prefix
}

// buildCompositedToolName creates the final tool name with component prefix
// e.g., prefix="component_1", exposedName="add_stuff" -> "component_1_add_stuff"
func buildCompositedToolName(componentPrefix, exposedName string) string {
	return componentPrefix + "_" + exposedName
}
