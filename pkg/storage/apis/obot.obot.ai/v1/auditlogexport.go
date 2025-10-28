package v1

import (
	"slices"

	"github.com/obot-platform/nah/pkg/fields"
	"github.com/obot-platform/obot/apiclient/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	_ fields.Fields = (*AuditLogExport)(nil)
	_ DeleteRefs    = (*AuditLogExport)(nil)
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type AuditLogExport struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AuditLogExportSpec   `json:"spec,omitempty"`
	Status AuditLogExportStatus `json:"status,omitempty"`
}

func (a *AuditLogExport) Has(field string) (exists bool) {
	return slices.Contains(a.FieldNames(), field)
}

func (a *AuditLogExport) Get(field string) (value string) {
	switch field {
	case "spec.status":
		return string(a.Status.State)
	}
	return ""
}

func (a *AuditLogExport) FieldNames() []string {
	return []string{"spec.status"}
}

func (*AuditLogExport) GetColumns() [][]string {
	return [][]string{
		{"Name", "Name"},
		{"Status", "Status.State"},
		{"Start Time", "{{.Spec.StartTime.Format \"2006-01-02 15:04:05\"}}"},
		{"End Time", "{{.Spec.EndTime.Format \"2006-01-02 15:04:05\"}}"},
		{"Created", "{{ago .CreationTimestamp}}"},
	}
}

func (a *AuditLogExport) DeleteRefs() []Ref {
	return []Ref{}
}

type AuditLogExportSpec struct {
	Name                   string                      `json:"name"`
	Bucket                 string                      `json:"bucket"`
	KeyPrefix              string                      `json:"keyPrefix,omitempty"`
	StartTime              metav1.Time                 `json:"startTime"`
	EndTime                metav1.Time                 `json:"endTime"`
	Filters                types.AuditLogExportFilters `json:"filters,omitempty"`
	WithRequestAndResponse bool                        `json:"withRequestAndResponse,omitempty"`
}

type AuditLogExportStatus struct {
	State           types.AuditLogExportState `json:"state"`
	Error           string                    `json:"error,omitempty"`
	ExportSize      int64                     `json:"exportSize,omitempty"`
	ExportPath      string                    `json:"exportPath,omitempty"`
	StartedAt       *metav1.Time              `json:"startedAt,omitempty"`
	CompletedAt     *metav1.Time              `json:"completedAt,omitempty"`
	StorageProvider types.StorageProviderType `json:"storageProvider,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type AuditLogExportList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AuditLogExport `json:"items"`
}
