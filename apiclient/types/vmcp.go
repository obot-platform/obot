package types

import (
	"fmt"
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
	UserID                  string     `json:"userID,omitempty"`
	StaticConfigurationHash string     `json:"staticConfigurationHash,omitempty"`
	Status                  VMCPStatus `json:"status,omitempty"`
}

// VMCPManifest contains the user-managed portion of a VMCP.
type VMCPManifest struct {
	DisplayName     string          `json:"displayName"`
	Description     string          `json:"description,omitempty"`
	Icon            string          `json:"icon,omitempty"`
	Components      []VMCPComponent `json:"components"`
	Profiles        []VMCPProfile   `json:"profiles,omitempty"`
	ForceSingleUser bool            `json:"forceSingleUser,omitempty"`
}

// VMCPComponent is a snapshot of one catalog entry and the policy applied to it.
// Runtime resolution uses CatalogEntry rather than resolving the source live.
type VMCPComponent struct {
	// ID is the immutable, server-assigned identity used to scope component configuration.
	ID                      string                        `json:"id,omitempty"`
	Name                    string                        `json:"name"`
	MCPCatalogID            string                        `json:"mcpCatalogID"`
	MCPServerCatalogEntryID string                        `json:"mcpServerCatalogEntryID"`
	CatalogEntry            MCPServerCatalogEntrySnapshot `json:"catalogEntry"`
	SourceDigest            string                        `json:"sourceDigest,omitempty"`
	Configuration           []VMCPConfigurationPolicy     `json:"configuration,omitempty"`
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
	Name          string    `json:"name"`
	Subjects      []Subject `json:"subjects"`
	AllowAllTools bool      `json:"allowAllTools,omitempty"`
	AllowedTools  []string  `json:"allowedTools,omitempty"`
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
	VMCPID       string   `json:"vmcpID"`
	EnabledTools []string `json:"enabledTools,omitempty"`
}

type VMCPInstanceStatus struct {
	Configured                   bool     `json:"configured,omitempty"`
	MissingRequiredConfiguration []string `json:"missingRequiredConfiguration,omitempty"`
	UserConfigurationHash        string   `json:"userConfigurationHash,omitempty"`
}

type VMCPInstanceList List[VMCPInstance]

// Default fills secure defaults that are omitted by clients.
func (m *VMCPManifest) Default() {
	m.DefaultConfigurationPolicies()

	if m.Profiles == nil {
		m.Profiles = []VMCPProfile{{
			Name:          "default",
			Subjects:      []Subject{{Type: SubjectTypeSelector, ID: "*"}},
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
		if component.MCPCatalogID == "" {
			return fmt.Errorf("component %q mcpCatalogID is required", component.Name)
		}
		if component.MCPServerCatalogEntryID == "" {
			return fmt.Errorf("component %q mcpServerCatalogEntryID is required", component.Name)
		}

		configurationKeys := make(map[string]struct{}, len(component.Configuration))
		for _, policy := range component.Configuration {
			if policy.Key == "" {
				return fmt.Errorf("component %q configuration key is required", component.Name)
			}
			if _, ok := configurationKeys[policy.Key]; ok {
				return fmt.Errorf("component %q has duplicate configuration key %q", component.Name, policy.Key)
			}
			configurationKeys[policy.Key] = struct{}{}
			switch policy.Policy {
			case "", VMCPConfigurationPolicyProhibited, VMCPConfigurationPolicyFixed, VMCPConfigurationPolicyUserAllowed:
			default:
				return fmt.Errorf("component %q configuration %q has invalid policy %q", component.Name, policy.Key, policy.Policy)
			}
			if policy.Policy != VMCPConfigurationPolicyFixed && policy.Value != "" {
				return fmt.Errorf("component %q configuration %q may only set value with fixed policy", component.Name, policy.Key)
			}
		}
	}

	profileNames := make(map[string]struct{}, len(m.Profiles))
	for _, profile := range m.Profiles {
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
	return nil
}
