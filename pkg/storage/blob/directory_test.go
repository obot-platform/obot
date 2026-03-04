package blob

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestDirectoryStore_NewRequiresBaseDir(t *testing.T) {
	_, err := NewDirectoryStore("")
	if err == nil {
		t.Fatal("expected error for empty base dir")
	}
}

func TestDirectoryStore_NewCreatesBaseDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "dir")
	store, err := NewDirectoryStore(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if store == nil {
		t.Fatal("expected non-nil store")
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("base dir should exist: %v", err)
	}
	if !info.IsDir() {
		t.Fatal("base dir should be a directory")
	}
}

func TestDirectoryStore_UploadAndDownload(t *testing.T) {
	store, err := NewDirectoryStore(t.TempDir())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ctx := context.Background()
	content := []byte("hello, world")

	if err := store.Upload(ctx, "bucket", "key.txt", bytes.NewReader(content)); err != nil {
		t.Fatalf("upload failed: %v", err)
	}

	rc, err := store.Download(ctx, "bucket", "key.txt")
	if err != nil {
		t.Fatalf("download failed: %v", err)
	}
	defer rc.Close()

	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if !bytes.Equal(got, content) {
		t.Fatalf("content mismatch: got %q, want %q", got, content)
	}
}

func TestDirectoryStore_UploadNestedKey(t *testing.T) {
	store, err := NewDirectoryStore(t.TempDir())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ctx := context.Background()
	content := []byte("nested content")

	if err := store.Upload(ctx, "bucket", "published-artifacts/my-artifact/v1.zip", bytes.NewReader(content)); err != nil {
		t.Fatalf("upload failed: %v", err)
	}

	rc, err := store.Download(ctx, "bucket", "published-artifacts/my-artifact/v1.zip")
	if err != nil {
		t.Fatalf("download failed: %v", err)
	}
	defer rc.Close()

	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if !bytes.Equal(got, content) {
		t.Fatalf("content mismatch: got %q, want %q", got, content)
	}
}

func TestDirectoryStore_DownloadNotFound(t *testing.T) {
	store, err := NewDirectoryStore(t.TempDir())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = store.Download(context.Background(), "bucket", "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent key")
	}
}

func TestDirectoryStore_Delete(t *testing.T) {
	store, err := NewDirectoryStore(t.TempDir())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ctx := context.Background()
	if err := store.Upload(ctx, "bucket", "key.txt", bytes.NewReader([]byte("data"))); err != nil {
		t.Fatalf("upload failed: %v", err)
	}

	if err := store.Delete(ctx, "bucket", "key.txt"); err != nil {
		t.Fatalf("delete failed: %v", err)
	}

	_, err = store.Download(ctx, "bucket", "key.txt")
	if err == nil {
		t.Fatal("expected error after delete")
	}
}

func TestDirectoryStore_DeleteNotFound(t *testing.T) {
	store, err := NewDirectoryStore(t.TempDir())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	err = store.Delete(context.Background(), "bucket", "nonexistent")
	if err == nil {
		t.Fatal("expected error for deleting nonexistent key")
	}
}

func TestDirectoryStore_PathTraversal(t *testing.T) {
	store, err := NewDirectoryStore(t.TempDir())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ctx := context.Background()

	// Traversal in bucket
	if err := store.Upload(ctx, "../escape", "key.txt", bytes.NewReader([]byte("bad"))); err == nil {
		t.Fatal("expected error for path traversal in bucket")
	}

	// Traversal in key
	if err := store.Upload(ctx, "bucket", "../../etc/passwd", bytes.NewReader([]byte("bad"))); err == nil {
		t.Fatal("expected error for path traversal in key")
	}

	// Traversal in download
	if _, err := store.Download(ctx, "bucket", "../secret"); err == nil {
		t.Fatal("expected error for path traversal in download")
	}

	// Traversal in delete
	if err := store.Delete(ctx, "../escape", "key"); err == nil {
		t.Fatal("expected error for path traversal in delete")
	}
}

func TestDirectoryStore_Test(t *testing.T) {
	store, err := NewDirectoryStore(t.TempDir())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := store.Test(context.Background()); err != nil {
		t.Fatalf("test failed: %v", err)
	}
}

func TestDirectoryStore_UploadOverwrite(t *testing.T) {
	store, err := NewDirectoryStore(t.TempDir())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ctx := context.Background()

	if err := store.Upload(ctx, "bucket", "key.txt", bytes.NewReader([]byte("first"))); err != nil {
		t.Fatalf("first upload failed: %v", err)
	}

	if err := store.Upload(ctx, "bucket", "key.txt", bytes.NewReader([]byte("second"))); err != nil {
		t.Fatalf("second upload failed: %v", err)
	}

	rc, err := store.Download(ctx, "bucket", "key.txt")
	if err != nil {
		t.Fatalf("download failed: %v", err)
	}
	defer rc.Close()

	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if string(got) != "second" {
		t.Fatalf("expected overwritten content 'second', got %q", got)
	}
}
