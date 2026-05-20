package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type Thread struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ThreadSpec   `json:"spec,omitempty"`
	Status ThreadStatus `json:"status,omitempty"`
}

func (in *Thread) GetColumns() [][]string {
	return [][]string{
		{"Name", "Name"},
		{"CurrentRun", "Status.CurrentRunName"},
		{"LastRun", "Status.LastRunName"},
		{"LastRunState", "Status.LastRunState"},
		{"Created", "{{ago .CreationTimestamp}}"},
	}
}

type ThreadSpec struct {
	Manifest ThreadManifest `json:"manifest,omitempty"`
	// Abort means that this thread should be aborted immediately
	Abort bool `json:"abort,omitempty"`
	// Env is the environment variable keys that expected to be set in the credential that matches the thread.Name
	Env []EnvVar `json:"env,omitempty"`
	// Ephemeral means that this thread is used once and then can be deleted after an interval
	Ephemeral bool `json:"ephemeral,omitempty"`
}

type ThreadStatus struct {
	LastRunName    string        `json:"lastRunName,omitempty"`
	CurrentRunName string        `json:"currentRunName,omitempty"`
	LastRunState   RunStateState `json:"lastRunState,omitempty"`
	LastUsedTime   metav1.Time   `json:"lastUsedTime,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type ThreadList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []Thread `json:"items"`
}

type ThreadManifest struct {
	Name                string              `json:"name"`
	Description         string              `json:"description,omitempty"`
	IntroductionMessage string              `json:"introductionMessage"`
	StarterMessages     []string            `json:"starterMessages"`
	Tools               []string            `json:"tools,omitempty"`
	ModelProvider       string              `json:"modelProvider,omitempty"`
	Model               string              `json:"model,omitempty"`
	Prompt              string              `json:"prompt"`
	SharedTasks         []string            `json:"sharedTasks,omitempty"`
	AllowedMCPTools     map[string][]string `json:"allowedMCPTools,omitempty"`
}

type EnvVar struct {
	Name        string `json:"name"`
	Value       string `json:"value"`
	Description string `json:"description"`
	Existing    bool   `json:"existing"`
}
