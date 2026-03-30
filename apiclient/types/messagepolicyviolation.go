package types

import (
	"encoding/json"
)

type MessagePolicyViolation struct {
	ID                   uint            `json:"id"`
	CreatedAt            Time            `json:"createdAt"`
	UserID               string          `json:"userID"`
	PolicyID             string          `json:"policyID"`
	PolicyName           string          `json:"policyName"`
	PolicyDefinition     string          `json:"policyDefinition"`
	Direction            string          `json:"direction"`
	ViolationExplanation string          `json:"violationExplanation"`
	BlockedContent       json.RawMessage `json:"blockedContent,omitempty"`
	ProjectID            string          `json:"projectID"`
	ThreadID             string          `json:"threadID"`
}

type MessagePolicyViolationList List[MessagePolicyViolation]

type MessagePolicyViolationResponse struct {
	MessagePolicyViolationList `json:",inline"`
	Total               int64 `json:"total"`
	Limit               int   `json:"limit"`
	Offset              int   `json:"offset"`
}

type MessagePolicyViolationStats struct {
	ByTime      []MessagePolicyViolationTimeBucket    `json:"byTime"`
	ByPolicy    []MessagePolicyViolationPolicyCount   `json:"byPolicy"`
	ByUser      []MessagePolicyViolationUserCount     `json:"byUser"`
	ByDirection MessagePolicyViolationDirectionCounts `json:"byDirection"`
}

type MessagePolicyViolationTimeBucket struct {
	Time     Time   `json:"time"`
	Category string `json:"category"`
	Count    int64  `json:"count"`
}

type MessagePolicyViolationPolicyCount struct {
	PolicyID   string `json:"policyID"`
	PolicyName string `json:"policyName"`
	Count      int64  `json:"count"`
}

type MessagePolicyViolationUserCount struct {
	UserID string `json:"userID"`
	Count  int64  `json:"count"`
}

type MessagePolicyViolationDirectionCounts struct {
	UserMessage int64 `json:"userMessage"`
	ToolCalls   int64 `json:"toolCalls"`
}
