// Package modeldata keeps the files the LLM gateway engine prices requests and looks up
// model capabilities from: a pricing datasheet and a model-parameters file.
//
// By default the engine downloads both from its vendor's website. Obot instead keeps them in
// the gateway's data dir and points the engine at them with file:// URLs, so the engine never
// fetches them itself and always has something to boot with: with an empty store and nothing
// to load, the engine exits the process.
//
// The files are converted from LiteLLM's model_prices_and_context_window.json (MIT; see
// data/LICENSE.litellm). Obot's images and release binaries embed a snapshot downloaded at
// build time (see Bundle), and Obot refreshes the files from a URL that serves the same
// conversion, Obot's upgrade server by default. The engine re-reads them on its own schedule.
package modeldata

import (
	"bytes"
	"compress/gzip"
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	pricingFile   = "pricing.json"
	paramsFile    = "model-parameters.json"
	generatedFile = "generated"
	dirName       = "model-data"

	// RefreshInterval is how often Obot downloads the files from the refresh URL.
	RefreshInterval = 6 * time.Hour
	// startupDownloadTimeout bounds the download a build without bundled data makes before
	// the gateway starts.
	startupDownloadTimeout = 15 * time.Second
	// maxSize bounds a downloaded file.
	maxSize = 64 << 20
	// minShare is how much of the current file a refresh must keep. A refresh that drops more
	// models than that is assumed to be broken, and isn't applied.
	minShare = 0.9
)

var (
	//go:embed all:data
	embedded embed.FS
	// bundled is the snapshot embedded at build time, if any; tests replace it.
	bundled = mustSub(embedded, "data")

	// requiredModels must be in every download, as a check that it is a full sheet.
	requiredModels = []string{"gpt-4o", "claude-sonnet-4-5"}
)

func mustSub(f fs.FS, dir string) fs.FS {
	sub, err := fs.Sub(f, dir)
	if err != nil {
		panic(err)
	}
	return sub
}

// Paths returns the files the engine reads model pricing and capabilities from.
func Paths(dataDir string) (pricing, params string) {
	dir := filepath.Join(dataDir, dirName)
	return filepath.Join(dir, pricingFile), filepath.Join(dir, paramsFile)
}

// FileURL returns the file:// URL of an absolute path.
func FileURL(path string) string {
	return (&url.URL{Scheme: "file", Path: filepath.ToSlash(path)}).String()
}

