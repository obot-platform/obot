package v1

import (
	"github.com/acorn-io/baaah/pkg/conditions"
	"github.com/gptscript-ai/otto/apiclient/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
)

const (
	OneDriveLinksFinalizer = "otto.gptscript.ai/onedrive-links"
	OneDriveLinksLabel     = "otto.gptscript.ai/onedrive-links"
)

var (
	_ conditions.Conditions = (*OneDriveLinks)(nil)
	_ fields.Fields         = (*SyncUploadRequest)(nil)
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type OneDriveLinks struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   OnedriveLinksSpec   `json:"spec,omitempty"`
	Status OnedriveLinksStatus `json:"status,omitempty"`
}

func (in *OneDriveLinks) DeleteRefs() []Ref {
	return []Ref{
		{ObjType: &Agent{}, Name: in.Spec.AgentName},
		{ObjType: &Workflow{}, Name: in.Spec.WorkflowName},
	}
}

func (in *OneDriveLinks) GetConditions() *[]metav1.Condition {
	return &in.Status.Conditions
}

type OnedriveLinksSpec struct {
	AgentName    string   `json:"agentName,omitempty"`
	WorkflowName string   `json:"workflowName,omitempty"`
	SharedLinks  []string `json:"sharedLinks,omitempty"`
}

type OnedriveLinksStatus struct {
	Conditions        []metav1.Condition `json:"conditions,omitempty"`
	ThreadName        string             `json:"threadName,omitempty"`
	RunName           string             `json:"runName,omitempty"`
	Status            string             `json:"status,omitempty"`
	Error             string             `json:"error,omitempty"`
	Folders           types.FolderSet    `json:"folders,omitempty"`
	LastReSyncStarted metav1.Time        `json:"lastReSyncStarted,omitempty"`
}

type OneDriveLinksConnectorStatus struct {
	Status  string                       `json:"status,omitempty"`
	Error   string                       `json:"error,omitempty"`
	Files   map[string]types.FileDetails `json:"files,omitempty"`
	Folders types.FolderSet              `json:"folders,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type OneDriveLinksList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []OneDriveLinks `json:"items"`
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
	case "spec.uploadName":
		return in.Spec.UploadName
	}

	return ""
}

func (*SyncUploadRequest) FieldNames() []string {
	return []string{"spec.uploadName"}
}

type SyncUploadRequestSpec struct {
	UploadName string `json:"uploadName,omitempty"`
}

type SyncUploadRequestStatus struct{}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type SyncUploadRequestList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SyncUploadRequest `json:"items"`
}
