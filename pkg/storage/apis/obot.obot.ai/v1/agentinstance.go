package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type AgentInstance struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	Spec              AgentInstanceSpec   `json:"spec"`
	Status            AgentInstanceStatus `json:"status"`
}

type ClaudeSessionRepository struct {
	URL      string `json:"url"`
	Revision string `json:"revision,omitempty"`
}

type AgentInstanceSpec struct {
	// Runner-reported Slack metadata is for display only, never authorization.
	SlackSenderName      string `json:"slackSenderName,omitempty"`
	SlackSenderHandle    string `json:"slackSenderHandle,omitempty"`
	SlackChannelName     string `json:"slackChannelName,omitempty"`
	SlackThreadTimestamp string `json:"slackThreadTimestamp,omitempty"`
	SlackLink            string `json:"slackLink,omitempty"`
	SlackMentioned       *bool  `json:"slackMentioned,omitempty"`

	// Nil means repository metadata was not provided; empty means no repositories.
	// +optional
	ClaudeRepositories []ClaudeSessionRepository `json:"claudeRepositories"`
	// ClaudeFinishedAt starts retention after the execution stops being used.
	ClaudeFinishedAt  metav1.Time `json:"claudeFinishedAt,omitzero"`
	ClaudeEnvironment string      `json:"claudeEnvironment,omitempty"`
	ClaudeRuntime     string      `json:"claudeRuntime,omitempty"`
	ClaudeSession     string      `json:"claudeSession,omitempty"`
	ClaudeAccount     string      `json:"claudeAccount,omitempty"`
	ClaudeOrder       string      `json:"claudeOrder,omitempty"`
	ClaudePrevious    string      `json:"claudePrevious,omitempty"`
	ClaudeDone        bool        `json:"claudeDone,omitempty"`
	ClaudeCheckpoint  bool        `json:"claudeCheckpoint,omitempty"`
	UserID            string      `json:"userID"`
	DisplayName       string      `json:"displayName"`
	Suspended         bool        `json:"suspended"`
}

type AgentInstanceStatus struct {
	ClaudeStarted      bool   `json:"claudeStarted,omitempty"`
	State              string `json:"state,omitempty"`
	Error              string `json:"error,omitempty"`
	ObservedGeneration int64  `json:"observedGeneration,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type AgentInstanceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []AgentInstance `json:"items"`
}

func (in *AgentInstance) Has(field string) bool { return field == "spec.userID" }

func (in *AgentInstance) Get(field string) string {
	if field == "spec.userID" {
		return in.Spec.UserID
	}
	return ""
}

func (in *AgentInstance) FieldNames() []string { return []string{"spec.userID"} }
