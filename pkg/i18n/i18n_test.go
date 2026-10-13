package i18n

import (
	"maps"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
)

func TestCatalogsMatch(t *testing.T) {
	checkCatalogTranslations(t, catalogs)
}

func checkCatalogTranslations(t *testing.T, catalogs map[string]map[string]string) {
	t.Helper()
	base := catalogs["en"]
	for key, english := range base {
		if english == "" {
			t.Errorf("English message %q is empty", key)
		}
	}
	for locale, catalog := range catalogs {
		for key, translated := range catalog {
			english, ok := base[key]
			if !ok {
				t.Errorf("%s message %q has no English source", locale, key)
				continue
			}
			if translated != "" && !slices.Equal(placeholders(english), placeholders(translated)) {
				t.Errorf("%s message %q has different placeholders", locale, key)
			}
		}
	}
}

func TestDirectoryCatalogsMatch(t *testing.T) {
	parsed, err := catalogFiles(files)
	if err != nil {
		t.Fatalf("catalogFiles() error = %v", err)
	}
	grouped := map[string]map[string]map[string]string{}
	for _, file := range parsed {
		if grouped[file.dir] == nil {
			grouped[file.dir] = map[string]map[string]string{}
		}
		grouped[file.dir][file.locale] = file.keys
	}
	for _, dir := range slices.Sorted(maps.Keys(grouped)) {
		t.Run(dir, func(t *testing.T) {
			if _, ok := grouped[dir]["en"]; !ok {
				t.Fatal("missing en.json")
			}
			checkCatalogTranslations(t, grouped[dir])
		})
	}
}

func TestReadCatalogs(t *testing.T) {
	tests := []struct {
		name      string
		files     fstest.MapFS
		want      map[string]string
		wantError string
	}{
		{
			name: "merge root and area catalogs",
			files: fstest.MapFS{
				"messages/README.md": {
					Data: []byte("not a catalog"),
				},
				"messages/en.json": {
					Data: []byte(`{"$schema":"https://inlang.com/schema/inlang-message-format","status_not_found":"not found"}`),
				},
				"messages/scim/en.json": {
					Data: []byte(`{"scim_unsupported":"{provider} does not support SCIM provisioning."}`),
				},
			},
			want: map[string]string{
				"status_not_found": "not found",
				"scim_unsupported": "{provider} does not support SCIM provisioning.",
			},
		},
		{
			name: "reject duplicate keys",
			files: fstest.MapFS{
				"messages/en.json": {
					Data: []byte(`{"status_not_found":"not found"}`),
				},
				"messages/scim/en.json": {
					Data: []byte(`{"status_not_found":"missing"}`),
				},
			},
			wantError: `duplicate message "status_not_found"`,
		},
		{
			name: "reject malformed JSON",
			files: fstest.MapFS{
				"messages/fr.json": {
					Data: []byte(`{"greeting":`),
				},
			},
			wantError: "parse messages/fr.json",
		},
		{
			name: "reject invalid locale file",
			files: fstest.MapFS{
				"messages/not a language.json": {
					Data: []byte(`{}`),
				},
			},
			wantError: "invalid locale file messages/not a language.json",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := readCatalogs(tt.files)
			if tt.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantError) {
					t.Fatalf("readCatalogs() error = %v, want %q", err, tt.wantError)
				}
				return
			}
			if err != nil {
				t.Fatalf("readCatalogs() error = %v", err)
			}
			if _, ok := got["en"]["$schema"]; ok {
				t.Error(`catalog includes "$schema"`)
			}
			for key, value := range tt.want {
				if got["en"][key] != value {
					t.Errorf("readCatalogs()[en][%s] = %q, want %q", key, got["en"][key], value)
				}
			}
		})
	}
}

func TestMessageFallbackAndArguments(t *testing.T) {
	args := map[string]string{"provider": "Okta"}
	if got := Message("ko", "scim_unsupported", args); got != "Okta에서는 SCIM 프로비저닝을 지원하지 않습니다." {
		t.Fatalf("Korean SCIM blocker = %q", got)
	}
	if got := Message("fr", "scim_unsupported", args); got != "Okta does not support SCIM provisioning." {
		t.Fatalf("English fallback = %q", got)
	}
}

func placeholders(message string) []string {
	result := make([]string, 0)
	result = append(result, placeholder.FindAllString(message, -1)...)
	slices.Sort(result)
	return result
}

