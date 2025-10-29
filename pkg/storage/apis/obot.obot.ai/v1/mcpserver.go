package v1

import (
	"slices"
	"strconv"

	"github.com/obot-platform/nah/pkg/fields"
	"github.com/obot-platform/obot/apiclient/types"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	_ fields.Fields = (*MCPServer)(nil)
	_ DeleteRefs    = (*MCPServer)(nil)
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type MCPServer struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MCPServerSpec   `json:"spec,omitempty"`
	Status MCPServerStatus `json:"status,omitempty"`
}

func (in *MCPServer) Has(field string) (exists bool) {
	return slices.Contains(in.FieldNames(), field)
}

func (in *MCPServer) Get(field string) (value string) {
	switch field {
	case "spec.threadName":
		return in.Spec.ThreadName
	case "spec.userID":
		return in.Spec.UserID
	case "spec.mcpServerCatalogEntryName":
		return in.Spec.MCPServerCatalogEntryName
	case "spec.mcpCatalogID":
		return in.Spec.MCPCatalogID
	case "spec.powerUserWorkspaceID":
		return in.Spec.PowerUserWorkspaceID
	case "spec.template":
		return strconv.FormatBool(in.Spec.Template)
	case "spec.compositeName":
		return in.Spec.CompositeName
	}
	return ""
}

func (in *MCPServer) FieldNames() []string {
	return []string{
		"spec.threadName",
		"spec.userID",
		"spec.mcpServerCatalogEntryName",
		"spec.mcpCatalogID",
		"spec.powerUserWorkspaceID",
		"spec.template",
		"spec.compositeName",
	}
}

func (in *MCPServer) DeleteRefs() []Ref {
	return []Ref{
		{ObjType: &Thread{}, Name: in.Spec.ThreadName},
		{ObjType: &MCPServerCatalogEntry{}, Name: in.Spec.MCPServerCatalogEntryName},
		{ObjType: &MCPCatalog{}, Name: in.Spec.MCPCatalogID},
		{ObjType: &PowerUserWorkspace{}, Name: in.Spec.PowerUserWorkspaceID},
		{ObjType: &MCPServer{}, Name: in.Spec.CompositeName},
	}
}

type MCPServerSpec struct {
	Manifest types.MCPServerManifest `json:"manifest"`
	// List of tool names that are known to not work well in Obot.
	UnsupportedTools []string `json:"unsupportedTools,omitempty"`
	// ThreadName is the project or thread that owns this server, if there is one.
	ThreadName string `json:"threadName,omitempty"`
	// Alias is a user-defined alias for the MCP server.
	// This may only be set for single user and remote MCP servers (i.e. where `MCPCatalogID` is "").
	Alias string `json:"alias,omitempty"`
	// UserID is the user that created this server.
	UserID string `json:"userID,omitempty"`
	// SharedWithinMCPCatalogName is a deprecated field. It is renamed to MCPCatalogID.
	SharedWithinMCPCatalogName string `json:"sharedWithinMCPCatalogName,omitempty"`
	// MCPCatalogID contains the name of the MCPCatalog inside of which this server was directly created by the admin, if there is one.
	MCPCatalogID string `json:"mcpCatalogID,omitempty"`
	// MCPServerCatalogEntryName contains the name of the MCPServerCatalogEntry from which this MCP server was created, if there is one.
	MCPServerCatalogEntryName string `json:"mcpServerCatalogEntryName,omitempty"`
	// NeedsURL indicates whether the server's URL needs to be updated to match the catalog entry.
	NeedsURL bool `json:"needsURL,omitempty"`
	// PreviousURL contains the URL of the server before it was updated to match the catalog entry.
	PreviousURL string `json:"previousURL,omitempty"`
	// PowerUserWorkspaceID contains the name of the PowerUserWorkspace that owns this MCP server, if there is one.
	PowerUserWorkspaceID string `json:"powerUserWorkspaceID,omitempty"`
	// Template indicates whether this MCP server is a template server.
	// Template servers are hidden from user views and are used for creating project instances.
	Template bool `json:"template,omitempty"`
	// CompositeName is the name of the composite server that this MCP server is a component of, if there is one.
	CompositeName string `json:"compositeName,omitempty"`
}

type MCPServerStatus struct {
	// NeedsUpdate indicates whether the configuration in this server's catalog entry has drift from this server's configuration.
	NeedsUpdate bool `json:"needsUpdate,omitempty"`
	// MCPServerInstanceUserCount contains the number of unique users with server instances pointing to this MCP server.
	MCPServerInstanceUserCount *int `json:"mcpInstanceUserCount,omitempty"`
	// DeploymentStatus indicates the overall status of the MCP server deployment (Ready, Progressing, Failed).
	DeploymentStatus string `json:"deploymentStatus,omitempty"`
	// DeploymentAvailableReplicas is the number of available replicas in the deployment.
	DeploymentAvailableReplicas *int32 `json:"deploymentAvailableReplicas,omitempty"`
	// DeploymentReadyReplicas is the number of ready replicas in the deployment.
	DeploymentReadyReplicas *int32 `json:"deploymentReadyReplicas,omitempty"`
	// DeploymentReplicas is the desired number of replicas in the deployment.
	DeploymentReplicas *int32 `json:"deploymentReplicas,omitempty"`
	// DeploymentConditions contains key deployment conditions that indicate deployment health.
	DeploymentConditions []DeploymentCondition `json:"deploymentConditions,omitempty"`
}

type DeploymentCondition struct {
	// Type of deployment condition.
	Type appsv1.DeploymentConditionType `json:"type"`
	// Last time the condition transitioned from one status to another.
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty"`
	// Last time the condition was updated.
	LastUpdateTime metav1.Time `json:"lastUpdateTime,omitempty"`
	// Status of the condition, one of True, False, Unknown.
	Status corev1.ConditionStatus `json:"status"`
	// The reason for the condition's last transition.
	Reason string `json:"reason,omitempty" protobuf:"bytes,4,opt,name=reason"`
	// A human readable message indicating details about the transition.
	Message string `json:"message,omitempty" protobuf:"bytes,5,opt,name=message"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type MCPServerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []MCPServer `json:"items"`
}
