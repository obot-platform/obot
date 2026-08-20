package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// MissingCatalogEntryAdminConfig describes admin-owned configuration that a catalog
// entry requires but that has not been provided yet.
type MissingCatalogEntryAdminConfig struct {
	SecretBoundFields []string
	StaticOAuth       bool
}

// Err returns a bad-request error describing the missing admin configuration, or
// nil when nothing is missing.
func (m MissingCatalogEntryAdminConfig) Err(entryID string) error {
	var parts []string
	if len(m.SecretBoundFields) > 0 {
		parts = append(parts, fmt.Sprintf("required Kubernetes Secret bindings are missing or empty for %s", strings.Join(m.SecretBoundFields, ", ")))
	}
	if m.StaticOAuth {
		parts = append(parts, "required static OAuth credentials have not been configured")
	}
	if len(parts) == 0 {
		return nil
	}
	return types.NewErrBadRequest("catalog entry %s cannot be connected because %s", entryID, strings.Join(parts, "; "))
}

// adminConfigManifestRef is one manifest to check, with the prefix that identifies it in an error.
type adminConfigManifestRef struct {
	prefix   string
	manifest types.MCPServerCatalogEntryManifest
}

// adminConfigManifestRefs returns the manifests whose admin-owned configuration gates connecting
// to the given catalog entry: the entry's own manifest, or for a composite, each single-user
// component's upstream. Multi-user components are configured through their own instance, and an
// unresolvable reference is skipped so a missing component degrades rather than blocks.
func adminConfigManifestRefs(ctx context.Context, storageClient kclient.Client, entry v1.MCPServerCatalogEntry) ([]adminConfigManifestRef, error) {
	manifest := entry.Spec.Manifest
	if manifest.Runtime != types.RuntimeComposite {
		return []adminConfigManifestRef{{manifest: manifest}}, nil
	}

	if manifest.CompositeConfig == nil {
		return nil, nil
	}

	var refs []adminConfigManifestRef
	for _, component := range manifest.CompositeConfig.ComponentServers {
		if component.MCPServerID != "" {
			continue
		}

		upstream, err := ResolveCompositeComponentRef(ctx, storageClient, component.CatalogEntryID, component.MCPServerID)
		if err != nil {
			return nil, err
		}
		if upstream.Missing {
			continue
		}

		refs = append(refs, adminConfigManifestRef{prefix: component.ComponentID(), manifest: upstream.Manifest})
	}

	return refs, nil
}

// EntryMissingAdminConfig reports the admin-owned configuration that the given
// catalog entry still needs before it can be connected. Component references resolve through
// storageClient; the secret bindings they carry resolve through localK8sClient.
func EntryMissingAdminConfig(ctx context.Context, storageClient, localK8sClient kclient.Client, obotNamespace string, entry v1.MCPServerCatalogEntry, secretBindingAllowedLabel string) (MissingCatalogEntryAdminConfig, error) {
	missing := MissingCatalogEntryAdminConfig{
		StaticOAuth: entryRequiresStaticOAuthCreds(entry),
	}

	refs, err := adminConfigManifestRefs(ctx, storageClient, entry)
	if err != nil {
		return missing, err
	}

	for _, ref := range refs {
		var remote *types.RemoteRuntimeConfig
		if ref.manifest.RemoteConfig != nil {
			remote = &types.RemoteRuntimeConfig{Headers: ref.manifest.RemoteConfig.Headers}
		}

		missingBindings, err := MissingSecretBindings(ctx, localK8sClient, obotNamespace, ref.manifest.Env, remote, secretBindingAllowedLabel)
		if err != nil {
			return missing, err
		}
		for _, binding := range missingBindings {
			missing.SecretBoundFields = append(missing.SecretBoundFields, secretBoundFieldLabel(ref.prefix, binding.Kind, binding.Header))
		}
	}

	return missing, nil
}

// CatalogEntryRequiresUserURL reports whether the catalog entry manifest needs a URL supplied by
// the user. A composite never does; each of its component servers carries its own NeedsURL.
func CatalogEntryRequiresUserURL(manifest types.MCPServerCatalogEntryManifest) bool {
	return manifest.Runtime == types.RuntimeRemote &&
		manifest.RemoteConfig != nil &&
		(manifest.RemoteConfig.Hostname != "" || manifest.RemoteConfig.URLTemplate != "")
}

