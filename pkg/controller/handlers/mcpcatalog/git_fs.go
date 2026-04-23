package mcpcatalog

import (
	"os"
	"sync/atomic"

	billy "github.com/go-git/go-billy/v5"
)

// sizeLimitedFS wraps a billy.Filesystem and rejects writes once maxBytes total
// have been written across all files. This is used during git clone to abort
// early if the repository exceeds the allowed size.
type sizeLimitedFS struct {
	billy.Filesystem
	written  atomic.Int64
	maxBytes int64
}

func (fs *sizeLimitedFS) Create(filename string) (billy.File, error) {
	f, err := fs.Filesystem.Create(filename)
	if err != nil {
		return nil, err
	}
	return &sizeLimitedFile{File: f, fs: fs}, nil
}

func (fs *sizeLimitedFS) OpenFile(filename string, flag int, perm os.FileMode) (billy.File, error) {
	f, err := fs.Filesystem.OpenFile(filename, flag, perm)
	if err != nil {
		return nil, err
	}
	return &sizeLimitedFile{File: f, fs: fs}, nil
}

func (fs *sizeLimitedFS) TempFile(dir, prefix string) (billy.File, error) {
	f, err := fs.Filesystem.TempFile(dir, prefix)
	if err != nil {
		return nil, err
	}
	return &sizeLimitedFile{File: f, fs: fs}, nil
}

// sizeLimitedFile wraps billy.File and intercepts Write calls to track cumulative
// bytes written. Returns errRepoTooLarge once the parent fs's limit is exceeded.
type sizeLimitedFile struct {
	billy.File
	fs *sizeLimitedFS
}

func (f *sizeLimitedFile) Write(p []byte) (int, error) {
	if f.fs.written.Add(int64(len(p))) > f.fs.maxBytes {
		return 0, errRepoTooLarge
	}
	return f.File.Write(p)
}
