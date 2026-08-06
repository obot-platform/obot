package mcpcatalog

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
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
	for i, env := range entry.Env {
		if env.Key == "" {
			env.Key = env.Name
		}
		if filepath.Ext(env.Key) != "" {
			env.Key = strings.ReplaceAll(env.Key, ".", "_")
			env.File = true
		}
		env.Key = strings.ReplaceAll(strings.ToUpper(env.Key), "-", "_")
		entry.Env[i] = env
	}

	if entry.Runtime == types.RuntimeRemote && entry.RemoteConfig != nil {
		for i, header := range entry.RemoteConfig.Headers {
			if header.Key == "" {
				header.Key = header.Name
			}
			header.Key = strings.ReplaceAll(strings.ToUpper(header.Key), "_", "-")
			entry.RemoteConfig.Headers[i] = header
		}
	}

	if entry.ServerUserType == "" {
		entry.ServerUserType = types.ServerUserTypeSingleUser
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
