package mcpcatalog

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/mcp"
)

type ValidationSummary struct {
	Files   int
	Entries int
}

func ValidatePaths(ctx context.Context, paths []string, requireEntryKey bool) (ValidationSummary, error) {
	var summary ValidationSummary
	var errs []error
	seenFiles := map[string]struct{}{}
	seenEntryKeys := map[string]string{}
	options := ValidationOptions{
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
	limitReached := false
	validateFile := func(path string) {
		absPath, err := filepath.Abs(path)
		if err != nil {
			absPath = filepath.Clean(path)
		}
		if _, ok := seenFiles[absPath]; ok {
			return
		}
		if summary.Files >= defaultMaxCatalogFiles {
			errs = append(errs, fmt.Errorf("too many files to process (limit: %d)", defaultMaxCatalogFiles))
			limitReached = true
			return
		}
		seenFiles[absPath] = struct{}{}
		summary.Files++

		entries, isArray, err := DecodeCatalogFile[types.MCPServerCatalogEntryManifest](path, true)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: invalid catalog entry: %w", path, err))
			return
		}
		summary.Entries += len(entries)
		for i := range entries {
			entry := &entries[i]
			label := path
			if isArray {
				label = fmt.Sprintf("%s[%d]", path, i)
			}
			NormalizeManifest(entry)

			if requireEntryKey && strings.TrimSpace(entry.EntryKey) == "" {
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

	for _, input := range paths {
		if limitReached {
			break
		}
		info, err := os.Stat(input)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", input, err))
			continue
		}
		if !info.IsDir() {
			validateFile(input)
			continue
		}

		files, _, err := WalkCatalogFiles(input)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", input, err))
			continue
		}
		for path, walkErr := range files {
			if walkErr != nil {
				errs = append(errs, fmt.Errorf("%s: %w", input, walkErr))
				break
			}
			validateFile(path)
			if limitReached {
				break
			}
		}
	}
	if summary.Files == 0 {
		errs = append(errs, fmt.Errorf("no catalog entry files found"))
	}

	return summary, errors.Join(errs...)
}
