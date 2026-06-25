package v1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ObotHelmValues stores the Helm values snapshot captured at install/upgrade time.
type ObotHelmValues struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ObotHelmValuesSpec   `json:"spec,omitempty"`
	Status ObotHelmValuesStatus `json:"status,omitempty"`
}

type ObotHelmValuesSpec struct {
	// ValuesYAML is the YAML snapshot of IT-configurable Helm values.
	ValuesYAML string `json:"valuesYAML,omitempty"`
}

type ObotHelmValuesStatus struct{}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type ObotHelmValuesList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []ObotHelmValues `json:"items"`
}
