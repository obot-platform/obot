package adminmcp

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/mcp"
	"github.com/obot-platform/obot/pkg/mcpcatalog"
	vmcpconfig "github.com/obot-platform/obot/pkg/vmcp"
	"sigs.k8s.io/yaml"
)

const (
	severityError   = "error"
	severityWarning = "warning"
)

// CatalogFile is a catalog source file submitted for validation.
type CatalogFile struct {
	Path    string `json:"path" jsonschema:"Path of the file in the catalog source repository."`
	Content string `json:"content" jsonschema:"The file contents, YAML or JSON."`
}

// Diagnostic is one problem found in a catalog file.
type Diagnostic struct {
	Severity string `json:"severity" jsonschema:"error: sync would fail or the file is wrong. warning: likely a mistake."`
	File     string `json:"file"`
	// Item is the index of the item in a file that holds a list.
	Item    *int   `json:"item,omitempty"`
	Message string `json:"message"`
}

// catalogValidator checks catalog files against one catalog source of this
// Obot instance, the way catalog sync would, without changing anything.
// ProfileAccess is what one profile of a vMCP grants once Obot applies the file.
type ProfileAccess struct {
	Profile    string            `json:"profile"`
	Components []ComponentAccess `json:"components" jsonschema:"The components the profile grants. Components it doesn't list grant nothing."`
}

// ComponentAccess is the tools a profile grants on one component.
type ComponentAccess struct {
	Component string `json:"component"`
	// Tools is nil when the component's entry has no tool list.
	Tools []string `json:"tools,omitempty" jsonschema:"Upstream names of the tools granted, after the component's toolOverrides."`
	// AllTools reports that the profile grants every tool the component enables, now or later.
	AllTools bool `json:"allTools,omitempty" jsonschema:"The profile grants every tool the component enables, including tools enabled later."`
	Unknown  bool `json:"unknown,omitempty" jsonschema:"The component's entry has no tool list, so the granted tools aren't known."`
}

// VMCPAccess is the access a vMCP in the submitted files grants, profile by profile.
type VMCPAccess struct {
	File     string          `json:"file"`
	Item     *int            `json:"item,omitempty"`
	VMCP     string          `json:"vmcp" jsonschema:"The vMCP's displayName."`
	Profiles []ProfileAccess `json:"profiles"`
}

type catalogValidator struct {
	catalogName string
	sourceURL   string
	// entries holds the catalog's entries by source reference.
	entries map[string]types.MCPServerCatalogEntryManifest
	// versionSkipped holds the source references of submitted entries that this Obot version
	// skips, so sync can't resolve them even if an older copy is synced.
	versionSkipped map[string]bool
	mcpBackend     string
	mcpOptions     mcp.ValidationOptions
	diagnostics    []Diagnostic
	access         []VMCPAccess
	// secretTargets maps each bindable Secret to its keys. It is nil when the
	// backend has no secret bindings.
	secretTargets map[string][]string
	// existingGroups reports which of the group ids exist.
	existingGroups func(ctx context.Context, ids []string) (map[string]bool, error)
	// existingUsers reports which of the user ids exist.
	existingUsers func(ctx context.Context, ids []string) (map[string]bool, error)
}

type catalogItem struct {
	file  string
	index *int
	data  json.RawMessage
}

func (v *catalogValidator) add(item catalogItem, severity, format string, args ...any) {
	v.diagnostics = append(v.diagnostics, Diagnostic{
		Severity: severity,
		File:     item.file,
		Item:     item.index,
		Message:  fmt.Sprintf(format, args...),
	})
}

