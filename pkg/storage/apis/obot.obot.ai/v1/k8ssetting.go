package v1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type K8sSettings struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   K8sSettingsSpec   `json:"spec,omitempty"`
	Status K8sSettingsStatus `json:"status,omitempty"`
}

type K8sSettingsSpec struct {
	// Affinity rules for MCP server pods
	// +k8s:openapi-gen=false
	Affinity *corev1.Affinity `json:"affinity,omitempty"`

	// Tolerations for MCP server pods
	// +k8s:openapi-gen=false
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`

	// Resource requests and limits
	// +k8s:openapi-gen=false
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`

	// RuntimeClassName specifies the RuntimeClass for MCP server pods
	// This allows running MCP servers with specific container runtimes (e.g., gVisor, Kata)
	// +k8s:openapi-gen=false
	RuntimeClassName *string `json:"runtimeClassName,omitempty"`

	// SetViaHelm indicates if these settings came from Helm (cannot be updated via API)
	SetViaHelm bool `json:"setViaHelm,omitempty"`
}

type K8sSettingsStatus struct{}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type K8sSettingsList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []K8sSettings `json:"items"`
}
