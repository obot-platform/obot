package v1

import (
	"slices"

	"github.com/obot-platform/nah/pkg/fields"
	"github.com/obot-platform/obot/apiclient/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	VMCPInstanceConfigurationSyncAnnotation = "obot.ai/vmcp-instance-configuration-hash"
)

var (
	_ fields.Fields = (*VMCPInstance)(nil)
	_ DeleteRefs    = (*VMCPInstance)(nil)
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type VMCPInstance struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`

	Spec   VMCPInstanceSpec   `json:"spec"`
	Status VMCPInstanceStatus `json:"status"`
}

type VMCPInstanceSpec struct {
	LegacySlug string `json:"legacySlug,omitempty"`
	// LegacyCreatedAt preserves canonical connection ordering after migration.
	LegacyCreatedAt *metav1.Time `json:"legacyCreatedAt,omitempty"`
	// LegacyComponents preserve per-connection snapshots and tool choices during migration.
	// They are not writable through the instance API.
	LegacyComponents         []types.VMCPComponent      `json:"legacyComponents,omitempty"`
	LegacyDisabledComponents []string                   `json:"legacyDisabledComponents,omitempty"`
	Manifest                 types.VMCPInstanceManifest `json:"manifest"`
	UserID                   string                     `json:"userID"`
}

type VMCPInstanceStatus struct {
	ConfigurationCheckHash       string   `json:"configurationCheckHash,omitempty"`
	Configured                   bool     `json:"configured,omitempty"`
	MissingRequiredConfiguration []string `json:"missingRequiredConfiguration,omitempty"`
	UserConfigurationHash        string   `json:"userConfigurationHash,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type VMCPInstanceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []VMCPInstance `json:"items"`
}

func (in *VMCPInstance) Has(field string) bool {
	return slices.Contains(in.FieldNames(), field)
}

func (in *VMCPInstance) Get(field string) string {
	switch field {
	case "spec.legacySlug":
		return in.Spec.LegacySlug
	case "spec.userID":
		return in.Spec.UserID
	case "spec.manifest.vmcpID":
		return in.Spec.Manifest.VMCPID
	}
	return ""
}

func (*VMCPInstance) FieldNames() []string {
	return []string{"spec.userID", "spec.manifest.vmcpID", "spec.legacySlug"}
}

func (in *VMCPInstance) DeleteRefs() []Ref {
	return []Ref{{ObjType: &VMCP{}, Name: in.Spec.Manifest.VMCPID}}
}
