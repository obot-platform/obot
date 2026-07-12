package v1

const (
	ModelProviderFinalizer         = "obot.obot.ai/model-provider"
	MCPServerFinalizer             = "obot.obot.ai/mcp-server"
	MCPServerCatalogEntryFinalizer = "obot.obot.ai/mcp-server-catalog-entry"
	MCPServerInstanceFinalizer     = "obot.obot.ai/mcp-server-instance"
	MCPSessionFinalizer            = "obot.obot.ai/mcp-session"
	OAuthClientFinalizer           = "obot.obot.ai/oauth-client"
	AccessControlRuleFinalizer     = "obot.obot.ai/access-control-rule"
	SystemMCPServerFinalizer       = "obot.obot.ai/system-mcp-server"
	NanobotAgentFinalizer          = "obot.obot.ai/nanobot-agent"
	ImagePullSecretFinalizer       = "obot.obot.ai/image-pull-secret"

	ModelProviderSyncAnnotation               = "obot.ai/model-provider-sync"
	AuthProviderSyncAnnotation                = "obot.ai/auth-provider-sync"
	MCPCatalogSyncAnnotation                  = "obot.ai/mcp-catalog-sync"
	SystemMCPCatalogSyncAnnotation            = "obot.ai/system-mcp-catalog-sync"
	SkillRepositorySyncAnnotation             = "obot.ai/skill-repository-sync"
	MCPServerCatalogEntrySyncAnnotation       = "obot.ai/mcp-server-catalog-entry-sync"
	SystemMCPServerCatalogEntrySyncAnnotation = "obot.ai/system-mcp-server-catalog-entry-sync"
	ModelInfoSourceSyncAnnotation             = "obot.ai/model-info-source-sync"
)
