package mcpcatalog

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/obot-platform/nah/pkg/apply"
	"github.com/obot-platform/nah/pkg/log"
	"github.com/obot-platform/nah/pkg/name"
	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/controller/handlers/usercatalogauthorization"
	"github.com/obot-platform/obot/pkg/create"
	gclient "github.com/obot-platform/obot/pkg/gateway/client"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Handler struct {
	allowedDockerImageRepos []string
	defaultCatalogPath      string
	gatewayClient           *gclient.Client
}

func New(allowedDockerImageRepos []string, defaultCatalogPath string, gatewayClient *gclient.Client) *Handler {
	return &Handler{
		allowedDockerImageRepos: allowedDockerImageRepos,
		defaultCatalogPath:      defaultCatalogPath,
		gatewayClient:           gatewayClient,
	}
}

func (h *Handler) Sync(req router.Request, resp router.Response) error {
	mcpCatalog := req.Object.(*v1.MCPCatalog)
	toAdd := make([]client.Object, 0)

	for _, sourceURL := range mcpCatalog.Spec.SourceURLs {
		objs, err := h.readMCPCatalog(mcpCatalog.Name, sourceURL)
		if err != nil {
			return fmt.Errorf("failed to read catalog %s: %w", sourceURL, err)
		}

		toAdd = append(toAdd, objs...)
	}

	mcpCatalog.Status.LastSyncTime = metav1.Now()
	if err := req.Client.Status().Update(req.Ctx, mcpCatalog); err != nil {
		return fmt.Errorf("failed to update catalog status: %w", err)
	}

	// We want to refresh this every hour.
	// TODO(g-linville): make this configurable.
	resp.RetryAfter(time.Hour)

	// I know we don't want to do apply anymore. But we were doing it before in a different place.
	// Now we're doing it here. It's not important enough to change right now.
	return apply.New(req.Client).WithOwnerSubContext(fmt.Sprintf("catalog-%s", mcpCatalog.Name)).
		WithPruneTypes(&v1.MCPServerCatalogEntry{}).Apply(req.Ctx, mcpCatalog, toAdd...)
}

func (h *Handler) readMCPCatalog(catalogName, sourceURL string) ([]client.Object, error) {
	var entries []types.MCPServerCatalogEntryManifest

	if strings.HasPrefix(sourceURL, "http://") || strings.HasPrefix(sourceURL, "https://") {
		if isGitHubURL(sourceURL) {
			var err error
			entries, err = readGitHubCatalog(sourceURL)
			if err != nil {
				return nil, fmt.Errorf("failed to read GitHub catalog %s: %w", sourceURL, err)
			}
		} else {
			// If it wasn't a GitHub repo, treat it as a raw file.
			resp, err := http.Get(sourceURL)
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

			if err = json.Unmarshal(contents, &entries); err != nil {
				return nil, fmt.Errorf("failed to decode catalog %s: %w", sourceURL, err)
			}
		}
	} else {
		fileInfo, err := os.Stat(sourceURL)
		if err != nil {
			return nil, fmt.Errorf("failed to stat catalog %s: %w", sourceURL, err)
		}

		if fileInfo.IsDir() {
			entries, err = h.readMCPCatalogDirectory(sourceURL)
			if err != nil {
				return nil, fmt.Errorf("failed to read catalog %s: %w", sourceURL, err)
			}
		} else {
			contents, err := os.ReadFile(sourceURL)
			if err != nil {
				return nil, fmt.Errorf("failed to read catalog %s: %w", sourceURL, err)
			}

			if err = json.Unmarshal(contents, &entries); err != nil {
				return nil, fmt.Errorf("failed to decode catalog %s: %w", sourceURL, err)
			}
		}
	}

	objs := make([]client.Object, 0, len(entries))

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

		if entry.Command != "" {
			switch entry.Command {
			case "npx", "uvx":
			case "docker":
				// Only allow docker commands if the image name starts with one of the allowed repos.
				if len(entry.Args) == 0 || len(h.allowedDockerImageRepos) > 0 && !slices.ContainsFunc(h.allowedDockerImageRepos, func(s string) bool {
					return strings.HasPrefix(entry.Args[len(entry.Args)-1], s)
				}) {
					continue
				}
			default:
				log.Infof("Ignoring MCP catalog entry %s: unsupported command %s", entry.Name, entry.Command)
				continue
			}

			// Sanitize the environment variables
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

			catalogEntry.Spec.CommandManifest = entry
		} else if entry.FixedURL != "" || entry.Hostname != "" {
			// Make sure that only one or the other is set.
			if entry.FixedURL != "" && entry.Hostname != "" {
				log.Warnf("Ignoring MCP catalog entry %s: both FixedURL and Hostname are set (only one can be set)", entry.Name)
				continue
			}

			if entry.FixedURL != "" {
				if u, err := url.Parse(entry.FixedURL); err != nil || u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" {
					log.Warnf("Ignoring MCP catalog entry %s: fixedURL is invalid (must be a valid, non-localhost URL)", entry.Name)
					continue
				}
			}

			// Sanitize the headers
			for i, header := range entry.Headers {
				if header.Key == "" {
					header.Key = header.Name
				}

				header.Key = strings.ReplaceAll(strings.ToUpper(header.Key), "_", "-")
				entry.Headers[i] = header
			}

			catalogEntry.Spec.URLManifest = entry
		} else {
			continue
		}

		objs = append(objs, &catalogEntry)
	}

	return objs, nil
}

