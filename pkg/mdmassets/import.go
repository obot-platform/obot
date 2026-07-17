package mdmassets

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"testing/fstest"
	"time"
)

const (
	maxSourceArchiveBytes = 256 << 20
	maxExtractedBytes     = 512 << 20
	maxBundleFiles        = 10_000
	maxTarEntries         = 10_000
	tarBlockBytes         = 512
	// A valid tar needs one header and at most one padding block per entry,
	// plus two end-of-archive blocks. Limiting the decoded stream separately
	// keeps compressed metadata and extension-header bombs bounded too.
	maxDecodedTarBytes = maxExtractedBytes + maxTarEntries*2*tarBlockBytes + 2*tarBlockBytes
	maxRedirects       = 5
)

// Import reads source, validates the complete assets snapshot, and normalizes
// it into deterministic archive bytes. HTTP(S) sources are tar archives. Local
// sources may be either the existing assets directory or a tar archive.
func Import(ctx context.Context, source string) ([]byte, error) {
	return importSource(ctx, source, newHTTPClient())
}

// RedactSource returns a source suitable for status and UI display. Local paths
// are unchanged. URL credentials, query parameters, and fragments are never
// exposed, including when a malformed HTTP URL cannot be parsed safely.
func RedactSource(source string) string {
	lower := strings.ToLower(strings.TrimSpace(source))
	isHTTP := strings.HasPrefix(lower, "http://")
	isHTTPS := strings.HasPrefix(lower, "https://")
	if !isHTTP && !isHTTPS {
		return source
	}
	parsed, err := url.Parse(source)
	if err != nil {
		if isHTTPS {
			return "https://[invalid URL redacted]"
		}
		return "http://[invalid URL redacted]"
	}
	parsed.User = nil
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String()
}

func newHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 2 * time.Minute,
		// Redirect targets are controlled by the remote source. Bound the chain
		// and keep it on credential-free HTTP(S) URLs to prevent redirect loops,
		// unsupported schemes, and URL credentials leaking through requests or errors.
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
				return fmt.Errorf("stopped after %d redirects", maxRedirects)
			}
			if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
				return fmt.Errorf("redirected to unsupported URL scheme %q", req.URL.Scheme)
			}
			if req.URL.User != nil {
				return fmt.Errorf("redirected to a URL containing user information")
			}
			return nil
		},
	}
}

func importSource(ctx context.Context, source string, client *http.Client) ([]byte, error) {
	if strings.TrimSpace(source) == "" {
		return nil, fmt.Errorf("MDM asset source is empty")
	}

	var (
		files map[string][]byte
		err   error
	)
	u, parseErr := url.Parse(source)
	if parseErr == nil && (u.Scheme == "http" || u.Scheme == "https") {
		files, err = readRemoteTar(ctx, client, u)
	} else {
		files, err = readLocalSource(ctx, source)
	}
	if err != nil {
		return nil, err
	}

	files, err = normalizeArchiveRoot(files)
	if err != nil {
		return nil, err
	}
	loader, err := loaderFromFiles(files)
	if err != nil {
		return nil, err
	}
	files = manifestClosure(files, loader)
	content, err := canonicalArchive(files)
	if err != nil {
		return nil, err
	}
	if len(content) > maxSourceArchiveBytes {
		return nil, fmt.Errorf("canonical MDM asset bundle exceeds maximum stored size of %d bytes", maxSourceArchiveBytes)
	}
	return content, nil
}

func loaderFromFiles(files map[string][]byte) (*Loader, error) {
	memory := make(fstest.MapFS, len(files))
	for name, data := range files {
		memory[name] = &fstest.MapFile{Data: data, Mode: 0o644}
	}
	return NewFS(memory)
}

// manifestClosure drops unrelated source-tree files before persistence. This
// prevents a broad directory or GitHub source archive from copying unrelated
// source code or secrets into the database and keeps irrelevant changes from
// changing the bundle digest.
func manifestClosure(files map[string][]byte, loader *Loader) map[string][]byte {
	needed := map[string]struct{}{"manifest.json": {}}
	manifest := loader.Manifest()
	for _, platform := range manifest.Platforms {
		if platform.Icon != "" {
			needed[platform.Icon] = struct{}{}
		}
	}
	for _, configuration := range manifest.Configurations {
		for _, asset := range configuration.Assets {
			needed[asset] = struct{}{}
		}
	}
	result := make(map[string][]byte, len(needed))
	for name := range needed {
		result[name] = files[name]
	}
	return result
}