// ServerManifestFromCatalogEntryManifest converts a catalog entry manifest to a server manifest.
// If the user is an admin, they can override anything from the catalog entry.
func ServerManifestFromCatalogEntryManifest(
	isAdmin bool,
	disableHostnameValidation bool,
	entry types.MCPServerCatalogEntryManifest,
	input types.MCPServerManifest,
) (types.MCPServerManifest, error) {
	var result types.MCPServerManifest

	if entry.Runtime == types.RuntimeComposite {
		if entry.CompositeConfig == nil {
			return result, fmt.Errorf("composite config is required for composite runtime")
		}

		result = types.MCPServerManifest{
			Name:             entry.Name,
			Icon:             entry.Icon,
			ShortDescription: entry.ShortDescription,
			Description:      entry.Description,
			Metadata:         entry.Metadata,
			Runtime:          types.RuntimeComposite,
			ToolPreview:      entry.ToolPreview,
			Resources:        entry.Resources,
			CompositeConfig: &types.CompositeRuntimeConfig{
				ComponentServers: make([]types.ComponentServer, 0, len(entry.CompositeConfig.ComponentServers)),
			},
		}

		var inputConfig types.CompositeRuntimeConfig
		if input.CompositeConfig != nil {
			inputConfig = *input.CompositeConfig
		}

		inputComponents := make(map[string]types.ComponentServer, len(inputConfig.ComponentServers))
		for _, componentServer := range inputConfig.ComponentServers {
			if id := componentServer.ComponentID(); id != "" {
				inputComponents[id] = componentServer
			}
		}

		// Store the reference plus the state the composite owns. Disabled is the one component
		// field taken from the request input, because it has no counterpart on the entry.
		for _, entryComponent := range entry.CompositeConfig.ComponentServers {
			result.CompositeConfig.ComponentServers = append(result.CompositeConfig.ComponentServers, types.ComponentServer{MCPServerID: entryComponent.MCPServerID, CatalogEntryID: entryComponent.CatalogEntryID, ToolOverrides: entryComponent.ToolOverrides,
				ToolPrefix: entryComponent.ToolPrefix,
				Disabled:   inputComponents[entryComponent.ComponentID()].Disabled})
		}
	} else {
		// Non-composite: use the mapping function from types package to convert catalog entry to server manifest
		var userURL string
		if entry.Runtime == types.RuntimeRemote &&
			entry.RemoteConfig != nil &&
			entry.RemoteConfig.Hostname != "" &&
			input.RemoteConfig != nil {
			userURL = input.RemoteConfig.URL
		}

		var err error
		result, err = types.MapCatalogEntryToServer(entry, userURL, disableHostnameValidation)
		if err != nil {
			return types.MCPServerManifest{}, err
		}
	}

	// If the user is an admin, they can override anything from the catalog entry.
	if isAdmin {
		result = mergeMCPServerManifests(result, input)
	}

	return *result.DeepCopy(), nil
}

// mergeMCPServerManifests overlays the non-empty fields of override onto existing.
func mergeMCPServerManifests(existing, override types.MCPServerManifest) types.MCPServerManifest {
	if override.Name != "" {
		existing.Name = override.Name
	}
	if override.ShortDescription != "" {
		existing.ShortDescription = override.ShortDescription
	}
	if override.Description != "" {
		existing.Description = override.Description
	}
	if override.Icon != "" {
		existing.Icon = override.Icon
	}
	if len(override.Env) > 0 {
		existing.Env = override.Env
	}
	if override.Resources != nil {
		existing.Resources = override.Resources
	}
	if override.Runtime != "" {
		existing.Runtime = override.Runtime
	}

	// Merge runtime-specific configurations
	if override.UVXConfig != nil {
		existing.UVXConfig = override.UVXConfig
	}
	if override.NPXConfig != nil {
		existing.NPXConfig = override.NPXConfig
	}
	if override.ContainerizedConfig != nil {
		existing.ContainerizedConfig = override.ContainerizedConfig
	}
	if override.RemoteConfig != nil {
		if existing.RemoteConfig == nil {
			existing.RemoteConfig = override.RemoteConfig
		} else {
			if override.RemoteConfig.URL != "" {
				existing.RemoteConfig.URL = override.RemoteConfig.URL
			}

			if len(override.RemoteConfig.Headers) > 0 {
				existing.RemoteConfig.Headers = override.RemoteConfig.Headers
			}
		}
	}

	return existing
}

// ExtractEnvVars returns the names of the ${VAR} references found in text.
func ExtractEnvVars(text string) []string {
	if text == "" {
		return nil
	}

	matches := envVarRegex.FindAllStringSubmatch(text, -1)

	vars := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) > 1 {
			vars = append(vars, match[1])
		}
	}

	return vars
}

