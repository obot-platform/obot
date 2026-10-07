package controller

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"reflect"
	"slices"

	"github.com/obot-platform/obot/apiclient/types"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/utils"
	vmcpconfig "github.com/obot-platform/obot/pkg/vmcp"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// Run again to repair fixed fields left in legacy connection snapshots by the first version.
	catalogEntryStaticConfigurationMigrationName = "catalog_entry_static_configuration_credentials_v2"
)

// staticConfigurationMigration moves literal configuration values of catalog entries, and the
// copies of them in MCPServers and vMCP snapshots, into catalog entry credentials. Copies holding
// the same values as their entry share its revision, so the migration reports no new drift.
type staticConfigurationMigration struct {
	client kclient.Client
	store  mcp.StaticConfigurationStore
	// revisions maps an entry and a digest of its values to the revision that stores them.
	revisions map[[2]string]string
	// literals holds each entry's literal values as they were before the entry was migrated.
	literals map[string]map[string]string
}

func migrateCatalogEntryStaticConfiguration(ctx context.Context, client kclient.Client, store mcp.StaticConfigurationStore) error {
	m := staticConfigurationMigration{
		client:    client,
		store:     store,
		revisions: make(map[[2]string]string),
		literals:  make(map[string]map[string]string),
	}

	var (
		entries   v1.MCPServerCatalogEntryList
		servers   v1.MCPServerList
		vmcps     v1.VMCPList
		instances v1.VMCPInstanceList
	)
	for _, list := range []kclient.ObjectList{&entries, &servers, &vmcps, &instances} {
		if err := client.List(ctx, list); err != nil {
			return err
		}
	}

	// Drift is judged against the entries as they were before any of them was migrated. A retry
	// after a partial failure finds some entries migrated already, so their literal values are
	// recovered from the revisions the failed attempt stored.
	upToDate := make(map[[2]string]bool)
	entriesByName := make(map[kclient.ObjectKey]*v1.MCPServerCatalogEntry, len(entries.Items))
	unmigrated := make(map[kclient.ObjectKey]types.MCPServerCatalogEntrySnapshot, len(entries.Items))
	for i := range entries.Items {
		entry := &entries.Items[i]
		literals, err := m.entryLiterals(ctx, entry)
		if err != nil {
			return fmt.Errorf("failed to read static configuration of catalog entry %q: %w", entry.Name, err)
		}
		key := kclient.ObjectKeyFromObject(entry)
		entriesByName[key] = entry
		m.literals[entry.Name] = literals
		unmigrated[key] = unmigratedSnapshot(*entry, literals)
	}
	for _, vmcp := range vmcps.Items {
		for _, component := range vmcp.Spec.Manifest.Components {
			if snapshot, ok := unmigrated[kclient.ObjectKey{Namespace: vmcp.Namespace, Name: component.MCPServerCatalogEntryID}]; ok {
				upToDate[[2]string{vmcp.Name, component.ID}] = !vmcpconfig.NeedsUpdate(component, snapshot)
			}
		}
	}

	for i := range entries.Items {
		if err := m.migrateEntry(ctx, &entries.Items[i]); err != nil {
			return err
		}
	}

	for i := range servers.Items {
		if err := m.migrateServer(ctx, &servers.Items[i]); err != nil {
			return err
		}
	}

	for i := range vmcps.Items {
		vmcp := &vmcps.Items[i]
		if err := m.migrateVMCP(ctx, vmcp, entriesByName, upToDate, instances.Items); err != nil {
			return err
		}
	}

	for i := range instances.Items {
		if err := m.migrateInstance(ctx, &instances.Items[i]); err != nil {
			return err
		}
	}

	return nil
}

func entrySnapshot(entry v1.MCPServerCatalogEntry) types.MCPServerCatalogEntrySnapshot {
	return types.MCPServerCatalogEntrySnapshot{
		Manifest:         entry.Spec.Manifest,
		UnsupportedTools: entry.Spec.UnsupportedTools,
	}
}

// entryLiterals returns an entry's literal values as they were before it was migrated.
func (m *staticConfigurationMigration) entryLiterals(ctx context.Context, entry *v1.MCPServerCatalogEntry) (map[string]string, error) {
	literals := literalValues(entry.Spec.Manifest.Config)
	if !mcp.HasStaticConfiguration(entry.Spec.Manifest.Config) {
		return literals, nil
	}

	stored, err := mcp.RevealStaticConfiguration(ctx, m.store, entry.Name, entry.Spec.Manifest.StaticConfigurationRevision)
	if err != nil {
		return nil, err
	}
	for _, field := range entry.Spec.Manifest.Config {
		if value := stored[field.Key]; field.Static && value != "" {
			literals[field.Key] = value
		}
	}
	return literals, nil
}

