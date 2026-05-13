package localagents

func DirectInstallers() []DirectInstaller {
	return []DirectInstaller{
		NewClaudeCode(),
	}
}
