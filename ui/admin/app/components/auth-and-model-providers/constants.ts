export const CommonModelProviderIds = {
	OLLAMA: "ollama-model-provider",
	GROQ: "groq-model-provider",
	VLLM: "vllm-model-provider",
	VOYAGE: "voyage-model-provider",
	ANTHROPIC: "anthropic-model-provider",
	OPENAI: "openai-model-provider",
	AZURE_OPENAI: "azure-openai-model-provider",
	ANTHROPIC_BEDROCK: "anthropic-bedrock-model-provider",
	XAI: "xai-model-provider",
	DEEPSEEK: "deepseek-model-provider",
	GEMINI_VERTEX: "gemini-vertex-model-provider",
	GENERIC_OPENAI: "generic-openai-model-provider",
};

export const ModelProviderLinks = {
	[CommonModelProviderIds.VOYAGE]: "https://www.voyageai.com/",
	[CommonModelProviderIds.OLLAMA]: "https://ollama.com/",
	[CommonModelProviderIds.GROQ]: "https://groq.com/",
	[CommonModelProviderIds.VLLM]: "https://docs.vllm.ai/",
	[CommonModelProviderIds.AZURE_OPENAI]:
		"https://azure.microsoft.com/en-us/explore/",
	[CommonModelProviderIds.ANTHROPIC]: "https://www.anthropic.com",
	[CommonModelProviderIds.OPENAI]: "https://openai.com/",
	[CommonModelProviderIds.ANTHROPIC_BEDROCK]:
		"https://aws.amazon.com/bedrock/claude/",
	[CommonModelProviderIds.XAI]: "https://x.ai/",
	[CommonModelProviderIds.DEEPSEEK]: "https://www.deepseek.com/",
	[CommonModelProviderIds.GEMINI_VERTEX]: "https://cloud.google.com/vertex-ai",
	[CommonModelProviderIds.GENERIC_OPENAI]:
		"https://platform.openai.com/docs/api-reference/introduction",
};

export const RecommendedModelProviders = [
	CommonModelProviderIds.OPENAI,
	CommonModelProviderIds.AZURE_OPENAI,
];

export const ModelProviderTooltips: {
	[key: string]: string;
} = {
	// OpenAI
	OBOT_OPENAI_MODEL_PROVIDER_API_KEY:
		"OpenAI API Key. Can be created and fetched from https://platform.openai.com/settings/organization/api-keys or https://platform.openai.com/api-keys",

	// Azure OpenAI
	OBOT_AZURE_OPENAI_MODEL_PROVIDER_ENDPOINT:
		"Endpoint for the Azure OpenAI service (e.g. https://<resource-name>.<region>.api.cognitive.microsoft.com/)",
	OBOT_AZURE_OPENAI_MODEL_PROVIDER_CLIENT_ID:
		"Unique identifier for the application when using Azure Active Directory. Can typically be found in App Registrations > [application].",
	OBOT_AZURE_OPENAI_MODEL_PROVIDER_CLIENT_SECRET:
		"Password or key that app uses to authenticate with Azure Active Directory. Can typically be found in App Registrations > [application] > Certificates & Secrets",
	OBOT_AZURE_OPENAI_MODEL_PROVIDER_TENANT_ID:
		"Identifier of instance where the app and resources reside. Can typically be found in Azure Active Directory > Overview > Directory ID",
	OBOT_AZURE_OPENAI_MODEL_PROVIDER_SUBSCRIPTION_ID:
		"Identifier of user's Azure subscription. Can typically be found in Azure Portal > Subscriptions > Overview.",
	OBOT_AZURE_OPENAI_MODEL_PROVIDER_RESOURCE_GROUP:
		"Container that holds related Azure resources. Can typically be found in Azure Portal > Resource Groups > [OpenAI Resource Group] > Overview",

	// Ollama
	OBOT_OLLAMA_MODEL_PROVIDER_HOST:
		"IP Address for the ollama server (eg. 127.0.0.1:1234)",

	// Groq
	OBOT_GROQ_MODEL_PROVIDER_API_KEY:
		"Groq API Key. Can be created and fetched from https://console.groq.com/keys",

	// vLLM
	OBOT_VLLM_MODEL_PROVIDER_ENDPOINT:
		"Endpoint for the vLLM OpenAI service (eg. http://localhost:8000)",
	OBOT_VLLM_MODEL_PROVIDER_API_KEY:
		"VLLM API Key set when starting the vLLM server",

	// DeepSeek
	OBOT_DEEPSEEK_MODEL_PROVIDER_API_KEY:
		"DeepSeek API Key. Can be created and fetched from https://platform.deepseek.com/api_keys",

	// Anthropic Bedrock
	OBOT_ANTHROPIC_BEDROCK_MODEL_PROVIDER_ACCESS_KEY_ID: "AWS Access Key ID",
	OBOT_ANTHROPIC_BEDROCK_MODEL_PROVIDER_SECRET_ACCESS_KEY:
		"AWS Secret Access Key",
	OBOT_ANTHROPIC_BEDROCK_MODEL_PROVIDER_SESSION_TOKEN: "AWS Session Token",
	OBOT_ANTHROPIC_BEDROCK_MODEL_PROVIDER_REGION:
		"AWS Region - make sure that the models you want to use are available in this region: https://docs.aws.amazon.com/bedrock/latest/userguide/models-regions.html",

	// Gemini Vertex
	OBOT_GEMINI_VERTEX_MODEL_PROVIDER_GOOGLE_CREDENTIALS_JSON:
		"Google Cloud Account Credentials - JSON File Contents: https://cloud.google.com/iam/docs/keys-create-delete#creating",
	OBOT_GEMINI_VERTEX_MODEL_PROVIDER_GOOGLE_CLOUD_PROJECT:
		"Google Cloud Project ID",

	// Generic OpenAI
	OBOT_GENERIC_OPENAI_MODEL_PROVIDER_BASE_URL:
		"Base URL for the OpenAI-compatible API, e.g. http://localhost:1234/v1",
	OBOT_GENERIC_OPENAI_MODEL_PROVIDER_API_KEY:
		"API Key for the OpenAI-compatible API. Some providers like Ollama don't enforce Authentication, so this is optional.",
};