// unmigratedSnapshot returns the snapshot of an entry as it was before it was migrated.
func unmigratedSnapshot(entry v1.MCPServerCatalogEntry, literals map[string]string) types.MCPServerCatalogEntrySnapshot {
	snapshot := entrySnapshot(*entry.DeepCopy())
	if !mcp.HasStaticConfiguration(snapshot.Manifest.Config) {
		return snapshot
	}
	for i := range snapshot.Manifest.Config {
		if field := &snapshot.Manifest.Config[i]; field.Static {
			field.Value, field.Static = literals[field.Key], false
		}
	}
	snapshot.Manifest.StaticConfigurationRevision = ""
	return snapshot
}

// literalValues returns the literal values of config.
func literalValues(config []types.MCPConfig) map[string]string {
	values := make(map[string]string)
	for _, field := range config {
		if field.Value != "" && field.SecretBinding == nil {
			values[field.Key] = field.Value
		}
	}
	return values
}

// extractLiteralValues moves the literal values of config out of it and marks those fields Static.
// Fields that are already Static were migrated earlier and keep their stored values.
func extractLiteralValues(config []types.MCPConfig) map[string]string {
	values := make(map[string]string)
	for i := range config {
		field := &config[i]
		if field.Value == "" || field.SecretBinding != nil {
			continue
		}
		values[field.Key] = field.Value
		field.Value = ""
		field.Static = true
	}
	return values
}

// revision returns the revision of an entry's static configuration holding values, storing them
// in a new revision if no revision holding them has been seen yet.
func (m *staticConfigurationMigration) revision(ctx context.Context, entryName string, values map[string]string) (string, error) {
	key := [2]string{entryName, utils.Digest(values)}
	if revision, ok := m.revisions[key]; ok {
		return revision, nil
	}
	revision, err := mcp.StoreStaticConfigurationValues(ctx, m.store, entryName, values, "", nil)
	if err != nil {
		return "", err
	}
	m.revisions[key] = revision
	return revision, nil
}

// remember records a revision stored before this migration, so copies with the same values
// share it after a retried migration.
func (m *staticConfigurationMigration) remember(ctx context.Context, entryName, revision string) error {
	if revision == "" {
		return nil
	}
	values, err := mcp.RevealStaticConfiguration(ctx, m.store, entryName, revision)
	if err != nil {
		return err
	}
	m.revisions[[2]string{entryName, utils.Digest(values)}] = revision
	return nil
}

// migrateManifestConfig moves the literal values of config, and the values held outside it for
// its fields, into a revision of entryName's static configuration and reports whether anything
// changed.
func (m *staticConfigurationMigration) migrateManifestConfig(ctx context.Context, entryName string, config []types.MCPConfig, held map[string]string, revision *string) (bool, error) {
	values := extractLiteralValues(config)
	for i := range config {
		field := &config[i]
		if value, ok := held[field.Key]; ok && field.Value == "" && !field.Static && field.SecretBinding == nil {
			values[field.Key] = value
			field.Static = true
		}
	}
	if len(values) == 0 {
		return false, nil
	}
	if *revision != "" {
		// A manifest cannot hold both literal values and a revision; keep the stored values too.
		stored, err := mcp.RevealStaticConfiguration(ctx, m.store, entryName, *revision)
		if err != nil {
			return false, err
		}
		for key, value := range stored {
			if _, ok := values[key]; !ok {
				values[key] = value
			}
		}
	}

	var err error
	*revision, err = m.revision(ctx, entryName, values)
	return true, err
}

func (m *staticConfigurationMigration) migrateEntry(ctx context.Context, entry *v1.MCPServerCatalogEntry) error {
	if err := m.remember(ctx, entry.Name, entry.Spec.Manifest.StaticConfigurationRevision); err != nil {
		return err
	}

	changed, err := m.migrateManifestConfig(ctx, entry.Name, entry.Spec.Manifest.Config, nil, &entry.Spec.Manifest.StaticConfigurationRevision)
	if err != nil {
		return fmt.Errorf("failed to migrate static configuration of catalog entry %q: %w", entry.Name, err)
	}
	if !changed {
		return nil
	}
	return m.client.Update(ctx, entry)
}

