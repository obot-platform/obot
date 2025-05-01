package v1

import (
	"slices"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type ProjectInvitation struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ProjectInvitationSpec   `json:"spec,omitempty"`
	Status ProjectInvitationStatus `json:"status,omitempty"`
}

func (pi *ProjectInvitation) DeleteRefs() []Ref {
	return []Ref{
		{
			ObjType: &Thread{},
			Name:    pi.Spec.ThreadID,
		},
	}
}

func (pi *ProjectInvitation) GetColumns() [][]string {
	return [][]string{
		{"Name", "Name"},
		{"Status", "Spec.Status"},
		{"Project ID", "Spec.ThreadID"},
		{"Created", "{{ago .CreationTimestamp}}"},
	}
}

func (pi *ProjectInvitation) Has(field string) (exists bool) {
	return slices.Contains(pi.FieldNames(), field)
}

func (pi *ProjectInvitation) Get(field string) (value string) {
	switch field {
	case "spec.status":
		return string(pi.Spec.Status)
	case "spec.threadID":
		return pi.Spec.ThreadID
	}
	return ""
}

func (pi *ProjectInvitation) FieldNames() []string {
	return []string{"spec.status", "spec.threadID"}
}

type ProjectInvitationSpec struct {
	Status   string `json:"status,omitempty"`
	ThreadID string `json:"threadID,omitempty"`
}

type ProjectInvitationStatus struct {
	// RespondedTime is the time the invitation was accepted, rejected, or marked as expired.
	RespondedTime *metav1.Time `json:"respondedTime,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type ProjectInvitationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []ProjectInvitation `json:"items"`
}
