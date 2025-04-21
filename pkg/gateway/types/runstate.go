package types

import (
	"time"
)

type RunState struct {
	UserID           string    `json:"userID"`
	Name             string    `json:"name" gorm:"primaryKey"`
	Namespace        string    `json:"namespace" gorm:"primaryKey"`
	CreatedAt        time.Time `json:"createdAt"`
	UpdatedAt        time.Time `json:"updatedAt"`
	ThreadName       string    `json:"threadName,omitempty"`
	Program          []byte    `json:"program,omitempty"`
	ChatState        []byte    `json:"chatState,omitempty"`
	CallFrame        []byte    `json:"callFrame,omitempty"`
	Output           []byte    `json:"output,omitempty"`
	Done             bool      `json:"done,omitempty"`
	Error            string    `json:"error,omitempty"`
	PromptTokens     int       `json:"promptTokens,omitempty"`
	CompletionTokens int       `json:"completionTokens,omitempty"`
	TotalTokens      int       `json:"totalTokens,omitempty"`
}