func (m *staticConfigurationMigration) migrateServer(ctx context.Context, server *v1.MCPServer) error {
	if server.Spec.MCPServerCatalogEntryName == "" {
		// Servers without a source entry have no entry credential to reference.
		return nil
	}

	// Servers can hold their entry's literal values in their own credential instead of their
	// manifest. Those values are moved to the entry's credential too. vMCP servers are rebuilt
	// from their migrated snapshots, so only their manifests are migrated here.
	held := make(map[string]string)
	if server.Spec.VMCPID == "" && server.Spec.VMCPInstanceID == "" {
		literals := m.literals[server.Spec.MCPServerCatalogEntryName]
		if len(literals) > 0 {
			configuration, err := m.revealConfiguration(ctx, server.CredentialContext(server.Spec.UserID), server.Name)
			if err != nil {
				return fmt.Errorf("failed to read configuration of MCP server %q: %w", server.Name, err)
			}
			for key := range literals {
				if value := configuration[key]; value != "" {
					held[key] = value
				}
			}
		}
	}

	changed, err := m.migrateManifestConfig(ctx, server.Spec.MCPServerCatalogEntryName, server.Spec.Manifest.Config, held, &server.Spec.Manifest.StaticConfigurationRevision)
	if err != nil {
		return fmt.Errorf("failed to migrate static configuration of MCP server %q: %w", server.Name, err)
	}
	if changed {
		if err := m.client.Update(ctx, server); err != nil {
			return err
		}
	}
	if server.Spec.VMCPID != "" || server.Spec.VMCPInstanceID != "" {
		// vMCP servers' credentials are rebuilt from their vMCP's configuration.
		return nil
	}

	// Once the manifest references the entry's revision, the server's copies of those values are
	// removed, so they are never mistaken for configuration the server's user supplied.
	var keys []string
	for _, field := range server.Spec.Manifest.Config {
		if field.Static {
			keys = append(keys, field.Key)
		}
	}
	if err := m.scrubConfiguration(ctx, server.CredentialContext(server.Spec.UserID), server.Name, keys); err != nil {
		return fmt.Errorf("failed to remove static configuration from MCP server %q: %w", server.Name, err)
	}
	return nil
}

// migrateComponent migrates a vMCP snapshot. Migrations to vMCPs moved some of an entry's literal
// values out of the snapshot and into configuration for a policy of the given type; a value that
// still matches the entry's literal is moved to the entry's revision like the literals the
// snapshot still holds. A value that differs is a vMCP override and stays with its policy.
func (m *staticConfigurationMigration) migrateComponent(ctx context.Context, component *types.VMCPComponent, configuration map[string]string, policy types.VMCPConfigurationPolicyType) (bool, error) {
	if component.MCPServerCatalogEntryID == "" {
		return false, nil
	}

	literals := m.literals[component.MCPServerCatalogEntryID]
	held := make(map[string]string)
	for _, configured := range component.Configuration {
		if configured.Policy != policy || configured.SecretBinding != nil {
			continue
		}
		if value := configuration[vmcpconfig.ConfigurationKey(component.ID, configured.Key)]; value != "" && value == literals[configured.Key] {
			held[configured.Key] = value
		}
	}

	manifest := &component.CatalogEntry.Manifest
	changed, err := m.migrateManifestConfig(ctx, component.MCPServerCatalogEntryID, manifest.Config, held, &manifest.StaticConfigurationRevision)
	if err != nil || len(held) == 0 {
		return changed, err
	}

	// The entry supplies the moved values now, so their policies are removed with them.
	component.Configuration = slices.DeleteFunc(component.Configuration, func(configured types.VMCPConfigurationPolicy) bool {
		_, moved := held[configured.Key]
		return moved && configured.Policy == policy
	})
	return true, nil
}

// movedConfigurationKeys returns the credential keys of a component's Static fields that no
// policy of the given type supplies. They hold values moved to the entry's revision.
func movedConfigurationKeys(component types.VMCPComponent, policy types.VMCPConfigurationPolicyType) []string {
	var keys []string
	for _, field := range component.CatalogEntry.Manifest.Config {
		if !field.Static || slices.ContainsFunc(component.Configuration, func(configured types.VMCPConfigurationPolicy) bool {
			return configured.Key == field.Key && configured.Policy == policy
		}) {
			continue
		}
		keys = append(keys, vmcpconfig.ConfigurationKey(component.ID, field.Key))
	}
	return keys
}

