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

	// PodSecurityAdmission contains Pod Security Admission settings for the MCP namespace
	PodSecurityAdmission *PodSecurityAdmissionSettings `json:"podSecurityAdmission,omitempty"`

	// SetViaHelm indicates if these settings came from Helm (cannot be updated via API)
	SetViaHelm bool `json:"setViaHelm,omitempty"`
}

// PodSecurityAdmissionSettings contains Pod Security Admission configuration
type PodSecurityAdmissionSettings struct {
	// Enabled indicates whether PSA labels should be applied to the MCP namespace
	Enabled bool `json:"enabled,omitempty"`

	// Enforce is the Pod Security Standards level to enforce (privileged, baseline, or restricted)
	Enforce string `json:"enforce,omitempty"`

	// EnforceVersion is the Kubernetes version for the enforce policy (e.g., "latest", "v1.28")
	EnforceVersion string `json:"enforceVersion,omitempty"`

	// Audit is the Pod Security Standards level to audit (privileged, baseline, or restricted)
	Audit string `json:"audit,omitempty"`

	// AuditVersion is the Kubernetes version for the audit policy
	AuditVersion string `json:"auditVersion,omitempty"`

	// Warn is the Pod Security Standards level to warn about (privileged, baseline, or restricted)
	Warn string `json:"warn,omitempty"`

	// WarnVersion is the Kubernetes version for the warn policy
	WarnVersion string `json:"warnVersion,omitempty"`
}

type K8sSettingsStatus struct{}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type K8sSettingsList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []K8sSettings `json:"items"`
}
