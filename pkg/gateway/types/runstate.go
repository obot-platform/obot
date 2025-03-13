package types

import (
	"time"
)

type RunState struct {
	Name       string    `json:"name" gorm:"primaryKey"`
	Namespace  string    `json:"namespace" gorm:"primaryKey"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
	ThreadName string    `json:"threadName,omitempty"`
	Program    []byte    `json:"program,omitempty"`
	ChatState  []byte    `json:"chatState,omitempty"`
	CallFrame  []byte    `json:"callFrame,omitempty"`
	Output     []byte    `json:"output,omitempty"`
	Done       bool      `json:"done,omitempty"`
	Error      string    `json:"error,omitempty"`
}
