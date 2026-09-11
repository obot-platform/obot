package types

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

const (
	VMCPConfigurationPolicyProhibited  VMCPConfigurationPolicyType = "prohibited"
	VMCPConfigurationPolicyFixed       VMCPConfigurationPolicyType = "fixed"
	VMCPConfigurationPolicyUserAllowed VMCPConfigurationPolicyType = "userAllowed"
)

// VMCP is a stable, optionally multi-component MCP endpoint definition.
type VMCP struct {
	Metadata                `json:",inline"`
	VMCPManifest            `json:",inline"`
	LegacySlug              string     `json:"legacySlug,omitempty"`
	UserID                  string     `json:"userID,omitempty"`
	StaticConfigurationHash string     `json:"staticConfigurationHash,omitempty"`
	Status                  VMCPStatus `json:"status,omitempty"`
}

// VMCPManifest contains the user-managed portion of a VMCP.
type VMCPManifest struct {
	DisplayName string          `json:"displayName"`
	Description string          `json:"description,omitempty"`
	Icon        string          `json:"icon,omitempty"`
	Components  []VMCPComponent `json:"components"`
	Profiles    []VMCPProfile   `json:"profiles,omitempty"`
}

// VMCPComponent is a snapshot of one catalog entry and the policy applied to it.
// Runtime resolution uses CatalogEntry rather than resolving the source live.
// The API populates MCPCatalogID, CatalogEntry, and SourceDigest from the entry ID.
type VMCPComponent struct {
	// ID is the immutable, server-assigned identity used to scope component configuration.
	ID                      string                        `json:"id,omitempty"`
	Name                    string                        `json:"name"`
	MCPCatalogID            string                        `json:"mcpCatalogID"`
	MCPServerCatalogEntryID string                        `json:"mcpServerCatalogEntryID"`
	CatalogEntry            MCPServerCatalogEntrySnapshot `json:"catalogEntry"`
	SourceDigest            string                        `json:"sourceDigest,omitempty"`
	Configuration           []VMCPConfigurationPolicy     `json:"configuration,omitempty"`
	ForceSingleUser         bool                          `json:"forceSingleUser,omitempty"`
	OAuthCredentialID       string                        `json:"oauthCredentialID,omitempty"`
	AllowedTools            []string                      `json:"allowedTools,omitempty"`
	ToolPrefix              string                        `json:"toolPrefix,omitempty"`
	ToolOverrides           []ToolOverride                `json:"toolOverrides,omitempty"`
}

// MCPServerCatalogEntrySnapshot is the catalog-entry data retained by a VMCP.
// Source ownership and mutable status are deliberately not included.
type MCPServerCatalogEntrySnapshot struct {
	Manifest         MCPServerCatalogEntryManifest `json:"manifest"`
	UnsupportedTools []string                      `json:"unsupportedTools,omitempty"`
}

// MarshalJSON excludes tool previews from storage, API responses, and snapshot
// digests. Previews are fetched on demand, not part of the deployed definition.
func (s MCPServerCatalogEntrySnapshot) MarshalJSON() ([]byte, error) {
	type snapshot MCPServerCatalogEntrySnapshot
	s.Manifest.ToolPreview = nil
	return json.Marshal(snapshot(s))
}

type VMCPConfigurationPolicy struct {
	Key    string                      `json:"key"`
	Policy VMCPConfigurationPolicyType `json:"policy,omitempty"`
	// Value is write-only fixed configuration. The API removes it from the
	// persisted VMCP manifest and stores it in the VMCP credential.
	Value string `json:"value,omitempty"`
}

type VMCPConfigurationPolicyType string

// VMCPProfile grants access and tools to matching users and groups. Profiles
// are additive. AllowAllTools means all tools enabled on the VMCP are granted;
// otherwise only AllowedTools are granted, including an intentionally empty set.
type VMCPProfile struct {
	Name          string      `json:"name"`
	Subjects      []Subject   `json:"subjects"`
	AllowAllTools bool        `json:"allowAllTools,omitempty"`
	AllowedTools  VMCPToolSet `json:"allowedTools,omitempty"`
}

// VMCPToolSet maps component IDs to original upstream tool names.
type VMCPToolSet map[string][]string

