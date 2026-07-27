package mcpcatalog

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/obot-platform/obot/pkg/git"
)

func readGitCatalogEntries[T any](ctx context.Context, catalogURL, token string) ([]T, error) {
	return readGitCatalogEntriesFromSubdir(ctx, catalogURL, token, "", readCatalogDirectory[T])
}

func readGitCatalogEntriesFromSubdir[T any](ctx context.Context, catalogURL, token, subdir string, readDir func(string) ([]T, error)) ([]T, error) {
	if strings.HasPrefix(catalogURL, "http://") {
		return nil, fmt.Errorf("only HTTPS is supported for git catalogs")
	}

	if subdir != "" {
		cleanSubdir := filepath.Clean(subdir)
		if filepath.IsAbs(cleanSubdir) || cleanSubdir == ".." || strings.HasPrefix(cleanSubdir, "../") {
			return nil, fmt.Errorf("invalid catalog subdirectory %q", subdir)
		}
		subdir = cleanSubdir
	}

	dir, _, cleanup, err := git.Clone(ctx, catalogURL, token, "")
	if err != nil {
		return nil, err
	}
	defer cleanup()

	if subdir != "" {
		dir = filepath.Join(dir, subdir)
	}
	return readDir(dir)
}
