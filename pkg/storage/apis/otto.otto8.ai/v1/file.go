package v1

import (
	"crypto/sha256"
	"fmt"

	"github.com/acorn-io/acorn/apiclient/types"
	"github.com/acorn-io/nah/pkg/fields"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type KnowledgeFile struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   KnowledgeFileSpec   `json:"spec,omitempty"`
	Status KnowledgeFileStatus `json:"status,omitempty"`
}

var _ fields.Fields = (*KnowledgeFile)(nil)

func (k *KnowledgeFile) GetColumns() [][]string {
	return [][]string{
		{"Name", "Name"},
		{"KnowledgeSource", "Spec.KnowledgeSourceName"},
		{"State", "{{ .PublicState }}"},
		{"Error", "Status.Error"},
		{"Filename", "Spec.FileName"},
		{"Created", "{{ago .CreationTimestamp}}"},
	}
}

func (k *KnowledgeFile) PublicState() types.KnowledgeFileState {
	state := k.Status.State
	if k.Spec.Approved != nil && !*k.Spec.Approved {
		state = types.KnowledgeFileStateUnapproved
	}
	if state == "" {
		state = types.KnowledgeFileStatePending
	}
	if state == types.KnowledgeFileStatePending && k.Spec.Approved == nil {
		state = types.KnowledgeFileStatePendingApproval
	}
	return state
}

func (k *KnowledgeFile) DeleteRefs() []Ref {
	return []Ref{
		{ObjType: new(KnowledgeSource), Name: k.Spec.KnowledgeSourceName},
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
	case "spec.knowledgeSourceName":
		return k.Spec.KnowledgeSourceName
	case "spec.knowledgeSetName":
		return k.Spec.KnowledgeSetName
	}

	return ""
}

func (*KnowledgeFile) FieldNames() []string {
	return []string{"spec.knowledgeSourceName", "spec.knowledgeSetName"}
}

var _ fields.Fields = (*KnowledgeFile)(nil)

type KnowledgeFileSpec struct {
	KnowledgeSetName    string `json:"knowledgeSetName,omitempty"`
	KnowledgeSourceName string `json:"knowledgeSourceName,omitempty"`
	Approved            *bool  `json:"approved,omitempty"`

	FileName    string `json:"fileName,omitempty"`
	URL         string `json:"url,omitempty"`
	UpdatedAt   string `json:"updatedAt,omitempty"`
	Checksum    string `json:"checksum,omitempty"`
	SizeInBytes int64  `json:"sizeInBytes,omitempty"`

	IngestGeneration int64 `json:"ingestGeneration,omitempty"`
}

type KnowledgeFileStatus struct {
	State types.KnowledgeFileState `json:"state,omitempty"`
	Error string                   `json:"error,omitempty"`

	URL       string `json:"url,omitempty"`
	UpdatedAt string `json:"updatedAt,omitempty"`
	Checksum  string `json:"checksum,omitempty"`

	RunNames               []string    `json:"runNames,omitempty"`
	LastIngestionStartTime metav1.Time `json:"lastIngestionStartTime,omitempty"`
	LastIngestionEndTime   metav1.Time `json:"lastIngestionEndTime,omitempty"`

	IngestGeneration int64 `json:"ingestGeneration,omitempty"`
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