func (v *catalogValidator) validate(ctx context.Context, files []CatalogFile) ([]Diagnostic, error) {
	sourceID := mcp.SourceIDForURL(v.sourceURL)
	var vmcps []catalogItem
	seenEntryKeys := map[string]string{}
	for _, file := range files {
		items, isList, err := mcpcatalog.DecodeCatalogContents[json.RawMessage]([]byte(file.Content))
		if err != nil {
			v.add(catalogItem{file: file.Path}, severityError, "invalid catalog file: %v", err)
			continue
		}
		for i, data := range items {
			item := catalogItem{file: file.Path, data: data}
			if isList {
				item.index = &i
			}

			var header struct {
				Type string `json:"type"`
			}
			if err := yaml.Unmarshal(data, &header); err != nil {
				v.add(item, severityError, "invalid catalog item: %v", err)
				continue
			}
			switch header.Type {
			case "vmcp":
				vmcps = append(vmcps, item)
			case "", "entry":
				if entry, ok := v.validateEntry(ctx, item, seenEntryKeys); ok && entry.EntryKey != "" {
					// The submitted entry replaces the synced one when the change merges.
					v.entries[mcpcatalog.SourceRef(sourceID, entry.EntryKey)] = entry
				}
			default:
				v.add(item, severityError, "unsupported catalog item type %q; use vmcp or entry", header.Type)
			}
		}
	}

	seenVMCPKeys := map[string]string{}
	for _, item := range vmcps {
		if err := v.validateVMCP(ctx, item, seenVMCPKeys); err != nil {
			return nil, err
		}
	}
	return v.diagnostics, nil
}

func label(item catalogItem) string {
	if item.index != nil {
		return fmt.Sprintf("%s[%d]", item.file, *item.index)
	}
	return item.file
}

// validateEntry checks a catalog entry item the way sync does and returns its
// manifest if it is in this Obot version's range.
func (v *catalogValidator) validateEntry(ctx context.Context, item catalogItem, seenEntryKeys map[string]string) (types.MCPServerCatalogEntryManifest, bool) {
	bounds, err := mcpcatalog.DecodeObotVersionBounds(item.data)
	if err != nil {
		v.add(item, severityError, "%v", err)
		return types.MCPServerCatalogEntryManifest{}, false
	}
	if err := mcpcatalog.ValidateEntryKey(bounds.EntryKey); err != nil {
		v.add(item, severityError, "%v", err)
	}
	if bounds.EntryKey != "" {
		if previous, ok := seenEntryKeys[bounds.EntryKey]; ok {
			v.add(item, severityError, "duplicate entryKey %q also used by %s", bounds.EntryKey, previous)
		} else {
			seenEntryKeys[bounds.EntryKey] = label(item)
		}
	}
	if !bounds.InCurrentRange() {
		v.add(item, severityWarning, "entry %q is limited to %s, so this Obot instance skips it", bounds.Name, versionRange(bounds))
		if bounds.EntryKey != "" {
			// Once this change merges, sync skips the entry, so the synced copy no longer resolves.
			reference := mcpcatalog.SourceRef(mcp.SourceIDForURL(v.sourceURL), bounds.EntryKey)
			delete(v.entries, reference)
			if v.versionSkipped == nil {
				v.versionSkipped = map[string]bool{}
			}
			v.versionSkipped[reference] = true
		}
		return types.MCPServerCatalogEntryManifest{}, false
	}

	if err := mcpcatalog.ValidateConfigurationFields(item.data); err != nil {
		v.add(item, severityError, "%v", err)
		return types.MCPServerCatalogEntryManifest{}, false
	}
	var entry types.MCPServerCatalogEntryManifest
	if err := yaml.Unmarshal(item.data, &entry); err != nil {
		v.add(item, severityError, "invalid catalog entry: %v", err)
		return types.MCPServerCatalogEntryManifest{}, false
	}
	mcpcatalog.NormalizeManifest(&entry)

	valid := true
	if err := mcpcatalog.ValidateSourceFields(entry); err != nil {
		v.add(item, severityError, "%v", err)
		valid = false
	}
	if err := mcpcatalog.ValidateManifest(ctx, entry, mcpcatalog.ValidationOptions{
		MCP:        v.mcpOptions,
		MCPBackend: v.mcpBackend,
		GitManaged: true,
	}); err != nil {
		v.add(item, severityError, "%v", err)
		valid = false
	}
	if entry.EntryKey == "" {
		v.add(item, severityWarning, "entry %q has no entryKey, so vMCPs can't reference it", entry.Name)
	}
	return entry, valid
}

