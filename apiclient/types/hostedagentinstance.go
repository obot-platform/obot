package types

import "fmt"

type HostedAgentInstance struct {
	Metadata                    `json:",inline"`
	HostedAgentInstanceManifest `json:",inline"`
	HostedAgentID               string                    `json:"hostedAgentID,omitempty"`
	UserID                      string                    `json:"userID,omitempty"`
	PoolID                      string                    `json:"poolID,omitempty"`
	Status                      HostedAgentInstanceStatus `json:"status,omitempty"`

	// ResolvedIcon and ResolvedIconDark are the icon to show for this instance,
	// already resolved through the chain a client would otherwise have to walk:
	// the instance's own icon, then its agent's, then its harness's.
	//
	// They are computed on read rather than stored. An instance created before
	// its agent's icon changed would otherwise keep displaying the old one, and
	// a client that wrote the instance back would persist an icon the user
	// never chose.
	ResolvedIcon     string `json:"resolvedIcon,omitempty"`
	ResolvedIconDark string `json:"resolvedIconDark,omitempty"`
}

type HostedAgentInstanceManifest struct {
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	Icon        string `json:"icon,omitempty"`

	// Answers holds the user's responses to the agent's questions, keyed by
	// question key. Values are strings regardless of question type; the agent's
	// manifest is the schema they are validated against.
	Answers map[string]string `json:"answers,omitempty"`

	// GitRepo is a git repository the user supplied for this instance. Only
	// accepted when the agent sets AllowUserGitRepo; it overrides the agent's
	// GitRepo if one is configured.
	GitRepo string `json:"gitRepo,omitempty"`
	// GitRef pins GitRepo to a branch, tag or commit SHA. It belongs to the
	// repository beside it: a user who overrides the repository chooses the ref
	// for their own repository, and the agent's ref does not carry over.
	GitRef string `json:"gitRef,omitempty"`

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

	// Error is retained for compatibility with clients created before the
	// structured Reason and Message diagnostics were introduced.
	Error   string `json:"error,omitempty"`
	Reason  string `json:"reason,omitempty"`
	Message string `json:"message,omitempty"`

	ObservedRevision string `json:"observedRevision,omitempty"`
	LastObservedTime *Time  `json:"lastObservedTime,omitempty"`

	BackendID         string `json:"backendID,omitempty"`
	BackendGeneration int64  `json:"backendGeneration,omitempty"`
}

func (m HostedAgentInstanceManifest) Validate() error {
	if m.Name == "" {
		return fmt.Errorf("name is required")
	}
	if m.GitRepo != "" {
		if err := ValidateGitRepoURL(m.GitRepo); err != nil {
			return err
		}
	}
	if err := ValidateGitRef(m.GitRef); err != nil {
		return err
	}
	if m.GitRef != "" && m.GitRepo == "" {
		return fmt.Errorf("gitRef requires gitRepo")
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

	if m.GitRepo != "" && !agent.AllowUserGitRepo {
		return fmt.Errorf("this agent does not allow a user-defined git repository")
	}
	if m.GitRef != "" && !agent.AllowUserGitRepo {
		return fmt.Errorf("this agent does not allow a user-defined git repository")
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
