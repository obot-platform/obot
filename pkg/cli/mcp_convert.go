package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/mcpcatalog"
	"github.com/spf13/cobra"
	"go.yaml.in/yaml/v3"
	strict "sigs.k8s.io/yaml"
)

var (
	errCompositeCatalogEntry = errors.New("composite catalog entries must be converted to vMCPs separately")
)

type MCPConvertCatalogYAML struct{}

func (*MCPConvertCatalogYAML) Customize(cmd *cobra.Command) {
	cmd.Use = "convert-catalog-yaml <path>"
	cmd.Short = "Convert MCP catalog YAML to the config schema in place"
	cmd.Long = "Convert a catalog YAML file, or recursively convert YAML files in a directory, in place. Directory traversal honors .obotcatalogs and .ignoreobotcatalogs, just like catalog sync. Review the changes in version control before publishing the catalog."
	cmd.Args = cobra.ExactArgs(1)
}

func (*MCPConvertCatalogYAML) Run(cmd *cobra.Command, args []string) error {
	info, err := os.Stat(args[0])
	if err != nil {
		return err
	}
	converted := 0
	_, err = validateCatalogPaths(args, func(path string) error {
		// The shared walker also selects JSON files; this command converts YAML only.
		if ext := filepath.Ext(path); info.IsDir() && ext != ".yaml" && ext != ".yml" {
			return nil
		}
		changed, err := convertCatalogYAMLFile(path)
		if err != nil {
			if errors.Is(err, errCompositeCatalogEntry) {
				fmt.Fprintf(cmd.ErrOrStderr(), "Warning: %s contains a composite catalog entry, which is no longer supported by catalog sync. The file was left unchanged; convert composites to vMCPs separately.\n", path)
			}
			return fmt.Errorf("%s: %w", path, err)
		}
		if changed {
			converted++
			fmt.Fprintf(cmd.OutOrStdout(), "Converted %s\n", path)
		}
		return nil
	})
	fmt.Fprintf(cmd.OutOrStdout(), "Converted %d catalog files.\n", converted)
	return err
}

func convertCatalogYAMLFile(path string) (bool, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return false, err
	}
	if !info.Mode().IsRegular() {
		return false, fmt.Errorf("expected a regular file (symlinks are not converted)")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	converted, err := convertCatalogYAML(data)
	if err != nil || converted == nil {
		return false, err
	}
	// Replace only after conversion and validation succeed. A failed write must
	// never truncate the original catalog, which may contain static secrets.
	file, err := os.CreateTemp(filepath.Dir(path), ".obot-catalog-*")
	if err != nil {
		return false, err
	}
	defer os.Remove(file.Name())
	_, writeErr := file.Write(converted)
	closeErr := file.Close()
	if writeErr != nil {
		return false, writeErr
	}
	if closeErr != nil {
		return false, closeErr
	}
	if err := os.Chmod(file.Name(), info.Mode().Perm()); err != nil {
		return false, err
	}
	if err := os.Rename(file.Name(), path); err != nil {
		return false, err
	}
	return true, nil
}

