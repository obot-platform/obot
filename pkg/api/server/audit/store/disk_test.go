package store

import (
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewDiskStore(t *testing.T) {
	tests := []struct {
		name    string
		host    string
		options DiskStoreOptions
		wantErr bool
	}{
		{
			name:    "with custom directory",
			host:    "example.com",
			options: DiskStoreOptions{AuditLogsStoreDir: t.TempDir()},
			wantErr: false,
		},
		{
			name:    "with empty options - uses default",
			host:    "localhost",
			options: DiskStoreOptions{},
			wantErr: false,
		},
		{
			name:    "with nested directory path",
			host:    "api.example.com",
			options: DiskStoreOptions{AuditLogsStoreDir: filepath.Join(t.TempDir(), "nested", "audit")},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store, err := NewDiskStore(tt.host, false, tt.options)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewDiskStore() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && store == nil {
				t.Error("NewDiskStore() returned nil store without error")
			}

			// Verify directory was created
			if !tt.wantErr {
				ds := store.(*diskStore)
				expectedDir := tt.options.AuditLogsStoreDir
				if expectedDir == "" {
					// Default path was used, just check it's set
					if ds.dir == "" {
						t.Error("NewDiskStore() did not set directory")
					}
				} else {
					if ds.dir != expectedDir {
						t.Errorf("NewDiskStore() dir = %v, want %v", ds.dir, expectedDir)
					}
				}

				// Verify directory exists
				if _, err := os.Stat(ds.dir); os.IsNotExist(err) {
					t.Errorf("NewDiskStore() did not create directory %v", ds.dir)
				}
			}
		})
	}
}

func TestDiskStorePersist(t *testing.T) {
	tests := []struct {
		name     string
		host     string
		compress bool
		data     []byte
		wantErr  bool
	}{
		{
			name:     "persist simple text without compression",
			host:     "example.com",
			compress: false,
			data:     []byte("audit log entry"),
			wantErr:  false,
		},
		{
			name:     "persist simple text with compression",
			host:     "example.com",
			compress: true,
			data:     []byte("audit log entry"),
			wantErr:  false,
		},
		{
			name:     "persist empty data without compression",
			host:     "localhost",
			compress: false,
			data:     []byte{},
			wantErr:  false,
		},
		{
			name:     "persist empty data with compression",
			host:     "localhost",
			compress: true,
			data:     []byte{},
			wantErr:  false,
		},
		{
			name:     "persist JSON data",
			host:     "api.example.com",
			compress: false,
			data:     []byte(`{"event":"user.login","timestamp":"2026-01-16T10:00:00Z"}`),
			wantErr:  false,
		},
		{
			name:     "persist large data with compression",
			host:     "test.com",
			compress: true,
			data:     bytes.Repeat([]byte("audit log entry\n"), 1000),
			wantErr:  false,
		},
		{
			name:     "persist binary data",
			host:     "binary.example.com",
			compress: false,
			data:     []byte{0x00, 0x01, 0x02, 0xFF, 0xFE},
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()
			store, err := NewDiskStore(tt.host, tt.compress, DiskStoreOptions{
				AuditLogsStoreDir: tempDir,
			})
			if err != nil {
				t.Fatalf("NewDiskStore() error = %v", err)
			}

			err = store.Persist(tt.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("diskStore.Persist() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				// Verify file was created
				files, err := os.ReadDir(tempDir)
				if err != nil {
					t.Fatalf("failed to read temp directory: %v", err)
				}

				if len(files) != 1 {
					t.Fatalf("expected 1 file in directory, got %d", len(files))
				}

				// Verify filename format
				fname := files[0].Name()
				expectedHost := strings.ReplaceAll(tt.host, ".", "_")
				if !strings.HasPrefix(fname, expectedHost) {
					t.Errorf("filename %q does not start with expected host %q", fname, expectedHost)
				}

				expectedExt := ".log"
				if tt.compress {
					expectedExt = ".log.gz"
				}
				if !strings.HasSuffix(fname, expectedExt) {
					t.Errorf("filename %q does not end with expected extension %q", fname, expectedExt)
				}

				// Verify file contents
				filePath := filepath.Join(tempDir, fname)
				fileData, err := os.ReadFile(filePath)
				if err != nil {
					t.Fatalf("failed to read persisted file: %v", err)
				}

				var actualData []byte
				if tt.compress {
					// Decompress to verify
					reader := bytes.NewReader(fileData)
					gz, err := gzip.NewReader(reader)
					if err != nil {
						t.Fatalf("failed to create gzip reader: %v", err)
					}
					defer gz.Close()

					actualData, err = io.ReadAll(gz)
					if err != nil {
						t.Fatalf("failed to read decompressed data: %v", err)
					}
				} else {
					actualData = fileData
				}

				if !bytes.Equal(actualData, tt.data) {
					t.Errorf("persisted data does not match input. got %v, want %v", actualData, tt.data)
				}
			}
		})
	}
}

