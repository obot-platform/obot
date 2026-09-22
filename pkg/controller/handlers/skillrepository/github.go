package skillrepository

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"
)

type fetchedRepository struct {
	RepoRoot  string
	CommitSHA string
	cleanup   func()
}

func (f *fetchedRepository) Cleanup() {
	if f != nil && f.cleanup != nil {
		f.cleanup()
	}
}

func safeJoinWithin(baseDir, relPath string) (string, error) {
	cleanPath := path.Clean(filepath.ToSlash(relPath))
	if cleanPath == "." || cleanPath == "" {
		return baseDir, nil
	}
	if strings.HasPrefix(cleanPath, "../") || path.IsAbs(cleanPath) {
		return "", fmt.Errorf("path %q escapes the repository root", relPath)
	}

	joined := filepath.Join(baseDir, filepath.FromSlash(cleanPath))
	absJoined, err := filepath.Abs(joined)
	if err != nil {
		return "", err
	}
	absBase, err := filepath.Abs(baseDir)
	if err != nil {
		return "", err
	}
	if absJoined != absBase && !strings.HasPrefix(absJoined, absBase+string(filepath.Separator)) {
		return "", fmt.Errorf("path %q escapes the repository root", relPath)
	}

	return absJoined, nil
}
