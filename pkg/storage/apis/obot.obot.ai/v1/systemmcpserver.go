package v1

import (
	"slices"

	"github.com/obot-platform/nah/pkg/fields"
	"github.com/obot-platform/obot/apiclient/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	_ fields.Fields = (*SystemMCPServer)(nil)
	_ DeleteRefs    = (*SystemMCPServer)(nil)
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type SystemMCPServer struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SystemMCPServerSpec   `json:"spec,omitempty"`
	Status SystemMCPServerStatus `json:"status,omitempty"`
}

func (in *SystemMCPServer) Has(field string) (exists bool) {
	return slices.Contains(in.FieldNames(), field)
}

func (in *SystemMCPServer) Get(field string) (value string) {
	switch field {
	case "spec.manifest.runtime":
		return string(in.Spec.Manifest.Runtime)
	}
	return ""
}

// TODO(g-linville) - remove this field selector if we don't need it

func (in *SystemMCPServer) FieldNames() []string {
	return []string{
		"spec.manifest.runtime",
	}
}

func (in *SystemMCPServer) DeleteRefs() []Ref {
	// SystemMCPServers don't reference other resources for deletion
	return nil
}

type SystemMCPServerSpec struct {
	// Manifest contains the server configuration
	Manifest types.SystemMCPServerManifest `json:"manifest"`
}

type SystemMCPServerStatus struct {
	// DeploymentStatus indicates overall status (Ready, Progressing, Failed)
	DeploymentStatus string `json:"deploymentStatus,omitempty"`
	// DeploymentAvailableReplicas is the number of available replicas
	DeploymentAvailableReplicas *int32 `json:"deploymentAvailableReplicas,omitempty"`
	// DeploymentReadyReplicas is the number of ready replicas
	DeploymentReadyReplicas *int32 `json:"deploymentReadyReplicas,omitempty"`
	// DeploymentReplicas is the desired number of replicas
	DeploymentReplicas *int32 `json:"deploymentReplicas,omitempty"`
	// DeploymentConditions contains deployment health conditions
	DeploymentConditions []DeploymentCondition `json:"deploymentConditions,omitempty"`
	// K8sSettingsHash contains the hash of K8s settings this was deployed with
	K8sSettingsHash string `json:"k8sSettingsHash,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type SystemMCPServerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []SystemMCPServer `json:"items"`
}
