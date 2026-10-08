package i18n

import (
	"maps"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
)

func TestCatalogsMatch(t *testing.T) {
	base := catalogs["en"]
	for _, locale := range supported[1:] {
		catalog := catalogs[locale]
		if !slices.Equal(slices.Sorted(maps.Keys(base)), slices.Sorted(maps.Keys(catalog))) {
			t.Errorf("%s keys differ from English", locale)
		}
		for key, english := range base {
			if key == "$schema" {
				continue
			}
			translated := catalog[key]
			if translated == "" {
				t.Errorf("%s message %q is empty", locale, key)
			}
			if !slices.Equal(placeholders(english), placeholders(translated)) {
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
	grouped := map[string]map[string][]string{}
	for _, file := range parsed {
		if grouped[file.dir] == nil {
			grouped[file.dir] = map[string][]string{}
		}
		grouped[file.dir][file.locale] = slices.Sorted(maps.Keys(file.keys))
	}
	for _, dir := range slices.Sorted(maps.Keys(grouped)) {
		locales := grouped[dir]
		base, ok := locales["en"]
		if !ok {
			t.Errorf("%s is missing en.json", dir)
			continue
		}
		for _, locale := range supported[1:] {
			keys, ok := locales[locale]
			if !ok {
				t.Errorf("%s is missing %s.json", dir, locale)
				continue
			}
			if !slices.Equal(base, keys) {
				t.Errorf("%s keys in %s differ from English", locale, dir)
			}
		}
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
			name: "reject unsupported locale file",
			files: fstest.MapFS{
				"messages/fr.json": {
					Data: []byte(`{}`),
				},
			},
			wantError: "unsupported locale file messages/fr.json",
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
	for _, match := range placeholder.FindAllString(message, -1) {
		result = append(result, match)
	}
	slices.Sort(result)
	return result
}
