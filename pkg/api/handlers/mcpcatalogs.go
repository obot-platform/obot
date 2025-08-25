package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/gptscript-ai/gptscript/pkg/hash"
	"github.com/obot-platform/nah/pkg/name"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	gclient "github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/obot-platform/obot/pkg/validation"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type MCPCatalogHandler struct {
	defaultCatalogPath string
	serverURL          string
	sessionManager     *mcp.SessionManager
	oauthChecker       MCPOAuthChecker
	gatewayClient      *gclient.Client
}

func NewMCPCatalogHandler(defaultCatalogPath string, serverURL string, sessionManager *mcp.SessionManager, oauthChecker MCPOAuthChecker, gatewayClient *gclient.Client) *MCPCatalogHandler {
	return &MCPCatalogHandler{
		defaultCatalogPath: defaultCatalogPath,
		serverURL:          serverURL,
		sessionManager:     sessionManager,
		oauthChecker:       oauthChecker,
		gatewayClient:      gatewayClient,
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

// ListEntriesForCatalog lists all entries for a catalog.
func (h *MCPCatalogHandler) ListEntriesForCatalog(req api.Context) error {
	catalogName := req.PathValue("catalog_id")

	var list v1.MCPServerCatalogEntryList
	if err := req.List(&list, client.MatchingFields{
		"spec.mcpCatalogName": catalogName,
	}); err != nil {
		return fmt.Errorf("failed to list entries: %w", err)
	}

	var items []types.MCPServerCatalogEntry
	for _, entry := range list.Items {
		items = append(items, convertMCPServerCatalogEntry(entry))
	}

	return req.Write(types.MCPServerCatalogEntryList{Items: items})
}

// GetEntry returns a specific entry from a catalog.
func (h *MCPCatalogHandler) GetEntry(req api.Context) error {
	catalogName := req.PathValue("catalog_id")
	entryName := req.PathValue("entry_id")

	var entry v1.MCPServerCatalogEntry
	if err := req.Get(&entry, entryName); err != nil {
		return fmt.Errorf("failed to get entry: %w", err)
	}

	if entry.Spec.MCPCatalogName != catalogName {
		return types.NewErrBadRequest("entry does not belong to catalog")
	}

	return req.Write(convertMCPServerCatalogEntry(entry))
}

// CreateEntry creates a new entry for a catalog.
func (h *MCPCatalogHandler) CreateEntry(req api.Context) error {
	catalogName := req.PathValue("catalog_id")

	var catalog v1.MCPCatalog
	if err := req.Get(&catalog, catalogName); err != nil {
		return fmt.Errorf("failed to get catalog: %w", err)
	}

	var manifest types.MCPServerCatalogEntryManifest
	if err := req.Read(&manifest); err != nil {
		return types.NewErrBadRequest("failed to read entry manifest: %v", err)
	}

	if err := validation.ValidateCatalogEntryManifest(manifest); err != nil {
		return types.NewErrBadRequest("failed to validate entry manifest: %v", err)
	}

	cleanName := strings.ToLower(strings.ReplaceAll(manifest.Name, " ", "-"))

	entry := v1.MCPServerCatalogEntry{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: name.SafeHashConcatName(catalogName, cleanName),
			Namespace:    req.Namespace(),
		},
		Spec: v1.MCPServerCatalogEntrySpec{
			MCPCatalogName: catalogName,
			Editable:       true,
			Manifest:       manifest,
			// TODO(g-linville): add support for unsupportedTools field?
		},
	}

	if err := req.Create(&entry); err != nil {
		return fmt.Errorf("failed to create entry: %w", err)
	}

	return req.Write(convertMCPServerCatalogEntry(entry))
}

func (h *MCPCatalogHandler) UpdateEntry(req api.Context) error {
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

	entry.Spec.Manifest = manifest

	if err := req.Update(&entry); err != nil {
		return fmt.Errorf("failed to update entry: %w", err)
	}

	return req.Write(convertMCPServerCatalogEntry(entry))
}

func (h *MCPCatalogHandler) DeleteEntry(req api.Context) error {
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
		var credCtx string
		if server.Spec.SharedWithinMCPCatalogName != "" {
			credCtx = fmt.Sprintf("%s-%s", server.Spec.SharedWithinMCPCatalogName, server.Name)
		} else {
			credCtx = fmt.Sprintf("%s-%s", server.Spec.UserID, server.Name)
		}

		cred, err := req.GPTClient.RevealCredential(req.Context(), []string{credCtx}, server.Name)
		if err != nil && !errors.As(err, &gptscript.ErrNotFound{}) {
			return fmt.Errorf("failed to find credential: %w", err)
		}

		items = append(items, convertMCPServer(server, cred.Env, h.serverURL))
	}

	return req.Write(types.MCPServerList{Items: items})
}

// GenerateToolPreviews launches a temporary instance of an MCP server from a catalog entry
// to generate tool preview data, then cleans up the instance.
func (h *MCPCatalogHandler) GenerateToolPreviews(req api.Context) error {
	catalogName := req.PathValue("catalog_id")
	entryName := req.PathValue("entry_id")

	// Get the catalog entry
	var entry v1.MCPServerCatalogEntry
	if err := req.Get(&entry, entryName); err != nil {
		return fmt.Errorf("failed to get catalog entry: %w", err)
	}

	if entry.Spec.MCPCatalogName != catalogName {
		return types.NewErrBadRequest("entry does not belong to catalog")
	}

	if !entry.Spec.Editable {
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
	toolPreviews, err := h.sessionManager.GenerateToolPreviews(req.Context(), server, serverConfig)
	if err != nil {
		return fmt.Errorf("failed to launch temporary instance: %w", err)
	}

	// Update the catalog entry with the tool preview
	entry.Spec.Manifest.ToolPreview = toolPreviews
	if err := req.Update(&entry); err != nil {
		return fmt.Errorf("failed to update catalog entry: %w", err)
	}

	// Return the updated catalog entry
	return req.Write(convertMCPServerCatalogEntry(entry))
}

func (h *MCPCatalogHandler) GenerateToolPreviewsOAuthURL(req api.Context) error {
	catalogName := req.PathValue("catalog_id")
	entryName := req.PathValue("entry_id")

	// Get the catalog entry
	var entry v1.MCPServerCatalogEntry
	if err := req.Get(&entry, entryName); err != nil {
		return fmt.Errorf("failed to get catalog entry: %w", err)
	}

	if entry.Spec.MCPCatalogName != catalogName {
		return types.NewErrBadRequest("entry does not belong to catalog")
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
		if categories, exists := entry.Spec.Manifest.Metadata["categories"]; exists && categories != "" {
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
