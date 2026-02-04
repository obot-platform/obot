package listing

import (
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
)

// RequiresConfiguration checks if a catalog entry requires user configuration before it can be used.
// This includes environment variables, headers, or URL configuration.
func RequiresConfiguration(manifest types.MCPServerCatalogEntryManifest) bool {
	// Check for required env vars
	for _, env := range manifest.Env {
		if env.Required {
			return true
		}
	}

	// Check for remote config that needs URL or headers
	if manifest.Runtime == types.RuntimeRemote && manifest.RemoteConfig != nil {
		// Needs URL if FixedURL is empty
		if manifest.RemoteConfig.FixedURL == "" {
			return true
		}
		// Check for required headers
		for _, header := range manifest.RemoteConfig.Headers {
			if header.Required {
				return true
			}
		}
	}

	return false
}

// RequiresURLConfiguration checks if a remote server catalog entry needs URL configuration.
func RequiresURLConfiguration(manifest types.MCPServerCatalogEntryManifest) bool {
	if manifest.Runtime != types.RuntimeRemote || manifest.RemoteConfig == nil {
		return false
	}
	// Needs URL if FixedURL is empty
	return manifest.RemoteConfig.FixedURL == ""
}

// HasRequiredHeaders checks if a remote server has required headers that need user input.
func HasRequiredHeaders(manifest types.MCPServerCatalogEntryManifest) bool {
	if manifest.Runtime != types.RuntimeRemote || manifest.RemoteConfig == nil {
		return false
	}
	for _, header := range manifest.RemoteConfig.Headers {
		if header.Required {
			return true
		}
	}
	return false
}

// SearchByKeyword filters items by case-insensitive search in name and description.
func SearchByKeyword[T any](items []T, keyword string, nameGetter func(T) string, descGetter func(T) string) []T {
	if keyword == "" {
		return items
	}

	keyword = strings.ToLower(keyword)
	var result []T
	for _, item := range items {
		name := strings.ToLower(nameGetter(item))
		desc := strings.ToLower(descGetter(item))
		if strings.Contains(name, keyword) || strings.Contains(desc, keyword) {
			result = append(result, item)
		}
	}
	return result
}

// SearchCatalogEntries filters catalog entries by keyword search.
func SearchCatalogEntries(entries []v1.MCPServerCatalogEntry, keyword string) []v1.MCPServerCatalogEntry {
	return SearchByKeyword(entries, keyword,
		func(e v1.MCPServerCatalogEntry) string { return e.Spec.Manifest.Name },
		func(e v1.MCPServerCatalogEntry) string { return e.Spec.Manifest.Description },
	)
}

// SearchServers filters MCP servers by keyword search.
func SearchServers(servers []v1.MCPServer, keyword string) []v1.MCPServer {
	return SearchByKeyword(servers, keyword,
		func(s v1.MCPServer) string { return s.Spec.Manifest.Name },
		func(s v1.MCPServer) string { return s.Spec.Manifest.Description },
	)
}
