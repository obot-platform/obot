package localagents

func DetectedAgents() []Agent {
	return []Agent{
		NewClaudeCode(),
	}
}

func SetupTargets() []SetupTarget {
	return []SetupTarget{
		NewClaudeCode(),
		NewSharedAgents(),
	}
}
