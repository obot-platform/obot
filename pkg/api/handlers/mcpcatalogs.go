package handlers

import (
	"fmt"
	"net/url"
	"slices"
	"strings"

	"github.com/obot-platform/nah/pkg/name"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type MCPCatalogHandler struct {
	allowedDockerImageRepos []string
}

func NewMCPCatalogHandler(allowedDockerImageRepos []string) *MCPCatalogHandler {
	return &MCPCatalogHandler{
		allowedDockerImageRepos: allowedDockerImageRepos,
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

	if catalog.Annotations[v1.MCPCatalogSyncAnnotation] != "" {
		delete(catalog.Annotations, v1.MCPCatalogSyncAnnotation)
	} else {
		if catalog.Annotations == nil {
			catalog.Annotations = make(map[string]string)
		}
		catalog.Annotations[v1.MCPCatalogSyncAnnotation] = "true"
	}

	return req.Update(&catalog)
}

// Create creates a new catalog.
func (h *MCPCatalogHandler) Create(req api.Context) error {
	var manifest types.MCPCatalogManifest
	if err := req.Read(&manifest); err != nil {
		return fmt.Errorf("failed to read catalog manifest: %w", err)
	}

	// Validate the URLs
	for _, urlStr := range manifest.SourceURLs {
		u, err := url.Parse(urlStr)
		if err != nil {
			return fmt.Errorf("invalid URL: %w", err)
		}

		if u.Scheme != "https" {
			return fmt.Errorf("only HTTPS URLs are supported")
		}
	}

	catalog := v1.MCPCatalog{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: system.CatalogPrefix,
			Namespace:    req.Namespace(),
		},
		Spec: v1.MCPCatalogSpec{
			DisplayName:    manifest.DisplayName,
			SourceURLs:     manifest.SourceURLs,
			AllowedUserIDs: manifest.AllowedUserIDs,
		},
	}

	if err := req.Create(&catalog); err != nil {
		return fmt.Errorf("failed to create catalog: %w", err)
	}

	return req.Write(convertMCPCatalog(catalog))
}

// Update updates a catalog.
func (h *MCPCatalogHandler) Update(req api.Context) error {
	var manifest types.MCPCatalogManifest
	if err := req.Read(&manifest); err != nil {
		return fmt.Errorf("failed to read catalog manifest: %w", err)
	}

	var catalog v1.MCPCatalog
	if err := req.Get(&catalog, req.PathValue("catalog_id")); err != nil {
		return fmt.Errorf("failed to get catalog: %w", err)
	}

	if catalog.Spec.IsReadOnly {
		return fmt.Errorf("cannot update a read-only catalog")
	}

	if manifest.DisplayName == "" {
		return fmt.Errorf("display name is required")
	}

	if len(manifest.SourceURLs) == 0 {
		return fmt.Errorf("at least one source URL is required")
	}

	for _, urlStr := range manifest.SourceURLs {
		u, err := url.Parse(urlStr)
		if err != nil {
			return fmt.Errorf("invalid URL: %w", err)
		}

		if u.Scheme != "https" {
			return fmt.Errorf("only HTTPS URLs are supported")
		}
	}

	catalog.Spec.DisplayName = manifest.DisplayName
	catalog.Spec.SourceURLs = manifest.SourceURLs
	catalog.Spec.AllowedUserIDs = manifest.AllowedUserIDs

	if err := req.Update(&catalog); err != nil {
		return fmt.Errorf("failed to update catalog: %w", err)
	}

	return req.Write(convertMCPCatalog(catalog))
}

// Delete deletes a catalog.
func (h *MCPCatalogHandler) Delete(req api.Context) error {
	var catalog v1.MCPCatalog
	if err := req.Get(&catalog, req.PathValue("catalog_id")); err != nil {
		return fmt.Errorf("failed to get catalog: %w", err)
	}

	if catalog.Spec.IsReadOnly {
		return fmt.Errorf("cannot delete a read-only catalog")
	}

	if err := req.Delete(&catalog); err != nil {
		return fmt.Errorf("failed to delete catalog: %w", err)
	}

	return nil
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

// CreateEntry creates a new entry for a catalog.
func (h *MCPCatalogHandler) CreateEntry(req api.Context) error {
	catalogName := req.PathValue("catalog_id")

	var catalog v1.MCPCatalog
	if err := req.Get(&catalog, catalogName); err != nil {
		return fmt.Errorf("failed to get catalog: %w", err)
	}

	if catalog.Spec.IsReadOnly {
		return fmt.Errorf("cannot create an entry for a read-only catalog")
	}

	var manifest types.MCPServerCatalogEntryManifest
	if err := req.Read(&manifest); err != nil {
		return fmt.Errorf("failed to read entry manifest: %w", err)
	}

	hasCommand, hasURL, err := h.validateMCPServerCatalogEntryManifest(manifest)
	if err != nil {
		return fmt.Errorf("failed to validate entry manifest: %w", err)
	}

	entry := v1.MCPServerCatalogEntry{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: name.SafeHashConcatName(catalogName, manifest.Server.Name),
			Namespace:    req.Namespace(),
		},
		Spec: v1.MCPServerCatalogEntrySpec{
			MCPCatalogName: catalogName,
			Editable:       true,
			// TODO: add support for unsupportedTools field?
		},
	}

	if hasCommand {
		entry.Spec.CommandManifest = manifest
	} else if hasURL {
		entry.Spec.URLManifest = manifest
	} else {
		// Should be impossible since we validated this earlier.
		return fmt.Errorf("invalid manifest")
	}

	if err := req.Create(&entry); err != nil {
		return fmt.Errorf("failed to create entry: %w", err)
	}

	return req.Write(convertMCPServerCatalogEntry(entry))
}

func (h *MCPCatalogHandler) UpdateEntry(req api.Context) error {
	catalogName := req.PathValue("catalog_id")
	entryName := req.PathValue("entry_id")

	var entry v1.MCPServerCatalogEntry
	if err := req.Get(&entry, entryName); err != nil {
		return fmt.Errorf("failed to get entry: %w", err)
	}

	if entry.Spec.MCPCatalogName != catalogName {
		return fmt.Errorf("entry does not belong to catalog")
	}

	if !entry.Spec.Editable {
		return fmt.Errorf("entry is not editable")
	}

	var manifest types.MCPServerCatalogEntryManifest
	if err := req.Read(&manifest); err != nil {
		return fmt.Errorf("failed to read entry manifest: %w", err)
	}

	hasCommand, hasURL, err := h.validateMCPServerCatalogEntryManifest(manifest)
	if err != nil {
		return fmt.Errorf("failed to validate entry manifest: %w", err)
	}

	if hasCommand {
		entry.Spec.CommandManifest = manifest
		entry.Spec.URLManifest = types.MCPServerCatalogEntryManifest{}
	} else if hasURL {
		entry.Spec.URLManifest = manifest
		entry.Spec.CommandManifest = types.MCPServerCatalogEntryManifest{}
	} else {
		// Should be impossible since we validated this earlier.
		return fmt.Errorf("invalid manifest")
	}

	if err := req.Update(&entry); err != nil {
		return fmt.Errorf("failed to update entry: %w", err)
	}

	return req.Write(convertMCPServerCatalogEntry(entry))
}

func (h *MCPCatalogHandler) DeleteEntry(req api.Context) error {
	catalogName := req.PathValue("catalog_id")
	entryName := req.PathValue("entry_id")

	var entry v1.MCPServerCatalogEntry
	if err := req.Get(&entry, entryName); err != nil {
		return fmt.Errorf("failed to get entry: %w", err)
	}

	if entry.Spec.MCPCatalogName != catalogName {
		return fmt.Errorf("entry does not belong to catalog")
	}

	if !entry.Spec.Editable {
		return fmt.Errorf("entry is not editable and cannot be manually deleted")
	}

	if err := req.Delete(&entry); err != nil {
		return fmt.Errorf("failed to delete entry: %w", err)
	}

	return nil
}

func (h *MCPCatalogHandler) validateMCPServerCatalogEntryManifest(manifest types.MCPServerCatalogEntryManifest) (bool, bool, error) {
	if manifest.Server.Name == "" {
		return false, false, fmt.Errorf("server name is required")
	}

	var (
		hasCommand, hasURL bool
	)
	if manifest.Server.Command != "" {
		hasCommand = true

		if manifest.Server.Command == "docker" {
			if len(manifest.Server.Args) == 0 || len(h.allowedDockerImageRepos) > 0 && !slices.ContainsFunc(h.allowedDockerImageRepos, func(s string) bool {
				return strings.HasPrefix(manifest.Server.Args[len(manifest.Server.Args)-1], s)
			}) {
				return false, false, fmt.Errorf("docker command must be followed by a valid image name from one of the allowed repos (%s)", strings.Join(h.allowedDockerImageRepos, ", "))
			}
		} else if manifest.Server.Command != "npx" && manifest.Server.Command != "uvx" {
			return false, false, fmt.Errorf("unsupported command: %s", manifest.Server.Command)
		}
	}
	if manifest.Server.URL != "" {
		hasURL = true
	}

	if hasCommand && hasURL {
		return false, false, fmt.Errorf("only one of command or url is allowed")
	}

	if !hasCommand && !hasURL {
		return false, false, fmt.Errorf("one of command or url is required")
	}

	if manifest.GitHubStars < 0 {
		return false, false, fmt.Errorf("github stars must be non-negative")
	}

	return hasCommand, hasURL, nil
}

func convertMCPCatalog(catalog v1.MCPCatalog) types.MCPCatalog {
	return types.MCPCatalog{
		Metadata: MetadataFrom(&catalog),
		MCPCatalogManifest: types.MCPCatalogManifest{
			DisplayName:    catalog.Spec.DisplayName,
			SourceURLs:     catalog.Spec.SourceURLs,
			AllowedUserIDs: catalog.Spec.AllowedUserIDs,
		},
		IsReadOnly: catalog.Spec.IsReadOnly,
		LastSynced: *types.NewTime(catalog.Status.LastSyncTime.Time),
	}
}
