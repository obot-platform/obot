package system

const (
	OpenAIModelProvider              = "openai-model-provider"
	AnthropicModelProvider           = "anthropic-model-provider"
	GenericResponsesModelProvider    = "generic-responses-model-provider"
	AmazonBedrockModelProvider       = "amazon-bedrock-model-provider"
	AmazonBedrockAPIKeyModelProvider = "amazon-bedrock-api-key-model-provider"
	AzureModelProvider               = "azure-model-provider"
	AzureEntraModelProvider          = "azure-entra-model-provider"

	// LocalAuthProvider is the built-in username/password auth provider, implemented in
	// pkg/localauth. It runs in-process instead of as a daemon from the provider registry.
	LocalAuthProvider = "local-auth-provider"

	// BootstrapName is the reserved name used for the bootstrap user and auth provider.
	BootstrapName = "bootstrap"

	OpenAIAPIKeyEnvVar    = "OPENAI_API_KEY"
	AnthropicAPIKeyEnvVar = "ANTHROPIC_API_KEY"

	DefaultNamespace       = "default"
	DefaultCatalog         = "default"
	DefaultSkillRepository = "default"
	DefaultAgentCatalog    = "default"
	DefaultModelInfoSource = "default"
	DefaultMDMAssetSource  = "default"
	DefaultRoleSettingName = "user-default-role-setting"
	K8sSettingsName        = "k8s-settings"
	AppPreferencesName     = "app-preferences"
	AppNotificationName    = "app-notification"

	GenericModelProviderCredentialContext = "model-provider"
	GenericAuthProviderCredentialContext  = "auth-provider"
	StagedProviderCredentialContext       = "provider-staged"

	// ReplacementAuthProviderCredentialContext holds the settings of a replacement auth provider
	// while the current one keeps serving logins, persisting until an owner activates or discards
	// it. StagedProviderCredentialContext, by contrast, only parks a credential until the
	// controller applies it. It is its own context because isAuthProviderConfigured reads only a
	// provider's own context and the generic one, so a replacement is never the login provider.
	ReplacementAuthProviderCredentialContext = "replacement-auth-provider"

	MCPWebhookValidationCredentialContext = "mcp-webhook-context"

	JWKCredentialContext = "jwk"
)
