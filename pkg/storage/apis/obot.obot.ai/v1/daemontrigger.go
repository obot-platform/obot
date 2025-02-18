package v1

import (
	"slices"

	"github.com/obot-platform/nah/pkg/fields"
	"github.com/obot-platform/obot/apiclient/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	_ fields.Fields = (*DaemonTrigger)(nil)
	_ Generationed  = (*DaemonTrigger)(nil)
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type DaemonTriggerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []DaemonTrigger `json:"items"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type DaemonTrigger struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DaemonTriggerSpec   `json:"spec"`
	Status DaemonTriggerStatus `json:"status,omitempty"`
}

type DaemonTriggerSpec struct {
	types.DaemonTriggerManifest
	ThreadName string
}

type DaemonTriggerStatus struct {
	OptionsValid       *bool `json:"optionsValid,omitempty"`
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

func (d *DaemonTrigger) Has(field string) (exists bool) {
	return slices.Contains(d.FieldNames(), field)
}

func (d *DaemonTrigger) Get(field string) (value string) {
	switch field {
	case "spec.threadName":
		return d.Spec.ThreadName
	case "spec.workflow":
		return d.Spec.Workflow
	case "spec.provider":
		return d.Spec.Provider
	}
	return ""
}

func (d *DaemonTrigger) FieldNames() []string {
	return []string{"spec.threadName", "spec.workflow", "spec.provider"}
}

func (*DaemonTrigger) GetColumns() [][]string {
	return [][]string{
		{"Name", "Name"},
		{"Workflow", "Spec.Workflow"},
		{"Daemon Trigger Provider", "Spec.Provider"},
		{"Configuration Valid", "Status.OptionsValid"},
		{"Created", "{{ago .CreationTimestamp}}"},
		{"Description", "Spec.Description"},
	}
}

func (d *DaemonTrigger) GetObservedGeneration() int64 {
	return d.Status.ObservedGeneration
}

func (d *DaemonTrigger) SetObservedGeneration(gen int64) {
	d.Status.ObservedGeneration = gen
}

func (*DaemonTrigger) DeleteRefs() []Ref {
	return nil
}
