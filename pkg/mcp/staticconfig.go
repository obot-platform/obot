package mcp

import (
	"maps"
	"net/url"
	"slices"
	"strconv"

	"github.com/obot-platform/obot/apiclient/types"
)

// StaticConfigurationCredentialName returns the name of the credential used to
// store a resource's sensitive static configuration.
func StaticConfigurationCredentialName(resourceName string) string {
	return resourceName + "-static-configuration"
}

// CatalogHasSensitiveStaticConfiguration reports whether a catalog manifest or
// any of its composite components contains a sensitive field eligible for
// static credential storage.
func CatalogHasSensitiveStaticConfiguration(m *types.MCPServerCatalogEntryManifest) bool {
	if fieldsHaveSensitiveStaticConfiguration(m.Env, catalogHeaders(m.RemoteConfig)) {
		return true
	}
	if m.CompositeConfig != nil {
		for i := range m.CompositeConfig.ComponentServers {
			if CatalogHasSensitiveStaticConfiguration(&m.CompositeConfig.ComponentServers[i].Manifest) {
				return true
			}
		}
	}
	return false
}

// ServerHasSensitiveStaticConfiguration reports whether a server manifest or
// any of its composite components contains a sensitive field eligible for
// static credential storage.
func ServerHasSensitiveStaticConfiguration(m *types.MCPServerManifest) bool {
	if fieldsHaveSensitiveStaticConfiguration(m.Env, runtimeHeaders(m.RemoteConfig)) {
		return true
	}
	if m.CompositeConfig != nil {
		for i := range m.CompositeConfig.ComponentServers {
			if ServerHasSensitiveStaticConfiguration(&m.CompositeConfig.ComponentServers[i].Manifest) {
				return true
			}
		}
	}
	return false
}

// CatalogHasStaticConfigurationValues reports whether a catalog manifest or
// any of its composite components contains a non-empty static configuration
// value eligible for credential storage.
func CatalogHasStaticConfigurationValues(m *types.MCPServerCatalogEntryManifest) bool {
	if fieldsHaveStaticConfigurationValues(m.Env, catalogHeaders(m.RemoteConfig)) {
		return true
	}
	if m.CompositeConfig != nil {
		for i := range m.CompositeConfig.ComponentServers {
			if CatalogHasStaticConfigurationValues(&m.CompositeConfig.ComponentServers[i].Manifest) {
				return true
			}
		}
	}
	return false
}

// ServerHasStaticConfigurationValues reports whether a server manifest or any
// of its composite components contains a non-empty static configuration value
// eligible for credential storage.
func ServerHasStaticConfigurationValues(m *types.MCPServerManifest) bool {
	if fieldsHaveStaticConfigurationValues(m.Env, runtimeHeaders(m.RemoteConfig)) {
		return true
	}
	if m.CompositeConfig != nil {
		for i := range m.CompositeConfig.ComponentServers {
			if ServerHasStaticConfigurationValues(&m.CompositeConfig.ComponentServers[i].Manifest) {
				return true
			}
		}
	}
	return false
}

// SystemCatalogHasStaticConfigurationValues reports whether a system catalog
// manifest contains a non-empty static configuration value eligible for
// credential storage.
func SystemCatalogHasStaticConfigurationValues(m *types.SystemMCPServerCatalogEntryManifest) bool {
	return fieldsHaveStaticConfigurationValues(m.Env, catalogHeaders(m.RemoteConfig))
}

// SystemServerHasStaticConfigurationValues reports whether a system server
// manifest contains a non-empty static configuration value eligible for
// credential storage.
func SystemServerHasStaticConfigurationValues(m *types.SystemMCPServerManifest) bool {
	return fieldsHaveStaticConfigurationValues(m.Env, runtimeHeaders(m.RemoteConfig))
}

func fieldsHaveStaticConfigurationValues(env []types.MCPEnv, headers []types.MCPHeader) bool {
	for _, f := range env {
		if isPotentialStaticField(f.MCPHeader) && f.Value != "" {
			return true
		}
	}
	for _, f := range headers {
		if isPotentialStaticField(f) && f.Value != "" {
			return true
		}
	}
	return false
}

