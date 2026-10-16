package modeldata

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

func readModels(t *testing.T, path string) map[string]json.RawMessage {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var models map[string]json.RawMessage
	if err := json.Unmarshal(data, &models); err != nil {
		t.Fatal(err)
	}
	return models
}

// sheet returns a model data file with the required models plus n others.
func sheet(n int) string {
	models := map[string]any{}
	for _, name := range requiredModels {
		models[name] = map[string]any{"provider": "test"}
	}
	for i := range n {
		models[fmt.Sprintf("model-%d", i)] = map[string]any{"provider": "test"}
	}
	data, _ := json.Marshal(models)
	return string(data)
}

// server serves model data files, by name, from files.
func server(t *testing.T, files map[string]string) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, ok := files[strings.TrimPrefix(r.URL.Path, "/v1/model-data/")]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv.URL + "/v1/model-data"
}

// withBundled makes the data Bundle wrote into dir the build's bundled data, or makes the
// build have none when dir is empty.
func withBundled(t *testing.T, dir string) {
	t.Helper()
	prior := bundled
	if dir == "" {
		bundled = fstest.MapFS{}
	} else {
		bundled = os.DirFS(dir)
	}
	t.Cleanup(func() { bundled = prior })
}

func TestBundleAndPrepare(t *testing.T) {
	url := server(t, map[string]string{pricingFile: sheet(20), paramsFile: sheet(20)})
	bundleDir := t.TempDir()
	if err := Bundle(t.Context(), url, bundleDir); err != nil {
		t.Fatal(err)
	}
	withBundled(t, bundleDir)

	// A data dir without model data gets the bundled data.
	dataDir := t.TempDir()
	if err := Prepare(t.Context(), dataDir, ""); err != nil {
		t.Fatal(err)
	}
	pricingPath, paramsPath := Paths(dataDir)
	for _, path := range []string{pricingPath, paramsPath} {
		if n := len(readModels(t, path)); n != 20+len(requiredModels) {
			t.Errorf("%s has %d models, want the bundled %d", path, n, 20+len(requiredModels))
		}
	}

	// Files newer than the bundled data, as a refresh downloads them, are kept.
	if err := write(pricingPath, []byte(sheet(1)), time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := Prepare(t.Context(), dataDir, ""); err != nil {
		t.Fatal(err)
	}
	if n := len(readModels(t, pricingPath)); n != 1+len(requiredModels) {
		t.Errorf("pricing has %d models: newer downloaded data was replaced", n)
	}

	// Files older than the bundled data, from an earlier Obot release, are replaced.
	if err := os.Chtimes(pricingPath, time.Unix(1, 0), time.Unix(1, 0)); err != nil {
		t.Fatal(err)
	}
	if err := Prepare(t.Context(), dataDir, ""); err != nil {
		t.Fatal(err)
	}
	if n := len(readModels(t, pricingPath)); n != 20+len(requiredModels) {
		t.Errorf("pricing has %d models: older data was kept", n)
	}
}

// A build without bundled data, such as a build from source, downloads the model data when
// it starts, and starts without model prices when it can't.
func TestPrepareWithoutBundledData(t *testing.T) {
	withBundled(t, "")

	dataDir := t.TempDir()
	if err := Prepare(t.Context(), dataDir, server(t, map[string]string{pricingFile: sheet(5), paramsFile: sheet(5)})); err != nil {
		t.Fatal(err)
	}
	pricingPath, _ := Paths(dataDir)
	if n := len(readModels(t, pricingPath)); n != 5+len(requiredModels) {
		t.Errorf("pricing has %d models, want the downloaded %d", n, 5+len(requiredModels))
	}

	empty := t.TempDir()
	if err := Prepare(t.Context(), empty, server(t, nil)); err != nil {
		t.Fatal(err)
	}
	pricingPath, paramsPath := Paths(empty)
	for _, path := range []string{pricingPath, paramsPath} {
		if n := len(readModels(t, path)); n != 0 {
			t.Errorf("%s has %d models, want none when the download fails", path, n)
		}
	}

	// A refresh that succeeds later replaces the empty files.
	if err := Download(t.Context(), http.DefaultClient, empty, server(t, map[string]string{pricingFile: sheet(3), paramsFile: sheet(3)})); err != nil {
		t.Fatal(err)
	}
	if n := len(readModels(t, pricingPath)); n != 3+len(requiredModels) {
		t.Errorf("pricing has %d models after a refresh, want %d", n, 3+len(requiredModels))
	}
}

func TestBundleRefusesIncompleteData(t *testing.T) {
	dir := t.TempDir()
	err := Bundle(t.Context(), server(t, map[string]string{pricingFile: `{"model-0":{}}`, paramsFile: sheet(1)}), dir)
	if err == nil || !strings.Contains(err.Error(), "missing gpt-4o") {
		t.Fatalf("err = %v, want one about the missing model", err)
	}
	if _, err := fs.Stat(os.DirFS(dir), generatedFile); err == nil {
		t.Error("an incomplete download was bundled")
	}
}

func TestDownload(t *testing.T) {
	files := map[string]string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, ok := files[strings.TrimPrefix(r.URL.Path, "/v1/model-data/")]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()
	baseURL := srv.URL + "/v1/model-data/"

	dir := t.TempDir()
	pricingPath, paramsPath := Paths(dir)
	const current = 40
	for _, path := range []string{pricingPath, paramsPath} {
		if err := write(path, []byte(sheet(current)), time.Unix(1, 0)); err != nil {
			t.Fatal(err)
		}
	}
	have := current + len(requiredModels)

	tests := []struct {
		name    string
		pricing string
		params  string
		wantErr string
	}{
		{
			name:    "not served",
			wantErr: "404",
		},
		{
			name:    "not JSON",
			pricing: "<html>",
			params:  sheet(current),
			wantErr: "not a JSON object",
		},
		{
			name:    "missing a well-known model",
			pricing: `{"model-0":{}}`,
			params:  sheet(current),
			wantErr: "missing gpt-4o",
		},
		{
			name:    "lost most of its models",
			pricing: sheet(current / 2),
			params:  sheet(current),
			wantErr: "down from",
		},
		{
			name:    "a broken parameters file holds back the pricing file too",
			pricing: sheet(current),
			params:  "[]",
			wantErr: "model-parameters.json",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files = map[string]string{}
			if tt.pricing != "" {
				files[pricingFile] = tt.pricing
			}
			if tt.params != "" {
				files[paramsFile] = tt.params
			}
			err := Download(t.Context(), srv.Client(), dir, baseURL)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("err = %v, want one containing %q", err, tt.wantErr)
			}
			if n := len(readModels(t, pricingPath)); n != have {
				t.Errorf("pricing has %d models after a failed refresh, want the %d it had", n, have)
			}
		})
	}

	files = map[string]string{pricingFile: sheet(current + 5), paramsFile: sheet(current + 5)}
	if err := Download(t.Context(), srv.Client(), dir, baseURL); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{pricingPath, paramsPath} {
		if n := len(readModels(t, path)); n != have+5 {
			t.Errorf("%s has %d models after a refresh, want %d", path, n, have+5)
		}
	}
}

func TestRefreshURL(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    string
		wantErr bool
	}{
		{
			name: "default",
			raw:  "https://upgrade-server.obot.ai/v1/model-data",
			want: "https://upgrade-server.obot.ai/v1/model-data",
		},
		{
			name: "empty turns refreshing off",
			raw:  "",
		},
		{
			name: "off",
			raw:  " Off ",
		},
		{
			name:    "a file path",
			raw:     "/var/lib/model-data",
			wantErr: true,
		},
		{
			name:    "another scheme",
			raw:     "ftp://example.com/model-data",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := RefreshURL(tt.raw)
			if (err != nil) != tt.wantErr || got != tt.want {
				t.Errorf("RefreshURL(%q) = %q, %v; want %q, error %v", tt.raw, got, err, tt.want, tt.wantErr)
			}
		})
	}
}
