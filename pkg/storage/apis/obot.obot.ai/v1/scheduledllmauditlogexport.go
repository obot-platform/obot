package v1

import (
	"slices"

	"github.com/obot-platform/nah/pkg/fields"
	"github.com/obot-platform/obot/apiclient/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	_ fields.Fields = (*ScheduledLLMAuditLogExport)(nil)
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type ScheduledLLMAuditLogExport struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ScheduledLLMAuditLogExportSpec   `json:"spec,omitempty"`
	Status ScheduledLLMAuditLogExportStatus `json:"status,omitempty"`
}

func (s *ScheduledLLMAuditLogExport) Enabled() bool {
	return s.Spec.Enabled
}

func (s *ScheduledLLMAuditLogExport) GetSchedule() Schedule {
	return s.Spec.Schedule
}

func (s *ScheduledLLMAuditLogExport) LastRunAt() *metav1.Time {
	return s.Status.LastRunAt
}

func (s *ScheduledLLMAuditLogExport) SetLastRunAt(t metav1.Time) {
	s.Status.LastRunAt = &t
}

func (s *ScheduledLLMAuditLogExport) Has(field string) (exists bool) {
	return slices.Contains(s.FieldNames(), field)
}

func (s *ScheduledLLMAuditLogExport) Get(field string) (value string) {
	switch field {
	case "spec.enabled":
		if s.Spec.Enabled {
			return "true"
		}
		return "false"
	case "spec.schedule.interval":
		return s.Spec.Schedule.Interval
	}
	return ""
}

func (s *ScheduledLLMAuditLogExport) FieldNames() []string {
	return []string{"spec.enabled", "spec.schedule.interval"}
}

func (*ScheduledLLMAuditLogExport) GetColumns() [][]string {
	return [][]string{
		{"Name", "Name"},
		{"Created", "{{ago .CreationTimestamp}}"},
	}
}

type ScheduledLLMAuditLogExportSpec struct {
	Name                  string                         `json:"name"`
	Bucket                string                         `json:"bucket"`
	KeyPrefix             string                         `json:"keyPrefix,omitempty"`
	Enabled               bool                           `json:"enabled"`
	Schedule              Schedule                       `json:"schedule"`
	RetentionPeriodInDays int                            `json:"retentionPeriodInDays,omitempty"`
	Filters               types.LLMAuditLogExportFilters `json:"filters,omitempty"`
	WithSensitiveFields   bool                           `json:"withSensitiveFields,omitempty"`
}

type ScheduledLLMAuditLogExportStatus struct {
	TotalExportsCreated int64        `json:"totalExportsCreated,omitempty"`
	Error               string       `json:"error,omitempty"`
	LastRunAt           *metav1.Time `json:"lastRunAt,omitempty"`
	NextRunAt           *metav1.Time `json:"nextRunAt,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type ScheduledLLMAuditLogExportList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ScheduledLLMAuditLogExport `json:"items"`
}
