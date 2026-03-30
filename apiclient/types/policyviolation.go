package types

import (
	"encoding/json"
	"time"
)

type PolicyViolation struct {
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

type PolicyViolationList List[PolicyViolation]

type PolicyViolationResponse struct {
	PolicyViolationList `json:",inline"`
	Total               int64 `json:"total"`
	Limit               int   `json:"limit"`
	Offset              int   `json:"offset"`
}

type PolicyViolationStats struct {
	ByTime      []PolicyViolationTimeBucket    `json:"byTime"`
	ByPolicy    []PolicyViolationPolicyCount   `json:"byPolicy"`
	ByUser      []PolicyViolationUserCount     `json:"byUser"`
	ByDirection PolicyViolationDirectionCounts `json:"byDirection"`
}

type PolicyViolationTimeBucket struct {
	Time  time.Time `json:"time"`
	Count int64     `json:"count"`
}

type PolicyViolationPolicyCount struct {
	PolicyID   string `json:"policyID"`
	PolicyName string `json:"policyName"`
	Count      int64  `json:"count"`
}

type PolicyViolationUserCount struct {
	UserID string `json:"userID"`
	Count  int64  `json:"count"`
}

type PolicyViolationDirectionCounts struct {
	UserMessage int64 `json:"userMessage"`
	ToolCalls   int64 `json:"toolCalls"`
}