func (v *catalogValidator) validateVMCP(ctx context.Context, item catalogItem, seenVMCPKeys map[string]string) error {
	manifest, err := mcpcatalog.DecodeVMCPManifest(item.data)
	if err != nil {
		v.add(item, severityError, "invalid vMCP: %v", err)
		return nil
	}
	var header struct {
		EntryKey string             `json:"entryKey"`
		Profiles *[]json.RawMessage `json:"profiles"`
	}
	if err := yaml.Unmarshal(item.data, &header); err != nil {
		v.add(item, severityError, "invalid vMCP: %v", err)
		return nil
	}

	if manifest.DisplayName == "" {
		v.add(item, severityError, "vMCP displayName is required")
		return nil
	}
	if err := mcpcatalog.ValidateEntryKey(header.EntryKey); err != nil {
		v.add(item, severityError, "%v", err)
	}
	if header.EntryKey == "" {
		v.add(item, severityWarning, "vMCP %q has no entryKey, so changing its displayName replaces it with a new vMCP", manifest.DisplayName)
	}
	key := mcpcatalog.VMCPEntryKey(header.EntryKey, manifest.DisplayName)
	if previous, ok := seenVMCPKeys[key]; ok {
		v.add(item, severityError, "duplicate vMCP entry key %q also used by %s", key, previous)
	} else {
		seenVMCPKeys[key] = label(item)
	}
	if header.Profiles == nil {
		v.add(item, severityWarning, "vMCP %q has no profiles, so only admins can use it; write profiles, even as an empty list", manifest.DisplayName)
	}

	if !v.resolveComponents(item, &manifest) {
		return nil
	}
	v.checkSensitiveValues(item, manifest)

	if err := manifest.Validate(); err != nil {
		v.add(item, severityError, "%v", err)
		return nil
	}
	if err := mcp.ValidateSecretBindingsVMCP(manifest, v.mcpBackend); err != nil {
		v.add(item, severityError, "%v", err)
	}
	v.checkSecretBindingTargets(item, manifest)
	v.checkRequiredConfiguration(item, manifest)
	v.checkToolNames(item, manifest)
	v.checkProfileGrants(item, manifest)
	return v.checkSubjects(ctx, item, manifest)
}

// resolveComponents resolves each component's entry reference as sync does
// and reports whether they all resolved.
func (v *catalogValidator) resolveComponents(item catalogItem, manifest *types.VMCPManifest) bool {
	sourceID := mcp.SourceIDForURL(v.sourceURL)
	resolved := true
	for index := range manifest.Components {
		component := &manifest.Components[index]
		reference := component.MCPServerCatalogEntryID
		refSourceID, entryKey, _, valid := mcpcatalog.ParseSourceRef(sourceID, reference)
		if !valid {
			v.add(item, severityError, "component %q has invalid mcpServerCatalogEntryKey %q; use entryKey or sourceID::entryKey", component.ID, reference)
			resolved = false
			continue
		}
		entry, ok := v.entries[mcpcatalog.SourceRef(refSourceID, entryKey)]
		if !ok && v.versionSkipped[mcpcatalog.SourceRef(refSourceID, entryKey)] {
			v.add(item, severityError, "component %q references %q, which this change limits to other Obot versions, so this Obot instance skips it and the vMCP can't use it", component.ID, reference)
			resolved = false
			continue
		}
		if !ok {
			v.add(item, severityError, "component %q references %q, but catalog %q has no entry with entryKey %q in source %q; find references with search_catalog_entries", component.ID, reference, v.catalogName, entryKey, refSourceID)
			resolved = false
			continue
		}
		component.CatalogEntry = types.MCPServerCatalogEntrySnapshot{Manifest: entry}
		if component.Name == "" {
			component.Name = entry.Name
		}
	}
	return resolved
}

// checkSensitiveValues reports fixed values written into the file for sensitive configuration.
func (v *catalogValidator) checkSensitiveValues(item catalogItem, manifest types.VMCPManifest) {
	for _, component := range manifest.Components {
		for _, policy := range component.Configuration {
			if policy.Value == "" {
				continue
			}
			if slices.ContainsFunc(vmcpconfig.ComponentConfig(component), func(config types.MCPConfig) bool {
				return config.Key == policy.Key && config.Sensitive
			}) {
				v.add(item, severityError, "component %q configuration %q is sensitive, so its value must not be in the file; use a secretBinding (see list_secret_bindings)", component.ID, policy.Key)
			}
		}
	}
}