func fieldsHaveSensitiveStaticConfiguration(env []types.MCPEnv, headers []types.MCPHeader) bool {
	for _, f := range env {
		if isPotentialStaticField(f.MCPHeader) {
			return true
		}
	}
	return slices.ContainsFunc(headers, isPotentialStaticField)
}

// ExtractStaticCatalogConfiguration removes sensitive static values from a
// catalog manifest and returns the values to store as a credential. When
// editable is true, configured placeholders preserve matching existing values.
// Composite component manifests are processed recursively.
func ExtractStaticCatalogConfiguration(m *types.MCPServerCatalogEntryManifest, existing map[string]string, editable bool) map[string]string {
	secrets, seen := cloneConfiguration(existing), map[string]struct{}{}
	walkCatalogFields(m, "", secrets, seen, editable)
	removeUnseenStaticValues(secrets, seen)
	return secrets
}

func walkCatalogFields(m *types.MCPServerCatalogEntryManifest, prefix string, secrets map[string]string, seen map[string]struct{}, editable bool) {
	walkFields(m.Env, catalogHeaders(m.RemoteConfig), prefix, secrets, seen, editable)
	if m.CompositeConfig != nil {
		componentOccurrences := map[string]int{}
		for i := range m.CompositeConfig.ComponentServers {
			c := &m.CompositeConfig.ComponentServers[i]
			walkCatalogFields(&c.Manifest, nextComponentStaticPath(prefix, c.ComponentID(), componentOccurrences), secrets, seen, editable)
		}
	}
}

// ExtractStaticServerConfiguration removes sensitive static values from a
// server manifest and returns the values to store as a credential. When
// editable is true, configured placeholders preserve matching existing values.
// Composite component manifests are processed recursively.
func ExtractStaticServerConfiguration(m *types.MCPServerManifest, existing map[string]string, editable bool) map[string]string {
	secrets, seen := cloneConfiguration(existing), map[string]struct{}{}
	walkServerFields(m, "", secrets, seen, editable)
	removeUnseenStaticValues(secrets, seen)
	return secrets
}

func walkServerFields(m *types.MCPServerManifest, prefix string, secrets map[string]string, seen map[string]struct{}, editable bool) {
	walkFields(m.Env, runtimeHeaders(m.RemoteConfig), prefix, secrets, seen, editable)
	if m.CompositeConfig != nil {
		componentOccurrences := map[string]int{}
		for i := range m.CompositeConfig.ComponentServers {
			c := &m.CompositeConfig.ComponentServers[i]
			walkServerFields(&c.Manifest, nextComponentStaticPath(prefix, c.ComponentID(), componentOccurrences), secrets, seen, editable)
		}
	}
}

// ExtractStaticSystemCatalogConfiguration removes sensitive static values from
// a system catalog manifest and returns the values to store as a credential.
// When editable is true, configured placeholders preserve matching existing
// values.
func ExtractStaticSystemCatalogConfiguration(m *types.SystemMCPServerCatalogEntryManifest, existing map[string]string, editable bool) map[string]string {
	secrets, seen := cloneConfiguration(existing), map[string]struct{}{}
	walkFields(m.Env, catalogHeaders(m.RemoteConfig), "", secrets, seen, editable)
	removeUnseenStaticValues(secrets, seen)
	return secrets
}

// ExtractStaticSystemServerConfiguration removes sensitive static values from
// a system server manifest and returns the values to store as a credential.
// When editable is true, configured placeholders preserve matching existing
// values.
func ExtractStaticSystemServerConfiguration(m *types.SystemMCPServerManifest, existing map[string]string, editable bool) map[string]string {
	secrets, seen := cloneConfiguration(existing), map[string]struct{}{}
	walkFields(m.Env, runtimeHeaders(m.RemoteConfig), "", secrets, seen, editable)
	removeUnseenStaticValues(secrets, seen)
	return secrets
}

func walkFields(env []types.MCPEnv, headers []types.MCPHeader, prefix string, secrets map[string]string, seen map[string]struct{}, editable bool) {
	for i := range env {
		processStaticField(&env[i].MCPHeader, staticFieldPath(prefix, env[i].Key), secrets, seen, editable)
	}
	for i := range headers {
		processStaticField(&headers[i], staticFieldPath(prefix, headers[i].Key), secrets, seen, editable)
	}
}