func readRemoteTar(ctx context.Context, client *http.Client, source *url.URL) (map[string][]byte, error) {
	if source.User != nil {
		return nil, fmt.Errorf("MDM asset source URL must not contain user information")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, source.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("creating MDM asset source request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		// net/http's URL error contains the full (possibly signed) URL. Keep the
		// underlying transport error while excluding URL credentials/query data.
		for {
			var urlErr *url.Error
			if !errors.As(err, &urlErr) || urlErr.Err == nil || urlErr.Err == err {
				break
			}
			err = urlErr.Err
		}
		return nil, fmt.Errorf("downloading MDM asset source: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("downloading MDM asset source returned status %d", resp.StatusCode)
	}
	content, err := readBounded(resp.Body, maxSourceArchiveBytes, "MDM asset source archive")
	if err != nil {
		return nil, err
	}
	return readTar(ctx, content)
}

func readLocalSource(ctx context.Context, source string) (map[string][]byte, error) {
	info, err := os.Stat(source)
	if err != nil {
		return nil, fmt.Errorf("opening local MDM asset source: %w", err)
	}
	if info.IsDir() {
		return readDirectory(ctx, source)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("local MDM asset source %q is not a directory or regular file", source)
	}
	f, err := os.Open(source)
	if err != nil {
		return nil, fmt.Errorf("opening local MDM asset archive: %w", err)
	}
	defer f.Close()
	content, err := readBounded(contextReader{ctx: ctx, reader: f}, maxSourceArchiveBytes, "local MDM asset archive")
	if err != nil {
		return nil, err
	}
	return readTar(ctx, content)
}

func readDirectory(ctx context.Context, root string) (map[string][]byte, error) {
	rootHandle, err := os.OpenRoot(root)
	if err != nil {
		return nil, fmt.Errorf("opening local MDM asset source root: %w", err)
	}
	defer rootHandle.Close()

	files := map[string][]byte{}
	var total int64
	err = fs.WalkDir(rootHandle.FS(), ".", func(name string, entry fs.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if walkErr != nil {
			return walkErr
		}
		if name == "." {
			return nil
		}
		rel := filepath.ToSlash(name)
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("local MDM asset source contains symbolic link %q", rel)
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("local MDM asset source contains non-regular file %q", rel)
		}
		if err := validateImportPath(rel); err != nil {
			return err
		}
		if len(files) >= maxBundleFiles {
			return fmt.Errorf("MDM asset source exceeds maximum file count of %d", maxBundleFiles)
		}
		remaining := int64(maxExtractedBytes) - total
		if info.Size() < 0 || info.Size() > remaining {
			return fmt.Errorf("MDM asset source exceeds maximum extracted size of %d bytes", maxExtractedBytes)
		}
		file, err := rootHandle.Open(name)
		if err != nil {
			return err
		}
		openedInfo, err := file.Stat()
		if err != nil {
			_ = file.Close()
			return err
		}
		if !openedInfo.Mode().IsRegular() {
			_ = file.Close()
			return fmt.Errorf("local MDM asset source contains non-regular file %q", rel)
		}
		data, err := readBounded(contextReader{ctx: ctx, reader: file}, remaining, fmt.Sprintf("local MDM asset source file %q", rel))
		closeErr := file.Close()
		if err == nil {
			err = closeErr
		}
		if err != nil {
			return err
		}
		total += int64(len(data))
		files[rel] = data
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("reading local MDM asset source: %w", err)
	}
	return files, nil
}

func readTar(ctx context.Context, content []byte) (map[string][]byte, error) {
	return readTarWithDecodedLimit(ctx, content, maxDecodedTarBytes)
}

func readTarWithDecodedLimit(ctx context.Context, content []byte, maxDecodedBytes int64) (map[string][]byte, error) {
	var reader io.Reader = contextReader{ctx: ctx, reader: bytes.NewReader(content)}
	if len(content) >= 2 && content[0] == 0x1f && content[1] == 0x8b {
		gz, err := gzip.NewReader(reader)
		if err != nil {
			return nil, fmt.Errorf("opening compressed MDM asset archive: %w", err)
		}
		defer gz.Close()
		reader = contextReader{ctx: ctx, reader: gz}
	}
	reader = &decodedLimitReader{reader: reader, remaining: maxDecodedBytes, limit: maxDecodedBytes}

	tr := tar.NewReader(reader)
	files := map[string][]byte{}
	var total int64
	entries := 0
	for {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("reading MDM asset tar archive: %w", err)
		}
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("reading MDM asset tar archive: %w", err)
		}
		entries++
		if entries > maxTarEntries {
			return nil, fmt.Errorf("MDM asset archive exceeds maximum entry count of %d", maxTarEntries)
		}

		name := strings.TrimPrefix(header.Name, "./")
		if header.Typeflag == tar.TypeDir {
			continue
		}
		if header.Typeflag != tar.TypeReg {
			return nil, fmt.Errorf("MDM asset archive contains unsupported entry %q", name)
		}
		if err := validateImportPath(name); err != nil {
			return nil, err
		}
		if _, exists := files[name]; exists {
			return nil, fmt.Errorf("MDM asset archive contains duplicate file %q", name)
		}
		if len(files) >= maxBundleFiles {
			return nil, fmt.Errorf("MDM asset archive exceeds maximum file count of %d", maxBundleFiles)
		}
		remaining := int64(maxExtractedBytes) - total
		if header.Size < 0 || header.Size > remaining {
			return nil, fmt.Errorf("MDM asset archive exceeds maximum extracted size of %d bytes", maxExtractedBytes)
		}
		data, err := readBounded(tr, header.Size, fmt.Sprintf("MDM asset archive file %q", name))
		if err != nil {
			return nil, err
		}
		if int64(len(data)) != header.Size {
			return nil, fmt.Errorf("MDM asset archive file %q is truncated", name)
		}
		total += int64(len(data))
		files[name] = data
	}
	return files, nil
}