func ToolSetFromReferences(refs []VMCPToolReference) VMCPToolSet {
	if refs == nil {
		return nil
	}
	result := VMCPToolSet{}
	for _, ref := range refs {
		result[ref.ComponentID] = append(result[ref.ComponentID], ref.Name)
	}
	return result
}

func (s VMCPToolSet) References() []VMCPToolReference {
	if s == nil {
		return nil
	}
	refs := []VMCPToolReference{}
	for componentID, names := range s {
		for _, name := range names {
			refs = append(refs, VMCPToolReference{ComponentID: componentID, Name: name})
		}
	}
	return refs
}

// VMCPToolReference identifies an upstream tool independently of display names,
// prefixes, and tool-name overrides.
type VMCPToolReference struct {
	ComponentID string `json:"componentID"`
	Name        string `json:"name"`
}

func (r VMCPToolReference) Validate() error {
	if r.ComponentID == "" || r.Name == "" {
		return fmt.Errorf("tool reference requires componentID and original tool name")
	}
	return nil
}

// ValidateToolReference checks component ownership and explicit component restrictions.
// A snapshot without overrides need not contain a complete, current tool preview.
func (m VMCPManifest) ValidateToolReference(ref VMCPToolReference) error {
	if err := ref.Validate(); err != nil {
		return err
	}
	for _, component := range m.Components {
		if component.ID != ref.ComponentID {
			continue
		}
		if ref.Name == "*" || len(component.ToolOverrides) == 0 {
			return nil
		}
		for _, tool := range component.ToolOverrides {
			if tool.Name == ref.Name && tool.Enabled {
				return nil
			}
		}
		return fmt.Errorf("tool %q is not enabled on component %q", ref.Name, ref.ComponentID)
	}
	return fmt.Errorf("unknown tool component %q", ref.ComponentID)
}

func (m VMCPManifest) ValidateToolSet(tools VMCPToolSet) error {
	for componentID, names := range tools {
		if componentID == "" || !slices.ContainsFunc(m.Components, func(component VMCPComponent) bool { return component.ID == componentID }) {
			return fmt.Errorf("unknown tool component %q", componentID)
		}
		for _, name := range names {
			if err := m.ValidateToolReference(VMCPToolReference{ComponentID: componentID, Name: name}); err != nil {
				return err
			}
		}
	}
	return nil
}

type VMCPStatus struct {
	Ready      bool                  `json:"ready,omitempty"`
	Components []VMCPComponentStatus `json:"components,omitempty"`
}

type VMCPComponentStatus struct {
	Name          string `json:"name"`
	Ready         bool   `json:"ready,omitempty"`
	Error         string `json:"error,omitempty"`
	SourceMissing bool   `json:"sourceMissing,omitempty"`
	NeedsUpdate   bool   `json:"needsUpdate,omitempty"`
}

type VMCPList List[VMCP]

type VMCPInstance struct {
	LegacySlug           string `json:"legacySlug,omitempty"`
	Metadata             `json:",inline"`
	VMCPInstanceManifest `json:",inline"`
	UserID               string             `json:"userID"`
	Status               VMCPInstanceStatus `json:"status,omitempty"`
}

// VMCPConfiguration groups configuration values by VMCP component ID.
type VMCPConfiguration struct {
	Components map[string]map[string]string `json:"components"`
}

type VMCPInstanceManifest struct {
	VMCPID string `json:"vmcpID"`
	// Nil follows the current grant; an empty map explicitly selects no tools.
	EnabledTools VMCPToolSet `json:"enabledTools"`
}

type VMCPInstanceStatus struct {
	Configured                   bool     `json:"configured,omitempty"`
	MissingRequiredConfiguration []string `json:"missingRequiredConfiguration,omitempty"`
	UserConfigurationHash        string   `json:"userConfigurationHash,omitempty"`
}

type VMCPInstanceList List[VMCPInstance]

// Default fills secure defaults that are omitted by clients.
func (m *VMCPManifest) Default(personalServer bool, userID string) {
	m.DefaultConfigurationPolicies()

	if personalServer {
		m.Profiles = nil
	} else if m.Profiles == nil {
		m.Profiles = []VMCPProfile{{
			Name:          "me",
			Subjects:      []Subject{{Type: SubjectTypeUser, ID: userID}},
			AllowAllTools: true,
		}}
	}
}

