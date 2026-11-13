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

	// VMCPConditionAgentSelectionAmbiguous is true when more than one profile names every agent, through the agent
	// wildcard (agents:*) or the selector wildcard (subjects:*). Such a vMCP predates the rule that allows at most one,
	// and keeps working for users, but an agent cannot be given one wildcard profile's configuration and OAuth grants.
	// So until an administrator leaves one such profile, agent selection fails closed: an agent without its own
	// profile cannot connect, and an agent with its own profile cannot connect while a component it is granted needs
	// setup from the fallback (selection_ambiguous).
	VMCPConditionAgentSelectionAmbiguous = "AgentSelectionAmbiguous"

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
	// ID is the profile's immutable identity, which profile configuration and OAuth grants are stored under. The API
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
	// Conditions report states of the vMCP as a whole, such as VMCPConditionAgentSelectionAmbiguous.
	Conditions []VMCPCondition `json:"conditions,omitempty"`
}

// VMCPCondition is one condition of a vMCP's status.
type VMCPCondition struct {
	Type string `json:"type"`
	// Status is True, False or Unknown.
	Status             string `json:"status"`
	Reason             string `json:"reason,omitempty"`
	Message            string `json:"message,omitempty"`
	LastTransitionTime Time   `json:"lastTransitionTime"`
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

// VMCPProfileConfigurationUpdate changes the configuration that a vMCP profile gives the agents it applies to, by
// component ID and then configuration key. A value replaces the key's value, and null (or a blank value) clears the
// key. Keys left out keep their values. Only keys a user would enter on their own connection (the userAllowed policy)
// can be set; fixed values always come from the vMCP.
type VMCPProfileConfigurationUpdate struct {
	Components map[string]map[string]*string `json:"components"`
}

// VMCPProfileStatus reports how far a vMCP profile is set up for agents. It describes the profile alone, without
// falling back to any other profile, and never carries a configuration value.
type VMCPProfileStatus struct {
	ProfileID string `json:"profileID"`
	// AgentCapable reports whether the profile can apply to agents (VMCPProfile.IsAgentCapable). Only such a profile
	// can be given configuration.
	AgentCapable bool `json:"agentCapable"`
	// Components cover every component of the vMCP, including those the profile does not grant: an agent may be
	// granted a component by another profile and still take its configuration from this one.
	Components []VMCPProfileComponentStatus `json:"components"`
	// PendingCollection reports that revisions of the profile's configuration that it no longer uses are still stored,
	// until no agent's instance uses them and the vMCP controller deletes them. Agents are no longer given their values,
	// but until then the profile counts as having data, even after a reset.
	PendingCollection bool `json:"pendingCollection,omitempty"`
}

// VMCPProfileComponentStatus is the setup of one component on a vMCP profile.
type VMCPProfileComponentStatus struct {
	ComponentID   string                         `json:"componentID"`
	Configuration VMCPProfileConfigurationStatus `json:"configuration"`
	// OAuth is the profile's OAuth grant for the component. Only a remote component can have one.
	OAuth *VMCPProfileOAuthStatus `json:"oauth,omitempty"`
}

const (
	// VMCPProfileOAuthAuthorized is the state of a profile's OAuth grant that agents can use.
	VMCPProfileOAuthAuthorized = "authorized"
	// VMCPProfileOAuthMissing is the state of a profile that holds no OAuth grant for a component.
	VMCPProfileOAuthMissing = "missing"
	// VMCPProfileOAuthInvalid is the state of a profile's OAuth grant that no longer works: the provider refused to
	// refresh it, or the component's target changed since it was authorized. An administrator must authorize it again.
	VMCPProfileOAuthInvalid = "invalid"
)

// VMCPProfileOAuthStatus is the OAuth grant a vMCP profile holds for one component, which every agent that resolves
// to the profile uses. It describes the grant, never its tokens.
type VMCPProfileOAuthStatus struct {
	// Required reports that the component is known to need OAuth: a remote server that requires administrator static
	// OAuth. Another remote component can still ask for OAuth when an agent calls it.
	Required bool `json:"required"`
	// State is VMCPProfileOAuthAuthorized, VMCPProfileOAuthMissing or VMCPProfileOAuthInvalid.
	State string `json:"state"`
	// AuthorizedBy is the administrator who authorized the grant, as a user ID.
	AuthorizedBy string `json:"authorizedBy,omitempty"`
	// AuthorizedAt is when the grant was authorized.
	AuthorizedAt *Time `json:"authorizedAt,omitempty"`
	// Target is the upstream URL the grant was authorized for. Agents use the grant only for that URL.
	Target string `json:"target,omitempty"`
	// Scopes are the scopes the grant was authorized with.
	Scopes []string `json:"scopes,omitempty"`
	// InvalidReason says why an invalid grant stopped working.
	InvalidReason string `json:"invalidReason,omitempty"`
}

// VMCPProfileOAuthAuthorization starts an administrator's authorization of a vMCP profile's OAuth grant for one
// component. The administrator opens AuthURL; the provider's callback stores the grant for the profile.
type VMCPProfileOAuthAuthorization struct {
	AuthURL string `json:"authURL"`
}

// VMCPProfileConfigurationStatus names the configuration keys of a component that a profile can set, by whether it
// sets them. It never carries their values.
type VMCPProfileConfigurationStatus struct {
	// Set are the keys the profile sets.
	Set []string `json:"set"`
	// MissingRequired are the required keys the profile does not set.
	MissingRequired []string `json:"missingRequired"`
	// OptionalUnset are the optional keys the profile does not set.
	OptionalUnset []string `json:"optionalUnset"`
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
	// agentProfiles maps each explicit agent ID to the profile that names it.
	agentProfiles := map[string]string{}
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
		// An agent's configuration and OAuth grants come from the one profile that names it, so no two profiles may
		// name the same agent. No stored vMCP predates agent subjects, so this has no legacy exception.
		for _, subject := range profile.Subjects {
			if subject.Type != SubjectTypeAgent || subject.ID == SubjectWildcardID {
				continue
			}
			if other, ok := agentProfiles[subject.ID]; ok && other != profile.Name {
				return fmt.Errorf("agent %q is named by profiles %q and %q; an agent may be named by at most one profile", subject.ID, other, profile.Name)
			}
			agentProfiles[subject.ID] = profile.Name
		}
		if len(profile.Subjects) == 0 {
			return fmt.Errorf("profile %q must have at least one subject", profile.Name)
		}
		for _, subject := range profile.Subjects {
			if err := subject.ValidateFor(PolicySystemVMCPProfile); err != nil {
				return fmt.Errorf("profile %q has invalid subject: %w", profile.Name, err)
			}
		}
		// Profiles have never required a wildcard to be their only subject, and existing ones may combine the selector
		// wildcard with other subjects, so only the rules among wildcards apply here. The rules across a vMCP's
		// profiles are separate.
		if err := validateWildcardSubjects(profile.Subjects); err != nil {
			return fmt.Errorf("profile %q has invalid subjects: %w", profile.Name, err)
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

// ValidateWildcardProfiles checks the wildcards across a vMCP's profiles: either at most one profile names everyone
// with the selector wildcard (subjects:*), or at most one names every user (users:*) and at most one every agent
// (agents:*), and not both forms. Each profile's own wildcards are checked by Validate.
//
// Validate does not apply this rule, because vMCPs stored before it may break it and must keep working for users.
// Callers apply it to new vMCPs and to changes that it does not exempt (see pkg/vmcp.CheckWildcardProfiles).
func (m VMCPManifest) ValidateWildcardProfiles() error {
	const mixedWildcardProfiles = "name everyone with the selector wildcard in one profile, or every user and every agent with their own wildcards, not both"

	var selector, users, agents []string
	for _, profile := range m.Profiles {
		hasSelector, hasUsers, hasAgents := wildcardKinds(profile.Subjects)
		if hasSelector {
			selector = append(selector, profile.Name)
		}
		if hasUsers {
			users = append(users, profile.Name)
		}
		if hasAgents {
			agents = append(agents, profile.Name)
		}
	}
	switch {
	case len(selector) > 0 && len(users) > 0:
		return fmt.Errorf("profile %q uses the selector wildcard (*) and profile %q the user wildcard: %s", selector[0], users[0], mixedWildcardProfiles)
	case len(selector) > 0 && len(agents) > 0:
		return fmt.Errorf("profile %q uses the selector wildcard (*) and profile %q the agent wildcard: %s", selector[0], agents[0], mixedWildcardProfiles)
	case len(selector) > 1:
		return fmt.Errorf("profiles %q and %q both use the selector wildcard (*); at most one profile may name everyone", selector[0], selector[1])
	case len(users) > 1:
		return fmt.Errorf("profiles %q and %q both use the user wildcard; at most one profile may name every user", users[0], users[1])
	case len(agents) > 1:
		return fmt.Errorf("profiles %q and %q both use the agent wildcard; at most one profile may name every agent", agents[0], agents[1])
	}
	return nil
}

// AgentWildcardProfiles returns the profiles that name every agent: those with the agent wildcard (agents:*) or the
// selector wildcard (subjects:*). An agent that no profile names explicitly falls back to such a profile, so when there
// is more than one, agent selection is ambiguous (VMCPConditionAgentSelectionAmbiguous) and must fail closed.
func (m VMCPManifest) AgentWildcardProfiles() []VMCPProfile {
	var profiles []VMCPProfile
	for _, profile := range m.Profiles {
		if slices.ContainsFunc(profile.Subjects, func(subject Subject) bool {
			return subject.IsWildcard() && (subject.Type == SubjectTypeSelector || subject.Type == SubjectTypeAgent)
		}) {
			profiles = append(profiles, profile)
		}
	}
	return profiles
}

// Profile returns the profile with id, and whether the manifest has one. An empty id names no profile.
func (m VMCPManifest) Profile(id string) (VMCPProfile, bool) {
	index := slices.IndexFunc(m.Profiles, func(profile VMCPProfile) bool { return profile.ID == id })
	if id == "" || index < 0 {
		return VMCPProfile{}, false
	}
	return m.Profiles[index], true
}

// IsAgentCapable reports whether the profile can apply to an agent: it names every agent (agents:*), everyone
// (subjects:*), or an agent by ID. Only agent-capable profiles may carry profile configuration and OAuth grants.
func (p VMCPProfile) IsAgentCapable() bool {
	return slices.ContainsFunc(p.Subjects, func(subject Subject) bool {
		return subject.Type == SubjectTypeAgent || subject.Type == SubjectTypeSelector && subject.ID == SubjectWildcardID
	})
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
