package v1

import (
	"github.com/obot-platform/obot/apiclient/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type SystemMCPServer struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SystemMCPServerSpec   `json:"spec,omitempty"`
	Status SystemMCPServerStatus `json:"status,omitempty"`
}

type SystemMCPServerSpec struct {
	// Manifest contains the server configuration
	Manifest types.SystemMCPServerManifest `json:"manifest"`
}

type SystemMCPServerStatus struct {
	// DeploymentStatus indicates overall status (Available, Progressing, Unavailable, Needs Attention, Unknown)
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