func (m *staticConfigurationMigration) migrateVMCP(ctx context.Context, vmcp *v1.VMCP, entries map[kclient.ObjectKey]*v1.MCPServerCatalogEntry, upToDate map[[2]string]bool, instances []v1.VMCPInstance) error {
	configuration, err := m.revealConfiguration(ctx, vmcpconfig.StaticConfigurationCredentialContext(vmcp.Name), vmcpconfig.StaticConfigurationCredentialName(vmcp))
	if err != nil {
		return fmt.Errorf("failed to read static configuration of vMCP %q: %w", vmcp.Name, err)
	}

	var changed [][2]types.VMCPComponent
	for i := range vmcp.Spec.Manifest.Components {
		component := &vmcp.Spec.Manifest.Components[i]
		previous := *component.DeepCopy()
		if _, err := m.migrateComponent(ctx, component, configuration, types.VMCPConfigurationPolicyFixed); err != nil {
			return fmt.Errorf("failed to migrate static configuration of vMCP %q component %q: %w", vmcp.Name, component.ID, err)
		}

		// The entry was migrated too, so a component that matched it must still match it, even
		// when the component itself holds none of the entry's literal values.
		if entry := entries[kclient.ObjectKey{Namespace: vmcp.Namespace, Name: component.MCPServerCatalogEntryID}]; entry != nil && len(m.literals[entry.Name]) > 0 && upToDate[[2]string{vmcp.Name, component.ID}] {
			component.SourceDigest = vmcpconfig.SourceDigest(entrySnapshot(*entry))
		}
		if !reflect.DeepEqual(previous, *component) {
			changed = append(changed, [2]types.VMCPComponent{previous, *component})
		}
	}
	if len(changed) > 0 {
		if err := m.client.Update(ctx, vmcp); err != nil {
			return err
		}
		for _, components := range changed {
			if err := m.retainLegacyComponents(ctx, *vmcp, components[0], components[1], instances); err != nil {
				return err
			}
		}
	}

	var keys []string
	for _, component := range vmcp.Spec.Manifest.Components {
		keys = append(keys, movedConfigurationKeys(component, types.VMCPConfigurationPolicyFixed)...)
	}
	if err := m.scrubConfiguration(ctx, vmcpconfig.StaticConfigurationCredentialContext(vmcp.Name), vmcpconfig.StaticConfigurationCredentialName(vmcp), keys); err != nil {
		return fmt.Errorf("failed to remove moved configuration from vMCP %q: %w", vmcp.Name, err)
	}
	return nil
}

// retainLegacyComponents keeps the legacy components of vMCP instances bound to a component that
// the migration changed. They are bound by a digest of the component they were created for.
func (m *staticConfigurationMigration) retainLegacyComponents(ctx context.Context, vmcp v1.VMCP, previous, component types.VMCPComponent, instances []v1.VMCPInstance) error {
	staticHash := vmcp.Spec.ComponentStaticConfigurationHashes[component.ID]
	previousDigest := utils.Digest([]any{previous, staticHash})
	for i := range instances {
		instance := &instances[i]
		if instance.Namespace != vmcp.Namespace || instance.Spec.Manifest.VMCPID != vmcp.Name {
			continue
		}
		var changed bool
		for j := range instance.Spec.LegacyComponents {
			legacy := &instance.Spec.LegacyComponents[j]
			if legacy.ID == component.ID && legacy.SourceDigest == previousDigest {
				legacy.SourceDigest = utils.Digest([]any{component, staticHash})
				changed = true
			}
		}
		if changed {
			if err := m.client.Update(ctx, instance); err != nil {
				return fmt.Errorf("failed to update vMCP instance %q: %w", instance.Name, err)
			}
		}
	}
	return nil
}

