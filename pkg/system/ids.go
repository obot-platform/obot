package system

import "strings"

const (
	ThreadPrefix            = "t1"
	AgentPrefix             = "a1"
	RunPrefix               = "r1"
	WorkflowPrefix          = "w1"
	WorkflowExecutionPrefix = "we1"
	WorkflowStepPrefix      = "ws1"
	WorkspacePrefix         = "wksp1"
	WebhookPrefix           = "wh1"
	CronJobPrefix           = "cj1"
	KnowledgeSourcePrefix   = "ks1"
	OAuthAppPrefix          = "oa1"
	KnowledgeSetPrefix      = "kst1"
	OAuthAppLoginPrefix     = "oal1"
	ModelPrefix             = "m1"
)

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
