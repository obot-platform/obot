package v1

import (
	"slices"

	"github.com/obot-platform/nah/pkg/fields"
	"github.com/obot-platform/obot/apiclient/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	_ fields.Fields = (*HostedAgentInstance)(nil)
	_ DeleteRefs    = (*HostedAgentInstance)(nil)
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type HostedAgentInstance struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`

	Spec   HostedAgentInstanceSpec   `json:"spec"`
	Status HostedAgentInstanceStatus `json:"status"`
}

func (in *HostedAgentInstance) Has(field string) bool {
	return slices.Contains(in.FieldNames(), field)
}

func (in *HostedAgentInstance) Get(field string) string {
	switch field {
	case "spec.userID":
		return in.Spec.UserID
	case "spec.hostedAgentName":
		return in.Spec.HostedAgentName
	case "spec.poolID":
		return in.Spec.PoolID
	}
	return ""
}

func (in *HostedAgentInstance) FieldNames() []string {
	return []string{"spec.userID", "spec.hostedAgentName", "spec.poolID"}
}

func (in *HostedAgentInstance) DeleteRefs() []Ref {
	return []Ref{
		{ObjType: &HostedAgent{}, Name: in.Spec.HostedAgentName},
		{ObjType: &HostedAgentPool{}, Name: in.Spec.PoolID},
	}
}

func (in *HostedAgentInstance) GetColumns() [][]string {
	return [][]string{
		{"Name", "Name"},
		{"Display Name", "Spec.Manifest.Name"},
		{"Hosted Agent", "Spec.HostedAgentName"},
		{"Pool", "Spec.PoolID"},
		{"User", "Spec.UserID"},
		{"State", "Status.State"},
		{"Created", "{{ago .CreationTimestamp}}"},
	}
}

type HostedAgentInstanceSpec struct {
	UserID          string                            `json:"userID,omitempty"`
	HostedAgentName string                            `json:"hostedAgentName,omitempty"`
	PoolID          string                            `json:"poolID,omitempty"`
	Manifest        types.HostedAgentInstanceManifest `json:"manifest"`
}

type HostedAgentInstanceStatus struct {
	State types.HostedAgentState `json:"state,omitempty"`
	URL   string                 `json:"url,omitempty"`

	Error   string `json:"error,omitempty"`
	Reason  string `json:"reason,omitempty"`
	Message string `json:"message,omitempty"`

	ObservedRevision string       `json:"observedRevision,omitempty"`
	LastObservedTime *metav1.Time `json:"lastObservedTime,omitempty"`

	BackendID         string `json:"backendID,omitempty"`
	BackendGeneration int64  `json:"backendGeneration,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type HostedAgentInstanceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []HostedAgentInstance `json:"items"`
}
