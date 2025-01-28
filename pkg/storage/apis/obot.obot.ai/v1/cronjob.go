package v1

import (
	"slices"

	"github.com/obot-platform/nah/pkg/fields"
	"github.com/obot-platform/obot/apiclient/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ fields.Fields = (*CronJob)(nil)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type CronJob struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              CronJobSpec   `json:"spec,omitempty"`
	Status            CronJobStatus `json:"status,omitempty"`
}

func (c *CronJob) Has(field string) (exists bool) {
	return slices.Contains(c.FieldNames(), field)
}

func (c *CronJob) Get(field string) (value string) {
	switch field {
	case "spec.threadName":
		return c.Spec.ThreadName
	case "spec.workflow":
		return c.Spec.Workflow
	}
	return ""
}

func (c *CronJob) FieldNames() []string {
	return []string{"spec.threadName", "spec.workflow"}
}

func (*CronJob) GetColumns() [][]string {
	return [][]string{
		{"Name", "Name"},
		{"Workflow", "Spec.Workflow"},
		{"Schedule", "Spec.Schedule"},
		{"Last Success", "{{ago .Status.LastSuccessfulRunCompleted}}"},
		{"Last Run", "{{ago .Status.LastRunStartedAt}}"},
		{"Created", "{{ago .CreationTimestamp}}"},
		{"Description", "Spec.Description"},
	}
}

func (c *CronJob) DeleteRefs() []Ref {
	return nil
}

type CronJobSpec struct {
	types.CronJobManifest `json:",inline"`
	ThreadName            string `json:"threadName,omitempty"`
}

type CronJobStatus struct {
	LastRunStartedAt           *metav1.Time `json:"lastRunStartedAt,omitempty"`
	LastSuccessfulRunCompleted *metav1.Time `json:"lastSuccessfulRunCompleted,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type CronJobList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CronJob `json:"items"`
}