func (h *Handler) readMCPCatalogDirectory(catalog string) ([]types.MCPServerCatalogEntryManifest, error) {
	files, err := os.ReadDir(catalog)
	if err != nil {
		return nil, fmt.Errorf("failed to read catalog directory %s: %w", catalog, err)
	}

	var entries []types.MCPServerCatalogEntryManifest
	for _, file := range files {
		if file.IsDir() {
			nestedEntries, err := h.readMCPCatalogDirectory(filepath.Join(catalog, file.Name()))
			if err != nil {
				return nil, fmt.Errorf("failed to read nested catalog directory %s: %w", file.Name(), err)
			}
			entries = append(entries, nestedEntries...)
		} else {
			contents, err := os.ReadFile(filepath.Join(catalog, file.Name()))
			if err != nil {
				return nil, fmt.Errorf("failed to read catalog file %s: %w", file.Name(), err)
			}

			var entry types.MCPServerCatalogEntryManifest
			if err = json.Unmarshal(contents, &entry); err != nil {
				return nil, fmt.Errorf("failed to decode catalog file %s: %w", file.Name(), err)
			}
			entries = append(entries, entry)
		}
	}

	return entries, nil
}

// DeleteUnauthorizedMCPServers deletes all MCP servers that are no longer authorized to run.
// This can happen when a user has launched an MCP server that they used to have permission for,
// but their access to the catalog that the server came from has been revoked.
func (h *Handler) DeleteUnauthorizedMCPServers(req router.Request, _ router.Response) error {
	catalog := req.Object.(*v1.MCPCatalog)

	allowedUserIDs := map[string]struct{}{}
	for _, userID := range catalog.Spec.AllowedUserIDs {
		allowedUserIDs[userID] = struct{}{}
	}

	if _, ok := allowedUserIDs["*"]; ok {
		// Everyone is allowed, so there are no unauthorized servers to delete.
		return nil
	}

	var entries v1.MCPServerCatalogEntryList
	if err := req.Client.List(req.Ctx, &entries, client.InNamespace(req.Namespace), client.MatchingFields{
		"spec.mcpCatalogName": catalog.Name,
	}); err != nil {
		return fmt.Errorf("failed to list entries: %w", err)
	}

	// TODO(g-linville): if this is too inefficient, we can do it in a handler for individual MCPServerCatalogEntry objects instead.
	// Then we would only need to loop over servers, and not over entries also.
	for _, entry := range entries.Items {
		var servers v1.MCPServerList
		if err := req.Client.List(req.Ctx, &servers, client.InNamespace(req.Namespace), client.MatchingFields{
			"spec.mcpServerCatalogEntryName": entry.Name,
		}); err != nil {
			return fmt.Errorf("failed to list servers: %w", err)
		}

		for _, server := range servers.Items {
			// Admin users can run whatever they want, so don't shut down any of their servers.
			if user, err := h.gatewayClient.UserByID(req.Ctx, server.Spec.UserID); err == nil && user.Role == types.RoleAdmin {
				continue
			}

			if _, ok := allowedUserIDs[server.Spec.UserID]; !ok {
				if err := req.Client.Delete(req.Ctx, &server); err != nil {
					return fmt.Errorf("failed to delete server %s: %w", server.Name, err)
				}
			}
		}
	}

	return nil
}

