package blob

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// DirectoryStore implements BlobStore using the local filesystem.
// Objects are stored at <baseDir>/<bucket>/<key>.
type DirectoryStore struct {
	baseDir string
}

// NewDirectoryStore creates a new DirectoryStore rooted at baseDir.
// The directory is created if it does not exist.
func NewDirectoryStore(baseDir string) (*DirectoryStore, error) {
	if baseDir == "" {
		return nil, fmt.Errorf("base directory is required")
	}
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create base directory %s: %w", baseDir, err)
	}
	return &DirectoryStore{baseDir: baseDir}, nil
}

func (d *DirectoryStore) objectPath(bucket, key string) (string, error) {
	if strings.Contains(bucket, "..") || strings.Contains(key, "..") {
		return "", fmt.Errorf("invalid path: bucket and key must not contain '..'")
	}
	return filepath.Join(d.baseDir, bucket, key), nil
}

func (d *DirectoryStore) Upload(_ context.Context, bucket, key string, data io.Reader) error {
	log.Debugf("Directory upload: bucket=%s key=%s baseDir=%s", bucket, key, d.baseDir)
	p, err := d.objectPath(bucket, key)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return fmt.Errorf("failed to create directories for %s: %w", p, err)
	}

	// Write to a temp file in the same directory, then rename for atomicity.
	tmp, err := os.CreateTemp(filepath.Dir(p), ".blob-upload-*")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpName := tmp.Name()

	if _, err := io.Copy(tmp, data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("failed to write data: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("failed to close temp file: %w", err)
	}
	if err := os.Rename(tmpName, p); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	log.Debugf("Directory upload complete: %s", p)
	return nil
}

func (d *DirectoryStore) Download(_ context.Context, bucket, key string) (io.ReadCloser, error) {
	log.Debugf("Directory download: bucket=%s key=%s baseDir=%s", bucket, key, d.baseDir)
	p, err := d.objectPath(bucket, key)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(p)
	if err != nil {
		return nil, fmt.Errorf("failed to open %s: %w", p, err)
	}
	return f, nil
}

func (d *DirectoryStore) Delete(_ context.Context, bucket, key string) error {
	log.Debugf("Directory delete: bucket=%s key=%s baseDir=%s", bucket, key, d.baseDir)
	p, err := d.objectPath(bucket, key)
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil {
		return fmt.Errorf("failed to delete %s: %w", p, err)
	}
	return nil
}

func (d *DirectoryStore) Test(_ context.Context) error {
	tmp, err := os.CreateTemp(d.baseDir, ".blob-test-*")
	if err != nil {
		return fmt.Errorf("directory store test failed: %w", err)
	}
	name := tmp.Name()
	tmp.Close()
	os.Remove(name)
	return nil
}
