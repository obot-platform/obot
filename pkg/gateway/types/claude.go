package types

import (
	"encoding/json"
	"time"

	api "github.com/obot-platform/obot/apiclient/types"
)

type ClaudeWorker struct {
	WorkerReplicas int    `json:"workerReplicas,omitempty"`
	CPULimit       string `json:"cpuLimit,omitempty"`
	MemoryLimit    string `json:"memoryLimit,omitempty"`
}

// ClaudeResources can be overridden by profiles. Zero/empty values inherit the
// environment value; matching profiles are combined using the maximum per field.
type ClaudeResources struct {
	ActorCPU             string `json:"actorCPU,omitempty"`
	ActorMemory          string `json:"actorMemory,omitempty"`
	IdleTimeoutMinutes   int    `json:"idleTimeoutMinutes,omitempty"`
	SessionRetentionDays int    `json:"sessionRetentionDays,omitempty"`
}

type ClaudeRuntime struct {
	ManagedSettings json.RawMessage `json:"managedSettings,omitempty"`
	Image           string          `json:"image"`
	WorkerImage     string          `json:"workerImage"`
	Worker          ClaudeWorker    `json:"worker"`
	AllowAllEgress  bool            `json:"allowAllEgress,omitempty"`
	EgressHosts     []string        `json:"egressHosts"`
	Resources       ClaudeResources `json:"resources"`
}

// ClaudeEgressSecret values are write-only in the API and stored separately on the environment.
type ClaudeEgressSecret struct {
	ID       string `json:"id"`
	Hostname string `json:"hostname"`
	Header   string `json:"header"`
	Value    string `json:"value,omitempty"`
}

type ClaudeEnvironmentProfile struct {
	EgressSecrets []ClaudeEgressSecret `json:"egressSecrets,omitempty"`
	Name          string               `json:"name"`
	Subjects      []api.Subject        `json:"subjects"`
	// nil inherits the environment allowlist; [] grants no vMCPs.
	MCPServerIDs   []string        `json:"mcpServerIDs"`
	EgressHosts    []string        `json:"egressHosts,omitempty"`
	AllowAllEgress bool            `json:"allowAllEgress,omitempty"`
	Resources      ClaudeResources `json:"resources"`
}

type ClaudeEnvironment struct {
	EgressSecrets          []ClaudeEgressSecret       `gorm:"serializer:json" json:"egressSecrets,omitempty"`
	EgressSecretValues     map[string]string          `gorm:"serializer:json" json:"-"`
	EgressSecretsEncrypted bool                       `json:"-"`
	ID                     string                     `gorm:"primaryKey" json:"id"`
	DisplayName            string                     `json:"displayName"`
	OrganizationID         string                     `json:"organizationID"`
	EnvironmentID          string                     `gorm:"uniqueIndex" json:"environmentID"`
	MCPServerIDs           []string                   `gorm:"serializer:json" json:"mcpServerIDs"`
	Profiles               []ClaudeEnvironmentProfile `gorm:"serializer:json" json:"profiles"`
	Runtime                ClaudeRuntime              `gorm:"serializer:json" json:"runtime"`
	Revision               string                     `json:"-"`
	ClaudeTagUserID        string                     `json:"claudeTagUserID,omitempty"`
	ClaudeTagEnabled       bool                       `json:"claudeTagEnabled"`
}

// ClaudeOrganizationAccount is an admin-managed or just-in-time identity mapping shared by
// every environment in an organization. Empty UserID explicitly leaves it unmapped.
type ClaudeOrganizationAccount struct {
	OrganizationID string `gorm:"primaryKey" json:"organizationID"`
	Account        string `gorm:"primaryKey" json:"account"`
	UUID           string `json:"uuid"`
	DisplayName    string `json:"displayName"`
	Email          string `json:"email"`
	UserID         string `json:"userID"`
}

// ClaudeWorkOrder prevents redelivery from launching a deleted execution again.
type ClaudeWorkOrder struct {
	Name      string `gorm:"primaryKey"`
	CreatedAt time.Time
}
