package types

const (
	APIKeyScopeAPI         = "api"
	APIKeyScopeSkills      = "skills"
	APIKeyScopeLLM         = "llm"
	APIKeyScopeAllMCP      = "all-mcp"
	APIKeyScopeDeviceScans = "device-scans"
)

func DefaultCLIAPIKeyScopes() []string {
	return []string{APIKeyScopeLLM, APIKeyScopeDeviceScans, APIKeyScopeSkills}
}