func (m *staticConfigurationMigration) migrateInstance(ctx context.Context, instance *v1.VMCPInstance) error {
	if len(instance.Spec.LegacyComponents) == 0 {
		return nil
	}
	configuration, err := m.revealConfiguration(ctx, vmcpconfig.InstanceConfigurationCredentialContext(instance.Name), vmcpconfig.ConfigurationCredentialName())
	if err != nil {
		return fmt.Errorf("failed to read configuration of vMCP instance %q: %w", instance.Name, err)
	}

	var vmcp v1.VMCP
	if err := m.client.Get(ctx, kclient.ObjectKey{Namespace: instance.Namespace, Name: instance.Spec.Manifest.VMCPID}, &vmcp); kclient.IgnoreNotFound(err) != nil {
		return err
	}

	var changed bool
	for i := range instance.Spec.LegacyComponents {
		legacy := &instance.Spec.LegacyComponents[i]
		for _, component := range vmcp.Spec.Manifest.Components {
			if component.ID != legacy.ID || component.MCPServerCatalogEntryID != legacy.MCPServerCatalogEntryID || legacy.SourceDigest != utils.Digest([]any{component, vmcp.Spec.ComponentStaticConfigurationHashes[component.ID]}) {
				continue
			}

			fixedChanged, err := m.migrateLegacyFixedConfiguration(ctx, legacy, component)
			if err != nil {
				return fmt.Errorf("failed to migrate fixed configuration of vMCP instance %q: %w", instance.Name, err)
			}
			changed = changed || fixedChanged
		}

		// Legacy connections keep their values as user configuration.
		componentChanged, err := m.migrateComponent(ctx, &instance.Spec.LegacyComponents[i], configuration, types.VMCPConfigurationPolicyUserAllowed)
		if err != nil {
			return fmt.Errorf("failed to migrate static configuration of vMCP instance %q: %w", instance.Name, err)
		}
		changed = changed || componentChanged
	}
	if changed {
		if err := m.client.Update(ctx, instance); err != nil {
			return err
		}
	}

	var keys []string
	for _, component := range instance.Spec.LegacyComponents {
		keys = append(keys, movedConfigurationKeys(component, types.VMCPConfigurationPolicyUserAllowed)...)
	}
	if err := m.scrubConfiguration(ctx, vmcpconfig.InstanceConfigurationCredentialContext(instance.Name), vmcpconfig.ConfigurationCredentialName(), keys); err != nil {
		return fmt.Errorf("failed to remove moved configuration from vMCP instance %q: %w", instance.Name, err)
	}
	return nil
}

// migrateLegacyFixedConfiguration restores values moved out of the vMCP credential to its
// catalog snapshot. User overrides keep their own configuration and revision.
func (m *staticConfigurationMigration) migrateLegacyFixedConfiguration(ctx context.Context, legacy *types.VMCPComponent, component types.VMCPComponent) (bool, error) {
	held := make(map[string]string)
	for _, field := range component.CatalogEntry.Manifest.Config {
		if field.Static && slices.ContainsFunc(legacy.Configuration, func(policy types.VMCPConfigurationPolicy) bool {
			return policy.Key == field.Key && policy.Policy == types.VMCPConfigurationPolicyFixed && policy.SecretBinding == nil
		}) {
			held[field.Key] = ""
		}
	}
	if len(held) == 0 {
		return false, nil
	}

	config, err := mcp.ResolveStaticConfiguration(ctx, m.store, component.MCPServerCatalogEntryID, component.CatalogEntry.Manifest.StaticConfigurationRevision, component.CatalogEntry.Manifest.Config)
	if err != nil {
		return false, err
	}
	for _, field := range config {
		if _, ok := held[field.Key]; ok {
			held[field.Key] = field.Value
		}
	}

	manifest := &legacy.CatalogEntry.Manifest
	changed, err := m.migrateManifestConfig(ctx, legacy.MCPServerCatalogEntryID, manifest.Config, held, &manifest.StaticConfigurationRevision)
	if err != nil {
		return false, err
	}
	previous := len(legacy.Configuration)
	legacy.Configuration = slices.DeleteFunc(legacy.Configuration, func(policy types.VMCPConfigurationPolicy) bool {
		_, moved := held[policy.Key]
		return moved && policy.Policy == types.VMCPConfigurationPolicyFixed && policy.SecretBinding == nil
	})
	return changed || len(legacy.Configuration) != previous, nil
}

// scrubConfiguration removes keys from a credential, if it has any of them.
func (m *staticConfigurationMigration) scrubConfiguration(ctx context.Context, credentialContext, name string, keys []string) error {
	if len(keys) == 0 {
		return nil
	}
	configuration, err := m.revealConfiguration(ctx, credentialContext, name)
	if err != nil {
		return err
	}
	scrubbed := maps.Clone(configuration)
	for _, key := range keys {
		delete(scrubbed, key)
	}
	if len(scrubbed) == len(configuration) {
		return nil
	}
	return m.store.UpsertCredential(ctx, gatewaytypes.Credential{Context: credentialContext, Name: name, Secrets: scrubbed})
}

// revealConfiguration returns a credential's values, or none when it does not exist.
func (m *staticConfigurationMigration) revealConfiguration(ctx context.Context, credentialContext, name string) (map[string]string, error) {
	credential, err := m.store.RevealCredential(ctx, []string{credentialContext}, name)
	if errors.As(err, &gateway.CredentialNotFoundError{}) {
		return map[string]string{}, nil
	}
	return credential.Secrets, err
}
