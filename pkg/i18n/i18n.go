// Package i18n renders API-owned messages from the backend translation catalog.
package i18n

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"maps"
	"path"
	"regexp"
	"slices"
	"strings"

	"golang.org/x/text/language"
)

var (
	// files embeds every locale catalog under messages, including area subdirectories.
	//
	//go:embed messages
	files embed.FS

	reference   = regexp.MustCompile(`\$\{([a-zA-Z][a-zA-Z0-9_]*)\}`)
	placeholder = regexp.MustCompile(`\{([a-zA-Z][a-zA-Z0-9]*)\}`)
	catalogs    = loadCatalogs()
	supported   = catalogLocales(catalogs)
)

type localeKey struct{}

type catalogFile struct {
	path   string
	dir    string
	locale string
	keys   map[string]string
}

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

// Locale selects a catalog or additional locale with a positive Accept-Language weight.
func Locale(header string, additional ...string) string {
	return selectLocale(header, append(slices.Clone(supported), additional...))
}

func selectLocale(header string, available []string) string {
	tags, weights, err := language.ParseAcceptLanguage(header)
	if err != nil {
		return "en"
	}
	candidates := map[string]language.Tag{}
	for _, locale := range available {
		if tag, err := language.Parse(locale); err == nil {
			candidates[tag.String()] = tag
		}
	}
	names := slices.Sorted(maps.Keys(candidates))
	for i, tag := range tags {
		if weights[i] <= 0 {
			continue
		}
		for parent := tag; parent != language.Und; parent = parent.Parent() {
			if _, ok := candidates[parent.String()]; ok {
				return parent.String()
			}
		}
		base, _ := tag.Base()
		script, _ := tag.Script()
		for _, name := range names {
			candidateBase, _ := candidates[name].Base()
			candidateScript, _ := candidates[name].Script()
			if base == candidateBase && script == candidateScript {
				return name
			}
		}
	}
	return "en"
}

func catalogLocales(catalogs map[string]map[string]string) []string {
	locales := []string{"en"}
	for _, locale := range slices.Sorted(maps.Keys(catalogs)) {
		if locale != "en" {
			locales = append(locales, locale)
		}
	}
	return locales
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

// Resolve substitutes catalog references in provider display text once. Missing or empty
// translations fall back to English; unknown references remain unchanged.
func Resolve(locale, value string) string {
	return reference.ReplaceAllStringFunc(value, func(match string) string {
		key := match[2 : len(match)-1]
		if translated := catalogs[locale][key]; translated != "" {
			return translated
		}
		if english := catalogs["en"][key]; english != "" {
			return english
		}
		return match
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
	if len(catalogs["en"]) == 0 {
		panic("missing English backend messages")
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
	catalogs := make(map[string]map[string]string)
	defined := make(map[string]map[string]string)
	for _, file := range parsed {
		if catalogs[file.locale] == nil {
			catalogs[file.locale] = map[string]string{}
			defined[file.locale] = map[string]string{}
		}
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
		tag, err := language.Parse(locale)
		if err != nil || tag == language.Und {
			return fmt.Errorf("invalid locale file %s", filePath)
		}
		locale = tag.String()
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
