package v1

import (
	"github.com/acorn-io/baaah/pkg/conditions"
	"github.com/gptscript-ai/otto/apiclient/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
)

const (
	RemoteKnowledgeSourceFinalizer = "otto.gptscript.ai/remote-knowledge-source"
	RemoteKnowledgeSourceLabel     = "otto.gptscript.ai/remote-knowledge-source"
)

var (
	_ conditions.Conditions = (*RemoteKnowledgeSource)(nil)
	_ fields.Fields         = (*SyncUploadRequest)(nil)
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type RemoteKnowledgeSource struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RemoteKnowledgeSourceSpec   `json:"spec,omitempty"`
	Status RemoteKnowledgeSourceStatus `json:"status,omitempty"`
}

func (in *RemoteKnowledgeSource) DeleteRefs() []Ref {
	return []Ref{
		{ObjType: &Agent{}, Name: in.Spec.AgentName},
		{ObjType: &Workflow{}, Name: in.Spec.WorkflowName},
	}
}

func (in *RemoteKnowledgeSource) GetConditions() *[]metav1.Condition {
	return &in.Status.Conditions
}

type RemoteKnowledgeSourceSpec struct {
	types.RemoteKnowledgeSourceManifest `json:",inline"`
	AgentName                           string `json:"agentName,omitempty"`
	WorkflowName                        string `json:"workflowName,omitempty"`
}

type RemoteKnowledgeSourceStatus struct {
	Conditions        []metav1.Condition               `json:"conditions,omitempty"`
	ThreadName        string                           `json:"threadName,omitempty"`
	RunName           string                           `json:"runName,omitempty"`
	Status            string                           `json:"status,omitempty"`
	Error             string                           `json:"error,omitempty"`
	State             types.RemoteKnowledgeSourceState `json:"state,omitempty"`
	LastReSyncStarted metav1.Time                      `json:"lastReSyncStarted,omitempty"`
}

type RemoteConnectorStatus struct {
	Status string                           `json:"status,omitempty"`
	Error  string                           `json:"error,omitempty"`
	Files  map[string]types.FileDetails     `json:"files,omitempty"`
	State  types.RemoteKnowledgeSourceState `json:"state,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type RemoteKnowledgeSourceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []RemoteKnowledgeSource `json:"items"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type SyncUploadRequest struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              SyncUploadRequestSpec   `json:"spec,omitempty"`
	Status            SyncUploadRequestStatus `json:"status,omitempty"`
}

func (in *SyncUploadRequest) Has(field string) bool {
	return in.Get(field) != ""
}

func (in *SyncUploadRequest) Get(field string) string {
	if in == nil {
		return ""
	}

	switch field {
	case "spec.remoteKnowledgeSourceName":
		return in.Spec.RemoteKnowledgeSourceName
	}

	return ""
}

func (*SyncUploadRequest) FieldNames() []string {
	return []string{"spec.remoteKnowledgeSourceName"}
}

type SyncUploadRequestSpec struct {
	RemoteKnowledgeSourceName string `json:"remoteKnowledgeSourceName,omitempty"`
}

type SyncUploadRequestStatus struct{}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type SyncUploadRequestList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SyncUploadRequest `json:"items"`
}
