package handlers

import (
	"slices"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
)

// redactedValue replaces a sensitive value that is set but hidden from the caller, as convertVMCP does, so a hidden
// value is not mistaken for one that was never set.
const redactedValue = "******"

// canSeeSensitiveConfig reports whether the caller may see the sensitive values stored inline in admin-managed
// manifests. Admin read-only users can read those manifests but not reveal their secrets, so the values are masked
// in what they are sent. Auditors see the values as they always have, including when combined with Admin Read-Only.
func canSeeSensitiveConfig(req api.Context) bool {
	return req.UserIsAdmin() || req.UserIsAuditor()
}

// redactSensitiveConfig returns a copy of config with the set values of its sensitive fields masked.
func redactSensitiveConfig(config []types.MCPConfig) []types.MCPConfig {
	if !slices.ContainsFunc(config, func(field types.MCPConfig) bool { return field.Sensitive && field.Value != "" }) {
		return config
	}
	config = slices.Clone(config)
	for i := range config {
		if config[i].Sensitive && config[i].Value != "" {
			config[i].Value = redactedValue
		}
	}
	return config
}

// redactSensitiveEnv returns a copy of env with the set values of its sensitive fields masked.
func redactSensitiveEnv(env []types.MCPEnv) []types.MCPEnv {
	if !slices.ContainsFunc(env, func(field types.MCPEnv) bool { return field.Sensitive && field.Value != "" }) {
		return env
	}
	env = slices.Clone(env)
	for i := range env {
		if env[i].Sensitive && env[i].Value != "" {
			env[i].Value = redactedValue
		}
	}
	return env
}

// redactSensitiveHeaders returns a copy of headers with the set values of its sensitive headers masked.
func redactSensitiveHeaders(headers []types.MCPHeader) []types.MCPHeader {
	if !slices.ContainsFunc(headers, func(header types.MCPHeader) bool { return header.Sensitive && header.Value != "" }) {
		return headers
	}
	headers = slices.Clone(headers)
	for i := range headers {
		if headers[i].Sensitive && headers[i].Value != "" {
			headers[i].Value = redactedValue
		}
	}
	return headers
}

// redactSystemMCPServerManifest returns a copy of manifest with its sensitive values masked. The manifest it was
// given is left as it was.
func redactSystemMCPServerManifest(manifest types.SystemMCPServerManifest) types.SystemMCPServerManifest {
	manifest.Config = redactSensitiveConfig(manifest.Config)
	manifest.DeprecatedEnv = redactSensitiveEnv(manifest.DeprecatedEnv)
	if manifest.RemoteConfig != nil {
		remote := *manifest.RemoteConfig
		remote.DeprecatedHeaders = redactSensitiveHeaders(remote.DeprecatedHeaders)
		manifest.RemoteConfig = &remote
	}
	return manifest
}

// systemMCPServerForCaller masks the sensitive values in server unless the caller may see them.
func systemMCPServerForCaller(req api.Context, server types.SystemMCPServer) types.SystemMCPServer {
	if !canSeeSensitiveConfig(req) {
		server.SystemMCPServerManifest = redactSystemMCPServerManifest(server.SystemMCPServerManifest)
	}
	return server
}

// mcpWebhookValidationForCaller masks the sensitive values in validation unless the caller may see them.
func mcpWebhookValidationForCaller(req api.Context, validation types.MCPWebhookValidation) types.MCPWebhookValidation {
	if !canSeeSensitiveConfig(req) {
		if validation.Secret != "" {
			validation.Secret = redactedValue
		}
		if validation.SystemMCPServerManifest != nil {
			manifest := redactSystemMCPServerManifest(*validation.SystemMCPServerManifest)
			validation.SystemMCPServerManifest = &manifest
		}
	}
	return validation
}

// systemMCPServerCatalogEntryForCaller masks the sensitive values in entry unless the caller may see them.
func systemMCPServerCatalogEntryForCaller(req api.Context, entry types.SystemMCPServerCatalogEntry) types.SystemMCPServerCatalogEntry {
	if !canSeeSensitiveConfig(req) {
		entry.Manifest.Config = redactSensitiveConfig(entry.Manifest.Config)
	}
	return entry
}
