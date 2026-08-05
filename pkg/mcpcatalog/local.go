package mcpcatalog

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/mcp"
	"sigs.k8s.io/yaml"
)

const maxCatalogFiles = 1000

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
		info, err := os.Lstat(input)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", input, err))
			continue
		}
		if info.Mode()&os.ModeSymlink != 0 {
			errs = append(errs, fmt.Errorf("%s: symbolic links are not allowed", input))
			continue
		}
		if !info.IsDir() {
			addDiscoveredFile(seen, input)
			continue
		}
		if err := discoverDirectory(input, seen); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", input, err))
		}
	}

	files := make([]string, 0, len(seen))
	for _, path := range seen {
		files = append(files, path)
	}
	slices.Sort(files)
	if len(files) > maxCatalogFiles {
		errs = append(errs, fmt.Errorf("too many files to process (limit: %d)", maxCatalogFiles))
		files = files[:maxCatalogFiles]
	}
	if len(files) == 0 {
		errs = append(errs, fmt.Errorf("no catalog entry files found"))
	}
	return files, errors.Join(errs...)
}

func discoverDirectory(root string, seen map[string]string) error {
	patterns, err := readPatterns(filepath.Join(root, ".obotcatalogs"), []string{"*.json", "*.yaml", "*.yml"})
	if err != nil {
		return err
	}
	ignorePatterns, err := readPatterns(filepath.Join(root, ".ignoreobotcatalogs"), nil)
	if err != nil {
		return err
	}

	return filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relPath, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if relPath == ".git" || strings.HasPrefix(relPath, ".git"+string(filepath.Separator)) || matchesAny(ignorePatterns, relPath) {
				return filepath.SkipDir
			}
			return nil
		}
		if !matchesAny(patterns, filepath.Base(relPath)) || matchesAny(ignorePatterns, relPath) {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("%s: symbolic links are not allowed", path)
		}
		addDiscoveredFile(seen, path)
		return nil
	})
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

func readPatterns(path string, defaults []string) ([]string, error) {
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return defaults, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var patterns []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" && !strings.HasPrefix(line, "#") {
			patterns = append(patterns, line)
		}
	}
	if err := scanner.Err(); err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	if len(patterns) == 0 {
		return defaults, nil
	}
	return patterns, nil
}

func matchesAny(patterns []string, path string) bool {
	for _, pattern := range patterns {
		if matched, _ := filepath.Match(pattern, path); matched {
			return true
		}
	}
	return false
}

func decodeCatalogFile(path string) ([]types.MCPServerCatalogEntryManifest, bool, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return nil, false, err
	}
	var shape any
	if err := yaml.Unmarshal(contents, &shape); err != nil {
		return nil, false, fmt.Errorf("invalid YAML or JSON: %w", err)
	}
	if shape == nil {
		return nil, false, fmt.Errorf("catalog file is empty")
	}
	if _, ok := shape.([]any); ok {
		var entries []types.MCPServerCatalogEntryManifest
		if err := yaml.UnmarshalStrict(contents, &entries); err != nil {
			return nil, true, fmt.Errorf("invalid catalog entries: %w", err)
		}
		return entries, true, nil
	}

	var entry types.MCPServerCatalogEntryManifest
	if err := yaml.UnmarshalStrict(contents, &entry); err != nil {
		return nil, false, fmt.Errorf("invalid catalog entry: %w", err)
	}
	return []types.MCPServerCatalogEntryManifest{entry}, false, nil
}
