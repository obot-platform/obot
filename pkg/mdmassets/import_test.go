package mdmassets

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestImportDirectoryProducesImmutableDatabaseBundle(t *testing.T) {
	dir := writeAssets(t, SchemaVersion)
	first, err := Import(t.Context(), dir)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Import(t.Context(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("the same source did not produce deterministic bundle bytes")
	}

	installer := filepath.Join(dir, "windows", "intune", "obot-sentry.intunewin")
	if err := os.WriteFile(installer, []byte("changed-after-import"), 0o644); err != nil {
		t.Fatal(err)
	}
	loader, err := OpenArchive(first)
	if err != nil {
		t.Fatal(err)
	}
	if version := loader.Manifest().ObotSentryVersion; version != "1.2.3" {
		t.Fatalf("obot-sentry version = %q", version)
	}
	c, err := loader.Find("intune", "windows")
	if err != nil {
		t.Fatal(err)
	}
	var download bytes.Buffer
	if err := loader.Zip(&download, c, completedValues(t, loader)); err != nil {
		t.Fatal(err)
	}
	if got := zipEntries(t, download.Bytes())["obot-sentry.intunewin"]; got != "fake-intunewin" {
		t.Fatalf("database bundle followed source mutation: %q", got)
	}
}

func TestImportPersistsOnlyManifestClosure(t *testing.T) {
	dir := writeAssets(t, SchemaVersion)
	before, err := Import(t.Context(), dir)
	if err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(dir, "unrelated-secret.txt"), "must-not-enter-database")
	after, err := Import(t.Context(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("an unrelated source file changed the persisted bundle")
	}
	if _, exists := zipEntries(t, after)["unrelated-secret.txt"]; exists {
		t.Fatal("an unrelated source file was persisted in the bundle")
	}
}

func TestImportRemoteTarStripsCommonRoot(t *testing.T) {
	dir := writeAssets(t, SchemaVersion)
	want, err := Import(t.Context(), dir)
	if err != nil {
		t.Fatal(err)
	}
	archive := tarDirectory(t, dir, "obot-mdm-assets-v1")

	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": {"application/x-tar"}},
			Body:       io.NopCloser(bytes.NewReader(archive)),
		}, nil
	})}
	got, err := importSource(t.Context(), "https://example.test/release.tar", client)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("directory and equivalent remote tar did not normalize identically")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestImportRejectsUnsafeArchiveEntries(t *testing.T) {
	tests := []tar.Header{
		{Name: "../manifest.json", Mode: 0o644, Size: 2, Typeflag: tar.TypeReg},
		{Name: "/manifest.json", Mode: 0o644, Size: 2, Typeflag: tar.TypeReg},
		{Name: "root/link", Linkname: "root/manifest.json", Typeflag: tar.TypeSymlink},
	}
	for _, header := range tests {
		t.Run(strings.ReplaceAll(header.Name, "/", "_"), func(t *testing.T) {
			var buf bytes.Buffer
			tw := tar.NewWriter(&buf)
			if err := tw.WriteHeader(&header); err != nil {
				t.Fatal(err)
			}
			if header.Size > 0 {
				_, _ = tw.Write([]byte("{}"))
			}
			if err := tw.Close(); err != nil {
				t.Fatal(err)
			}
			if _, err := readTar(t.Context(), buf.Bytes()); err == nil {
				t.Fatalf("unsafe tar entry %q was accepted", header.Name)
			}
		})
	}
}

func TestImportRejectsOverflowingTarSize(t *testing.T) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	if err := tw.WriteHeader(&tar.Header{
		Name:     "manifest.json",
		Mode:     0o644,
		Size:     math.MaxInt64,
		Typeflag: tar.TypeReg,
		Format:   tar.FormatGNU,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := readTar(t.Context(), buf.Bytes()); err == nil || !strings.Contains(err.Error(), "maximum extracted size") {
		t.Fatalf("overflowing tar entry should be rejected by the size limit, got %v", err)
	}
}

func TestImportRejectsDirectoryHeaderBomb(t *testing.T) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for i := 0; i <= maxTarEntries; i++ {
		if err := tw.WriteHeader(&tar.Header{
			Name:     fmt.Sprintf("directory-%05d/", i),
			Mode:     0o755,
			Typeflag: tar.TypeDir,
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}

	_, err := readTar(t.Context(), buf.Bytes())
	if err == nil || !strings.Contains(err.Error(), "maximum entry count") {
		t.Fatalf("directory header bomb should be rejected by the entry limit, got %v", err)
	}
}

func TestImportRejectsDecodedGzipExpansion(t *testing.T) {
	payload := bytes.Repeat([]byte("x"), 8<<10)
	var tarBytes bytes.Buffer
	tw := tar.NewWriter(&tarBytes)
	if err := tw.WriteHeader(&tar.Header{
		Name:     "manifest.json",
		Mode:     0o644,
		Size:     int64(len(payload)),
		Typeflag: tar.TypeReg,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(payload); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}

	var compressed bytes.Buffer
	gw := gzip.NewWriter(&compressed)
	if _, err := gw.Write(tarBytes.Bytes()); err != nil {
		t.Fatal(err)
	}
	if err := gw.Close(); err != nil {
		t.Fatal(err)
	}
	const decodedLimit = 1 << 10
	if compressed.Len() >= decodedLimit {
		t.Fatalf("test archive did not compress below decoded limit: %d bytes", compressed.Len())
	}

	_, err := readTarWithDecodedLimit(t.Context(), compressed.Bytes(), decodedLimit)
	if err == nil || !strings.Contains(err.Error(), "maximum decoded size") {
		t.Fatalf("gzip expansion should be rejected by the decoded-size limit, got %v", err)
	}
}

func TestImportTarScanHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := readTar(ctx, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled tar scan returned %v", err)
	}
}

func TestRemoteImportErrorDoesNotLeakSignedURL(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, &url.Error{URL: "https://redirect.example/assets.tar?token=redirect-secret", Err: errors.New("failed")}
	})}
	_, err := importSource(t.Context(), "https://example.test/assets.tar?token=source-secret", client)
	if err == nil {
		t.Fatal("expected download error")
	}
	if strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), "token=") {
		t.Fatalf("download error leaked a signed URL: %v", err)
	}
}

func TestRedactSourceDoesNotLeakMalformedSignedURL(t *testing.T) {
	got := RedactSource("https://user:password@exa%mple.test/assets.tar?token=secret")
	if strings.Contains(got, "password") || strings.Contains(got, "secret") || strings.Contains(got, "token=") {
		t.Fatalf("malformed source was not redacted: %q", got)
	}
}

func TestImportRejectsLocalSymlink(t *testing.T) {
	dir := writeAssets(t, SchemaVersion)
	if err := os.Symlink(filepath.Join(dir, "manifest.json"), filepath.Join(dir, "manifest-link.json")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := Import(t.Context(), dir); err == nil || !strings.Contains(err.Error(), "symbolic link") {
		t.Fatalf("symlink should be rejected, got %v", err)
	}
}

func tarDirectory(t *testing.T, root, prefix string) []byte {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	err := filepath.WalkDir(root, func(name string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, name)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(name)
		if err != nil {
			return err
		}
		header := &tar.Header{
			Name:     prefix + "/" + filepath.ToSlash(rel),
			Mode:     0o644,
			Size:     int64(len(data)),
			Typeflag: tar.TypeReg,
		}
		if err := tw.WriteHeader(header); err != nil {
			return err
		}
		_, err = io.Copy(tw, bytes.NewReader(data))
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
