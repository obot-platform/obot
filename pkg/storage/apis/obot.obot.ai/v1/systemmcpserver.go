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
	Manifest             types.MCPServerManifest     `json:"manifest"`
	SystemServerSettings types.SystemServerSettings  `json:"systemServerSettings"`
	SourceURL            string                      `json:"sourceURL,omitempty"`
	Editable             bool                        `json:"editable,omitempty"`
}

type SystemMCPServerStatus struct {
	// Note: Configured, MissingRequiredEnvVars, and MissingRequiredHeaders are
	// computed dynamically based on credentials and are not stored in Status.
	// They appear in the API response type but not in the storage layer.

	// Deployment status (reusing existing patterns)
	DeploymentStatus            string               `json:"deploymentStatus,omitempty"`
	DeploymentAvailableReplicas *int32               `json:"deploymentAvailableReplicas,omitempty"`
	DeploymentReadyReplicas     *int32               `json:"deploymentReadyReplicas,omitempty"`
	DeploymentReplicas          *int32               `json:"deploymentReplicas,omitempty"`
	DeploymentConditions        []DeploymentCondition `json:"deploymentConditions,omitempty"`
	K8sSettingsHash             string               `json:"k8sSettingsHash,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type SystemMCPServerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []SystemMCPServer `json:"items"`
}
