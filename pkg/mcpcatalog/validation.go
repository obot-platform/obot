package mcpcatalog

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/mcp"
	kvalidation "k8s.io/apimachinery/pkg/util/validation"
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
	if entry.EntryKey == "" {
		return nil
	}
	if errs := kvalidation.IsDNS1123Subdomain(entry.EntryKey); len(errs) > 0 {
		return fmt.Errorf("source entry key %q must be DNS-friendly: %s", entry.EntryKey, strings.Join(errs, "; "))
	}
	return nil
}

func ValidateManifest(ctx context.Context, entry types.MCPServerCatalogEntryManifest, options ValidationOptions) error {
	return errors.Join(
		mcp.ValidateCatalogEntryManifest(ctx, entry, options.GitManaged, options.MCP),
		mcp.ValidateSecretBindingsCatalogEntry(entry, options.GitManaged, false, options.MCPBackend),
		mcp.ValidateTemplateReferencesCatalogEntry(entry),
	)
}
