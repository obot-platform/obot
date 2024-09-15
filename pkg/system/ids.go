package system

import "strings"

const (
	ThreadPrefix            = "t1"
	AgentPrefix             = "a1"
	RunPrefix               = "r1"
	WorkflowPrefix          = "w1"
	WorkflowExecutionPrefix = "we1"
)

var typePrefixes = []string{
	ThreadPrefix,
	AgentPrefix,
	RunPrefix,
	WorkflowPrefix,
	WorkflowExecutionPrefix,
}

func IsThreadID(id string) bool {
	return strings.HasPrefix(id, ThreadPrefix)
}

func IsAgentID(id string) bool {
	return strings.HasPrefix(id, AgentPrefix)
}

func IsRunID(id string) bool {
	return strings.HasPrefix(id, RunPrefix)
}

func IsWorkflowID(id string) bool {
	return strings.HasPrefix(id, WorkflowPrefix)
}

func IsWorkflowExecutionID(id string) bool {
	return strings.HasPrefix(id, WorkflowExecutionPrefix)
}

func IsSystemID(id string) bool {
	for _, prefix := range typePrefixes {
		if strings.HasPrefix(id, prefix) {
			return true
		}
	}
	return false
}
