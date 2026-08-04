package v1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type OktaGroupMigration struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`

	Spec   OktaGroupMigrationSpec `json:"spec"`
	Status EmptyStatus            `json:"status"`
}

type OktaGroupMigrationSpec struct {
	// IDMapping maps old group IDs to new group IDs.
	IDMapping map[string]string `json:"idMapping,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type OktaGroupMigrationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []OktaGroupMigration `json:"items"`
}