// AddExtractedEnvVars extracts and adds environment variables to the server definition
func AddExtractedEnvVars(server *v1.MCPServer) {
	// Keep track of existing env vars in the spec to avoid duplicates
	existing := make(map[string]struct{})
	for _, env := range server.Spec.Manifest.Env {
		existing[env.Key] = struct{}{}
	}

	// Extract variables based on runtime type
	var toExtract []string
	switch server.Spec.Manifest.Runtime {
	case types.RuntimeUVX:
		if server.Spec.Manifest.UVXConfig != nil {
			toExtract = []string{server.Spec.Manifest.UVXConfig.Command}
			if len(server.Spec.Manifest.UVXConfig.Args) > 0 {
				toExtract = append(toExtract, server.Spec.Manifest.UVXConfig.Args...)
			}
		}
	case types.RuntimeNPX:
		if server.Spec.Manifest.NPXConfig != nil && len(server.Spec.Manifest.NPXConfig.Args) > 0 {
			toExtract = append(toExtract, server.Spec.Manifest.NPXConfig.Args...)
		}
	case types.RuntimeContainerized:
		if server.Spec.Manifest.ContainerizedConfig != nil {
			toExtract = []string{server.Spec.Manifest.ContainerizedConfig.Command}
			if len(server.Spec.Manifest.ContainerizedConfig.Args) > 0 {
				toExtract = append(toExtract, server.Spec.Manifest.ContainerizedConfig.Args...)
			}
		}
	case types.RuntimeRemote:
		if server.Spec.Manifest.RemoteConfig != nil {
			toExtract = []string{server.Spec.Manifest.RemoteConfig.URL}
		}
	}

	for _, v := range toExtract {
		for _, env := range ExtractEnvVars(v) {
			if _, exists := existing[env]; !exists {
				server.Spec.Manifest.Env = append(server.Spec.Manifest.Env, types.MCPEnv{
					Name:        env,
					Key:         env,
					Description: "Automatically detected variable",
					Sensitive:   true,
					Required:    true,
				})
			}
		}
	}
}

// AddExtractedEnvVarsToCatalogEntry extracts and adds environment variables to the catalog entry manifest
func AddExtractedEnvVarsToCatalogEntry(entry *v1.MCPServerCatalogEntry) {
	addExtractedEnvVarsToCatalogEntryManifest(&entry.Spec.Manifest)
}

// addExtractedEnvVarsToCatalogEntryManifest extracts and adds environment variables to the given
// catalog entry manifest. A composite has no runtime configuration of its own to extract from.
func addExtractedEnvVarsToCatalogEntryManifest(manifest *types.MCPServerCatalogEntryManifest) {
	if manifest == nil || manifest.Runtime == types.RuntimeComposite {
		return
	}

	// Keep track of existing env vars in the manifest to avoid duplicates
	existing := make(map[string]struct{})
	for _, env := range manifest.Env {
		existing[env.Key] = struct{}{}
	}

	// Extract variables based on runtime type
	var toExtract []string

	switch manifest.Runtime {
	case types.RuntimeUVX:
		if manifest.UVXConfig != nil {
			toExtract = append(toExtract, manifest.UVXConfig.Command)
			if len(manifest.UVXConfig.Args) > 0 {
				toExtract = append(toExtract, manifest.UVXConfig.Args...)
			}
		}
	case types.RuntimeNPX:
		if manifest.NPXConfig != nil && len(manifest.NPXConfig.Args) > 0 {
			toExtract = append(toExtract, manifest.NPXConfig.Args...)
		}
	case types.RuntimeContainerized:
		if manifest.ContainerizedConfig != nil {
			toExtract = append(toExtract, manifest.ContainerizedConfig.Command)
			if len(manifest.ContainerizedConfig.Args) > 0 {
				toExtract = append(toExtract, manifest.ContainerizedConfig.Args...)
			}
		}
	case types.RuntimeRemote:
		if manifest.RemoteConfig != nil {
			// Add the existing headers to the existing map.
			for _, header := range manifest.RemoteConfig.Headers {
				existing[header.Key] = struct{}{}
			}

			toExtract = append(toExtract, manifest.RemoteConfig.URLTemplate)
		}
	}

	for _, v := range toExtract {
		for _, env := range ExtractEnvVars(v) {
			if _, exists := existing[env]; !exists {
				if manifest.Runtime != types.RuntimeRemote {
					manifest.Env = append(manifest.Env, types.MCPEnv{
						Name:        env,
						Key:         env,
						Description: "Automatically detected variable",
						Sensitive:   true,
						Required:    true,
					})
				} else if manifest.RemoteConfig != nil {
					manifest.RemoteConfig.Headers = append(manifest.RemoteConfig.Headers, types.MCPHeader{
						Name:        env,
						Key:         env,
						Description: "Automatically detected variable",
						Sensitive:   false,
						Required:    true,
					})
				}
			}
		}
	}
}
