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
	// PreconfiguredOAuth records the OAuth grants that administrators preconfigure for the vMCP's remote components,
	// one per component, which everyone who reaches the component through the vMCP uses (pkg/vmcp
	// PreconfiguredOAuthStore). The grants are in the gateway database, never here. Each grant is recorded before it is
	// stored, so the vMCP controller deletes every one it may have stored when its component leaves the manifest, and
	// the store deletes exactly the recorded ones when an administrator revokes it.
	PreconfiguredOAuth []VMCPPreconfiguredOAuthGrant `json:"preconfiguredOAuth,omitempty"`
	// ComponentOAuth records, by component ID, whether the remote components of a shared vMCP ask for OAuth, as the
	// vMCP controller last found by probing them without any credential. Agent readiness and the preconfigured OAuth
	// state read it, and never probe. It never holds a URL.
	ComponentOAuth map[string]VMCPComponentOAuth `json:"componentOAuth,omitempty"`
}

// VMCPComponentOAuthRequirement is what a probe of a component found: that it asks for OAuth
// (VMCPComponentOAuthRequired), that it does not (VMCPComponentOAuthNotRequired), or, when the probe failed, that it is
// unknown (VMCPComponentOAuthUnknown). Unknown counts as not required, so a server that cannot be reached never keeps
// an agent from connecting.
type VMCPComponentOAuthRequirement string

// VMCPComponentOAuth records the probe of one component, for its current target.
type VMCPComponentOAuth struct {
	// TargetHash identifies what the probe was made for (vmcp.ComponentOAuthTargetHash): the component's definition
	// and the values fixed for everyone that its URL is built from. A change to it discards the probe, and the
	// component is probed again.
	TargetHash string `json:"targetHash"`
	// Required is what the last probe found, or, when it failed, what the last one that succeeded found, if any.
	Required VMCPComponentOAuthRequirement `json:"required"`
	// CheckedAt is when the component was last probed, whether the probe succeeded or not.
	CheckedAt metav1.Time `json:"checkedAt"`
	// Failures counts the probes in a row that failed. The next probe waits longer after each.
	Failures int `json:"failures,omitempty"`
}

// VMCPPreconfiguredOAuthGrant records the preconfigured OAuth grants that may be stored for one component of a vMCP.
type VMCPPreconfiguredOAuthGrant struct {
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