// checkSecretBindingTargets reports secret bindings that name a Secret or key Obot can't bind.
func (v *catalogValidator) checkSecretBindingTargets(item catalogItem, manifest types.VMCPManifest) {
	if v.secretTargets == nil {
		return
	}
	for _, component := range manifest.Components {
		for _, policy := range component.Configuration {
			binding := policy.SecretBinding
			if binding == nil {
				continue
			}
			keys, ok := v.secretTargets[binding.Name]
			if !ok {
				v.add(item, severityError, "component %q configuration %q binds Secret %q, which is not a bindable Secret on this Obot instance (see list_secret_bindings)", component.ID, policy.Key, binding.Name)
			} else if !slices.Contains(keys, binding.Key) {
				v.add(item, severityError, "component %q configuration %q binds key %q, which Secret %q doesn't have", component.ID, policy.Key, binding.Key, binding.Name)
			}
		}
	}
}

// checkRequiredConfiguration reports required configuration that sync can't fill.
func (v *catalogValidator) checkRequiredConfiguration(item catalogItem, manifest types.VMCPManifest) {
	values := vmcpconfig.ExtractStaticConfiguration(&manifest, nil)
	for _, component := range manifest.Components {
		policies := map[string]types.VMCPConfigurationPolicyType{}
		for _, policy := range component.Configuration {
			policies[policy.Key] = policy.Policy
		}
		for _, missing := range vmcpconfig.MissingRequiredConfiguration(component, values, false) {
			_, key, _ := vmcpconfig.ParseConfigurationKey(missing)
			if policies[key] == types.VMCPConfigurationPolicyFixed {
				v.add(item, severityWarning, "component %q required configuration %q is fixed without a value or secretBinding; sync keeps a value already stored for this vMCP and fails if there is none", component.ID, key)
			} else {
				v.add(item, severityError, "component %q required configuration %q must be fixed with a value or secretBinding", component.ID, key)
			}
		}
	}
}

// checkToolNames reports tool names that the referenced entry doesn't have.
func (v *catalogValidator) checkToolNames(item catalogItem, manifest types.VMCPManifest) {
	tools := map[string][]string{}
	for _, component := range manifest.Components {
		for _, tool := range component.CatalogEntry.Manifest.ToolPreview {
			tools[component.ID] = append(tools[component.ID], tool.Name)
		}
	}

	unknownTools := map[string]bool{}
	check := func(componentID, tool, where string) {
		known, ok := tools[componentID]
		if !ok {
			if !unknownTools[componentID] {
				unknownTools[componentID] = true
				v.add(item, severityWarning, "component %q tool names can't be checked because its entry has no tool list; get them with list_entry_tools", componentID)
			}
			return
		}
		if !slices.Contains(known, tool) {
			v.add(item, severityError, "%s names tool %q, which component %q doesn't have; tool names are upstream names, see list_entry_tools", where, tool, componentID)
		}
	}
	for _, component := range manifest.Components {
		for _, override := range component.ToolOverrides {
			check(component.ID, override.Name, fmt.Sprintf("component %q toolOverrides", component.ID))
		}
	}
	for _, profile := range manifest.Profiles {
		for componentID, set := range profile.Permissions.AllowedComponents {
			for _, tool := range set.AllowedTools {
				check(componentID, tool, fmt.Sprintf("profile %q", profile.Name))
			}
		}
	}
}

// checkProfileGrants reports profiles that grant a component's tools without listing them, and
// records the tools each profile grants once Obot applies the file.
func (v *catalogValidator) checkProfileGrants(item catalogItem, manifest types.VMCPManifest) {
	access := VMCPAccess{File: item.file, Item: item.index, VMCP: manifest.DisplayName, Profiles: []ProfileAccess{}}
	for _, profile := range manifest.Profiles {
		granted := ProfileAccess{Profile: profile.Name, Components: []ComponentAccess{}}
		for _, component := range manifest.Components {
			set, listed := profile.Permissions.AllowedComponents[component.ID]
			if !profile.Permissions.AllowAllComponents && !listed {
				continue
			}
			enabled, known := enabledTools(component)
			allTools := profile.Permissions.AllowAllComponents || set.AllowedTools == nil
			if allTools && !profile.Permissions.AllowAllComponents {
				v.add(item, severityWarning, "profile %q grants every tool on component %q, including tools enabled on it later; list allowedTools unless that's intended", profile.Name, component.ID)
			}
			tools := enabled
			if !allTools {
				tools = []string{}
				// The vMCP's own validation already rejected tools the component doesn't enable.
				for _, tool := range set.AllowedTools {
					if !known || slices.Contains(enabled, tool) {
						tools = append(tools, tool)
					}
				}
			}
			componentAccess := ComponentAccess{Component: component.ID, AllTools: allTools, Unknown: !known && allTools}
			if !componentAccess.Unknown {
				componentAccess.Tools = tools
			}
			granted.Components = append(granted.Components, componentAccess)
		}
		access.Profiles = append(access.Profiles, granted)
	}
	v.access = append(v.access, access)
}

