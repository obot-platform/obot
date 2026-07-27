package system

import nanobottypes "github.com/obot-platform/nanobot/pkg/types"

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

	OpenAIAPIKeyEnvVar    = "OPENAI_API_KEY"
	AnthropicAPIKeyEnvVar = "ANTHROPIC_API_KEY"

	DefaultNamespace       = "default"
	DefaultCatalog         = "default"
	DefaultSkillRepository = "default"
	DefaultModelInfoSource = "default"
	DefaultMDMAssetSource  = "default"
	DefaultRoleSettingName = "user-default-role-setting"
	K8sSettingsName        = "k8s-settings"
	AppPreferencesName     = "app-preferences"
	AppNotificationName    = "app-notification"

	ModelProviderCredential = "sys.model.provider.credential"

	GenericModelProviderCredentialContext = "model-provider"
	GenericAuthProviderCredentialContext  = "auth-provider"

	MCPWebhookValidationCredentialContext = "mcp-webhook-context"

	JWKCredentialContext = "jwk"
)

// IsResponsesDialect reports whether dialect uses the Responses API format.
func IsResponsesDialect(dialect string) bool {
	return dialect == string(nanobottypes.DialectOpenAIResponses) || dialect == string(nanobottypes.DialectOpenResponses)
}
