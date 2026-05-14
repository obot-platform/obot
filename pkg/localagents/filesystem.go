package localagents

import (
	"fmt"
	"os"
	"path/filepath"
)

type installFile struct {
	RelPath string
	Content []byte
	Mode    os.FileMode
}

func replaceDir(target string, files []installFile) error {
	parent := filepath.Dir(target)
	if err := os.MkdirAll(parent, 0755); err != nil {
		return fmt.Errorf("failed to create %s: %w", parent, err)
	}

	tmp, err := os.MkdirTemp(parent, "."+filepath.Base(target)+".tmp-")
	if err != nil {
		return fmt.Errorf("failed to create temporary install directory: %w", err)
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.RemoveAll(tmp)
		}
	}()

	for _, file := range files {
		rel, err := cleanArchiveRelPath(file.RelPath)
		if err != nil {
			return err
		}
		if err := writeInstallFile(tmp, rel, file.Content, conservativeFileMode(file.Mode)); err != nil {
			return err
		}
	}

	if err := os.Chmod(tmp, 0755); err != nil {
		return fmt.Errorf("failed to set permissions on %s: %w", tmp, err)
	}

	if err := os.RemoveAll(target); err != nil {
		return fmt.Errorf("failed to remove existing install target %s: %w", target, err)
	}
	if err := os.Rename(tmp, target); err != nil {
		return fmt.Errorf("failed to move install target into place: %w", err)
	}
	cleanup = false

	return nil
}

func writeInstallFile(root, rel string, content []byte, mode os.FileMode) error {
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("failed to create directory for %s: %w", rel, err)
	}
	if err := os.WriteFile(path, content, mode); err != nil {
		return fmt.Errorf("failed to write %s: %w", rel, err)
	}
	return nil
}

func conservativeFileMode(mode os.FileMode) os.FileMode {
	if mode&0111 != 0 {
		return 0755
	}
	return 0644
}
