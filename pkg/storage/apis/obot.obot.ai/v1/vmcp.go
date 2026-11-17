package v1

import (
	"slices"

	"github.com/obot-platform/nah/pkg/fields"
	"github.com/obot-platform/obot/apiclient/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// VMCPSnapshotDigestAnnotation records the snapshot applied to a component server.
	VMCPSnapshotDigestAnnotation = "obot.ai/vmcp-snapshot-digest"
	// OAuthCredentialRevisionAnnotation persists after reconciliation so every consumer observes writes.
	OAuthCredentialRevisionAnnotation = "obot.ai/oauth-credential-revision"

	VMCPComponentOAuthRequired    VMCPComponentOAuthRequirement = "True"
	VMCPComponentOAuthNotRequired VMCPComponentOAuthRequirement = "False"
	VMCPComponentOAuthUnknown     VMCPComponentOAuthRequirement = "Unknown"
)

var (
	_ fields.Fields = (*VMCP)(nil)
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type VMCP struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`

	Spec   VMCPSpec   `json:"spec"`
	Status VMCPStatus `json:"status"`
}

type VMCPSpec struct {
	SourceURL  string             `json:"sourceURL,omitempty"`
	LegacySlug string             `json:"legacySlug,omitempty"`
	Manifest   types.VMCPManifest `json:"manifest"`
	// Adopted is nil when adoption is unsupported, false when awaiting catalog
	// adoption, and true after catalog sync has taken over the vMCP.
	Adopted *bool `json:"adopted,omitempty"`
	// AdoptionSourceURL and AdoptionEntryKey are recorded by composite migration.
	// They identify the catalog source and key a vMCP must be synced from for
	// catalog sync to adopt it without changing its identity.
	AdoptionSourceURL string `json:"adoptionSourceURL,omitempty"`
	AdoptionEntryKey  string `json:"adoptionEntryKey,omitempty"`
	// UserID is set for a personal VMCP and empty for an administrator-created shared VMCP.
	UserID string `json:"userID,omitempty"`
	// CreatorUserID is the user ID of the user who created the VMCP. It is used to determine which admin created a server.
	CreatorUserID           string `json:"creatorUserID,omitempty"`
	StaticConfigurationHash string `json:"staticConfigurationHash,omitempty"`
	// StaticConfigurationCredentialName selects an immutable configuration revision.
	// Empty selects the legacy configuration credential.
	StaticConfigurationCredentialName string `json:"staticConfigurationCredentialName,omitempty"`
	// ComponentStaticConfigurationHashes retire migrated overrides only for the changed component.
	ComponentStaticConfigurationHashes map[string]string `json:"componentStaticConfigurationHashes,omitempty"`
	// ComponentCatalogReferences maps component IDs to normalized sourceID::entryKey references.
	ComponentCatalogReferences map[string]string `json:"componentCatalogReferences,omitempty"`
}

type VMCPStatus struct {
	Ready      bool                  `json:"ready,omitempty"`
	Components []VMCPComponentStatus `json:"components,omitempty"`
	// ProfileIDs are the profile IDs the vMCP controller last observed in the manifest. A profile whose ID is here but
	// no longer in the manifest was deleted, so the controller deletes the data stored outside the manifest for it
	// (vmcp.ProfileDataStore) and then records the manifest's IDs. A handler that stores such data for a profile must
	// first make sure its ID is recorded here, so that a deletion the controller has not yet observed still cleans it up.
	ProfileIDs []string `json:"profileIDs,omitempty"`
	// ProfileConfigurationRevisions record, by profile ID, the configuration that administrators set on agent-capable
	// profiles for agents (pkg/vmcp profile configuration). The values are in credentials, never here. A profile
	// without configuration has no entry, unless it has superseded revisions left to delete.
	ProfileConfigurationRevisions map[string]VMCPProfileConfigurationRevision `json:"profileConfigurationRevisions,omitempty"`
	// ProfileOAuthGrants record the OAuth grants that administrators authorize on agent-capable profiles for agents,
	// by profile and component (pkg/vmcp profile OAuth grants). The grants are in the gateway database, never here.
	// Each grant is recorded before it is stored, so the vMCP controller deletes every one it may have stored when its
	// component leaves the manifest, and the store deletes exactly the recorded ones when an administrator resets it.
	ProfileOAuthGrants []VMCPProfileOAuthGrant `json:"profileOAuthGrants,omitempty"`
	// ComponentOAuth records, by component ID, whether the remote components that agents reach ask for OAuth, as the
	// vMCP controller last found by probing them without any credential. Agent readiness and profile status read it,
	// and never probe. It never holds a URL: each result is keyed by a digest of the URL it is for.
	ComponentOAuth map[string]VMCPComponentOAuth `json:"componentOAuth,omitempty"`
	// Conditions report states of the vMCP as a whole, such as types.VMCPConditionAgentSelectionAmbiguous.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// VMCPComponentOAuthRequirement is what a probe of a component found: that it asks for OAuth
// (VMCPComponentOAuthRequired), that it does not (VMCPComponentOAuthNotRequired), or, when the probe failed, that it is
// unknown (VMCPComponentOAuthUnknown). Unknown counts as not required, so a server that cannot be reached never keeps
// an agent from connecting.
type VMCPComponentOAuthRequirement string

// VMCPComponentOAuth records the probes of one component, for its current definition.
type VMCPComponentOAuth struct {
	// DefinitionHash is the component's definition (vmcp.ProfileOAuthGrantDefinitionHash) that the probes were made
	// for. A change to it discards them, and the component is probed again.
	DefinitionHash string `json:"definitionHash"`
	// Probes are the results for each URL that agents' connections to the component reach: the URL its definition
	// fixes, or each distinct URL that the configuration of its profiles resolves to.
	Probes []VMCPComponentOAuthProbe `json:"probes,omitempty"`
}

// VMCPComponentOAuthProbe is the result of probing one URL of a component.
type VMCPComponentOAuthProbe struct {
	// URLDigest is the digest of the URL probed (vmcp.ComponentOAuthURLDigest), never the URL, which may come from
	// configuration.
	URLDigest string `json:"urlDigest"`
	// Required is what the last probe found, or, when it failed, what the last one that succeeded found, if any.
	Required VMCPComponentOAuthRequirement `json:"required"`
	// CheckedAt is when the URL was last probed, whether the probe succeeded or not.
	CheckedAt metav1.Time `json:"checkedAt"`
	// Failures counts the probes in a row that failed. The next probe waits longer after each.
	Failures int `json:"failures,omitempty"`
}

// VMCPProfileConfigurationRevision records the configuration of one vMCP profile. Each change to it is stored as a new
// credential, a revision, which is published here with the keys it sets, so the configuration an agent resolves to
// and the revision its values are copied from always agree.
type VMCPProfileConfigurationRevision struct {
	// Name names the credential of the current revision, in the profile's credential context. It is empty when the
	// profile has no configuration.
	Name string `json:"name,omitempty"`
	// Keys are the configuration keys the current revision sets, as vmcp.ConfigurationKey encodes them. They never
	// carry a value.
	Keys []string `json:"keys,omitempty"`
	// Superseded are earlier revisions. The vMCP controller deletes each once no agent's instance records that its
	// configuration was resolved from it.
	Superseded []string `json:"superseded,omitempty"`
}

// VMCPProfileOAuthGrant records the OAuth grants that may be stored for one component of one vMCP profile.
type VMCPProfileOAuthGrant struct {
	ProfileID   string `json:"profileID"`
	ComponentID string `json:"componentID"`
	// GrantIDs identify the grants that may be stored for the component: the one last published, and any being
	// written since. Only one is stored at a time, and it has one of these IDs.
	GrantIDs []string `json:"grantIDs,omitempty"`
}

type VMCPComponentStatus struct {
	ConfigurationCheckHash    string `json:"configurationCheckHash,omitempty"`
	ConfigurationError        string `json:"configurationError,omitempty"`
	OAuthCredentialCheckHash  string `json:"oauthCredentialCheckHash,omitempty"`
	OAuthCredentialConfigured bool   `json:"oauthCredentialConfigured,omitempty"`
	Name                      string `json:"name"`
	Ready                     bool   `json:"ready,omitempty"`
	Error                     string `json:"error,omitempty"`
	SourceMissing             bool   `json:"sourceMissing,omitempty"`
	NeedsUpdate               bool   `json:"needsUpdate,omitempty"`
	// ConnectionSnapshot is an outdated snapshot that migrated connections retain when the
	// component itself is current. Updating the vMCP releases it.
	ConnectionSnapshot *types.MCPServerCatalogEntrySnapshot `json:"connectionSnapshot,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type VMCPList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []VMCP `json:"items"`
}

func (in *VMCP) Has(field string) bool {
	return slices.Contains(in.FieldNames(), field)
}

func (in *VMCP) Get(field string) string {
	if in != nil {
		switch field {
		case "spec.legacySlug":
			return in.Spec.LegacySlug
		case "spec.userID":
			return in.Spec.UserID
		}
	}
	return ""
}

func (*VMCP) FieldNames() []string {
	return []string{"spec.userID", "spec.legacySlug"}
}

func (in *VMCP) IsPersonal() bool {
	return in.Spec.UserID != ""
}
