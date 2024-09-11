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
	Links   map[string]string `json:"links,omitempty"`
}

func MetadataFrom(obj kclient.Object, linkKV ...string) Metadata {
	m := Metadata{
		ID:      obj.GetName(),
		Created: obj.GetCreationTimestamp().Time,
		Links:   map[string]string{},
	}
	for i := 0; i < len(linkKV); i += 2 {
		m.Links[linkKV[i]] = linkKV[i+1]
	}
	return m
}

type Agent struct {
	Metadata
	v1.AgentManifest
}

type AgentList List[Agent]

type Thread struct {
	ID            string                   `json:"id,omitempty"`
	Created       time.Time                `json:"created,omitempty"`
	Description   string                   `json:"description,omitempty"`
	AgentID       string                   `json:"agentID,omitempty"`
	Input         string                   `json:"input,omitempty"`
	LastRunName   string                   `json:"lastRunName,omitempty"`
	LastRunState  gptscriptclient.RunState `json:"lastRunState,omitempty"`
	LastRunOutput string                   `json:"lastRunOutput,omitempty"`
	LastRunError  string                   `json:"lastRunError,omitempty"`
}

type ThreadList List[Thread]

type FileList List[string]

type Run struct {
	ID            string                   `json:"id,omitempty"`
	Created       time.Time                `json:"created,omitempty"`
	ThreadID      string                   `json:"threadID,omitempty"`
	AgentID       string                   `json:"agentID,omitempty"`
	PreviousRunID string                   `json:"previousRunID,omitempty"`
	Input         string                   `json:"input"`
	State         gptscriptclient.RunState `json:"state,omitempty"`
	Output        string                   `json:"output,omitempty"`
	Error         string                   `json:"error,omitempty"`
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

type Progress struct {
	Content string       `json:"content,omitempty"`
	Error   string       `json:"error,omitempty"`
	Tool    ToolProgress `json:"tool,omitempty"`
}

type ToolProgress struct {
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	Input       string `json:"input,omitempty"`
}