func validateImportPath(name string) error {
	if name == "" || strings.Contains(name, `\`) || path.IsAbs(name) || !fs.ValidPath(name) {
		return fmt.Errorf("MDM asset source contains unsafe path %q", name)
	}
	return nil
}

func normalizeArchiveRoot(files map[string][]byte) (map[string][]byte, error) {
	if _, ok := files["manifest.json"]; ok {
		return files, nil
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("MDM asset source is empty")
	}
	root := ""
	for name := range files {
		first, _, ok := strings.Cut(name, "/")
		if !ok || first == "" {
			return nil, fmt.Errorf("MDM asset source does not contain manifest.json at its root")
		}
		if root == "" {
			root = first
		} else if root != first {
			return nil, fmt.Errorf("MDM asset archive must contain one common top-level directory")
		}
	}
	prefix := root + "/"
	if _, ok := files[prefix+"manifest.json"]; !ok {
		return nil, fmt.Errorf("MDM asset source does not contain manifest.json")
	}
	normalized := make(map[string][]byte, len(files))
	for name, data := range files {
		normalized[strings.TrimPrefix(name, prefix)] = data
	}
	return normalized, nil
}

func canonicalArchive(files map[string][]byte) ([]byte, error) {
	names := make([]string, 0, len(files))
	for name := range files {
		if err := validateImportPath(name); err != nil {
			return nil, err
		}
		names = append(names, name)
	}
	sort.Strings(names)

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, name := range names {
		header := &zip.FileHeader{Name: name, Method: zip.Deflate}
		header.SetMode(0o644)
		w, err := zw.CreateHeader(header)
		if err != nil {
			_ = zw.Close()
			return nil, fmt.Errorf("creating canonical MDM asset archive: %w", err)
		}
		if _, err := w.Write(files[name]); err != nil {
			_ = zw.Close()
			return nil, fmt.Errorf("writing canonical MDM asset archive: %w", err)
		}
	}
	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("closing canonical MDM asset archive: %w", err)
	}
	return buf.Bytes(), nil
}

// OpenArchive validates and opens canonical database bundle content without
// extracting it to disk.
func OpenArchive(content []byte) (*Loader, error) {
	if len(content) == 0 {
		return nil, fmt.Errorf("MDM asset bundle is empty")
	}
	if len(content) > maxSourceArchiveBytes {
		return nil, fmt.Errorf("MDM asset bundle exceeds maximum size of %d bytes", maxSourceArchiveBytes)
	}
	zr, err := zip.NewReader(bytes.NewReader(content), int64(len(content)))
	if err != nil {
		return nil, fmt.Errorf("opening MDM asset bundle: %w", err)
	}
	seen := map[string]struct{}{}
	var total uint64
	for _, file := range zr.File {
		if err := validateImportPath(file.Name); err != nil {
			return nil, err
		}
		if file.FileInfo().IsDir() || !file.Mode().IsRegular() {
			return nil, fmt.Errorf("MDM asset bundle contains non-regular file %q", file.Name)
		}
		if _, ok := seen[file.Name]; ok {
			return nil, fmt.Errorf("MDM asset bundle contains duplicate file %q", file.Name)
		}
		seen[file.Name] = struct{}{}
		if file.UncompressedSize64 > uint64(maxExtractedBytes)-total {
			return nil, fmt.Errorf("MDM asset bundle exceeds maximum extracted size of %d bytes", maxExtractedBytes)
		}
		total += file.UncompressedSize64
	}
	if len(seen) > maxBundleFiles {
		return nil, fmt.Errorf("MDM asset bundle exceeds maximum file count of %d", maxBundleFiles)
	}
	return NewFS(zr)
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r contextReader) Read(buffer []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(buffer)
}

// decodedLimitReader allows exactly limit bytes. If a caller asks for more,
// it probes one byte so an archive whose decoded size exactly equals the limit
// can still end normally while an expanding stream gets a deterministic error.
type decodedLimitReader struct {
	reader    io.Reader
	remaining int64
	limit     int64
}

func (r *decodedLimitReader) Read(buffer []byte) (int, error) {
	if len(buffer) == 0 {
		return 0, nil
	}
	if r.remaining == 0 {
		var probe [1]byte
		n, err := r.reader.Read(probe[:])
		if n > 0 {
			return 0, fmt.Errorf("MDM asset archive exceeds maximum decoded size of %d bytes", r.limit)
		}
		if err == nil {
			return 0, io.ErrNoProgress
		}
		return 0, err
	}
	if int64(len(buffer)) > r.remaining {
		buffer = buffer[:int(r.remaining)]
	}
	n, err := r.reader.Read(buffer)
	r.remaining -= int64(n)
	return n, err
}

func readBounded(reader io.Reader, limit int64, label string) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", label, err)
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("%s exceeds maximum size of %d bytes", label, limit)
	}
	return data, nil
}
