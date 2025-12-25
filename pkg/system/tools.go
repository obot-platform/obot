package system

const (
	KnowledgeIngestTool        = "knowledge-ingest"
	KnowledgeLoadTool          = "knowledge-load"
	KnowledgeDeleteTool        = "knowledge-delete"
	KnowledgeDeleteFileTool    = "knowledge-delete-file"
	KnowledgeRetrievalTool     = "knowledge-retrieval"
	WebsiteCleanTool           = "website-cleaner"
	ResultFormatterTool        = "result-formatter"
	ModelProviderTool          = "obot-model-provider"
	WorkflowTool               = "workflow"
	TasksTool                  = "tasks"
	TasksWorkflowTool          = "tasks-workflow"
	DockerTool                 = "docker"
	ShellTool                  = "shell"
	DockerShellIDTool          = "docker-shell-id"
	ExistingCredTool           = "existing-credential"
	KnowledgeCredID            = "knowledge"
	TaskInvoke                 = "task-invoke"
	OpenAIModelProviderTool    = "openai-model-provider"
	AnthropicModelProviderTool = "anthropic-model-provider"

	OpenAIAPIKeyEnvVar    = "OPENAI_API_KEY"
	AnthropicAPIKeyEnvVar = "ANTHROPIC_API_KEY"

	DefaultNamespace       = "default"
	DefaultCatalog         = "default"
	DefaultRoleSettingName = "user-default-role-setting"
	K8sSettingsName        = "k8s-settings"
	AppPreferencesName     = "app-preferences"

	ModelProviderCredential = "sys.model.provider.credential"

	GenericModelProviderCredentialContext       = "model-provider"
	GenericAuthProviderCredentialContext        = "auth-provider"
	GenericFileScannerProviderCredentialContext = "file-scanner-provider"

	MCPWebhookValidationCredentialContext = "mcp-webhook-context"

	JWKCredentialContext = "jwk"
)
