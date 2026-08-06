package mcpcatalog

import (
	"bufio"
	"fmt"
	"io/fs"
	"iter"
	"os"
	"path/filepath"
	"strings"

	"github.com/obot-platform/obot/logger"
	"sigs.k8s.io/yaml"
)

const defaultMaxCatalogFiles = 1000

var log = logger.Package()

// WalkCatalogFiles returns catalog manifest paths selected by .obotcatalogs and
// .ignoreobotcatalogs. Traversal errors are yielded in the second value.
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
				if relPath == ".git" || strings.HasPrefix(relPath, ".git"+string(filepath.Separator)) || matchesCatalogPattern(ignorePatterns, relPath) {
					return filepath.SkipDir
				}
				return nil
			}
			if !matchesCatalogPattern(patterns, filepath.Base(relPath)) || matchesCatalogPattern(ignorePatterns, relPath) {
				return nil
			}
			info, err := os.Lstat(path)
			if err != nil {
				log.Warnf("Skipping unsafe file %s: failed to get file info: %v", relPath, err)
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
		log.Warnf("Failed to read %s file: %v", filepath.Base(path), err)
		return defaults, true
	}
	if len(patterns) == 0 {
		return defaults, true
	}
	return patterns, true
}

func matchesCatalogPattern(patterns []string, path string) bool {
	for _, pattern := range patterns {
		if matched, _ := filepath.Match(pattern, path); matched {
			return true
		}
	}
	return false
}