// enabledTools returns the upstream names of the tools a component enables, and whether they
// are known. Without toolOverrides, a component enables every tool its entry lists.
func enabledTools(component types.VMCPComponent) ([]string, bool) {
	if len(component.ToolOverrides) > 0 {
		tools := []string{}
		for _, override := range component.ToolOverrides {
			if override.Enabled {
				tools = append(tools, override.Name)
			}
		}
		return tools, true
	}
	preview := component.CatalogEntry.Manifest.ToolPreview
	if len(preview) == 0 {
		return nil, false
	}
	tools := make([]string, 0, len(preview))
	for _, tool := range preview {
		tools = append(tools, tool.Name)
	}
	return tools, true
}

// checkSubjects reports subjects that don't exist and subjects that grant broadly.
func (v *catalogValidator) checkSubjects(ctx context.Context, item catalogItem, manifest types.VMCPManifest) error {
	var groupIDs, userIDs []string
	for _, profile := range manifest.Profiles {
		for _, subject := range profile.Subjects {
			switch subject.Type {
			case types.SubjectTypeGroup:
				groupIDs = append(groupIDs, subject.ID)
			case types.SubjectTypeUser:
				v.add(item, severityWarning, "profile %q grants user %q; user ids are specific to this Obot instance, so prefer a group", profile.Name, subject.ID)
				userIDs = append(userIDs, subject.ID)
			case types.SubjectTypeSelector:
				v.add(item, severityWarning, "profile %q grants every user", profile.Name)
			case types.SubjectTypeObotGroup:
				if subject.ID == types.GroupAuthenticated {
					v.add(item, severityWarning, "profile %q grants every authenticated user", profile.Name)
				}
			}
		}
	}

	existingUsers, err := v.lookup(ctx, userIDs, v.existingUsers)
	if err != nil {
		return err
	}
	existingGroups, err := v.lookup(ctx, groupIDs, v.existingGroups)
	if err != nil {
		return err
	}
	for _, check := range []struct {
		subjectType types.SubjectType
		existing    map[string]bool
		message     string
	}{
		{subjectType: types.SubjectTypeUser, existing: existingUsers, message: "profile %q user %q doesn't exist; find user ids with find_user"},
		{subjectType: types.SubjectTypeGroup, existing: existingGroups, message: "profile %q group %q doesn't exist; find group ids with search_groups"},
	} {
		for _, profile := range manifest.Profiles {
			for _, subject := range profile.Subjects {
				if subject.Type == check.subjectType && !check.existing[subject.ID] {
					v.add(item, severityError, check.message, profile.Name, subject.ID)
				}
			}
		}
	}
	return nil
}

// lookup reports which of ids exist, in one call, or none if there are no ids.
func (v *catalogValidator) lookup(ctx context.Context, ids []string, exists func(context.Context, []string) (map[string]bool, error)) (map[string]bool, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	return exists(ctx, ids)
}

// versionRange describes the Obot versions an entry is limited to.
func versionRange(bounds mcpcatalog.ObotVersionBounds) string {
	switch {
	case bounds.MaxObotVersion == "":
		return "Obot " + bounds.MinObotVersion + " and later"
	case bounds.MinObotVersion == "":
		return "Obot " + bounds.MaxObotVersion + " and earlier"
	}
	return "Obot " + bounds.MinObotVersion + " to " + bounds.MaxObotVersion
}

// errorCount returns the number of error diagnostics.
func errorCount(diagnostics []Diagnostic) int {
	count := 0
	for _, diagnostic := range diagnostics {
		if diagnostic.Severity == severityError {
			count++
		}
	}
	return count
}
