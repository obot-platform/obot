package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/obot-platform/cmd"
	"github.com/obot-platform/obot/apiclient"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/cli/internal"
	"github.com/obot-platform/obot/pkg/mcp"
	"github.com/obot-platform/obot/pkg/mcpcatalog"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/spf13/cobra"
)

const mcpSearchPageLimit = 100

type MCP struct {
	root *Obot
}

func (m *MCP) Customize(c *cobra.Command) {
	c.Use = "mcp"
	c.Short = "Manage MCP servers"
	c.Args = cobra.NoArgs
	c.AddCommand(cmd.Command(&MCPSearch{root: m.root}))
	c.AddCommand(cmd.Command(&MCPValidateCatalogYAML{}))
	c.AddCommand(cmd.Command(&MCPValidateSystemCatalogYAML{}))
}

func (m *MCP) Run(cmd *cobra.Command, _ []string) error {
	return cmd.Help()
}

type MCPValidateCatalogYAML struct {
	RequireEntryKey bool `usage:"Require every catalog entry to set entryKey"`
}

func (m *MCPValidateCatalogYAML) Customize(cmd *cobra.Command) {
	cmd.Use = "validate-catalog-yaml <path>..."
	cmd.Short = "Validate MCP catalog entry files"
	cmd.Args = cobra.MinimumNArgs(1)
}

func (m *MCPValidateCatalogYAML) Run(cmd *cobra.Command, args []string) error {
	files, err := validateMCPCatalogPaths(cmd.Context(), args, m.RequireEntryKey)
	if err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Catalog entries in %d files are valid.\n", files)
	return nil
}

func validateMCPCatalogPaths(ctx context.Context, paths []string, requireEntryKey bool) (int, error) {
	seenEntryKeys := make(map[string]string)
	return validateCatalogPaths(paths, func(path string) error {
		return validateMCPCatalogFile(ctx, path, requireEntryKey, seenEntryKeys)
	})
}

type MCPValidateSystemCatalogYAML struct{}

func (m *MCPValidateSystemCatalogYAML) Customize(cmd *cobra.Command) {
	cmd.Use = "validate-system-catalog-yaml <path>..."
	cmd.Short = "Validate system MCP catalog entry files"
	cmd.Args = cobra.MinimumNArgs(1)
}

func (m *MCPValidateSystemCatalogYAML) Run(cmd *cobra.Command, args []string) error {
	files, err := validateSystemMCPCatalogPaths(cmd.Context(), args)
	if err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "System catalog entries in %d files are valid.\n", files)
	return nil
}

func validateSystemMCPCatalogPaths(ctx context.Context, paths []string) (int, error) {
	seenSanitizedNames := make(map[string]string)
	return validateCatalogPaths(paths, func(path string) error {
		return validateSystemMCPCatalogFile(ctx, path, seenSanitizedNames)
	})
}

func validateCatalogPaths(paths []string, validateFile func(string) error) (int, error) {
	var validationErr error
	seenFiles := make(map[string]struct{})
	validateFileOnce := func(path string) error {
		absPath, err := filepath.Abs(path)
		if err != nil {
			absPath = filepath.Clean(path)
		}
		if _, ok := seenFiles[absPath]; ok {
			return nil
		}
		seenFiles[absPath] = struct{}{}
		return validateFile(path)
	}

	for _, input := range paths {
		info, err := os.Stat(input)
		if err != nil {
			validationErr = errors.Join(validationErr, fmt.Errorf("%s: %w", input, err))
			continue
		}
		if !info.IsDir() {
			validationErr = errors.Join(validationErr, validateFileOnce(input))
			continue
		}

		files, _, err := mcpcatalog.WalkCatalogFiles(input)
		if err != nil {
			validationErr = errors.Join(validationErr, fmt.Errorf("%s: %w", input, err))
			continue
		}
		for path, walkErr := range files {
			if walkErr != nil {
				validationErr = errors.Join(validationErr, fmt.Errorf("%s: %w", input, walkErr))
				break
			}
			validationErr = errors.Join(validationErr, validateFileOnce(path))
		}
	}
	if len(seenFiles) == 0 {
		validationErr = errors.Join(validationErr, fmt.Errorf("no catalog entry files found"))
	}

	return len(seenFiles), validationErr
}

