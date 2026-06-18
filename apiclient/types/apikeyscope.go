package types

const (
	APIKeyScopeAPI                = "api"
	APIKeyScopeSkills             = "skills"
	APIKeyScopeLLM                = "llm"
	APIKeyScopePublishedArtifacts = "published-artifacts"
	APIKeyScopeAllMCP             = "all-mcp"
	APIKeyScopeDeviceScans        = "device-scans"
)

func DefaultCLIAPIKeyScopes() []string {
	return []string{APIKeyScopeLLM, APIKeyScopeDeviceScans, APIKeyScopeSkills}
}