func TestDiscoveredPartialCatalogs(t *testing.T) {
	loaded, err := readCatalogs(fstest.MapFS{
		"messages/en.json":         {Data: []byte(`{"greeting":"Hello {name}","missing":"English","empty":"English empty"}`)},
		"messages/fr.json":         {Data: []byte(`{"greeting":"Bonjour {name}","empty":""}`)},
		"messages/area/fr.json":    {Data: []byte(`{"area":"Français"}`)},
		"messages/area/en.json":    {Data: []byte(`{"area":"English area"}`)},
		"messages/area/pt-br.json": {Data: []byte(`{"area":"Português"}`)},
	})
	if err != nil {
		t.Fatal(err)
	}
	checkCatalogTranslations(t, loaded)
	originalCatalogs, originalSupported := catalogs, supported
	catalogs, supported = loaded, catalogLocales(loaded)
	t.Cleanup(func() { catalogs, supported = originalCatalogs, originalSupported })
	if !slices.Equal(supported, []string{"en", "fr", "pt-BR"}) {
		t.Fatalf("discovered locales = %v", supported)
	}
	if got := Locale("fr-CA,en;q=0.5"); got != "fr" {
		t.Fatalf("selected locale = %q", got)
	}
	if got := Message("fr", "greeting", map[string]string{"name": "Sam"}); got != "Bonjour Sam" {
		t.Fatalf("translated message = %q", got)
	}
	for key, want := range map[string]string{"missing": "English", "empty": "English empty"} {
		if got := Text("fr", key); got != want {
			t.Errorf("Text(fr, %s) = %q, want %q", key, got, want)
		}
	}
	if got := Locales("missing"); !maps.Equal(got, map[string]string{"fr": "English", "pt-BR": "English"}) {
		t.Fatalf("Locales(missing) = %v", got)
	}
}

func TestLocaleNegotiation(t *testing.T) {
	tests := []struct {
		name      string
		header    string
		available []string
		want      string
	}{
		{name: "weighted", header: "fr;q=0.5,ja;q=0.9", available: []string{"en", "fr", "ja"}, want: "ja"},
		{name: "zero weight", header: "fr;q=0,en;q=0.5", available: []string{"en", "fr"}, want: "en"},
		{name: "invalid header", header: "not a language", available: []string{"en", "fr"}, want: "en"},
		{name: "unavailable", header: "de", available: []string{"en", "fr"}, want: "en"},
		{name: "parent", header: "fr-CA", available: []string{"en", "fr"}, want: "fr"},
		{name: "exact region", header: "pt-BR", available: []string{"en", "pt", "pt-BR"}, want: "pt-BR"},
		{name: "canonical spelling", header: "pt-BR", available: []string{"en", "pt-br"}, want: "pt-BR"},
		{name: "deterministic region", header: "fr", available: []string{"fr-FR", "en", "fr-CA"}, want: "fr-CA"},
		{name: "Simplified script", header: "zh-Hans-CN", available: []string{"en", "zh-CN"}, want: "zh-CN"},
		{name: "Singapore", header: "zh-SG", available: []string{"en", "zh-CN"}, want: "zh-CN"},
		{name: "Traditional skips Simplified", header: "zh-TW,ja;q=0.5", available: []string{"en", "ja", "zh-CN"}, want: "ja"},
		{name: "different script", header: "sr-Latn", available: []string{"en", "sr"}, want: "en"},
		{name: "Hong Kong", header: "zh-HK", available: []string{"en", "zh-CN"}, want: "en"},
		{name: "Traditional available", header: "zh-HK", available: []string{"en", "zh-CN", "zh-TW"}, want: "zh-TW"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := selectLocale(tt.header, tt.available); got != tt.want {
				t.Fatalf("selectLocale(%q, %v) = %q, want %q", tt.header, tt.available, got, tt.want)
			}
		})
	}
}

func TestCanonicalCatalogDuplicate(t *testing.T) {
	_, err := readCatalogs(fstest.MapFS{
		"messages/pt-br.json":      {Data: []byte(`{"key":"one"}`)},
		"messages/area/pt-BR.json": {Data: []byte(`{"key":"two"}`)},
	})
	if err == nil || !strings.Contains(err.Error(), `duplicate message "key" for pt-BR`) {
		t.Fatalf("canonical duplicate error = %v", err)
	}
}

func TestResolve(t *testing.T) {
	original := catalogs
	catalogs = map[string]map[string]string{
		"en": {"name": "Name", "empty": "Fallback", "nested": "${name}", "args": "Hello {user}"},
		"ja": {"name": "名前", "empty": ""},
	}
	t.Cleanup(func() { catalogs = original })
	tests := []struct {
		name   string
		locale string
		input  string
		want   string
	}{
		{name: "complete field", locale: "ja", input: "${name}", want: "名前"},
		{name: "mixed and repeated", locale: "ja", input: "<${name}> ${empty} ${name}", want: "<名前> Fallback 名前"},
		{name: "missing translation", locale: "fr", input: "${name}", want: "Name"},
		{name: "unknown key", locale: "ja", input: "${unknown}", want: "${unknown}"},
		{name: "literal", locale: "ja", input: "plain {name} {{name}}", want: "plain {name} {{name}}"},
		{name: "empty", locale: "ja", input: "", want: ""},
		{name: "single pass", locale: "ja", input: "${nested}", want: "${name}"},
		{name: "arguments separate", locale: "en", input: "${args}", want: "Hello {user}"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Resolve(tt.locale, tt.input); got != tt.want {
				t.Fatalf("Resolve() = %q, want %q", got, tt.want)
			}
		})
	}
}