func validateMCPCatalogFile(ctx context.Context, path string, requireEntryKey bool, seenEntryKeys map[string]string) error {
	entries, isArray, err := mcpcatalog.DecodeCatalogFile[types.MCPServerCatalogEntryManifest](path, true)
	if err != nil {
		return fmt.Errorf("%s: invalid catalog entry: %w", path, err)
	}

	validationOptions := mcpcatalog.ValidationOptions{
		GitManaged: true,
		MCPBackend: mcp.RuntimeBackendKubernetes,
		MCP: mcp.ValidationOptions{
			RemoteMCPURLValidationConfig: mcp.RemoteMCPURLValidationConfig{
				AllowLocalhostMCP: true,
				AllowPrivateIPMCP: true,
				AllowLinkLocalMCP: true,
			},
		},
	}
	var errs []error
	for i := range entries {
		entry := &entries[i]
		label := path
		if isArray {
			label = fmt.Sprintf("%s[%d]", path, i)
		}
		mcpcatalog.NormalizeManifest(entry)

		if requireEntryKey && strings.TrimSpace(entry.EntryKey) == "" {
			errs = append(errs, fmt.Errorf("%s: entryKey is required", label))
		}
		if err := mcpcatalog.ValidateSourceFields(*entry); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", label, err))
		}
		if entry.EntryKey != "" {
			if previous, ok := seenEntryKeys[entry.EntryKey]; ok {
				errs = append(errs, fmt.Errorf("%s: duplicate source entry key %q also used by %s", label, entry.EntryKey, previous))
			} else {
				seenEntryKeys[entry.EntryKey] = label
			}
		}
		if err := mcpcatalog.ValidateManifest(ctx, *entry, validationOptions); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", label, err))
		}
	}

	return errors.Join(errs...)
}

func validateSystemMCPCatalogFile(ctx context.Context, path string, seenSanitizedNames map[string]string) error {
	entries, isArray, err := mcpcatalog.DecodeCatalogFile[types.SystemMCPServerCatalogEntryManifest](path, true)
	if err != nil {
		return fmt.Errorf("%s: invalid system catalog entry: %w", path, err)
	}

	validationOptions := mcp.ValidationOptions{
		RemoteMCPURLValidationConfig: mcp.RemoteMCPURLValidationConfig{
			AllowLocalhostMCP: true,
			AllowPrivateIPMCP: true,
			AllowLinkLocalMCP: true,
		},
	}
	var errs []error
	for i := range entries {
		entry := &entries[i]
		label := path
		if isArray {
			label = fmt.Sprintf("%s[%d]", path, i)
		}
		mcpcatalog.NormalizeSystemManifest(entry)

		sanitizedName := mcpcatalog.SanitizeName(entry.Name)
		if sanitizedName == "" {
			errs = append(errs, fmt.Errorf("%s: invalid system catalog entry name after sanitization: original=%q sanitized=%q", label, entry.Name, sanitizedName))
		} else if previous, ok := seenSanitizedNames[sanitizedName]; ok {
			errs = append(errs, fmt.Errorf("%s: duplicate sanitized system catalog entry name %q also used by %s", label, sanitizedName, previous))
		} else {
			seenSanitizedNames[sanitizedName] = label
		}
		if err := mcp.ValidateSystemMCPServerCatalogEntryManifest(ctx, *entry, validationOptions); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", label, err))
		}
	}

	return errors.Join(errs...)
}

type MCPSearch struct {
	PromptConfig

	Limit int  `usage:"Maximum number of MCP servers to return; 0 means no limit" default:"50"`
	JSON  bool `usage:"Print results as JSON"`

	root *Obot
}

func (m *MCPSearch) Customize(cmd *cobra.Command) {
	cmd.Use = "search [query...]"
	cmd.Short = "Search Obot for MCP servers"
	cmd.Args = cobra.ArbitraryArgs
}

func (m *MCPSearch) Run(cmd *cobra.Command, args []string) error {
	if m.root == nil || m.root.Client == nil {
		return fmt.Errorf("mcp search: no API client configured")
	}
	if m.Limit < 0 {
		return fmt.Errorf("--limit must be >= 0")
	}

	client := m.root.Client
	if m.JSON && client.Token == "" {
		token, err := internal.ExistingToken(cmd.Context(), client.BaseURL)
		if err != nil {
			return fmt.Errorf(`mcp search --json requires an existing login; run "obot login" first`)
		}
		client = client.WithTokenFetcher(nil).WithToken(token)
	}

	result, err := m.listServers(cmd, client, strings.TrimSpace(strings.Join(args, " ")))
	if err != nil {
		return registrySearchError(err)
	}

	appURL, _ := internal.AppURLForAPIBaseURL(client.BaseURL)
	output := normalizeRegistryServers(result, appURL)
	if m.JSON {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(mcpSearchOutput{Servers: output})
	}

	if len(output) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No MCP servers found")
		return nil
	}

	return writeMCPSearchTable(cmd, output)
}

