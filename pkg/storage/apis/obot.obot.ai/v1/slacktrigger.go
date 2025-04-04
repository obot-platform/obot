package v1

import (
	"slices"

	"github.com/obot-platform/nah/pkg/fields"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	_ fields.Fields = (*SlackTrigger)(nil)
	_ DeleteRefs    = (*SlackTrigger)(nil)
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type SlackTrigger struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SlackTriggerSpec   `json:"spec,omitempty"`
	Status SlackTriggerStatus `json:"status,omitempty"`
}

func (in *SlackTrigger) DeleteRefs() []Ref {
	return []Ref{
		{ObjType: new(Thread), Name: in.Spec.ThreadName},
		{ObjType: new(SlackReceiver), Name: in.Spec.SlackReceiverName},
	}
}

func (in *SlackTrigger) Has(field string) (exists bool) {
	return slices.Contains(in.FieldNames(), field)
}

func (in *SlackTrigger) Get(field string) (value string) {
	switch field {
	case "spec.appID":
		return in.Spec.AppID
	case "spec.teamID":
		return in.Spec.TeamID
	}
	return ""
}

func (in *SlackTrigger) FieldNames() []string {
	return []string{"spec.appID", "spec.teamID"}
}

type SlackTriggerSpec struct {
	AppID             string `json:"appID,omitempty"`
	TeamID            string `json:"teamID,omitempty"`
	ThreadName        string `json:"threadName,omitempty"`
	SlackReceiverName string `json:"slackReceiverName,omitempty"`
}

type SlackTriggerStatus struct {
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type SlackTriggerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []SlackTrigger `json:"items"`
}
