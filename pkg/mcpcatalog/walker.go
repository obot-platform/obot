package mcpcatalog

import (
	"bufio"
	"fmt"
	"io/fs"
	"iter"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"strings"

	"sigs.k8s.io/yaml"
)

const (
	defaultMaxCatalogFiles = 1000
)

// WalkCatalogFiles returns catalog manifest paths selected by .obotcatalogs and
// .ignoreobotcatalogs, skipping hidden child directories. Traversal errors are
// yielded in the second value.
func WalkCatalogFiles(root string) (iter.Seq2[string, error], bool, error) {
	patterns, usingObotCatalogsFile := catalogPatterns(root)
	ignorePatterns, _ := readCatalogPatterns(filepath.Join(root, ".ignoreobotcatalogs"), nil)
	return func(yield func(string, error) bool) {
		fileCount := 0
		err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			relPath, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			if entry.IsDir() {
				if relPath != "." && strings.HasPrefix(entry.Name(), ".") || matchesCatalogPattern(ignorePatterns, relPath) {
					return filepath.SkipDir
				}

				return nil
			}

			// Basename patterns apply at any depth; path patterns are relative to root.
			included := matchesCatalogPattern(patterns, filepath.Base(relPath)) || matchesCatalogPattern(patterns, relPath)
			if !included || matchesCatalogPattern(ignorePatterns, relPath) {
				return nil
			}

			info, err := os.Lstat(path)
			if err != nil {
				slog.Warn("Skipping unsafe file, failed to get file info", "path", relPath, "error", err)
				return nil
			}
			if info.Mode()&os.ModeSymlink != 0 {
				return nil
			}
			fileCount++
			if fileCount > defaultMaxCatalogFiles {
				return fmt.Errorf("too many files to process (limit: %d)", defaultMaxCatalogFiles)
			}
			if !yield(path, nil) {
				return fs.SkipAll
			}
			return nil
		})
		if err != nil {
			yield("", err)
		}
	}, usingObotCatalogsFile, nil
}

func DecodeCatalogFile[T any](path string, strict bool) ([]T, bool, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return nil, false, err
	}

	var shape any
	if err := yaml.Unmarshal(contents, &shape); err != nil {
		return nil, false, err
	}
	if shape == nil {
		if strict {
			return nil, false, fmt.Errorf("catalog file is empty")
		}
		return nil, true, nil
	}

	decode := yaml.Unmarshal
	if strict {
		decode = yaml.UnmarshalStrict
	}
	if _, ok := shape.([]any); ok {
		var entries []T
		if err := decode(contents, &entries); err != nil {
			return nil, true, err
		}
		return entries, true, nil
	}
	var entry T
	if err := decode(contents, &entry); err != nil {
		return nil, false, err
	}
	return []T{entry}, false, nil
}

func catalogPatterns(root string) ([]string, bool) {
	return readCatalogPatterns(filepath.Join(root, ".obotcatalogs"), []string{"*.json", "*.yaml", "*.yml"})
}

func readCatalogPatterns(path string, defaults []string) ([]string, bool) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return defaults, false
	}

	var patterns []string
	scanner := bufio.NewScanner(strings.NewReader(string(contents)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" && !strings.HasPrefix(line, "#") {
			patterns = append(patterns, line)
		}
	}
	if err := scanner.Err(); err != nil {
		slog.Warn("Failed to read file", "fileName", filepath.Base(path), "error", err)
		return defaults, true
	}
	if len(patterns) == 0 {
		return defaults, true
	}
	return patterns, true
}

func matchesCatalogPattern(patterns []string, candidate string) bool {
	// Catalog patterns use forward slashes on every platform.
	candidate = filepath.ToSlash(candidate)
	for _, pattern := range patterns {
		if matched, _ := path.Match(pattern, candidate); matched {
			return true
		}
	}
	return false
}