func (m *MCPSearch) listServers(cmd *cobra.Command, client *apiclient.Client, query string) ([]types.RegistryServerResponse, error) {
	var (
		cursor  string
		results []types.RegistryServerResponse
	)
	for {
		pageLimit := mcpSearchPageLimit
		if m.Limit > 0 && m.Limit-len(results) < pageLimit {
			pageLimit = m.Limit - len(results)
		}
		if pageLimit <= 0 {
			break
		}

		page, err := client.ListRegistryServers(cmd.Context(), apiclient.ListRegistryServersOptions{
			Search: query,
			Cursor: cursor,
			Limit:  pageLimit,
		})
		if err != nil {
			return nil, err
		}

		results = append(results, page.Servers...)
		if m.Limit > 0 && len(results) >= m.Limit {
			results = results[:m.Limit]
			break
		}
		if page.Metadata == nil || page.Metadata.NextCursor == "" {
			break
		}
		cursor = page.Metadata.NextCursor
	}
	return results, nil
}

type mcpSearchOutput struct {
	Servers []mcpSearchServer `json:"servers"`
}

type mcpSearchServer struct {
	Name                  string `json:"name"`
	Title                 string `json:"title"`
	Description           string `json:"description"`
	Status                string `json:"status"`
	ConfigurationRequired bool   `json:"configurationRequired"`
	URL                   string `json:"url"`
}

func normalizeRegistryServers(servers []types.RegistryServerResponse, appURL string) []mcpSearchServer {
	result := make([]mcpSearchServer, 0, len(servers))
	for _, registryServer := range servers {
		url := firstRegistryRemoteURL(registryServer.Server)
		configurationRequired := registryServer.Meta.Obot != nil && registryServer.Meta.Obot.ConfigurationRequired
		if configurationRequired && url == "" {
			url = registryServerConfigurationURL(appURL, registryServer.Server.Name)
		}
		result = append(result, mcpSearchServer{
			Name:                  registryServer.Server.Name,
			Title:                 registryServer.Server.Title,
			Description:           registryServer.Server.Description,
			Status:                registryServerStatus(configurationRequired, url),
			ConfigurationRequired: configurationRequired,
			URL:                   url,
		})
	}
	return result
}

func registryServerConfigurationURL(appURL, registryName string) string {
	if appURL == "" {
		return ""
	}

	id := registryName
	if _, resourceName, ok := strings.Cut(registryName, "/"); ok {
		id = resourceName
	}
	if id == "" {
		return ""
	}

	route := "c"
	if system.IsMCPServerID(id) {
		route = "s"
	}

	return strings.TrimRight(appURL, "/") + "/mcp-servers/" + route + "/" + url.PathEscape(id)
}

func firstRegistryRemoteURL(server types.RegistryServerDetail) string {
	if len(server.Remotes) == 0 {
		return ""
	}
	return server.Remotes[0].URL
}

func registryServerStatus(configurationRequired bool, url string) string {
	if configurationRequired {
		return "configuration required"
	}
	if url != "" {
		return "ready"
	}
	return "unknown"
}

func writeMCPSearchTable(cmd *cobra.Command, servers []mcpSearchServer) error {
	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "TITLE\tDESCRIPTION\tSTATUS\tURL")
	for _, server := range servers {
		url := server.URL
		if url == "" {
			url = "-"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			tableCell(server.Title),
			tableCell(server.Description),
			tableCell(server.Status),
			tableCell(url),
		)
	}
	return w.Flush()
}

func registrySearchError(err error) error {
	var httpErr *types.ErrHTTP
	if !errors.As(err, &httpErr) {
		return err
	}
	switch httpErr.Code {
	case http.StatusUnauthorized:
		return fmt.Errorf(`registry search requires login; run "obot login" first`)
	case http.StatusForbidden:
		return fmt.Errorf("authenticated user is not authorized to access the registry endpoint")
	default:
		return err
	}
}
