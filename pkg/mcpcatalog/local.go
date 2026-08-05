package mcpcatalog

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/mcp"
)

type ValidationSummary struct {
	Files   int
	Entries int
}

type LocalValidationOptions struct {
	RequireEntryKey bool
}

func ValidatePaths(ctx context.Context, paths []string, localOptions LocalValidationOptions) (ValidationSummary, error) {
	files, discoveryErr := discoverFiles(paths)
	summary := ValidationSummary{Files: len(files)}
	var errs []error
	if discoveryErr != nil {
		errs = append(errs, discoveryErr)
	}

	seenEntryKeys := map[string]string{}
	options := ValidationOptions{
		GitManaged: true,
		MCP: mcp.ValidationOptions{
			RemoteMCPURLValidationConfig: mcp.RemoteMCPURLValidationConfig{
				AllowLocalhostMCP: true,
				AllowPrivateIPMCP: true,
				AllowLinkLocalMCP: true,
			},
		},
	}
	for _, path := range files {
		entries, isArray, err := decodeCatalogFile(path)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", path, err))
			continue
		}
		summary.Entries += len(entries)
		for i := range entries {
			entry := &entries[i]
			label := path
			if isArray {
				label = fmt.Sprintf("%s[%d]", path, i)
			}
			NormalizeManifest(entry)

			if localOptions.RequireEntryKey && strings.TrimSpace(entry.EntryKey) == "" {
				errs = append(errs, fmt.Errorf("%s: entryKey is required", label))
			}
			if err := ValidateSourceFields(*entry); err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", label, err))
			}
			if entry.EntryKey != "" {
				if previous, ok := seenEntryKeys[entry.EntryKey]; ok {
					errs = append(errs, fmt.Errorf("%s: duplicate source entry key %q also used by %s", label, entry.EntryKey, previous))
				} else {
					seenEntryKeys[entry.EntryKey] = label
				}
			}
			if err := ValidateManifest(ctx, *entry, options); err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", label, err))
			}
		}
	}

	return summary, errors.Join(errs...)
}

func discoverFiles(paths []string) ([]string, error) {
	seen := map[string]string{}
	var errs []error
	for _, input := range paths {
		info, err := os.Stat(input)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", input, err))
			continue
		}
		if !info.IsDir() {
			addDiscoveredFile(seen, input)
			continue
		}
		files, _, err := DiscoverCatalogFiles(input)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", input, err))
			continue
		}
		for _, path := range files {
			addDiscoveredFile(seen, path)
		}
	}

	files := make([]string, 0, len(seen))
	for _, path := range seen {
		files = append(files, path)
	}
	slices.Sort(files)
	if len(files) > defaultMaxCatalogFiles {
		errs = append(errs, fmt.Errorf("too many files to process (limit: %d)", defaultMaxCatalogFiles))
		files = files[:defaultMaxCatalogFiles]
	}
	if len(files) == 0 {
		errs = append(errs, fmt.Errorf("no catalog entry files found"))
	}
	return files, errors.Join(errs...)
}

func addDiscoveredFile(seen map[string]string, path string) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		absPath = filepath.Clean(path)
	}
	if _, ok := seen[absPath]; !ok {
		seen[absPath] = path
	}
}

func decodeCatalogFile(path string) ([]types.MCPServerCatalogEntryManifest, bool, error) {
	entries, isArray, err := DecodeCatalogFile[types.MCPServerCatalogEntryManifest](path, true)
	if err != nil {
		return nil, isArray, fmt.Errorf("invalid catalog entry: %w", err)
	}
	return entries, isArray, nil
}
