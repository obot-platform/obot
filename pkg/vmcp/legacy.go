package vmcp

import (
	"slices"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/utils"
)

// ComponentsForInstance retains a migrated connection's snapshots until the
// administrator explicitly upgrades the corresponding vMCP component.
func ComponentsForInstance(vmcp v1.VMCP, instance v1.VMCPInstance) []types.VMCPComponent {
	components := vmcp.DeepCopy().Spec.Manifest.Components
	for i, component := range components {
		if legacy, ok := LegacyComponent(vmcp, instance, component); ok {
			components[i] = *legacy.DeepCopy()
		}
		// Hostname-constrained catalog entries require a URL for each connection.
		// Carry it through the same credential and consent flow as other user inputs.
		remote := components[i].CatalogEntry.Manifest.RemoteConfig
		if remote != nil && remote.FixedURL == "" && remote.Hostname != "" {
			if !slices.ContainsFunc(components[i].CatalogEntry.Manifest.Config, func(field types.MCPConfig) bool { return field.Key == "__url" }) {
				components[i].CatalogEntry.Manifest.Config = append(components[i].CatalogEntry.Manifest.Config, types.MCPConfig{
					Key:         "__url",
					Name:        "Server URL",
					Description: "URL must have hostname " + remote.Hostname,
					Usage:       types.Interpolated,
					Required:    true,
				})
			}
			if !slices.ContainsFunc(components[i].Configuration, func(policy types.VMCPConfigurationPolicy) bool { return policy.Key == "__url" }) {
				components[i].Configuration = append(components[i].Configuration, types.VMCPConfigurationPolicy{
					Key:    "__url",
					Policy: types.VMCPConfigurationPolicyUserAllowed,
				})
			}
		}
	}
	return slices.DeleteFunc(components, func(component types.VMCPComponent) bool {
		return slices.Contains(instance.Spec.LegacyDisabledComponents, component.ID)
	})
}

// LegacyComponent returns the snapshot a migrated connection retains for a vMCP component. A
// retained snapshot is bound to the component it was migrated with, so any change to the
// component releases it.
func LegacyComponent(vmcp v1.VMCP, instance v1.VMCPInstance, component types.VMCPComponent) (types.VMCPComponent, bool) {
	digest := utils.Digest([]any{component, vmcp.Spec.ComponentStaticConfigurationHashes[component.ID]})
	for _, legacy := range instance.Spec.LegacyComponents {
		if legacy.ID == component.ID && legacy.SourceDigest == digest {
			return legacy, true
		}
	}
	return types.VMCPComponent{}, false
}

// LegacyComponentNeedsUpdate reports whether a retained snapshot deploys a different server than
// its vMCP component, so that releasing it would update the connection.
func LegacyComponentNeedsUpdate(legacy, component types.VMCPComponent) bool {
	return SourceDigest(withoutConnectionConfiguration(legacy.CatalogEntry, true)) !=
		SourceDigest(withoutConnectionConfiguration(component.CatalogEntry, true))
}

// LegacySnapshot returns a retained snapshot without its connection's configuration, so that it
// can be shown to anyone who can read the vMCP.
func LegacySnapshot(legacy, component types.VMCPComponent) types.MCPServerCatalogEntrySnapshot {
	snapshot := withoutConnectionConfiguration(legacy.CatalogEntry, false)
	for i := range snapshot.Manifest.Config {
		field := &snapshot.Manifest.Config[i]
		for _, current := range component.CatalogEntry.Manifest.Config {
			if current.Key == field.Key {
				// Per-user access is vMCP policy, so show the component's.
				field.UserAllowed = current.UserAllowed
				break
			}
		}
	}
	return snapshot
}

// withoutConnectionConfiguration removes what a migrated snapshot holds for its connection rather
// than its server: configuration values, per-user access, and the URL a connection chose for a
// hostname-constrained or templated remote. Retained snapshots were converted from deployed
// servers, so their static configuration revision may also be the server's, which only
// identifies where static values are stored.
func withoutConnectionConfiguration(snapshot types.MCPServerCatalogEntrySnapshot, withoutRevision bool) types.MCPServerCatalogEntrySnapshot {
	snapshot = *snapshot.DeepCopy()
	for i := range snapshot.Manifest.Config {
		snapshot.Manifest.Config[i].Value = ""
		snapshot.Manifest.Config[i].UserAllowed = false
	}
	if remote := snapshot.Manifest.RemoteConfig; remote != nil && (remote.Hostname != "" || remote.URLTemplate != "") {
		remote.FixedURL = ""
	}
	if withoutRevision {
		snapshot.Manifest.StaticConfigurationRevision = ""
	}
	return snapshot
}
