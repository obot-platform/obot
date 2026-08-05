package mcpcatalog

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"sigs.k8s.io/yaml"
)

const defaultMaxCatalogFiles = 1000

// DiscoverCatalogFiles returns the catalog manifest files selected by a
// directory's .obotcatalogs and .ignoreobotcatalogs configuration.
func DiscoverCatalogFiles(root string) ([]string, bool, error) {
	patterns, usingObotCatalogsFile, err := catalogPatterns(root)
	if err != nil {
		return nil, false, err
	}
	ignorePatterns, err := readCatalogPatterns(filepath.Join(root, ".ignoreobotcatalogs"), nil)
	if err != nil {
		return nil, false, err
	}

	var files []string
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
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
		if entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		files = append(files, path)
		if len(files) > defaultMaxCatalogFiles {
			return fmt.Errorf("too many files to process (limit: %d)", defaultMaxCatalogFiles)
		}
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	return files, usingObotCatalogsFile, nil
}

func DecodeCatalogFile[T any](path string, strict bool) ([]T, bool, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return nil, false, err
	}

	if strict {
		var shape any
		if err := yaml.Unmarshal(contents, &shape); err != nil {
			return nil, false, err
		}
		if shape == nil {
			return nil, false, fmt.Errorf("catalog file is empty")
		}
		if _, ok := shape.([]any); ok {
			var entries []T
			if err := yaml.UnmarshalStrict(contents, &entries); err != nil {
				return nil, true, err
			}
			return entries, true, nil
		}
		var entry T
		if err := yaml.UnmarshalStrict(contents, &entry); err != nil {
			return nil, false, err
		}
		return []T{entry}, false, nil
	}

	var entries []T
	if err := yaml.Unmarshal(contents, &entries); err == nil {
		return entries, true, nil
	}

	var entry T
	if err := yaml.Unmarshal(contents, &entry); err != nil {
		return nil, false, err
	}
	return []T{entry}, false, nil
}

func catalogPatterns(root string) ([]string, bool, error) {
	patterns, err := readCatalogPatterns(filepath.Join(root, ".obotcatalogs"), []string{"*.json", "*.yaml", "*.yml"})
	if err != nil {
		return nil, false, err
	}
	_, err = os.Stat(filepath.Join(root, ".obotcatalogs"))
	return patterns, err == nil, nil
}

func readCatalogPatterns(path string, defaults []string) ([]string, error) {
	file, err := os.Open(path)
	if os.IsNotExist(err) {
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
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(patterns) == 0 {
		return defaults, nil
	}
	return patterns, nil
}

func matchesCatalogPattern(patterns []string, path string) bool {
	for _, pattern := range patterns {
		if matched, _ := filepath.Match(pattern, path); matched {
			return true
		}
	}
	return false
}