// Prepare makes sure the gateway's data dir has model pricing and capability files for the
// engine to read.
//   - With a bundled snapshot, the snapshot is written unless the files there are newer, as
//     they are once a refresh has downloaded them.
//   - Without one, as in a build from source, files already there are kept. Otherwise Prepare
//     tries to download them from refreshURL, and failing that writes empty files: the gateway
//     starts with no model prices until a refresh succeeds.
func Prepare(ctx context.Context, dataDir, refreshURL string) error {
	pricingPath, paramsPath := Paths(dataDir)
	generated, ok, err := bundledTime()
	if err != nil {
		return err
	}
	if ok {
		for path, name := range map[string]string{pricingPath: pricingFile, paramsPath: paramsFile} {
			if info, err := os.Stat(path); err == nil && !info.ModTime().Before(generated) {
				continue
			}
			data, err := readBundled(name)
			if err != nil {
				return err
			}
			if err := write(path, data, generated); err != nil {
				return err
			}
		}
		return nil
	}

	if exists(pricingPath) && exists(paramsPath) {
		return nil
	}
	if refreshURL != "" {
		ctx, cancel := context.WithTimeout(ctx, startupDownloadTimeout)
		err := Download(ctx, &http.Client{}, dataDir, refreshURL)
		cancel()
		if err == nil {
			return nil
		}
		slog.Warn("this build has no bundled model prices and downloading them failed; the LLM gateway starts without model prices until a refresh succeeds", "url", refreshURL, "error", err)
	} else {
		slog.Warn("this build has no bundled model prices and refreshing them is off; the LLM gateway has no model prices")
	}
	// Dated at the epoch, so bundled data or any download replaces them.
	for _, path := range []string{pricingPath, paramsPath} {
		if !exists(path) {
			if err := write(path, []byte("{}"), time.Unix(0, 0)); err != nil {
				return err
			}
		}
	}
	return nil
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// bundledTime is when the bundled snapshot was made, and whether there is one.
func bundledTime() (time.Time, bool, error) {
	data, err := fs.ReadFile(bundled, generatedFile)
	if errors.Is(err, fs.ErrNotExist) {
		return time.Time{}, false, nil
	} else if err != nil {
		return time.Time{}, false, err
	}
	t, err := time.Parse(time.RFC3339, strings.TrimSpace(string(data)))
	if err != nil {
		return time.Time{}, false, fmt.Errorf("invalid date of the bundled model data: %w", err)
	}
	return t, true, nil
}

func readBundled(name string) ([]byte, error) {
	f, err := bundled.Open(name + ".gz")
	if err != nil {
		return nil, fmt.Errorf("failed to read the bundled %s: %w", name, err)
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		return nil, fmt.Errorf("failed to read the bundled %s: %w", name, err)
	}
	defer zr.Close()
	return io.ReadAll(zr)
}

// write replaces path with data, which was current as of modTime.
func write(path string, data []byte, modTime time.Time) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("failed to create the model data dir: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*")
	if err != nil {
		return fmt.Errorf("failed to write %s: %w", path, err)
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("failed to write %s: %w", path, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("failed to write %s: %w", path, err)
	}
	if err := os.Chtimes(tmp.Name(), modTime, modTime); err != nil {
		return fmt.Errorf("failed to write %s: %w", path, err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("failed to write %s: %w", path, err)
	}
	return nil
}

// Refresh downloads the model pricing and capability files from baseURL every interval
// until ctx ends. A download that fails or doesn't pass validation leaves the current files
// in place.
func Refresh(ctx context.Context, dataDir, baseURL string, interval time.Duration) {
	client := &http.Client{Timeout: 2 * time.Minute}
	for {
		if err := Download(ctx, client, dataDir, baseURL); err != nil && ctx.Err() == nil {
			slog.Warn("failed to refresh the LLM gateway's model prices; keeping the current ones", "url", baseURL, "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(interval):
		}
	}
}

// Download replaces the gateway's model data files with the ones baseURL serves, if both
// pass validation.
func Download(ctx context.Context, client *http.Client, dataDir, baseURL string) error {
	pricingPath, paramsPath := Paths(dataDir)
	downloaded := map[string][]byte{}
	for path, name := range map[string]string{pricingPath: pricingFile, paramsPath: paramsFile} {
		data, err := fetch(ctx, client, strings.TrimSuffix(baseURL, "/")+"/"+name)
		if err != nil {
			return err
		}
		if err := validate(data, path); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		downloaded[path] = data
	}
	now := time.Now()
	for path, data := range downloaded {
		if err := write(path, data, now); err != nil {
			return err
		}
	}
	return nil
}

// Bundle downloads the model data baseURL serves into dir, compressed, for a build to embed.
func Bundle(ctx context.Context, baseURL, dir string) error {
	client := &http.Client{Timeout: 2 * time.Minute}
	files := map[string][]byte{}
	for _, name := range []string{pricingFile, paramsFile} {
		data, err := fetch(ctx, client, strings.TrimSuffix(baseURL, "/")+"/"+name)
		if err != nil {
			return err
		}
		if err := validate(data, ""); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		var buf bytes.Buffer
		zw, _ := gzip.NewWriterLevel(&buf, gzip.BestCompression)
		if _, err := zw.Write(data); err != nil {
			return err
		}
		if err := zw.Close(); err != nil {
			return err
		}
		files[name+".gz"] = buf.Bytes()
	}
	files[generatedFile] = []byte(time.Now().UTC().Format(time.RFC3339) + "\n")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func fetch(ctx context.Context, client *http.Client, rawURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", rawURL, resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxSize+1))
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", rawURL, err)
	}
	if len(data) > maxSize {
		return nil, fmt.Errorf("GET %s: larger than %d bytes", rawURL, maxSize)
	}
	return data, nil
}

// validate checks that a downloaded file is a full sheet: a JSON object of models that has
// the well-known ones, and, when it would replace currentPath, not many fewer models.
func validate(data []byte, currentPath string) error {
	var models map[string]json.RawMessage
	if err := json.Unmarshal(data, &models); err != nil {
		return fmt.Errorf("not a JSON object of models: %w", err)
	}
	for _, name := range requiredModels {
		if _, ok := models[name]; !ok {
			return fmt.Errorf("missing %s", name)
		}
	}
	if currentPath == "" {
		return nil
	}
	current, err := os.ReadFile(currentPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	var currentModels map[string]json.RawMessage
	if err := json.Unmarshal(current, &currentModels); err != nil {
		// The current file is broken; anything valid is better.
		return nil
	}
	if float64(len(models)) < minShare*float64(len(currentModels)) {
		return fmt.Errorf("has %d models, down from %d", len(models), len(currentModels))
	}
	return nil
}

// RefreshURL validates the configured model data URL. Empty or "off" turns refreshing off.
func RefreshURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.EqualFold(raw, "off") {
		return "", nil
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return "", fmt.Errorf("invalid LLM gateway model data URL %q: it must be an http(s) URL", raw)
	}
	return raw, nil
}