func (m *VMCPManifest) DefaultConfigurationPolicies() {
	for componentIndex := range m.Components {
		for policyIndex := range m.Components[componentIndex].Configuration {
			policy := &m.Components[componentIndex].Configuration[policyIndex]
			if policy.Policy == "" {
				policy.Policy = VMCPConfigurationPolicyProhibited
			}
		}
	}
}

func (m VMCPManifest) Validate() error {
	if m.DisplayName == "" {
		return fmt.Errorf("displayName is required")
	}

	componentNames := make(map[string]struct{}, len(m.Components))
	componentIDs := make(map[string]struct{}, len(m.Components))
	for _, component := range m.Components {
		if component.Name == "" {
			return fmt.Errorf("component name is required")
		}
		if strings.Contains(component.ID, ".") {
			return fmt.Errorf("component ID %q cannot contain a period", component.ID)
		}
		if component.ID != "" {
			if _, ok := componentIDs[component.ID]; ok {
				return fmt.Errorf("duplicate component ID %q", component.ID)
			}
			componentIDs[component.ID] = struct{}{}
		}
		if _, ok := componentNames[component.Name]; ok {
			return fmt.Errorf("duplicate component name %q", component.Name)
		}
		componentNames[component.Name] = struct{}{}
		if component.MCPServerCatalogEntryID == "" {
			return fmt.Errorf("component %q mcpServerCatalogEntryID is required", component.Name)
		}

		configurationKeys := make(map[string]VMCPConfigurationPolicyType, len(component.Configuration))
		for _, policy := range component.Configuration {
			if policy.Key == "" {
				return fmt.Errorf("component %q configuration key is required", component.Name)
			}
			if _, ok := configurationKeys[policy.Key]; ok {
				return fmt.Errorf("component %q has duplicate configuration key %q", component.Name, policy.Key)
			}
			configurationKeys[policy.Key] = policy.Policy
			switch policy.Policy {
			case "", VMCPConfigurationPolicyProhibited, VMCPConfigurationPolicyFixed, VMCPConfigurationPolicyUserAllowed:
			default:
				return fmt.Errorf("component %q configuration %q has invalid policy %q", component.Name, policy.Key, policy.Policy)
			}
			if policy.Policy != VMCPConfigurationPolicyFixed && policy.Value != "" {
				return fmt.Errorf("component %q configuration %q may only set value with fixed policy", component.Name, policy.Key)
			}
		}
		for _, config := range component.CatalogEntry.Manifest.Config {
			if config.Required && config.Value == "" && config.SecretBinding == nil {
				if policy := configurationKeys[config.Key]; policy == "" || policy == VMCPConfigurationPolicyProhibited {
					return fmt.Errorf("component %q required configuration %q cannot be prohibited", component.Name, config.Key)
				}
			}
		}
	}

	profileNames := make(map[string]struct{}, len(m.Profiles))
	for _, profile := range m.Profiles {
		if err := m.ValidateToolSet(profile.AllowedTools); err != nil {
			return fmt.Errorf("profile %q: %w", profile.Name, err)
		}
		if profile.Name == "" {
			return fmt.Errorf("profile name is required")
		}
		if _, ok := profileNames[profile.Name]; ok {
			return fmt.Errorf("duplicate profile name %q", profile.Name)
		}
		profileNames[profile.Name] = struct{}{}
		if len(profile.Subjects) == 0 {
			return fmt.Errorf("profile %q must have at least one subject", profile.Name)
		}
		for _, subject := range profile.Subjects {
			if err := subject.Validate(); err != nil {
				return fmt.Errorf("profile %q has invalid subject: %w", profile.Name, err)
			}
		}
	}

	return nil
}

func (m VMCPInstanceManifest) Validate() error {
	if m.VMCPID == "" {
		return fmt.Errorf("vmcpID is required")
	}
	for _, tool := range m.EnabledTools.References() {
		if err := tool.Validate(); err != nil {
			return err
		}
	}
	return nil
}