func (h *Handler) SetUpDefaultMCPCatalog(ctx context.Context, c client.Client) error {
	if h.defaultCatalogPath == "" {
		// Delete it if it exists.
		var catalog v1.MCPCatalog
		if err := c.Get(ctx, router.Key(system.DefaultNamespace, "default"), &catalog); err == nil {
			if err := c.Delete(ctx, &catalog); err != nil {
				return fmt.Errorf("failed to delete default catalog: %w", err)
			}
		}
		return nil
	}

	var existing v1.MCPCatalog
	if err := c.Get(ctx, router.Key(system.DefaultNamespace, "default"), &existing); err == nil {
		// See if the URL has changed.
		if len(existing.Spec.SourceURLs) > 0 && existing.Spec.SourceURLs[0] != h.defaultCatalogPath {
			existing.Spec.SourceURLs = []string{h.defaultCatalogPath}
			if err := c.Update(ctx, &existing); err != nil {
				return fmt.Errorf("failed to update default catalog: %w", err)
			}
		}
		return nil
	}

	if err := c.Create(ctx, &v1.MCPCatalog{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "default",
			Namespace: system.DefaultNamespace,
		},
		Spec: v1.MCPCatalogSpec{
			DisplayName:    "Default",
			SourceURLs:     []string{h.defaultCatalogPath},
			AllowedUserIDs: []string{"*"},
			IsReadOnly:     true,
		},
	}); err != nil {
		return fmt.Errorf("failed to create default catalog: %w", err)
	}

	return nil
}

func (h *Handler) SetUpUserCatalogAuthorizations(req router.Request, _ router.Response) error {
	mcpCatalog := req.Object.(*v1.MCPCatalog)

	authorizationNames := make(map[string]struct{}, len(mcpCatalog.Spec.AllowedUserIDs))
	for _, userID := range mcpCatalog.Spec.AllowedUserIDs {
		authorizationName := name.SafeHashConcatName(mcpCatalog.Name, userID)
		if userID == "*" {
			authorizationName = name.SafeHashConcatName(mcpCatalog.Name, "all-users")
		}

		authorizationNames[authorizationName] = struct{}{}

		if err := create.IfNotExists(req.Ctx, req.Client, &v1.UserCatalogAuthorization{
			ObjectMeta: metav1.ObjectMeta{
				Name:      authorizationName,
				Namespace: system.DefaultNamespace,
			},
			Spec: v1.UserCatalogAuthorizationSpec{
				UserID:         userID,
				MCPCatalogName: mcpCatalog.Name,
			},
		}); err != nil {
			return fmt.Errorf("failed to create user catalog authorization %s: %w", authorizationName, err)
		}
	}

	// Now delete any authorizations that are no longer needed.
	existingAuthorizations, err := usercatalogauthorization.GetAuthorizationsForCatalog(req.Ctx, req.Client, req.Namespace, mcpCatalog.Name)
	if err != nil {
		return fmt.Errorf("failed to get existing authorizations: %w", err)
	}

	for _, authorization := range existingAuthorizations {
		if _, ok := authorizationNames[authorization.Name]; !ok {
			if err := req.Client.Delete(req.Ctx, &v1.UserCatalogAuthorization{
				ObjectMeta: metav1.ObjectMeta{
					Name:      authorization.Name,
					Namespace: system.DefaultNamespace,
				},
			}); err != nil {
				return fmt.Errorf("failed to delete existing authorization %s: %w", authorization.Name, err)
			}
		}
	}

	return nil
}
