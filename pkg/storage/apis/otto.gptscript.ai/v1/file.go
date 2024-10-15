package v1

import (
	"crypto/sha256"
	"fmt"

	"github.com/acorn-io/baaah/pkg/fields"
	"github.com/otto8-ai/otto8/apiclient/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type KnowledgeFile struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   KnowledgeFileSpec   `json:"spec,omitempty"`
	Status KnowledgeFileStatus `json:"status,omitempty"`
}

func (k *KnowledgeFile) DeleteRefs() []Ref {
	return []Ref{
		{ObjType: new(Workspace), Name: k.Spec.WorkspaceName},
		{ObjType: new(RemoteKnowledgeSource), Name: k.Spec.RemoteKnowledgeSourceName},
	}
}

func (k *KnowledgeFile) Has(field string) bool {
	return k.Get(field) != ""
}

func (k *KnowledgeFile) Get(field string) string {
	if k == nil {
		return ""
	}

	switch field {
	case "spec.workspaceName":
		return k.Spec.WorkspaceName
	case "spec.remoteKnowledgeSourceName":
		return k.Spec.RemoteKnowledgeSourceName
	case "spec.remoteKnowledgeSourceType":
		return string(k.Spec.RemoteKnowledgeSourceType)
	}

	return ""
}

func (*KnowledgeFile) FieldNames() []string {
	return []string{"spec.workspaceName", "spec.remoteKnowledgeSourceName", "spec.remoteKnowledgeSourceType"}
}

var _ fields.Fields = (*KnowledgeFile)(nil)

type KnowledgeFileSpec struct {
	FileName                  string                          `json:"fileName"`
	WorkspaceName             string                          `json:"workspaceName,omitempty"`
	RemoteKnowledgeSourceName string                          `json:"remoteKnowledgeSourceName,omitempty"`
	RemoteKnowledgeSourceType types.RemoteKnowledgeSourceType `json:"remoteKnowledgeSourceType,omitempty"`
	Approved                  *bool                           `json:"approved,omitempty"`
}

type KnowledgeFileStatus struct {
	IngestionStatus types.IngestionStatus `json:"ingestionStatus,omitempty"`
	FileDetails     types.FileDetails     `json:"fileDetails,omitempty"`
	UploadID        string                `json:"uploadID,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type KnowledgeFileList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []KnowledgeFile `json:"items"`
}

func ObjectNameFromAbsolutePath(absolutePath string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(absolutePath)))
}
