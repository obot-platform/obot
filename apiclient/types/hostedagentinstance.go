package types

import "fmt"

type HostedAgentInstance struct {
	Metadata                    `json:",inline"`
	HostedAgentInstanceManifest `json:",inline"`
	HostedAgentID               string                    `json:"hostedAgentID,omitempty"`
	UserID                      string                    `json:"userID,omitempty"`
	Status                      HostedAgentInstanceStatus `json:"status,omitempty"`
}

type HostedAgentInstanceManifest struct {
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	Icon        string `json:"icon,omitempty"`

	// Answers holds the user's responses to the agent's questions, keyed by
	// question key. Values are strings regardless of question type; the agent's
	// manifest is the schema they are validated against.
	Answers map[string]string `json:"answers,omitempty"`

	// MCPServers, Skills, and Models are resources the user attached themselves.
	// They are only accepted when the agent allows the corresponding kind, and
	// only when the user has access to each one; the server checks both on create
	// and update. MCPServers holds MCP gateway IDs, and Models may hold
	// obot://<alias> references.
	MCPServers []string `json:"mcpServers,omitempty"`
	Skills     []string `json:"skills,omitempty"`
	Models     []string `json:"models,omitempty"`
}

type HostedAgentInstanceStatus struct {
	State HostedAgentState `json:"state,omitempty"`
	URL   string           `json:"url,omitempty"`
	Error string           `json:"error,omitempty"`
}

func (m HostedAgentInstanceManifest) Validate() error {
	if m.Name == "" {
		return fmt.Errorf("name is required")
	}
	return nil
}

// ValidateAgainstAgent checks an instance manifest against the agent it belongs
// to: answers must satisfy the agent's questions, and user-supplied resources
// are only allowed when the agent opts into them.
func (m HostedAgentInstanceManifest) ValidateAgainstAgent(agent HostedAgentManifest) error {
	if err := agent.ValidateAnswers(m.Answers); err != nil {
		return err
	}

	if len(m.MCPServers) > 0 && !agent.AllowUserMCPServers {
		return fmt.Errorf("this agent does not allow user-defined MCP servers")
	}
	if len(m.Skills) > 0 && !agent.AllowUserSkills {
		return fmt.Errorf("this agent does not allow user-defined skills")
	}
	if len(m.Models) > 0 && !agent.AllowUserModels {
		return fmt.Errorf("this agent does not allow user-defined models")
	}

	return nil
}

type HostedAgentInstanceList List[HostedAgentInstance]
