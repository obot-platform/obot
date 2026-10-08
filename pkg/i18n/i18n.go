// Package i18n renders API-owned messages from the backend translation catalog.
package i18n

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"strings"

	"golang.org/x/text/language"
)

// files embeds every locale catalog under messages, including area subdirectories.
//
//go:embed messages
var files embed.FS

var (
	placeholder = regexp.MustCompile(`\{([a-zA-Z][a-zA-Z0-9]*)\}`)
	catalogs    = loadCatalogs()
)

var supported = []string{"en", "ja", "ko", "zh-CN"}

type localeKey struct{}

// WithLocale carries a request's selected locale into API services.
func WithLocale(ctx context.Context, locale string) context.Context {
	return context.WithValue(ctx, localeKey{}, locale)
}

// FromContext returns the request locale, or English for background work.
func FromContext(ctx context.Context) string {
	if locale, ok := ctx.Value(localeKey{}).(string); ok && locale != "" {
		return locale
	}
	return "en"
}

// Locale selects the first supported language with a positive Accept-Language weight.
func Locale(header string) string {
	tags, weights, err := language.ParseAcceptLanguage(header)
	if err != nil {
		return "en"
	}
	for i, tag := range tags {
		if weights[i] <= 0 {
			continue
		}
		value := strings.ToLower(tag.String())
		switch {
		case value == "en" || strings.HasPrefix(value, "en-"):
			return "en"
		case value == "ja" || strings.HasPrefix(value, "ja-"):
			return "ja"
		case value == "ko" || strings.HasPrefix(value, "ko-"):
			return "ko"
		case value == "zh-cn" || value == "zh-sg" || value == "zh-hans" || strings.HasPrefix(value, "zh-hans-"):
			return "zh-CN"
		}
	}
	return "en"
}

// Message renders a catalog key. A missing translation falls back to English.
func Message(locale, key string, args map[string]string) string {
	value := catalogs[locale][key]
	if value == "" {
		value = catalogs["en"][key]
	}
	if value == "" {
		panic("missing backend message: " + key)
	}
	return placeholder.ReplaceAllStringFunc(value, func(match string) string {
		name := match[1 : len(match)-1]
		if replacement, ok := args[name]; ok {
			return replacement
		}
		panic(fmt.Sprintf("missing argument %q for backend message %q", name, key))
	})
}

// Text returns a catalog message without arguments.
func Text(locale, key string) string { return Message(locale, key, nil) }

// Locales returns a rendered value for each non-English locale.
func Locales(key string) map[string]string {
	result := make(map[string]string, len(supported)-1)
	for _, locale := range supported[1:] {
		result[locale] = Text(locale, key)
	}
	return result
}

func loadCatalogs() map[string]map[string]string {
	catalogs, err := readCatalogs(files)
	if err != nil {
		panic(err)
	}
	return catalogs
}

// readCatalogs merges every messages/**/{locale}.json file into one catalog per locale.
// The root file holds shared messages. Area subdirectories hold the rest.
// A message key can appear only once for a locale.
func readCatalogs(fsys fs.FS) (map[string]map[string]string, error) {
	parsed, err := catalogFiles(fsys)
	if err != nil {
		return nil, err
	}
	catalogs := make(map[string]map[string]string, len(supported))
	defined := make(map[string]map[string]string, len(supported))
	for _, locale := range supported {
		catalogs[locale] = map[string]string{}
		defined[locale] = map[string]string{}
	}
	for _, file := range parsed {
		for key, value := range file.keys {
			if previous, exists := defined[file.locale][key]; exists {
				return nil, fmt.Errorf("duplicate message %q for %s in %s and %s", key, file.locale, previous, file.path)
			}
			defined[file.locale][key] = file.path
			catalogs[file.locale][key] = value
		}
	}
	return catalogs, nil
}

type catalogFile struct {
	path   string
	dir    string
	locale string
	keys   map[string]string
}

func catalogFiles(fsys fs.FS) ([]catalogFile, error) {
	parsed := []catalogFile{}
	err := fs.WalkDir(fsys, "messages", func(filePath string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || path.Ext(filePath) != ".json" {
			return nil
		}
		locale := strings.TrimSuffix(path.Base(filePath), ".json")
		if !supportedLocale(locale) {
			return fmt.Errorf("unsupported locale file %s", filePath)
		}
		data, err := fs.ReadFile(fsys, filePath)
		if err != nil {
			return err
		}
		var messages map[string]string
		if err := json.Unmarshal(data, &messages); err != nil {
			return fmt.Errorf("parse %s: %w", filePath, err)
		}
		delete(messages, "$schema")
		if messages == nil {
			messages = map[string]string{}
		}
		parsed = append(parsed, catalogFile{
			path:   filePath,
			dir:    path.Dir(filePath),
			locale: locale,
			keys:   messages,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return parsed, nil
}

func supportedLocale(locale string) bool {
	for _, candidate := range supported {
		if locale == candidate {
			return true
		}
	}
	return false
}