func processStaticField(f *types.MCPHeader, path string, secrets map[string]string, seen map[string]struct{}, editable bool) {
	configured := f.ValueConfigured
	f.ValueConfigured = false
	if !isPotentialStaticField(*f) {
		return
	}
	seen[path] = struct{}{}
	if f.Value != "" {
		secrets[path], f.Value = f.Value, ""
		f.ValueConfigured = true
		return
	}
	if editable && configured && secrets[path] != "" {
		f.ValueConfigured = true
		return
	}
	delete(secrets, path)
}

func isPotentialStaticField(f types.MCPHeader) bool {
	return f.Sensitive && f.SecretBinding == nil && len(f.Options) == 0
}

// HydrateStaticCatalogConfiguration returns a deep copy of a catalog manifest
// with sensitive static values restored from a credential. The source manifest
// is not modified.
func HydrateStaticCatalogConfiguration(m types.MCPServerCatalogEntryManifest, secrets map[string]string) types.MCPServerCatalogEntryManifest {
	result := m.DeepCopy()
	rehydrateCatalog(result, "", secrets)
	return *result
}

func rehydrateCatalog(m *types.MCPServerCatalogEntryManifest, prefix string, secrets map[string]string) {
	rehydrateFields(m.Env, catalogHeaders(m.RemoteConfig), prefix, secrets)
	if m.CompositeConfig != nil {
		componentOccurrences := map[string]int{}
		for i := range m.CompositeConfig.ComponentServers {
			c := &m.CompositeConfig.ComponentServers[i]
			rehydrateCatalog(&c.Manifest, nextComponentStaticPath(prefix, c.ComponentID(), componentOccurrences), secrets)
		}
	}
}

// HydrateStaticServerConfiguration returns a deep copy of a server manifest
// with sensitive static values restored from a credential. The source manifest
// is not modified.
func HydrateStaticServerConfiguration(m types.MCPServerManifest, secrets map[string]string) types.MCPServerManifest {
	result := m.DeepCopy()
	rehydrateServer(result, "", secrets)
	return *result
}

func rehydrateServer(m *types.MCPServerManifest, prefix string, secrets map[string]string) {
	rehydrateFields(m.Env, runtimeHeaders(m.RemoteConfig), prefix, secrets)
	if m.CompositeConfig != nil {
		componentOccurrences := map[string]int{}
		for i := range m.CompositeConfig.ComponentServers {
			c := &m.CompositeConfig.ComponentServers[i]
			rehydrateServer(&c.Manifest, nextComponentStaticPath(prefix, c.ComponentID(), componentOccurrences), secrets)
		}
	}
}

// HydrateStaticSystemCatalogConfiguration returns a deep copy of a system
// catalog manifest with sensitive static values restored from a credential. The
// source manifest is not modified.
func HydrateStaticSystemCatalogConfiguration(m types.SystemMCPServerCatalogEntryManifest, secrets map[string]string) types.SystemMCPServerCatalogEntryManifest {
	result := m.DeepCopy()
	rehydrateFields(result.Env, catalogHeaders(result.RemoteConfig), "", secrets)
	return *result
}

// HydrateStaticSystemServerConfiguration returns a deep copy of a system server
// manifest with sensitive static values restored from a credential. The source
// manifest is not modified.
func HydrateStaticSystemServerConfiguration(m types.SystemMCPServerManifest, secrets map[string]string) types.SystemMCPServerManifest {
	result := m.DeepCopy()
	rehydrateFields(result.Env, runtimeHeaders(result.RemoteConfig), "", secrets)
	return *result
}

func rehydrateFields(env []types.MCPEnv, headers []types.MCPHeader, prefix string, secrets map[string]string) {
	for i := range env {
		if env[i].Sensitive && env[i].Value == "" {
			env[i].Value = secrets[staticFieldPath(prefix, env[i].Key)]
		}
	}
	for i := range headers {
		if headers[i].Sensitive && headers[i].Value == "" {
			headers[i].Value = secrets[staticFieldPath(prefix, headers[i].Key)]
		}
	}
}

// RedactStaticCatalogConfiguration removes sensitive values from a catalog
// manifest and its composite components while recording whether each value is
// configured. The supplied credential is used to account for extracted values.
func RedactStaticCatalogConfiguration(m *types.MCPServerCatalogEntryManifest, secrets map[string]string) {
	rehydrateCatalog(m, "", secrets)
	redactCatalog(m)
}

