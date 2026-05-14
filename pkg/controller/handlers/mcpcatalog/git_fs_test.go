package mcpcatalog

import (
	"testing"

	"github.com/go-git/go-billy/v5/osfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSizeLimitedFS(t *testing.T) {
	dir := t.TempDir()
	fs := &sizeLimitedFS{Filesystem: osfs.New(dir), maxBytes: 10}

	f, err := fs.Create("test.bin")
	require.NoError(t, err)
	defer f.Close()

	// Write within limit.
	_, err = f.Write([]byte("hello")) // 5 bytes
	require.NoError(t, err)

	// Write that crosses the limit.
	_, err = f.Write([]byte("world!")) // 6 bytes → 11 total > 10
	assert.ErrorIs(t, err, errRepoTooLarge)
}
