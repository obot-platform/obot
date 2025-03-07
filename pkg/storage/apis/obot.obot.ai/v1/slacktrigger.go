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

	// TeamID is the Slack team/workspace ID
	TeamID string `json:"teamID"`

	// AppID is the Slack app ID
	AppID string `json:"appID"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// SlackTriggerList contains a list of SlackTrigger resources
type SlackTriggerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SlackTrigger `json:"items"`
}