func redactCatalog(m *types.MCPServerCatalogEntryManifest) {
	redactFields(m.Env, catalogHeaders(m.RemoteConfig))
	if m.CompositeConfig != nil {
		for i := range m.CompositeConfig.ComponentServers {
			redactCatalog(&m.CompositeConfig.ComponentServers[i].Manifest)
		}
	}
}

// RedactStaticServerConfiguration removes sensitive values from a server
// manifest and its composite components while recording whether each value is
// configured. The supplied credential is used to account for extracted values.
func RedactStaticServerConfiguration(m *types.MCPServerManifest, secrets map[string]string) {
	rehydrateServer(m, "", secrets)
	redactServer(m)
}

func redactServer(m *types.MCPServerManifest) {
	redactFields(m.Env, runtimeHeaders(m.RemoteConfig))
	if m.CompositeConfig != nil {
		for i := range m.CompositeConfig.ComponentServers {
			redactServer(&m.CompositeConfig.ComponentServers[i].Manifest)
		}
	}
}

// RedactStaticSystemCatalogConfiguration removes sensitive values from a
// system catalog manifest while recording whether each value is configured.
// The supplied credential is used to account for extracted values.
func RedactStaticSystemCatalogConfiguration(m *types.SystemMCPServerCatalogEntryManifest, secrets map[string]string) {
	rehydrateFields(m.Env, catalogHeaders(m.RemoteConfig), "", secrets)
	redactFields(m.Env, catalogHeaders(m.RemoteConfig))
}

// RedactStaticSystemServerConfiguration removes sensitive values from a system
// server manifest while recording whether each value is configured. The
// supplied credential is used to account for extracted values.
func RedactStaticSystemServerConfiguration(m *types.SystemMCPServerManifest, secrets map[string]string) {
	rehydrateFields(m.Env, runtimeHeaders(m.RemoteConfig), "", secrets)
	redactFields(m.Env, runtimeHeaders(m.RemoteConfig))
}

func redactFields(env []types.MCPEnv, headers []types.MCPHeader) {
	for i := range env {
		if env[i].Sensitive {
			env[i].ValueConfigured, env[i].Value = env[i].ValueConfigured || env[i].Value != "", ""
		} else {
			env[i].ValueConfigured = false
		}
	}
	for i := range headers {
		if headers[i].Sensitive {
			headers[i].ValueConfigured, headers[i].Value = headers[i].ValueConfigured || headers[i].Value != "", ""
		} else {
			headers[i].ValueConfigured = false
		}
	}
}

// MergeRuntimeConfiguration combines user-supplied and static configuration,
// with static values taking precedence when both maps contain the same key.
func MergeRuntimeConfiguration(user, static map[string]string) map[string]string {
	result := cloneConfiguration(user)
	maps.Copy(result, static)
	return result
}

func cloneConfiguration(existing map[string]string) map[string]string {
	result := make(map[string]string, len(existing))
	maps.Copy(result, existing)
	return result
}

func removeUnseenStaticValues(secrets map[string]string, seen map[string]struct{}) {
	for key := range secrets {
		if _, ok := seen[key]; !ok {
			delete(secrets, key)
		}
	}
}

func staticFieldPath(prefix, key string) string {
	if prefix == "" {
		return key
	}
	return appendStaticPath(prefix, key)
}

func appendStaticPath(prefix string, parts ...string) string {
	result := prefix
	for _, part := range parts {
		if part == "" {
			continue
		}
		if result != "" {
			result += "/"
		}
		result += url.PathEscape(part)
	}
	return result
}

func nextComponentStaticPath(prefix, componentID string, occurrences map[string]int) string {
	occurrence := occurrences[componentID]
	occurrences[componentID]++
	return appendStaticPath(prefix, "component", componentID, strconv.Itoa(occurrence))
}

func catalogHeaders(c *types.RemoteCatalogConfig) []types.MCPHeader {
	if c == nil {
		return nil
	}
	return c.Headers
}

func runtimeHeaders(c *types.RemoteRuntimeConfig) []types.MCPHeader {
	if c == nil {
		return nil
	}
	return c.Headers
}
