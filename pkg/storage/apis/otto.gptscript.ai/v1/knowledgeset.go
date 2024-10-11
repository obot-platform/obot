package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type KnowledgeSet struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   KnowledgeSetSpec   `json:"spec,omitempty"`
	Status KnowledgeSetStatus `json:"status,omitempty"`
}

type KnowledgeSetSpec struct {
	Manifest KnowledgeSetManifest `json:"manifest,omitempty"`

	// AgentName is the name of the agent that created and owns this knowledge set
	AgentName string `json:"agentName,omitempty"`
	// ThreadName is the name of the thread that created and owns this knowledge set
	ThreadName string `json:"threadName,omitempty"`
}

func (in *KnowledgeSet) DeleteRefs() []Ref {
	return []Ref{
		{&Agent{}, in.Spec.AgentName},
		{&Thread{}, in.Spec.ThreadName},
	}
}

// KnowledgeSetManifest should be moved to types once we expose this API
type KnowledgeSetManifest struct {
	DataDescription string `json:"dataDescription,omitempty"`
}

type KnowledgeSource struct {
	Tool         string            `json:"tool,omitempty"`
	Args         map[string]string `json:"args,omitempty"`
	SyncSchedule string            `json:"syncSchedule,omitempty"`
	Exclude      []string          `json:"exclude,omitempty"`
}

type KnowledgeSetStatus struct {
	ObservedIngestionGeneration int64  `json:"observedIngestionGeneration,omitempty"`
	SuggestedDataDescription    string `json:"suggestedDataDescription,omitempty"`
	WorkspaceName               string `json:"workspaceName,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type KnowledgeSetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []KnowledgeSet `json:"items"`
}
