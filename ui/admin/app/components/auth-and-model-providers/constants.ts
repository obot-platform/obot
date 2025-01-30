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

export const RecommendedModelProviders = [
	CommonModelProviderIds.OPENAI,
	CommonModelProviderIds.AZURE_OPENAI,
];

export const CommonAuthProviderIds = {
	GOOGLE: "google-auth-provider",
	GITHUB: "github-auth-provider",
	OKTA: "okta-auth-provider",
};

export const CommonAuthProviderFriendlyNames: Record<string, string> = {
	"google-auth-provider": "Google",
	"github-auth-provider": "GitHub",
	"okta-auth-provider": "Okta",
};

export const AuthProviderLinks = {
	[CommonAuthProviderIds.GOOGLE]: "https://google.com",
	[CommonAuthProviderIds.GITHUB]: "https://github.com",
	[CommonAuthProviderIds.OKTA]: "https://okta.com",
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

	// Okta
	OBOT_OKTA_AUTH_PROVIDER_CLIENT_ID:
		"Client ID for your Okta OAuth app. Can be found in Okta Developer Console > Applications",
	OBOT_OKTA_AUTH_PROVIDER_CLIENT_SECRET:
		"Client secret for your Okta OAuth app. Can be found in Okta Developer Console > Applications",
	OBOT_OKTA_AUTH_PROVIDER_ISSUER_URL:
		"Issuer URL for Okta. Should be https://{your-okta-domain}/oauth2/{authorization-server}",
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

		// Okta
		OBOT_OKTA_AUTH_PROVIDER_CLIENT_ID: false,
		OBOT_OKTA_AUTH_PROVIDER_CLIENT_SECRET: true,
		OBOT_OKTA_AUTH_PROVIDER_ISSUER_URL: false,
	};
