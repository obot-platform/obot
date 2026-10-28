package analyzer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestAnalyzer(t *testing.T) {
	data := analysistest.TestData()
	a := newAnalyzer()
	if err := a.Flags.Set("root", data); err != nil {
		t.Fatal(err)
	}
	analysistest.Run(t, data, a, "example", i18nPath)
}

// Packages such as generated test mains can import i18n without any message
// keys. They must not require a source module or catalogs.
func TestNoKeys(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), newAnalyzer(), "nokeys")
}

func TestReadCatalogs(t *testing.T) {
	tests := []struct {
		name      string
		files     map[string]string
		wantError string
	}{
		{
			name: "merge and normalize locales",
			files: map[string]string{
				"en.json":         `{"$schema":"ignored","known":"Known"}`,
				"area/en.json":    `{"area":"Area"}`,
				"area/pt-br.json": `{"area":"Área"}`,
			},
		},
		{
			name: "duplicate normalized keys",
			files: map[string]string{
				"en.json":         `{"known":"Known"}`,
				"pt-br.json":      `{"known":"Um"}`,
				"area/pt-BR.json": `{"known":"Dois"}`,
			},
			wantError: `duplicate message "known" for pt-BR`,
		},
		{
			name:      "invalid JSON",
			files:     map[string]string{"en.json": `{`},
			wantError: "parse",
		},
		{
			name:      "invalid locale",
			files:     map[string]string{"not a locale.json": `{}`},
			wantError: "invalid locale",
		},
		{
			name:      "missing English",
			files:     map[string]string{"ja.json": `{}`},
			wantError: "missing English",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			for name, contents := range tt.files {
				path := filepath.Join(root, name)
				if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
					t.Fatal(err)
				}
			}
			catalogs, err := readCatalogs(root)
			if tt.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantError) {
					t.Fatalf("readCatalogs() error = %v, want %q", err, tt.wantError)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if catalogs["en"]["area"] != "Area" || catalogs["pt-BR"]["area"] != "Área" {
				t.Fatalf("catalogs = %v", catalogs)
			}
			if _, ok := catalogs["en"]["$schema"]; ok {
				t.Fatal("schema included as message")
			}
		})
	}
}
