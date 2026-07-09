package v1

import (
	"slices"

	"github.com/obot-platform/nah/pkg/fields"
	"github.com/obot-platform/obot/apiclient/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	_ fields.Fields = (*LLMAuditLogExport)(nil)
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type LLMAuditLogExport struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   LLMAuditLogExportSpec `json:"spec,omitempty"`
	Status AuditLogExportStatus  `json:"status,omitempty"`
}

func (l *LLMAuditLogExport) Bucket() string {
	return l.Spec.Bucket
}

func (l *LLMAuditLogExport) KeyPrefix() string {
	return l.Spec.KeyPrefix
}

func (l *LLMAuditLogExport) SpecName() string {
	return l.Spec.Name
}

func (l *LLMAuditLogExport) ExportStatus() *AuditLogExportStatus {
	return &l.Status
}

func (l *LLMAuditLogExport) Has(field string) (exists bool) {
	return slices.Contains(l.FieldNames(), field)
}

func (l *LLMAuditLogExport) Get(field string) (value string) {
	switch field {
	case "spec.status":
		return string(l.Status.State)
	}
	return ""
}

func (l *LLMAuditLogExport) FieldNames() []string {
	return []string{"spec.status"}
}

func (*LLMAuditLogExport) GetColumns() [][]string {
	return [][]string{
		{"Name", "Name"},
		{"Status", "Status.State"},
		{"Start Time", "{{.Spec.StartTime.Format \"2006-01-02 15:04:05\"}}"},
		{"End Time", "{{.Spec.EndTime.Format \"2006-01-02 15:04:05\"}}"},
		{"Created", "{{ago .CreationTimestamp}}"},
	}
}

type LLMAuditLogExportSpec struct {
	Name                string                         `json:"name"`
	Bucket              string                         `json:"bucket"`
	KeyPrefix           string                         `json:"keyPrefix,omitempty"`
	StartTime           metav1.Time                    `json:"startTime"`
	EndTime             metav1.Time                    `json:"endTime"`
	Filters             types.LLMAuditLogExportFilters `json:"filters,omitempty"`
	WithSensitiveFields bool                           `json:"withSensitiveFields,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type LLMAuditLogExportList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []LLMAuditLogExport `json:"items"`
}
