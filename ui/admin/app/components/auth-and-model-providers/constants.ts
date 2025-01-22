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
};

export const RecommendedModelProviders = [
	CommonModelProviderIds.OPENAI,
	CommonModelProviderIds.AZURE_OPENAI,
];

export const ModelProviderRequiredTooltips: {
	[key: string]: {
		[key: string]: string;
	};
} = {
	[CommonModelProviderIds.OLLAMA]: {
		Host: "IP Address for the ollama server (eg. 127.0.0.1:1234)",
	},
	[CommonModelProviderIds.GROQ]: {
		"Api Key":
			"Groq API Key. Can be created and fetched from https://console.groq.com/keys",
	},
	[CommonModelProviderIds.VLLM]: {
		Endpoint:
			"Endpoint for the vLLM OpenAI service (eg. http://localhost:8000)",
		"Api Key": "VLLM API Key set when starting the vLLM server",
	},
	[CommonModelProviderIds.DEEPSEEK]: {
		"Api Key":
			"DeepSeek API Key. Can be created and fetched from https://platform.deepseek.com/api_keys",
	},
	[CommonModelProviderIds.AZURE_OPENAI]: {
		Endpoint:
			"Endpoint for the Azure OpenAI service (e.g. https://<resource-name>.<region>.api.cognitive.microsoft.com/)",
		"Client Id":
			"Unique identifier for the application when using Azure Active Directory. Can typically be found in App Registrations > [application].",
		"Client Secret":
			"Password or key that app uses to authenticate with Azure Active Directory. Can typically be found in App Registrations > [application] > Certificates & Secrets",
		"Tenant Id":
			"Identifier of instance where the app and resources reside. Can typically be found in Azure Active Directory > Overview > Directory ID",
		"Subscription Id":
			"Identifier of user's Azure subscription. Can typically be found in Azure Portal > Subscriptions > Overview.",
		"Resource Group":
			"Container that holds related Azure resources. Can typically be found in Azure Portal > Resource Groups > [OpenAI Resource Group] > Overview",
	},
	[CommonModelProviderIds.ANTHROPIC_BEDROCK]: {
		"Access Key ID": "AWS Access Key ID",
		"Secret Access Key": "AWS Secret Access Key",
		"Session Token": "AWS Session Token",
		Region:
			"AWS Region - make sure that the models you want to use are available in this region: https://docs.aws.amazon.com/bedrock/latest/userguide/models-regions.html",
	},
	[CommonModelProviderIds.GEMINI_VERTEX]: {
		"Google Credentials JSON":
			"Google Cloud Account Credentials - JSON File Contents: https://cloud.google.com/iam/docs/keys-create-delete#creating",
		"Google Cloud Project": "Google Cloud Project ID",
	},
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

export const AuthProviderRequiredTooltips: {
	[key: string]: {
		[key: string]: string;
	};
} = {
	[CommonAuthProviderIds.GOOGLE]: {
		"Client Id":
			"Unique identifier for the application when using Google's OAuth. Can typically be found in Google Cloud Console > Credentials",
		"Client Secret":
			"Password or key that app uses to authenticate with Google's OAuth. Can typically be found in Google Cloud Console > Credentials",
		"Cookie Secret":
			"Secret used to encrypt cookies. Must be a random string of length 16, 24, or 32.",
		"Email Domains":
			"Comma separated list of email domains that are allowed to authenticate with this provider. * is a special value that allows all domains.",
	},
	[CommonAuthProviderIds.GITHUB]: {
		"Client Id":
			"Client ID for your GitHub OAuth app. Can be found in GitHub Developer Settings > OAuth Apps",
		"Client Secret":
			"Client secret for your GitHub OAuth app. Can be found in GitHub Developer Settings > OAuth Apps",
		"Cookie Secret":
			"Secret used to encrypt cookies. Must be a random string of length 16, 24, or 32.",
		"Email Domains":
			"Comma separated list of email domains that are allowed to authenticate with this provider. * is a special value that allows all domains.",
	},
};

export const AuthProviderOptionalTooltips: {
	[key: string]: {
		[key: string]: string;
	};
} = {
	[CommonAuthProviderIds.GITHUB]: {
		Teams:
			"Restrict logins to members of any of these GitHub teams (comma-separated list).",
		Org: "Restrict logins to members of this GitHub organization.",
		Repo: "Restrict logins to collaborators on this GitHub repository (formatted orgname/repo).",
		Token:
			"The token to use when verifying repository collaborators (must have push access to the repository).",
		"Allow Users":
			"Users allowed to log in, even if they do not belong to the specified org and team or collaborators.",
	},
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
