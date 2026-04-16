package v1

import (
	"slices"

	"github.com/obot-platform/nah/pkg/fields"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/system"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	_ fields.Fields = (*SystemMCPServer)(nil)
	_ DeleteRefs    = (*SystemMCPServer)(nil)
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type SystemMCPServer struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SystemMCPServerSpec   `json:"spec,omitempty"`
	Status SystemMCPServerStatus `json:"status,omitempty"`
}

type SystemMCPServerSpec struct {
	// Manifest contains the server configuration
	Manifest types.SystemMCPServerManifest `json:"manifest"`
	// WebhookValidationName is the name of the associated MCPWebhookValidation resource
	WebhookValidationName string `json:"webhookValidationName,omitempty"`
}

type SystemMCPServerStatus struct {
	// DeploymentStatus indicates overall status (Available, Progressing, Unavailable, Needs Attention, Shutdown, Unknown)
	DeploymentStatus string `json:"deploymentStatus,omitempty"`
	// DeploymentAvailableReplicas is the number of available replicas
	DeploymentAvailableReplicas *int32 `json:"deploymentAvailableReplicas,omitempty"`
	// DeploymentReadyReplicas is the number of ready replicas
	DeploymentReadyReplicas *int32 `json:"deploymentReadyReplicas,omitempty"`
	// DeploymentReplicas is the desired number of replicas
	DeploymentReplicas *int32 `json:"deploymentReplicas,omitempty"`
	// DeploymentConditions contains deployment health conditions
	DeploymentConditions []DeploymentCondition `json:"deploymentConditions,omitempty"`
	// K8sSettingsHash contains the hash of K8s settings this was deployed with
	K8sSettingsHash string `json:"k8sSettingsHash,omitempty"`
	// AuditLogTokenHash contains the hash of the audit log token
	AuditLogTokenHash string `json:"auditLogTokenHash,omitempty"`
}

func (in *SystemMCPServer) Has(field string) (exists bool) {
	return slices.Contains(in.FieldNames(), field)
}

func (in *SystemMCPServer) Get(field string) (value string) {
	switch field {
	case "auditLogTokenHash":
		return in.Status.AuditLogTokenHash
	}
	return ""
}

func (in *SystemMCPServer) FieldNames() []string {
	return []string{
		"auditLogTokenHash",
	}
}

func (in *SystemMCPServer) ValidConnectURLs(base string) []string {
	return []string{system.MCPConnectURL(base, in.Name)}
}

func (in *SystemMCPServer) DeleteRefs() []Ref {
	if in.Spec.WebhookValidationName != "" {
		return []Ref{{
			ObjType: new(MCPWebhookValidation),
			Name:    in.Spec.WebhookValidationName,
		}}
	}
	return nil
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type SystemMCPServerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []SystemMCPServer `json:"items"`
}
