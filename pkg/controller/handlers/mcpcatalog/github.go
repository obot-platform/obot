package mcpcatalog

import (
	"context"
	"fmt"
	"strings"

	"github.com/obot-platform/obot/pkg/git"
)

func readGitCatalogEntries[T any](ctx context.Context, catalogURL string, token string) ([]T, error) {
	if strings.HasPrefix(catalogURL, "http://") {
		return nil, fmt.Errorf("only HTTPS is supported for git catalogs")
	}

	dir, _, cleanup, err := git.Clone(ctx, catalogURL, token, "")
	if err != nil {
		return nil, err
	}
	defer cleanup()

	return readCatalogDirectory[T](dir)
}
