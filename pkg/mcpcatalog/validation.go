package mcpcatalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/obot-platform/nah/pkg/name"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/mcp"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/obot-platform/obot/pkg/utils"
	kvalidation "k8s.io/apimachinery/pkg/util/validation"
	"sigs.k8s.io/yaml"
)

var (
	invalidNameChars = regexp.MustCompile(`[^a-z0-9-]+`)
	multipleDashes   = regexp.MustCompile(`-{2,}`)
)

type ValidationOptions struct {
	MCP        mcp.ValidationOptions
	MCPBackend string
	GitManaged bool
}

// VMCPName gives catalog sync and composite migration the same resource identity.
func VMCPName(catalogName, sourceURL, entryKey, displayName string) string {
	if entryKey == "" {
		entryKey = SanitizeName(displayName)
		if entryKey == "" {
			entryKey = utils.Digest(displayName)[:12]
		}
	}

	return name.SafeHashConcatName(system.VMCPPrefix, catalogName, utils.Digest(mcp.SourceIDForURL(sourceURL))[:12], entryKey)
}

// SanitizeName converts a catalog entry name to the RFC 1123-compatible form
// used when catalog entries are persisted.
func SanitizeName(name string) string {
	name = strings.ToLower(name)
	name = invalidNameChars.ReplaceAllString(name, "-")
	name = multipleDashes.ReplaceAllString(name, "-")
	return strings.Trim(name, "-")
}

// NormalizeManifest applies the same compatibility normalization used when
// Obot imports a catalog source.
func NormalizeManifest(entry *types.MCPServerCatalogEntryManifest) {
	normalizeConfig(entry.Config)
}

// NormalizeSystemManifest applies the same compatibility normalization used
// when Obot imports a system catalog source.
func NormalizeSystemManifest(entry *types.SystemMCPServerCatalogEntryManifest) {
	normalizeConfig(entry.Config)
}

func normalizeConfig(configs []types.MCPConfig) {
	for i, config := range configs {
		if config.Key == "" {
			config.Key = config.Name
		}
		if config.Usage == types.Header {
			config.Key = strings.ReplaceAll(strings.ToUpper(config.Key), "_", "-")
		} else {
			if config.Usage == types.File || config.Usage == types.DynamicFile {
				config.Key = strings.ReplaceAll(config.Key, ".", "_")
			}
			config.Key = strings.ReplaceAll(strings.ToUpper(config.Key), "-", "_")
		}
		configs[i] = config
	}
}

func ValidateSourceFields(entry types.MCPServerCatalogEntryManifest) error {
	if SanitizeName(entry.Name) == "" {
		return fmt.Errorf("invalid catalog entry name after sanitization: original=%q sanitized=%q", entry.Name, SanitizeName(entry.Name))
	}
	return ValidateEntryKey(entry.EntryKey)
}

func ValidateEntryKey(key string) error {
	if key == "" {
		return nil
	}
	if errs := kvalidation.IsDNS1123Subdomain(key); len(errs) > 0 {
		return fmt.Errorf("source entry key %q must be DNS-friendly: %s", key, strings.Join(errs, "; "))
	}
	return nil
}

// ValidateConfigurationFields checks removed fields before typed decoding discards them.
// Other unknown fields remain allowed for forward compatibility.
func ValidateConfigurationFields(data []byte) error {
	var fields map[string]json.RawMessage
	if err := yaml.Unmarshal(data, &fields); err != nil {
		return err
	}

	for _, path := range [][]string{
		{"env"},
		{"remoteConfig", "headers"},
		{"multiUserConfig", "userDefinedHeaders"},
	} {
		parent := fields
		if len(path) == 2 {
			parent = nil
			if err := json.Unmarshal(fields[path[0]], &parent); err != nil {
				continue // Typed decoding reports malformed runtime configurations.
			}
		}
		if _, exists := parent[path[len(path)-1]]; exists {
			return fmt.Errorf("legacy configuration field %q is no longer supported; move it to top-level config", strings.Join(path, "."))
		}
	}
	return nil
}

// DecodeVMCPManifest maps catalog entry-key references to the resolved-ID
// field used by the vMCP API and storage model.
func DecodeVMCPManifest(data []byte) (types.VMCPManifest, error) {
	var manifest types.VMCPManifest
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return manifest, err
	}

	var references struct {
		EntryKey   string `json:"entryKey"`
		Components []struct {
			EntryKey string `json:"mcpServerCatalogEntryKey"`
		} `json:"components"`
	}
	if err := yaml.Unmarshal(data, &references); err != nil {
		return manifest, err
	}

	for i := range manifest.Components {
		if references.Components[i].EntryKey == "" {
			name := manifest.DisplayName
			if name == "" {
				name = references.EntryKey
			}
			if name == "" {
				name = "<unnamed>"
			}
			return manifest, fmt.Errorf("vMCP %q component %q mcpServerCatalogEntryKey is required", name, manifest.Components[i].Name)
		}
		manifest.Components[i].MCPServerCatalogEntryID = references.Components[i].EntryKey
	}

	return manifest, nil
}

func ValidateManifest(ctx context.Context, entry types.MCPServerCatalogEntryManifest, options ValidationOptions) error {
	return errors.Join(
		mcp.ValidateCatalogEntryManifest(ctx, entry, options.GitManaged, options.MCP),
		mcp.ValidateSecretBindingsCatalogEntry(entry, options.GitManaged, false, options.MCPBackend),
		mcp.ValidateTemplateReferencesCatalogEntry(entry),
	)
}
