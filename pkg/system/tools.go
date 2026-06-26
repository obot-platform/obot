package system

const (
	OpenAIModelProvider    = "openai-model-provider"
	AnthropicModelProvider = "anthropic-model-provider"

	OpenAIAPIKeyEnvVar    = "OPENAI_API_KEY"
	AnthropicAPIKeyEnvVar = "ANTHROPIC_API_KEY"

	DefaultNamespace       = "default"
	DefaultCatalog         = "default"
	DefaultSkillRepository = "default"
	DefaultModelInfoSource = "default"
	DefaultRoleSettingName = "user-default-role-setting"
	K8sSettingsName    = "k8s-settings"
	AppPreferencesName = "app-preferences"
	AppNotificationName    = "app-notification"

	ModelProviderCredential = "sys.model.provider.credential"

	GenericModelProviderCredentialContext = "model-provider"
	GenericAuthProviderCredentialContext  = "auth-provider"

	MCPWebhookValidationCredentialContext = "mcp-webhook-context"

	JWKCredentialContext = "jwk"
)