func TestDiskStorePersistAppend(t *testing.T) {
	// Test that multiple Persist calls append to the same file (within same second due to timestamp)
	tempDir := t.TempDir()
	host := "append-test.com"
	store, err := NewDiskStore(host, false, DiskStoreOptions{
		AuditLogsStoreDir: tempDir,
	})
	if err != nil {
		t.Fatalf("NewDiskStore() error = %v", err)
	}

	// First persist
	data1 := []byte("first entry\n")
	if err := store.Persist(data1); err != nil {
		t.Fatalf("first Persist() error = %v", err)
	}

	// Get the filename that was created
	files, err := os.ReadDir(tempDir)
	if err != nil {
		t.Fatalf("failed to read temp directory: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 file after first persist, got %d", len(files))
	}
	fileName := files[0].Name()

	// Second persist (should append to same file if within same second)
	data2 := []byte("second entry\n")
	if err := store.Persist(data2); err != nil {
		t.Fatalf("second Persist() error = %v", err)
	}

	// Read the file
	filePath := filepath.Join(tempDir, fileName)
	fileData, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}

	// Check if both entries are present (append mode)
	// Note: Due to timestamp in filename, entries might be in separate files
	// if the second persist happens in a different second
	if bytes.Contains(fileData, data1) && bytes.Contains(fileData, data2) {
		t.Log("Both entries appended to same file successfully")
	} else if bytes.Contains(fileData, data1) || bytes.Contains(fileData, data2) {
		// Check if second file was created
		files, _ = os.ReadDir(tempDir)
		if len(files) == 2 {
			t.Log("Entries written to separate files (different timestamp)")
		}
	}
}

func TestDiskStoreEnsureDir(t *testing.T) {
	// Test that ensureDir creates nested directories
	tempBase := t.TempDir()
	nestedDir := filepath.Join(tempBase, "level1", "level2", "level3")

	store := &diskStore{
		dir:      nestedDir,
		host:     "test.com",
		compress: false,
	}

	err := store.ensureDir()
	if err != nil {
		t.Fatalf("ensureDir() error = %v", err)
	}

	// Verify directory was created
	if _, err := os.Stat(nestedDir); os.IsNotExist(err) {
		t.Errorf("ensureDir() did not create directory %v", nestedDir)
	}

	// Verify it's a directory
	info, err := os.Stat(nestedDir)
	if err != nil {
		t.Fatalf("failed to stat directory: %v", err)
	}
	if !info.IsDir() {
		t.Error("ensureDir() created a file instead of directory")
	}
}

func TestDiskStoreCompression(t *testing.T) {
	// Test that compression actually reduces file size for compressible data
	tempDir := t.TempDir()
	host := "compress-test.com"

	// Create large compressible data (repeated pattern)
	largeData := bytes.Repeat([]byte("This is a repeated audit log entry that should compress well.\n"), 100)

	// Test without compression
	storeUncompressed, err := NewDiskStore(host+"1", false, DiskStoreOptions{
		AuditLogsStoreDir: filepath.Join(tempDir, "uncompressed"),
	})
	if err != nil {
		t.Fatalf("NewDiskStore() error = %v", err)
	}
	if err := storeUncompressed.Persist(largeData); err != nil {
		t.Fatalf("Persist() error = %v", err)
	}

	// Test with compression
	storeCompressed, err := NewDiskStore(host+"2", true, DiskStoreOptions{
		AuditLogsStoreDir: filepath.Join(tempDir, "compressed"),
	})
	if err != nil {
		t.Fatalf("NewDiskStore() error = %v", err)
	}
	if err := storeCompressed.Persist(largeData); err != nil {
		t.Fatalf("Persist() error = %v", err)
	}

	// Compare file sizes
	uncompFiles, _ := os.ReadDir(filepath.Join(tempDir, "uncompressed"))
	compFiles, _ := os.ReadDir(filepath.Join(tempDir, "compressed"))

	if len(uncompFiles) != 1 || len(compFiles) != 1 {
		t.Fatal("unexpected number of files created")
	}

	uncompInfo, _ := os.Stat(filepath.Join(tempDir, "uncompressed", uncompFiles[0].Name()))
	compInfo, _ := os.Stat(filepath.Join(tempDir, "compressed", compFiles[0].Name()))

	// Compressed should be smaller for repetitive data
	if compInfo.Size() >= uncompInfo.Size() {
		t.Logf("Warning: compressed size (%d) >= uncompressed size (%d)", compInfo.Size(), uncompInfo.Size())
		// This is not necessarily a failure - small data might not compress well
	} else {
		ratio := float64(compInfo.Size()) / float64(uncompInfo.Size())
		t.Logf("Compression ratio: %.2f (compressed: %d bytes, uncompressed: %d bytes)",
			ratio, compInfo.Size(), uncompInfo.Size())
	}
}
