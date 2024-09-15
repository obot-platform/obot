package types

import (
	"time"

	gptscriptclient "github.com/gptscript-ai/go-gptscript"
	"github.com/gptscript-ai/otto/pkg/storage/apis/otto.gptscript.ai/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type List[T any] struct {
	Items []T `json:"items"`
}

type Metadata struct {
	ID      string            `json:"id,omitempty"`
	Created time.Time         `json:"created,omitempty"`
	Deleted *time.Time        `json:"deleted,omitempty"`
	Links   map[string]string `json:"links,omitempty"`
}

func MetadataFrom(obj kclient.Object, linkKV ...string) Metadata {
	m := Metadata{
		ID:      obj.GetName(),
		Created: obj.GetCreationTimestamp().Time,
		Links:   map[string]string{},
	}
	if delTime := obj.GetDeletionTimestamp(); delTime != nil {
		m.Deleted = &delTime.Time
	}
	for i := 0; i < len(linkKV); i += 2 {
		m.Links[linkKV[i]] = linkKV[i+1]
	}
	return m
}

type Agent struct {
	Metadata
	v1.AgentManifest
	v1.AgentExternalStatus
}

type AgentList List[Agent]

type Workflow struct {
	Metadata
	v1.WorkflowManifest
	v1.WorkflowExternalStatus
}

type WorkflowList List[Workflow]

type Thread struct {
	Metadata
	Description   string                   `json:"description,omitempty"`
	AgentID       string                   `json:"agentID,omitempty"`
	LastRunName   string                   `json:"lastRunName,omitempty"`
	LastRunState  gptscriptclient.RunState `json:"lastRunState,omitempty"`
	LastRunOutput string                   `json:"lastRunOutput,omitempty"`
	LastRunError  string                   `json:"lastRunError,omitempty"`
}

type ThreadList List[Thread]

type FileList List[string]

type Run struct {
	ID            string    `json:"id,omitempty"`
	Created       time.Time `json:"created,omitempty"`
	ThreadID      string    `json:"threadID,omitempty"`
	AgentID       string    `json:"agentID,omitempty"`
	WorkflowID    string    `json:"workflowID,omitempty"`
	PreviousRunID string    `json:"previousRunID,omitempty"`
	Input         string    `json:"input"`
	State         string    `json:"state,omitempty"`
	Output        string    `json:"output,omitempty"`
	Error         string    `json:"error,omitempty"`
}

type RunList List[Run]

type RunDebug struct {
	Frames map[string]gptscriptclient.CallFrame `json:"frames"`
}

type InvokeResponse struct {
	Events   <-chan Progress
	RunID    string
	ThreadID string
}

type Progress v1.Progress
