package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// EntitlementUsage is what hosted MCP servers request, for display. There's one, default/entitlement-usage, and only
// while billing is on. The leader keeps its status current. Limits are never checked against it, since it can lag:
// writes are admitted against live counts.
type EntitlementUsage struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`

	Status EntitlementUsageStatus `json:"status"`
}

type EntitlementUsageStatus struct {
	MemoryMiB     int64 `json:"memoryMiB"`
	CPUMillicores int64 `json:"cpuMillicores"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type EntitlementUsageList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []EntitlementUsage `json:"items"`
}
