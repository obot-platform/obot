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
	LegacySlug string             `json:"legacySlug,omitempty"`
	Manifest   types.VMCPManifest `json:"manifest"`
	// UserID is set for a personal VMCP and empty for an administrator-created shared VMCP.
	UserID                  string `json:"userID,omitempty"`
	StaticConfigurationHash string `json:"staticConfigurationHash,omitempty"`
	// ComponentStaticConfigurationHashes retire migrated overrides only for the changed component.
	ComponentStaticConfigurationHashes map[string]string `json:"componentStaticConfigurationHashes,omitempty"`
}

type VMCPStatus struct {
	Ready      bool                  `json:"ready,omitempty"`
	Components []VMCPComponentStatus `json:"components,omitempty"`
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
