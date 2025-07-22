package v1

import (
	"fmt"
	"slices"

	"github.com/obot-platform/nah/pkg/fields"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/system"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	_ fields.Fields = (*MCPServer)(nil)
	_ DeleteRefs    = (*MCPServer)(nil)
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type ProjectMCPServer struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ProjectMCPServerSpec   `json:"spec,omitempty"`
	Status ProjectMCPServerStatus `json:"status,omitempty"`
}

func (in *ProjectMCPServer) Has(field string) (exists bool) {
	return slices.Contains(in.FieldNames(), field)
}

func (in *ProjectMCPServer) Get(field string) (value string) {
	switch field {
	case "spec.threadName":
		return in.Spec.ThreadName
	}
	return ""
}

func (in *ProjectMCPServer) FieldNames() []string {
	return []string{
		"spec.threadName",
	}
}

func (in *ProjectMCPServer) DeleteRefs() []Ref {
	refs := []Ref{{ObjType: &Thread{}, Name: in.Spec.ThreadName}}

	if system.IsMCPServerID(in.Spec.Manifest.MCPID) {
		refs = append(refs, Ref{ObjType: &MCPServer{}, Name: in.Spec.Manifest.MCPID})
	} else if system.IsMCPServerInstanceID(in.Spec.Manifest.MCPID) {
		refs = append(refs, Ref{ObjType: &MCPServerInstance{}, Name: in.Spec.Manifest.MCPID})
	}

	return refs
}

func (in *ProjectMCPServer) ConnectURL(base string) string {
	return fmt.Sprintf("%s/mcp-connect/%s", base, in.Spec.Manifest.MCPID)
}

type ProjectMCPServerSpec struct {
	Manifest   types.ProjectMCPServerManifest `json:"manifest,omitempty"`
	ThreadName string                         `json:"threadName,omitempty"`
	UserID     string                         `json:"userID,omitempty"`
}

type ProjectMCPServerStatus struct{}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type ProjectMCPServerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []ProjectMCPServer `json:"items"`
}
