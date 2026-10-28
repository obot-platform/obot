package analyzer

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/text/language"
)

// readCatalogs mirrors the runtime's recursive catalog merging and locale
// normalization, reading source files so catalog edits need no server rebuild.
func readCatalogs(root string) (map[string]map[string]string, error) {
	catalogs := map[string]map[string]string{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || filepath.Ext(path) != ".json" {
			return nil
		}
		tag, err := language.Parse(strings.TrimSuffix(entry.Name(), ".json"))
		if err != nil || tag == language.Und {
			return fmt.Errorf("invalid locale file %s", path)
		}
		locale := tag.String()
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var messages map[string]string
		if err := json.Unmarshal(data, &messages); err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}
		if catalogs[locale] == nil {
			catalogs[locale] = map[string]string{}
		}
		for key, value := range messages {
			if key == "$schema" {
				continue
			}
			if _, exists := catalogs[locale][key]; exists {
				return fmt.Errorf("duplicate message %q for %s in %s", key, locale, path)
			}
			catalogs[locale][key] = value
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(catalogs["en"]) == 0 {
		return nil, fmt.Errorf("missing English backend messages")
	}
	return catalogs, nil
}
