package devicescan

import (
	"encoding/json"
	"os"
	"unicode/utf8"

	"github.com/BurntSushi/toml"
	"github.com/obot-platform/obot/apiclient/types"
	"gopkg.in/yaml.v3"
)

// maxFileBytes is the per-file content cap. Files larger than this are
// recorded with Oversized=true and no Content.
const maxFileBytes int64 = 1 << 20 // 1 MiB

// readScanFile builds the wire DeviceScanFile for path. Callers stat
// the path themselves before invoking, so a missing/unreadable file
// here just returns a zero DeviceScanFile. Files larger than
// maxFileBytes get Oversized=true and no Content; non-UTF-8 contents
// are also dropped (size still recorded).
func readScanFile(path string) types.DeviceScanFile {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return types.DeviceScanFile{}
	}

	file := types.DeviceScanFile{Path: path, SizeBytes: info.Size()}
	if info.Size() > maxFileBytes {
		file.Oversized = true
		return file
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return file
	}

	if utf8.Valid(data) {
		file.Content = string(data)
	}

	return file
}

// readJSON reads and decodes a JSON file into T. Returns the zero T
// and false on any error. Use a typed struct for T to get compile-time
// schema validation; use map[string]any when the schema is genuinely
// open-ended.
func readJSON[T any](path string) (T, bool) {
	var (
		out       T
		data, err = os.ReadFile(path)
	)
	if err != nil {
		return out, false
	}

	if err := json.Unmarshal(data, &out); err != nil {
		return out, false
	}

	return out, true
}

// readYAML reads and decodes a YAML file into T. See readJSON.
func readYAML[T any](path string) (T, bool) {
	var (
		out       T
		data, err = os.ReadFile(path)
	)
	if err != nil {
		return out, false
	}

	if err := yaml.Unmarshal(data, &out); err != nil {
		return out, false
	}

	return out, true
}

// readTOML reads and decodes a TOML file into T. See readJSON.
func readTOML[T any](path string) (T, bool) {
	var (
		out       T
		data, err = os.ReadFile(path)
	)
	if err != nil {
		return out, false
	}

	if _, err := toml.Decode(string(data), &out); err != nil {
		return out, false
	}

	return out, true
}
