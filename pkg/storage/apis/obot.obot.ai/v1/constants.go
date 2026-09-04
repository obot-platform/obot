package v1

const (
	ModelProviderFinalizer         = "obot.obot.ai/model-provider"
	MCPServerFinalizer             = "obot.obot.ai/mcp-server"
	MCPServerCatalogEntryFinalizer = "obot.obot.ai/mcp-server-catalog-entry"
	MCPServerInstanceFinalizer     = "obot.obot.ai/mcp-server-instance"
	AccessControlRuleFinalizer     = "obot.obot.ai/access-control-rule"
	SystemMCPServerFinalizer       = "obot.obot.ai/system-mcp-server"
	NanobotAgentFinalizer          = "obot.obot.ai/nanobot-agent"
	ImagePullSecretFinalizer       = "obot.obot.ai/image-pull-secret"
	GitCredentialFinalizer         = "obot.obot.ai/git-credential"
	HostedAgentInstanceFinalizer   = "obot.obot.ai/hosted-agent-instance"
	HostedAgentPoolFinalizer       = "obot.obot.ai/hosted-agent-pool"

	ModelProviderSyncAnnotation         = "obot.ai/model-provider-sync"
	MCPCatalogSyncAnnotation            = "obot.ai/mcp-catalog-sync"
	SystemMCPCatalogSyncAnnotation      = "obot.ai/system-mcp-catalog-sync"
	SkillRepositorySyncAnnotation       = "obot.ai/skill-repository-sync"
	MDMAssetSourceSyncAnnotation        = "obot.ai/mdm-asset-source-sync"
	AgentCatalogSyncAnnotation          = "obot.ai/agent-catalog-sync"
	MCPServerCatalogEntrySyncAnnotation = "obot.ai/mcp-server-catalog-entry-sync"
	ModelInfoSourceSyncAnnotation       = "obot.ai/model-info-source-sync"

	// AuditLogCredentialRemovedAnnotation records that the token exchange and audit log
	// credential an older Obot stored for this server has been removed. Nothing creates that
	// credential any more, so the sweep only needs to run once per server, and this annotation
	// is what stops it running on every reconcile forever.
	AuditLogCredentialRemovedAnnotation = "obot.ai/audit-log-credential-removed"
)
