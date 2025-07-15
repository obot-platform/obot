package handlers

import (
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"

	"github.com/gptscript-ai/go-gptscript"
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
	defaultCatalogPath      string
	serverURL               string
}

func NewMCPCatalogHandler(allowedDockerImageRepos []string, defaultCatalogPath string, serverURL string) *MCPCatalogHandler {
	return &MCPCatalogHandler{
		allowedDockerImageRepos: allowedDockerImageRepos,
		defaultCatalogPath:      defaultCatalogPath,
		serverURL:               serverURL,
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

	hasCommand, hasURL, err := h.validateMCPServerCatalogEntryManifest(manifest)
	if err != nil {
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
			// TODO(g-linville): add support for unsupportedTools field?
		},
	}

	if hasCommand {
		manifest.Headers = nil
		entry.Spec.CommandManifest = manifest
	} else if hasURL {
		manifest.Args = nil
		entry.Spec.URLManifest = manifest
	} else {
		// Should be impossible since we validated this earlier.
		return types.NewErrBadRequest("invalid manifest")
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

	hasCommand, hasURL, err := h.validateMCPServerCatalogEntryManifest(manifest)
	if err != nil {
		return types.NewErrBadRequest("failed to validate entry manifest: %v", err)
	}

	if hasCommand {
		manifest.Headers = nil
		entry.Spec.CommandManifest = manifest
		entry.Spec.URLManifest = types.MCPServerCatalogEntryManifest{}
	} else if hasURL {
		manifest.Args = nil
		entry.Spec.URLManifest = manifest
		entry.Spec.CommandManifest = types.MCPServerCatalogEntryManifest{}
	} else {
		// Should be impossible since we validated this earlier.
		return types.NewErrBadRequest("invalid manifest")
	}

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

func (h *MCPCatalogHandler) validateMCPServerCatalogEntryManifest(manifest types.MCPServerCatalogEntryManifest) (bool, bool, error) {
	if manifest.Name == "" {
		return false, false, fmt.Errorf("server name is required")
	}

	var (
		hasCommand, hasURL bool
	)
	if manifest.Command != "" {
		hasCommand = true

		if len(manifest.Args) == 0 {
			return false, false, fmt.Errorf("command must be followed by at least one argument")
		}

		if manifest.Command == "docker" {
			if len(h.allowedDockerImageRepos) == 0 {
				return false, false, fmt.Errorf("docker command is not allowed")
			}

			if !slices.ContainsFunc(h.allowedDockerImageRepos, func(s string) bool {
				return strings.HasPrefix(manifest.Args[len(manifest.Args)-1], s)
			}) {
				return false, false, fmt.Errorf("docker command must be followed by a valid image name from one of the allowed repos (%s)", strings.Join(h.allowedDockerImageRepos, ", "))
			}
		} else if manifest.Command != "npx" && manifest.Command != "uvx" {
			return false, false, fmt.Errorf("unsupported command: %s", manifest.Command)
		}
	}
	if manifest.FixedURL != "" || manifest.Hostname != "" {
		hasURL = true
		if manifest.FixedURL != "" && manifest.Hostname != "" {
			return false, false, fmt.Errorf("only one of fixedURL or hostname is allowed")
		}
	}

	if hasCommand && hasURL {
		return false, false, fmt.Errorf("only one of command or url is allowed")
	}

	if !hasCommand && !hasURL {
		return false, false, fmt.Errorf("one of command or url is required")
	}

	return hasCommand, hasURL, nil
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
	}
}
