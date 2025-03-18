package types

import (
	"github.com/gptscript-ai/go-gptscript"
)

type Run struct {
	ID             string `json:"id,omitempty"`
	Created        Time   `json:"created,omitempty"`
	ThreadID       string `json:"threadID,omitempty"`
	AgentID        string `json:"agentID,omitempty"`
	WorkflowID     string `json:"workflowID,omitempty"`
	WorkflowStepID string `json:"workflowStepID,omitempty"`
	PreviousRunID  string `json:"previousRunID,omitempty"`
	Input          string `json:"input"`
	State          string `json:"state,omitempty"`
	Output         string `json:"output,omitempty"`
	Error          string `json:"error,omitempty"`
}

type RunList List[Run]

// +k8s:deepcopy-gen=false

// +k8s:openapi-gen=false
type RunDebug struct {
	// Spec is opaque, for human eyes only
	Spec any `json:"spec"`
	// Status is opaque, for human eyes only
	Status   any                            `json:"status"`
	Frames   map[string]gptscript.CallFrame `json:"frames"`
	RunState any                            `json:"runState,omitempty"`
}