export const ModelProviderSensitiveFields: Record<string, boolean | undefined> =
	{
		// OpenAI
		OBOT_OPENAI_MODEL_PROVIDER_API_KEY: true,

		// Azure OpenAI
		OBOT_AZURE_OPENAI_MODEL_PROVIDER_ENDPOINT: false,
		OBOT_AZURE_OPENAI_MODEL_PROVIDER_CLIENT_ID: false,
		OBOT_AZURE_OPENAI_MODEL_PROVIDER_CLIENT_SECRET: true,
		OBOT_AZURE_OPENAI_MODEL_PROVIDER_TENANT_ID: false,
		OBOT_AZURE_OPENAI_MODEL_PROVIDER_SUBSCRIPTION_ID: false,
		OBOT_AZURE_OPENAI_MODEL_PROVIDER_RESOURCE_GROUP: false,

		// Anthropic
		OBOT_ANTHROPIC_MODEL_PROVIDER_API_KEY: true,

		// Voyage
		OBOT_VOYAGE_MODEL_PROVIDER_API_KEY: true,

		// Ollama
		OBOT_OLLAMA_MODEL_PROVIDER_HOST: true,

		// Groq
		OBOT_GROQ_MODEL_PROVIDER_API_KEY: true,

		// VLLM
		OBOT_VLLM_MODEL_PROVIDER_ENDPOINT: false,
		OBOT_VLLM_MODEL_PROVIDER_API_KEY: true,

		// Anthropic Bedrock
		OBOT_ANTHROPIC_BEDROCK_MODEL_PROVIDER_ACCESS_KEY_ID: true,
		OBOT_ANTHROPIC_BEDROCK_MODEL_PROVIDER_SECRET_ACCESS_KEY: true,
		OBOT_ANTHROPIC_BEDROCK_MODEL_PROVIDER_SESSION_TOKEN: true,
		OBOT_ANTHROPIC_BEDROCK_MODEL_PROVIDER_REGION: false,

		// xAI
		OBOT_XAI_MODEL_PROVIDER_API_KEY: true,

		// DeepSeek
		OBOT_DEEPSEEK_MODEL_PROVIDER_API_KEY: true,

		// Gemini Vertex
		OBOT_GEMINI_VERTEX_MODEL_PROVIDER_GOOGLE_CREDENTIALS_JSON: true,
		OBOT_GEMINI_VERTEX_MODEL_PROVIDER_GOOGLE_CLOUD_PROJECT: false,

		// Generic OpenAI
		OBOT_GENERIC_OPENAI_MODEL_PROVIDER_BASE_URL: false,
		OBOT_GENERIC_OPENAI_MODEL_PROVIDER_API_KEY: true,
	};