func convertCatalogYAML(data []byte) ([]byte, error) {
	var document yaml.Node
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&document); err != nil {
		return nil, err
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("expected one YAML document; use a list for multiple entries")
	}
	root := document.Content[0]
	entries := []*yaml.Node{root}
	if root.Kind == yaml.SequenceNode {
		entries = root.Content
	}
	changed := false
	for i, entry := range entries {
		if entry.Kind != yaml.MappingNode {
			return nil, fmt.Errorf("entry %d: expected a mapping", i)
		}
		if field := catalogYAMLField(entry, "runtime"); field != nil {
			var runtime types.Runtime
			if err := field.Decode(&runtime); err != nil {
				return nil, fmt.Errorf("entry %d: %w", i, err)
			}
			if runtime == types.RuntimeComposite {
				return nil, fmt.Errorf("entry %d: %w", i, errCompositeCatalogEntry)
			}
		}
		config := catalogYAMLField(entry, "config")
		if config == nil {
			config = &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		}
		if config.Kind != yaml.SequenceNode {
			return nil, fmt.Errorf("entry %d: config must be a list", i)
		}
		for _, source := range []struct {
			parent *yaml.Node
			key    string
			header bool
		}{
			{parent: entry, key: "env"},
			{parent: catalogYAMLField(entry, "remoteConfig"), key: "headers", header: true},
			{parent: catalogYAMLField(entry, "multiUserConfig"), key: "userDefinedHeaders", header: true},
		} {
			fields := catalogYAMLField(source.parent, source.key)
			if fields == nil {
				continue
			}
			if fields.Kind != yaml.SequenceNode && fields.Tag != "!!null" {
				return nil, fmt.Errorf("entry %d: %s must be a list", i, source.key)
			}
			for _, field := range fields.Content {
				if field.Kind != yaml.MappingNode || catalogYAMLField(field, "usage") != nil {
					return nil, fmt.Errorf("entry %d: invalid legacy configuration item", i)
				}
				usage := types.Header
				if !source.header {
					var flags struct {
						File         bool `yaml:"file"`
						DynamicFile  bool `yaml:"dynamicFile"`
						Interpolated bool `yaml:"interpolated"`
					}
					if err := field.Decode(&flags); err != nil {
						return nil, fmt.Errorf("entry %d: %w", i, err)
					}
					usage = types.ConfigFromEnv(types.MCPEnv{File: flags.File, DynamicFile: flags.DynamicFile, Interpolated: flags.Interpolated}).Usage
					removeCatalogYAMLField(field, "file")
					removeCatalogYAMLField(field, "dynamicFile")
					removeCatalogYAMLField(field, "interpolated")
				}
				field.Content = append(field.Content, catalogYAMLString("usage"), catalogYAMLString(string(usage)))
				config.Content = append(config.Content, field)
			}
			removeCatalogYAMLField(source.parent, source.key)
			changed = true
		}
		if catalogYAMLField(entry, "config") == nil && len(config.Content) > 0 {
			entry.Content = append(entry.Content, catalogYAMLString("config"), config)
		}
		if removeCatalogYAMLField(entry, "serverUserType") {
			changed = true
		}
		if multi := catalogYAMLField(entry, "multiUserConfig"); multi != nil && (multi.Tag == "!!null" || multi.Kind == yaml.MappingNode && len(multi.Content) == 0) {
			removeCatalogYAMLField(entry, "multiUserConfig")
			changed = true
		}
	}
	var output bytes.Buffer
	encoder := yaml.NewEncoder(&output)
	encoder.SetIndent(2)
	if err := encoder.Encode(&document); err != nil {
		return nil, err
	}
	// Strict decoding rejects unsupported legacy composites and unknown fields
	// rather than silently dropping data. Validate config keys across all sources.
	for i, entry := range entries {
		data, err := yaml.Marshal(entry)
		if err != nil {
			return nil, err
		}
		if catalogYAMLField(entry, "systemMCPServerType") != nil || catalogYAMLField(entry, "filterConfig") != nil {
			var manifest types.SystemMCPServerCatalogEntryManifest
			if err := strict.UnmarshalStrict(data, &manifest); err != nil {
				return nil, fmt.Errorf("entry %d: %w", i, err)
			}
			mcpcatalog.NormalizeSystemManifest(&manifest)
			if err := manifest.ValidateConfig(); err != nil {
				return nil, fmt.Errorf("entry %d: %w", i, err)
			}
			continue
		}
		var manifest types.MCPServerCatalogEntryManifest
		if err := strict.UnmarshalStrict(data, &manifest); err != nil {
			return nil, fmt.Errorf("entry %d: %w", i, err)
		}
		mcpcatalog.NormalizeManifest(&manifest)
		if err := manifest.ValidateConfig(); err != nil {
			return nil, fmt.Errorf("entry %d: %w", i, err)
		}
	}
	if !changed {
		return nil, nil
	}
	return output.Bytes(), nil
}

func catalogYAMLString(value string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value}
}

func catalogYAMLField(node *yaml.Node, key string) *yaml.Node {
	if node != nil && node.Kind == yaml.MappingNode {
		for i := 0; i < len(node.Content); i += 2 {
			if node.Content[i].Value == key {
				return node.Content[i+1]
			}
		}
	}
	return nil
}

func removeCatalogYAMLField(node *yaml.Node, key string) bool {
	if node != nil && node.Kind == yaml.MappingNode {
		for i := 0; i < len(node.Content); i += 2 {
			if node.Content[i].Value == key {
				node.Content = append(node.Content[:i], node.Content[i+2:]...)
				return true
			}
		}
	}
	return false
}
