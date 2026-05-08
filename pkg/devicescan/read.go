package devicescan

import (
	"encoding/json"
	"io/fs"

	"github.com/BurntSushi/toml"
	"gopkg.in/yaml.v3"
)

// readJSON reads and decodes a JSON file at rel into T. Returns the zero
// T and false on any error. Use a typed struct for T to get compile-time
// schema validation; use map[string]any when the schema is genuinely
// open-ended.
func readJSON[T any](fsys fs.FS, rel string) (T, bool) {
	var out T
	data, err := fs.ReadFile(fsys, rel)
	if err != nil {
		return out, false
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return out, false
	}
	return out, true
}

// readYAML reads and decodes a YAML file at rel into T. See readJSON.
func readYAML[T any](fsys fs.FS, rel string) (T, bool) {
	var out T
	data, err := fs.ReadFile(fsys, rel)
	if err != nil {
		return out, false
	}
	if err := yaml.Unmarshal(data, &out); err != nil {
		return out, false
	}
	return out, true
}

// readTOML reads and decodes a TOML file at rel into T. See readJSON.
func readTOML[T any](fsys fs.FS, rel string) (T, bool) {
	var out T
	data, err := fs.ReadFile(fsys, rel)
	if err != nil {
		return out, false
	}
	if _, err := toml.Decode(string(data), &out); err != nil {
		return out, false
	}
	return out, true
}
