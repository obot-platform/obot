package v1

import (
	"slices"

	"github.com/obot-platform/nah/pkg/fields"
	"github.com/obot-platform/obot/apiclient/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	_ fields.Fields = (*ScheduledAuditLogExport)(nil)
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type ScheduledAuditLogExport struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ScheduledAuditLogExportSpec   `json:"spec,omitempty"`
	Status ScheduledAuditLogExportStatus `json:"status,omitempty"`
}

func (s *ScheduledAuditLogExport) Has(field string) (exists bool) {
	return slices.Contains(s.FieldNames(), field)
}

func (s *ScheduledAuditLogExport) Get(field string) (value string) {
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

func (s *ScheduledAuditLogExport) FieldNames() []string {
	return []string{"spec.enabled", "spec.schedule.interval", "spec.storageProvider"}
}

func (*ScheduledAuditLogExport) GetColumns() [][]string {
	return [][]string{
		{"Name", "Name"},
		{"Created", "{{ago .CreationTimestamp}}"},
	}
}

func (s *ScheduledAuditLogExport) DeleteRefs() []Ref {
	return []Ref{}
}

type ScheduledAuditLogExportSpec struct {
	Name                   string                      `json:"name"`
	Bucket                 string                      `json:"bucket"`
	KeyPrefix              string                      `json:"keyPrefix,omitempty"`
	Enabled                bool                        `json:"enabled"`
	Schedule               Schedule                    `json:"schedule"`
	RetentionPeriodInDays  int                         `json:"retentionPeriodInDays,omitempty"`
	Filters                types.AuditLogExportFilters `json:"filters,omitempty"`
	WithRequestAndResponse bool                        `json:"withRequestAndResponse,omitempty"`
}

type Schedule struct {
	// Valid values are: "hourly", "daily", "weekly", "monthly"
	Interval string `json:"interval"`
	Hour     int    `json:"hour"`
	Minute   int    `json:"minute"`
	Day      int    `json:"day"`
	Weekday  int    `json:"weekday"`
	TimeZone string `json:"timezone"`
}

type NotificationConfig struct {
	OnSuccess bool   `json:"onSuccess,omitempty"`
	OnFailure bool   `json:"onFailure,omitempty"`
	Webhook   string `json:"webhook,omitempty"`
	Email     string `json:"email,omitempty"`
}

type ScheduledAuditLogExportStatus struct {
	TotalExportsCreated int64        `json:"totalExportsCreated,omitempty"`
	Error               string       `json:"error,omitempty"`
	LastRunAt           *metav1.Time `json:"lastRunAt,omitempty"`
	NextRunAt           *metav1.Time `json:"nextRunAt,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type ScheduledAuditLogExportList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ScheduledAuditLogExport `json:"items"`
}