export const CommonAuthProviderIds = {
	GOOGLE: "google-auth-provider",
	GITHUB: "github-auth-provider",
};

export const CommonAuthProviderFriendlyNames: Record<string, string> = {
	"google-auth-provider": "Google",
	"github-auth-provider": "GitHub",
};

export const AuthProviderLinks = {
	[CommonAuthProviderIds.GOOGLE]: "https://google.com",
	[CommonAuthProviderIds.GITHUB]: "https://github.com",
};

export const AuthProviderTooltips: {
	[key: string]: string;
} = {
	// All
	OBOT_AUTH_PROVIDER_EMAIL_DOMAINS:
		"Comma separated list of email domains that are allowed to authenticate with this provider. * is a special value that allows all domains.",

	// Google
	OBOT_GOOGLE_AUTH_PROVIDER_CLIENT_ID:
		"Unique identifier for the application when using Google's OAuth. Can typically be found in Google Cloud Console > Credentials",
	OBOT_GOOGLE_AUTH_PROVIDER_CLIENT_SECRET:
		"Password or key that app uses to authenticate with Google's OAuth. Can typically be found in Google Cloud Console > Credentials",
	OBOT_GOOGLE_AUTH_PROVIDER_COOKIE_SECRET:
		"Secret used to encrypt cookies. Must be a random string of length 16, 24, or 32.",

	// GitHub
	OBOT_GITHUB_AUTH_PROVIDER_CLIENT_ID:
		"Client ID for your GitHub OAuth app. Can be found in GitHub Developer Settings > OAuth Apps",
	OBOT_GITHUB_AUTH_PROVIDER_CLIENT_SECRET:
		"Client secret for your GitHub OAuth app. Can be found in GitHub Developer Settings > OAuth Apps",
	OBOT_GITHUB_AUTH_PROVIDER_COOKIE_SECRET:
		"Secret used to encrypt cookies. Must be a random string of length 16, 24, or 32.",
	// GitHub - Optional
	OBOT_GITHUB_AUTH_PROVIDER_TEAMS:
		"Restrict logins to members of any of these GitHub teams (comma-separated list).",
	OBOT_GITHUB_AUTH_PROVIDER_ORG:
		"Restrict logins to members of this GitHub organization.",
	OBOT_GITHUB_AUTH_PROVIDER_REPO:
		"Restrict logins to collaborators on this GitHub repository (formatted orgname/repo).",
	OBOT_GITHUB_AUTH_PROVIDER_TOKEN:
		"The token to use when verifying repository collaborators (must have push access to the repository).",
	OBOT_GITHUB_AUTH_PROVIDER_ALLOW_USERS:
		"Users allowed to log in, even if they do not belong to the specified org and team or collaborators.",
};

export const AuthProviderSensitiveFields: Record<string, boolean | undefined> =
	{
		// All
		OBOT_AUTH_PROVIDER_EMAIL_DOMAINS: false,

		// Google
		OBOT_GOOGLE_AUTH_PROVIDER_CLIENT_ID: false,
		OBOT_GOOGLE_AUTH_PROVIDER_CLIENT_SECRET: true,

		// GitHub
		OBOT_GITHUB_AUTH_PROVIDER_CLIENT_ID: false,
		OBOT_GITHUB_AUTH_PROVIDER_CLIENT_SECRET: true,
		OBOT_GITHUB_AUTH_PROVIDER_TEAMS: false,
		OBOT_GITHUB_AUTH_PROVIDER_ORG: false,
		OBOT_GITHUB_AUTH_PROVIDER_REPO: false,
		OBOT_GITHUB_AUTH_PROVIDER_TOKEN: true,
	};
