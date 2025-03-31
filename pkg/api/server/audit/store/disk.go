package store

import (
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"

	"github.com/adrg/xdg"
)

type DiskStoreOptions struct {
	AuditLogsStoreDir string `usage:"Audit log store directory, defaults to $XDG_DATA_HOME/obot/audit"`
}

type diskStore struct {
	dir      string
	host     string
	compress bool
}

func NewDiskStore(host string, compress bool, options DiskStoreOptions) (Store, error) {
	dir := options.AuditLogsStoreDir
	if dir == "" {
		dir = filepath.Join(xdg.DataHome, "obot", "audit")
	}

	d := &diskStore{dir: dir, host: host, compress: compress}
	return d, d.ensureDir()
}

func (s *diskStore) Persist(b []byte) error {
	fname := filename(s.host)

	if err := s.ensureDir(); err != nil {
		return err
	}

	f, err := os.OpenFile(filepath.Join(s.dir, fname), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	if s.compress {
		gz := gzip.NewWriter(f)
		defer gz.Close()

		_, err = io.Copy(gz, bytes.NewReader(b))
		return err
	}

	_, err = f.Write(b)
	return err
}

func (s *diskStore) ensureDir() error {
	return os.MkdirAll(s.dir, 0755)
}
