package v1

import (
	"github.com/obot-platform/obot/apiclient/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type SlackReceiver struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              SlackReceiverSpec   `json:"spec,omitempty"`
	Status            SlackReceiverStatus `json:"status,omitempty"`
}

type SlackReceiverStatus struct {
}

func (r *SlackReceiver) Has(field string) bool {
	return r.Get(field) != ""
}

func (r *SlackReceiver) Get(field string) string {
	if r != nil {
		switch field {
		case "spec.threadName":
			return r.Spec.ThreadName
		case "spec.manifest.appID":
			return r.Spec.Manifest.AppID
		}
	}

	return ""
}

func (r *SlackReceiver) FieldNames() []string {
	return []string{"spec.threadName", "spec.manifest.appID"}
}

func (r *SlackReceiver) DeleteRefs() []Ref {
	return []Ref{
		{
			ObjType: &Thread{},
			Name:    r.Spec.ThreadName,
		},
	}
}

type SlackReceiverSpec struct {
	Manifest   types.SlackReceiverManifest `json:"manifest,omitempty"`
	ThreadName string                      `json:"threadName,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type SlackReceiverList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SlackReceiver `json:"items"`
}
