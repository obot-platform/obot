package localagents

func DirectInstallers() []DirectInstaller {
	return []DirectInstaller{
		NewClaudeCode(),
	}
}

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
