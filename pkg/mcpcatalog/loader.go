package mcpcatalog

import (
	"bufio"
	"fmt"
	"io/fs"
	"iter"
	"os"
	"path/filepath"
	"strings"

	"sigs.k8s.io/yaml"
)

const defaultMaxCatalogFiles = 1000

type CatalogWalker struct {
	root                  string
	patterns              []string
	ignorePatterns        []string
	usingObotCatalogsFile bool
}

func NewCatalogWalker(root string) (*CatalogWalker, error) {
	patterns, usingObotCatalogsFile, err := catalogPatterns(root)
	if err != nil {
		return nil, err
	}
	ignorePatterns, err := readCatalogPatterns(filepath.Join(root, ".ignoreobotcatalogs"), nil)
	if err != nil {
		return nil, err
	}
	return &CatalogWalker{
		root:                  root,
		patterns:              patterns,
		ignorePatterns:        ignorePatterns,
		usingObotCatalogsFile: usingObotCatalogsFile,
	}, nil
}

func (w *CatalogWalker) UsingObotCatalogsFile() bool {
	return w.usingObotCatalogsFile
}

// Files yields catalog manifest paths selected by .obotcatalogs and
// .ignoreobotcatalogs. Traversal errors are yielded in the second value.
func (w *CatalogWalker) Files() iter.Seq2[string, error] {
	return func(yield func(string, error) bool) {
		fileCount := 0
		err := filepath.WalkDir(w.root, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			relPath, err := filepath.Rel(w.root, path)
			if err != nil {
				return err
			}
			if entry.IsDir() {
				if relPath == ".git" || strings.HasPrefix(relPath, ".git"+string(filepath.Separator)) || matchesCatalogPattern(w.ignorePatterns, relPath) {
					return filepath.SkipDir
				}
				return nil
			}
			if !matchesCatalogPattern(w.patterns, filepath.Base(relPath)) || matchesCatalogPattern(w.ignorePatterns, relPath) {
				return nil
			}
			if entry.Type()&os.ModeSymlink != 0 {
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
	}
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
