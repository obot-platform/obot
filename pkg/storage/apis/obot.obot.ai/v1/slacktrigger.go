package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// SlackTrigger represents a Slack trigger configuration
type SlackTrigger struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec SlackTriggerSpec `json:"spec,omitempty"`
}

// SlackTriggerSpec defines the desired state of a SlackTrigger
type SlackTriggerSpec struct {
	// WorkflowName is the name of the workflow to trigger
	WorkflowName string `json:"workflowName"`

	// ThreadName is the name of the project thread where the trigger will be created
	ThreadName string `json:"threadName"`
}

func (r *SlackTrigger) Has(field string) bool {
	return r.Get(field) != ""
}

func (r *SlackTrigger) Get(field string) string {
	if r != nil {
		switch field {
		case "spec.threadName":
			return r.Spec.ThreadName
		}
	}
	return ""
}

func (r *SlackTrigger) DeleteRefs() []Ref {
	return []Ref{
		{
			ObjType: &Thread{},
			Name:    r.Spec.ThreadName,
		},
		{
			ObjType: &Workflow{},
			Name:    r.Spec.WorkflowName,
		},
	}
}

func (r *SlackTrigger) FieldNames() []string {
	return []string{"spec.threadName"}
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// SlackTriggerList contains a list of SlackTrigger resources
type SlackTriggerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SlackTrigger `json:"items"`
}
