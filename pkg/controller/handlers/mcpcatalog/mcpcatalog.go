package mcpcatalog

import (
	"bytes"
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
	"unicode"

	"github.com/obot-platform/nah/pkg/apply"
	"github.com/obot-platform/nah/pkg/log"
	"github.com/obot-platform/nah/pkg/name"
	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/controller/handlers/toolreference"
	"github.com/obot-platform/obot/pkg/controller/handlers/usercatalogauthorization"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Handler struct {
	allowedDockerImageRepos []string
	defaultCatalogPath      string
}

func New(allowedDockerImageRepos []string, defaultCatalogPath string) *Handler {
	return &Handler{
		allowedDockerImageRepos: allowedDockerImageRepos,
		defaultCatalogPath:      defaultCatalogPath,
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
	resp.RetryAfter(time.Hour)

	// I know we don't want to do apply anymore. But we were doing it before in a different place.
	// Now we're doing it here. It's not important enough to change right now.
	return apply.New(req.Client).WithOwnerSubContext(mcpCatalog.Name).Apply(req.Ctx, mcpCatalog, toAdd...)
}

func (h *Handler) readMCPCatalog(catalogName, sourceURL string) ([]client.Object, error) {
	var entries []toolreference.CatalogEntryInfo

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
		entry.FullName = string(slices.DeleteFunc(bytes.ToLower([]byte(entry.FullName)), func(r byte) bool {
			return r != '/' && !unicode.IsLetter(rune(r)) && !unicode.IsNumber(rune(r))
		}))

		if entry.Metadata["categories"] == "Official" {
			delete(entry.Metadata, "categories") // This shouldn't happen, but do this just in case.
			// We don't want to mark random MCP servers from the catalog as official.
		}

		catalogEntry := v1.MCPServerCatalogEntry{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name.SafeHashConcatName(strings.Split(entry.FullName, "/")...),
				Namespace: system.DefaultNamespace,
			},
			Spec: v1.MCPServerCatalogEntrySpec{
				MCPCatalogName: catalogName,
				Editable:       false, // entries from source URLs are not editable
			},
		}

		// Check the metadata for default disabled tools.
		if entry.Metadata["unsupportedTools"] != "" {
			catalogEntry.Spec.UnsupportedTools = strings.Split(entry.Metadata["unsupportedTools"], ",")
		}

		var preferredFound, addEntry bool
		for _, c := range entry.Manifest {
			if c.Command != "" {
				if preferredFound {
					continue
				}
				switch c.Command {
				case "npx", "uvx":
				case "docker":
					// Only allow docker commands if the image name starts with one of the allowed repos.
					if len(c.Args) == 0 || len(h.allowedDockerImageRepos) > 0 && !slices.ContainsFunc(h.allowedDockerImageRepos, func(s string) bool {
						return strings.HasPrefix(c.Args[len(c.Args)-1], s)
					}) {
						continue
					}
				default:
					log.Infof("Ignoring MCP catalog entry %s: unsupported command %s", entry.DisplayName, c.Command)
					continue
				}

				preferredFound = c.Preferred
				if !preferredFound && !isCommandPreferred(catalogEntry.Spec.CommandManifest.Server.Command, c.Command) {
					continue
				}

				// Sanitize the environment variables
				for i, env := range c.Env {
					if env.Key == "" {
						env.Key = env.Name
					}

					if filepath.Ext(env.Key) != "" {
						env.Key = strings.ReplaceAll(env.Key, ".", "_")
						env.File = true
					}

					env.Key = strings.ReplaceAll(strings.ToUpper(env.Key), "-", "_")

					c.Env[i] = env
				}

				addEntry = true
				catalogEntry.Spec.CommandManifest = types.MCPServerCatalogEntryManifest{
					URL:         entry.URL,
					GitHubStars: entry.Stars,
					Metadata:    entry.Metadata,
					Server: types.MCPServerManifest{
						Name:        entry.DisplayName,
						Description: entry.Description,
						Icon:        entry.Icon,
						Env:         c.Env,
						Command:     c.Command,
						Args:        c.Args,
						URL:         c.URL,
						Headers:     c.HTTPHeaders,
					},
				}
			} else if c.URL != "" || c.Remote {
				if c.URL != "" {
					if u, err := url.Parse(c.URL); err != nil || u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" {
						continue
					}
				}

				// Sanitize the headers
				for i, header := range c.HTTPHeaders {
					if header.Key == "" {
						header.Key = header.Name
					}

					header.Key = strings.ReplaceAll(strings.ToUpper(header.Key), "_", "-")

					c.HTTPHeaders[i] = header
				}

				addEntry = true
				catalogEntry.Spec.URLManifest = types.MCPServerCatalogEntryManifest{
					URL:         entry.URL,
					GitHubStars: entry.Stars,
					Metadata:    entry.Metadata,
					Server: types.MCPServerManifest{
						Name:        entry.DisplayName,
						Description: entry.Description,
						Icon:        entry.Icon,
						URL:         c.URL,
						Headers:     c.HTTPHeaders,
					},
				}
			}
		}

		if addEntry {
			objs = append(objs, &catalogEntry)
		}
	}

	return objs, nil
}

func (h *Handler) readMCPCatalogDirectory(catalog string) ([]toolreference.CatalogEntryInfo, error) {
	files, err := os.ReadDir(catalog)
	if err != nil {
		return nil, fmt.Errorf("failed to read catalog directory %s: %w", catalog, err)
	}

	var entries []toolreference.CatalogEntryInfo
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

			var entry toolreference.CatalogEntryInfo
			if err = json.Unmarshal(contents, &entry); err != nil {
				return nil, fmt.Errorf("failed to decode catalog file %s: %w", file.Name(), err)
			}
			entries = append(entries, entry)
		}
	}

	return entries, nil
}

func isCommandPreferred(existing, newer string) bool {
	if existing == "" {
		return true
	}
	if newer == "" || existing == "npx" {
		return false
	}

	if existing == "uvx" {
		return newer == "npx"
	}

	// This would mean that existing is docker and newer is either npx or uvx.
	return true
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

func (h *Handler) SetUpUserCatalogAuthorizations(req router.Request, resp router.Response) error {
	mcpCatalog := req.Object.(*v1.MCPCatalog)

	authorizationNames := map[string]struct{}{}
	for _, userID := range mcpCatalog.Spec.AllowedUserIDs {
		authorizationName := name.SafeHashConcatName(mcpCatalog.Name, userID)
		if userID == "*" {
			authorizationName = name.SafeHashConcatName(mcpCatalog.Name, "all-users")
		}

		authorizationNames[authorizationName] = struct{}{}

		// See if this authorization already exists.
		var existingAuthorization v1.UserCatalogAuthorization
		if err := req.Client.Get(req.Ctx, router.Key(system.DefaultNamespace, authorizationName), &existingAuthorization); apierrors.IsNotFound(err) {
			req.Client.Create(req.Ctx, &v1.UserCatalogAuthorization{
				ObjectMeta: metav1.ObjectMeta{
					Name:      authorizationName,
					Namespace: system.DefaultNamespace,
				},
				Spec: v1.UserCatalogAuthorizationSpec{
					UserID:         userID,
					MCPCatalogName: mcpCatalog.Name,
				},
			})
		}
	}

	// Now delete any authorizations that are no longer needed.
	existingAuthorizations, err := usercatalogauthorization.GetAuthorizationsForCatalog(req.Ctx, req.Client, mcpCatalog.Name)
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
