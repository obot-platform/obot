package types

import (
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"
)

const (
	// VMCPProfileIDPrefix starts every profile ID that Obot assigns, whether random (vMCPs managed through the API) or
	// derived from the vMCP and profile names (catalog-synced and migrated vMCPs). IDs set in catalog YAML need not
	// use it.
	VMCPProfileIDPrefix = "vp1"

	VMCPConfigurationPolicyProhibited  VMCPConfigurationPolicyType = "prohibited"
	VMCPConfigurationPolicyFixed       VMCPConfigurationPolicyType = "fixed"
	VMCPConfigurationPolicyUserAllowed VMCPConfigurationPolicyType = "userAllowed"
)

var (
	// vmcpProfileIDPattern is a DNS label, so that a profile ID is safe in URL paths, credential contexts and object
	// names.
	vmcpProfileIDPattern = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]{0,61}[a-z0-9])?$`)
)

// VMCP is a stable, optionally multi-component MCP endpoint definition.
type VMCP struct {
	Metadata     `json:",inline"`
	VMCPManifest `json:",inline"`
	// LocalhostCallbackPaths contains the effective callback paths for the requesting user.
	LocalhostCallbackPaths  []string   `json:"localhostCallbackPaths"`
	LegacySlug              string     `json:"legacySlug,omitempty"`
	SourceURL               string     `json:"sourceURL,omitempty"`
	Adopted                 *bool      `json:"adopted,omitempty"`
	UserID                  string     `json:"userID,omitempty"`
	CreatorUserID           string     `json:"creatorUserID,omitempty"`
	StaticConfigurationHash string     `json:"staticConfigurationHash,omitempty"`
	Status                  VMCPStatus `json:"status,omitempty"`
}

// VMCPManifest contains the user-managed portion of a VMCP.
type VMCPManifest struct {
	DisplayName string          `json:"displayName"`
	Description string          `json:"description,omitempty"`
	Icon        string          `json:"icon,omitempty"`
	ToolSearch  bool            `json:"toolSearch,omitempty"`
	Components  []VMCPComponent `json:"components"`
	Profiles    []VMCPProfile   `json:"profiles,omitempty"`
}

// VMCPComponent is a snapshot of one catalog entry and the policy applied to it.
// Runtime resolution uses CatalogEntry rather than resolving the source live.
// The API populates MCPCatalogID, CatalogEntry, and SourceDigest from the entry ID.
type VMCPComponent struct {
	// ID is the immutable identity used to scope component configuration. The API
	// assigns it; catalog manifests may declare it for profile references.
	ID                      string                        `json:"id,omitempty"`
	Name                    string                        `json:"name"`
	MCPCatalogID            string                        `json:"mcpCatalogID"`
	MCPServerCatalogEntryID string                        `json:"mcpServerCatalogEntryID"`
	CatalogEntry            MCPServerCatalogEntrySnapshot `json:"catalogEntry"`
	SourceDigest            string                        `json:"sourceDigest,omitempty"`
	Configuration           []VMCPConfigurationPolicy     `json:"configuration,omitempty"`
	ForceSingleUser         bool                          `json:"forceSingleUser,omitempty"`
	OAuthCredentialID       string                        `json:"oauthCredentialID,omitempty"`
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
	// SecretBinding sources fixed configuration from a Kubernetes Secret when
	// the component server launches, so the value is never stored by Obot.
	SecretBinding *MCPSecretBinding `json:"secretBinding,omitempty"`
}

type VMCPConfigurationPolicyType string

// VMCPProfile grants access and tools to matching users and groups. Profiles
// are additive. AllowAllComponents grants all tools enabled on every component
// otherwise only the specified components and their allowed tools are granted.
type VMCPProfile struct {
	// ID is the profile's immutable identity, which audit records name the profile by. The API
	// assigns it: a request keeps a profile's ID by sending it back, and a profile sent without one is a new profile,
	// even if it has the name of a profile being removed. Catalog YAML may set it; otherwise catalog sync derives it
	// from the vMCP and profile names, so renaming a profile in YAML without an ID makes it a new profile.
	ID          string                 `json:"id,omitempty"`
	Name        string                 `json:"name"`
	Subjects    []Subject              `json:"subjects"`
	Permissions VMCPProfilePermissions `json:"vmcpPermissions,omitempty"`
}

type VMCPProfilePermissions struct {
	AllowAllComponents bool                        `json:"allowAllComponents,omitempty"`
	AllowedComponents  map[string]VMCPComponentSet `json:"allowedComponents,omitempty"`
}

type VMCPComponentSet struct {
	// AllowedTools specifies the tools available for a component; nil means all
	// tools, while an empty slice grants no tools.
	// +optional
	AllowedTools []string `json:"allowedTools"`
}

func ComponentsFromToolReferences(refs []VMCPToolReference) map[string]VMCPComponentSet {
	if refs == nil {
		return nil
	}
	result := map[string]VMCPComponentSet{}
	for _, ref := range refs {
		component, exists := result[ref.ComponentID]
		if ref.Name == "*" {
			component.AllowedTools = nil
		} else if !exists || component.AllowedTools != nil {
			component.AllowedTools = append(component.AllowedTools, ref.Name)
		}
		result[ref.ComponentID] = component
	}
	return result
}

func ComponentToolReferences(components map[string]VMCPComponentSet) []VMCPToolReference {
	if components == nil {
		return nil
	}
	refs := []VMCPToolReference{}
	for componentID, component := range components {
		if component.AllowedTools == nil {
			refs = append(refs, VMCPToolReference{ComponentID: componentID, Name: "*"})
		}
		for _, name := range component.AllowedTools {
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

func (m VMCPManifest) ValidateComponents(components map[string]VMCPComponentSet) error {
	for componentID, component := range components {
		if componentID == "" || !slices.ContainsFunc(m.Components, func(component VMCPComponent) bool { return component.ID == componentID }) {
			return fmt.Errorf("unknown tool component %q", componentID)
		}
		for _, name := range component.AllowedTools {
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
	// ConnectionSnapshot is an outdated snapshot that migrated connections retain when the
	// component itself is current. Updating the vMCP releases it.
	ConnectionSnapshot *MCPServerCatalogEntrySnapshot `json:"connectionSnapshot,omitempty"`
}

type VMCPList List[VMCP]

// OrphanedVMCPCatalogItem is the catalog YAML that lets catalog sync take over
// an orphaned vMCP: one migrated from a catalog-synced composite entry that no
// catalog source defines yet.
type OrphanedVMCPCatalogItem struct {
	VMCPID      string `json:"vmcpID"`
	DisplayName string `json:"displayName"`
	EntryKey    string `json:"entryKey"`
	// YAML is a single catalog item. It is empty when Error is set.
	YAML string `json:"yaml,omitempty"`
	// Error explains why the vMCP cannot be expressed as catalog YAML.
	Error string `json:"error,omitempty"`
}

type OrphanedVMCPCatalogItemList List[OrphanedVMCPCatalogItem]

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

// VMCPConfigurationPresence lists fields with stored fixed values for one component.
type VMCPConfigurationPresence struct {
	// +listType=set
	ConfiguredFields []string `json:"configuredFields"`
}

const (
	// VMCPPreconfiguredOAuthNotRequired is the state of a remote component that the vMCP holds no preconfigured OAuth
	// for, and that is not known to need OAuth: its catalog entry does not require static OAuth, and Obot's background
	// probe, which connects without credentials, has not found that it asks for OAuth. An administrator can still
	// authorize it: a probe that failed, or has not run yet, counts as not required.
	VMCPPreconfiguredOAuthNotRequired = "not_required"
	// VMCPPreconfiguredOAuthNeedsAuthorization is the state of a remote component that the vMCP holds no preconfigured
	// OAuth for, and that is known to need OAuth. Users authorize it with their own accounts, and agents cannot use it,
	// until an administrator authorizes it.
	VMCPPreconfiguredOAuthNeedsAuthorization = "needs_authorization"
	// VMCPPreconfiguredOAuthAuthorized is the state of a vMCP's preconfigured OAuth for a component that works.
	VMCPPreconfiguredOAuthAuthorized = "authorized"
	// VMCPPreconfiguredOAuthInvalid is the state of a vMCP's preconfigured OAuth for a component that no longer works:
	// the provider refused to refresh it, or the component's upstream URL changed since it was authorized. An
	// administrator must authorize it again, or revoke it so users authorize the component themselves.
	VMCPPreconfiguredOAuthInvalid = "invalid"
)

// VMCPPreconfiguredOAuth is the OAuth an administrator preconfigured for one remote component of a shared vMCP: one
// account, which everyone who reaches the component through the vMCP uses upstream, users and agents alike. While it
// is set, users are not asked to authorize the component themselves, and their own tokens for it are not used. It
// describes the grant, never its tokens.
type VMCPPreconfiguredOAuth struct {
	VMCPID      string `json:"vmcpID"`
	ComponentID string `json:"componentID"`
	// State is VMCPPreconfiguredOAuthNotRequired, VMCPPreconfiguredOAuthNeedsAuthorization,
	// VMCPPreconfiguredOAuthAuthorized or VMCPPreconfiguredOAuthInvalid.
	State string `json:"state"`
	// AuthorizedBy is the administrator who authorized the grant, as a user ID.
	AuthorizedBy string `json:"authorizedBy,omitempty"`
	// AuthorizedAt is when the grant was authorized.
	AuthorizedAt *Time `json:"authorizedAt,omitempty"`
	// Target is the upstream URL the grant was authorized for, for a component whose catalog entry fixes the URL: that
	// URL is catalog data. The grant is used only for that URL. It is empty when the URL comes from the vMCP's fixed
	// configuration, which is never returned: see TargetHostname.
	Target string `json:"target,omitempty"`
	// TargetHostname is, for a component whose URL comes from the vMCP's fixed configuration (a hostname-constrained
	// URL or a URL template), only the hostname of the URL the grant was authorized for, never its port, path, query or
	// userinfo. It is not a URL.
	TargetHostname string `json:"targetHostname,omitempty"`
	// Scopes are the scopes the grant was authorized with.
	Scopes []string `json:"scopes,omitempty"`
	// InvalidReason says why an invalid grant stopped working.
	InvalidReason string `json:"invalidReason,omitempty"`
}

// VMCPPreconfiguredOAuthAuthorization starts an administrator's authorization of a vMCP's preconfigured OAuth for one
// component. The administrator opens AuthURL; the provider's callback stores the grant on the vMCP.
type VMCPPreconfiguredOAuthAuthorization struct {
	AuthURL string `json:"authURL"`
	// LocalhostCallback reports that the component's provider only redirects to a localhost callback, so AuthURL is
	// Obot's local login URL rather than the provider's: the administrator runs `obot mcp login <AuthURL>` on their own
	// machine, which listens for the callback and relays it to Obot in their browser, as for a user's own connection.
	// Only the authorization needs the relay; users and agents use and refresh the grant without it.
	LocalhostCallback bool `json:"localhostCallback,omitempty"`
}

type VMCPInstanceManifest struct {
	VMCPID string `json:"vmcpID"`
	// Nil follows the current grant; an empty map explicitly selects no tools.
	ComponentSet map[string]VMCPComponentSet `json:"componentSet"`
}

type VMCPInstanceStatus struct {
	Configured                   bool     `json:"configured,omitempty"`
	MissingRequiredConfiguration []string `json:"missingRequiredConfiguration,omitempty"`
	UserConfigurationHash        string   `json:"userConfigurationHash,omitempty"`
	ConfigurationCheckHash       string   `json:"configurationCheckHash,omitempty"`
}

type VMCPInstanceList List[VMCPInstance]

// Default fills secure defaults that are omitted by clients.
func (m *VMCPManifest) Default(personalServer bool) {
	m.DefaultConfigurationPolicies()

	if personalServer {
		m.Profiles = nil
	} else if m.Profiles == nil {
		m.Profiles = []VMCPProfile{{
			Name:        "default",
			Subjects:    []Subject{{Type: SubjectTypeObotGroup, ID: GroupAdmin}},
			Permissions: VMCPProfilePermissions{AllowAllComponents: true},
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

func validateConfigurationSecretBinding(component VMCPComponent, policy VMCPConfigurationPolicy) error {
	binding := policy.SecretBinding
	if binding == nil {
		return nil
	}
	if policy.Policy != VMCPConfigurationPolicyFixed {
		return fmt.Errorf("component %q configuration %q may only set secretBinding with fixed policy", component.Name, policy.Key)
	}
	if policy.Value != "" {
		return fmt.Errorf("component %q configuration %q secretBinding and value are mutually exclusive", component.Name, policy.Key)
	}
	if binding.Name == "" || binding.Key == "" {
		return fmt.Errorf("component %q configuration %q secretBinding requires both name and key", component.Name, policy.Key)
	}
	if !slices.ContainsFunc(component.CatalogEntry.Manifest.Config, func(config MCPConfig) bool { return config.Key == policy.Key }) {
		return fmt.Errorf("component %q configuration %q secretBinding does not match a catalog entry configuration field", component.Name, policy.Key)
	}
	return nil
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
			if err := validateConfigurationSecretBinding(component, policy); err != nil {
				return err
			}
		}
		for _, config := range component.CatalogEntry.Manifest.Config {
			if config.Required && !config.Static && config.SecretBinding == nil {
				if policy := configurationKeys[config.Key]; policy == "" || policy == VMCPConfigurationPolicyProhibited {
					return fmt.Errorf("component %q required configuration %q cannot be prohibited", component.Name, config.Key)
				}
			}
		}
	}

	profileNames := make(map[string]struct{}, len(m.Profiles))
	profileIDs := make(map[string]struct{}, len(m.Profiles))
	for _, profile := range m.Profiles {
		if err := m.ValidateComponents(profile.Permissions.AllowedComponents); err != nil {
			return fmt.Errorf("profile %q: %w", profile.Name, err)
		}
		if profile.Name == "" {
			return fmt.Errorf("profile name is required")
		}
		if _, ok := profileNames[profile.Name]; ok {
			return fmt.Errorf("duplicate profile name %q", profile.Name)
		}
		profileNames[profile.Name] = struct{}{}
		// The ID is assigned after validation on some paths, so a profile without one is accepted here.
		if profile.ID != "" {
			if err := ValidateVMCPProfileID(profile.ID); err != nil {
				return fmt.Errorf("profile %q: %w", profile.Name, err)
			}
			if _, ok := profileIDs[profile.ID]; ok {
				return fmt.Errorf("duplicate profile ID %q", profile.ID)
			}
			profileIDs[profile.ID] = struct{}{}
		}
		if len(profile.Subjects) == 0 {
			return fmt.Errorf("profile %q must have at least one subject", profile.Name)
		}
		for _, subject := range profile.Subjects {
			if err := subject.ValidateFor(PolicySystemVMCPProfile); err != nil {
				return fmt.Errorf("profile %q has invalid subject: %w", profile.Name, err)
			}
		}
	}

	return nil
}

// ValidateVMCPProfileID checks the format of a profile ID: a DNS label of lowercase letters, digits and hyphens, at
// most 63 characters long, so that it is safe in URL paths, credential contexts and object names.
func ValidateVMCPProfileID(id string) error {
	if !vmcpProfileIDPattern.MatchString(id) {
		return fmt.Errorf("invalid profile ID %q: use 1-63 lowercase letters, digits and hyphens, starting and ending with a letter or digit", id)
	}
	return nil
}

// Profile returns the profile with id, and whether the manifest has one. An empty id names no profile.
func (m VMCPManifest) Profile(id string) (VMCPProfile, bool) {
	index := slices.IndexFunc(m.Profiles, func(profile VMCPProfile) bool { return profile.ID == id })
	if id == "" || index < 0 {
		return VMCPProfile{}, false
	}
	return m.Profiles[index], true
}

func (m VMCPInstanceManifest) Validate() error {
	if m.VMCPID == "" {
		return fmt.Errorf("vmcpID is required")
	}
	for componentID := range m.ComponentSet {
		if componentID == "" {
			return fmt.Errorf("component ID is required")
		}
	}
	for _, tool := range ComponentToolReferences(m.ComponentSet) {
		if err := tool.Validate(); err != nil {
			return err
		}
	}
	return nil
}
