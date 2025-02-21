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

	// RelatedKnowledgeSetNames is a list of knowledge set that must match the same text embedding model
	// The best effort will be made to keep the same text embedding model used for the related knowledge sets
	RelatedKnowledgeSetNames []string `json:"relatedKnowledgeSetNames,omitempty"`
}

func (in *KnowledgeSet) GetColumns() [][]string {
	return [][]string{
		{"Name", "Name"},
		{"Agent", "Spec.AgentName"},
		{"Thread", "Spec.ThreadName"},
		{"Created", "{{ago .CreationTimestamp}}"},
	}
}

func (in *KnowledgeSet) DeleteRefs() []Ref {
	return []Ref{
		{ObjType: &Agent{}, Name: in.Spec.AgentName},
		{ObjType: &Thread{}, Name: in.Spec.ThreadName},
	}
}

func (in *KnowledgeSet) Has(field string) bool {
	return in.Get(field) != ""
}

func (in *KnowledgeSet) Get(field string) string {
	if in == nil {
		return ""
	}

	switch field {
	case "spec.agentName":
		return in.Spec.AgentName
	}

	return ""
}

func (*KnowledgeSet) FieldNames() []string {
	return []string{"spec.agentName"}
}

// KnowledgeSetManifest should be moved to types once we expose this API
type KnowledgeSetManifest struct {
	DataDescription string `json:"dataDescription,omitempty"`
}

type KnowledgeSetStatus struct {
	HasContent               bool   `json:"hasContent,omitempty"`
	DatasetCreated           bool   `json:"datasetCreated,omitempty"`
	SuggestedDataDescription string `json:"suggestedDataDescription,omitempty"`
	WorkspaceName            string `json:"workspaceName,omitempty"`
	ThreadName               string `json:"threadName,omitempty"`
	ExistingFile             string `json:"existingFile,omitempty"`
	TextEmbeddingModel       string `json:"textEmbeddingModel,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type KnowledgeSetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []KnowledgeSet `json:"items"`
}
