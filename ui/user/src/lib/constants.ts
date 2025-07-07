export const DEFAULT_PROJECT_NAME = 'My Agent';
export const DEFAULT_PROJECT_DESCRIPTION = 'Do more with AI';
export const DEFAULT_CUSTOM_SERVER_NAME = 'My Custom Server';

export const ABORTED_THREAD_MESSAGE = 'thread was aborted, cancelling run';
export const ABORTED_BY_USER_MESSAGE = 'aborted by user';

export const IGNORED_BUILTIN_TOOLS = new Set([
	'workspace-files',
	'tasks',
	'knowledge',

	'time',
	'threads',
	'github-com-obot-platform-tools-search-tavily-websiteknowl-d2d96'
]);

export const MCP_LIST_ORDER = [
	'github-bundle',
	'gitlab-bundle',
	'firecrawl',
	'postgres',
	'atlassian-jira-bundle',
	'aws-ec2-bundle',
	'pagerduty-bundle',
	'wordpress-bundle',
	'obot-search',
	'slack-bundle'
];

export const FEATURED_AGENT_PREFERRED_ORDER = [
	'google productivity assistant',
	'microsoft productivity assistant',
	'github productivity assistant',
	'wordpress blog assistant',
	'linkedin research assistant'
];

export const UNAUTHORIZED_PATHS = new Set([
	'/',
	'/privacy-policy',
	'/terms-of-service',
	'/v2/admin'
]);

export const PAGE_TRANSITION_DURATION = 200;

export const CommonModelProviderIds = {
	OLLAMA: 'ollama-model-provider',
	GROQ: 'groq-model-provider',
	VLLM: 'vllm-model-provider',
	VOYAGE: 'voyage-model-provider',
	ANTHROPIC: 'anthropic-model-provider',
	OPENAI: 'openai-model-provider',
	AZURE_OPENAI: 'azure-openai-model-provider',
	ANTHROPIC_BEDROCK: 'anthropic-bedrock-model-provider',
	XAI: 'xai-model-provider',
	DEEPSEEK: 'deepseek-model-provider',
	GEMINI_VERTEX: 'gemini-vertex-model-provider',
	GENERIC_OPENAI: 'generic-openai-model-provider'
};

export const RecommendedModelProviders = [
	CommonModelProviderIds.OPENAI,
	CommonModelProviderIds.AZURE_OPENAI
];

export const PROJECT_MCP_SERVER_NAME = 'MCP Servers';
export const DEFAULT_MCP_CATALOG_ID = 'default';
export const CommonAuthProviderIds = {
	GOOGLE: 'google-auth-provider',
	GITHUB: 'github-auth-provider',
	OKTA: 'okta-auth-provider',
	ENTRA: 'entra-auth-provider'
} as const;

export const BOOTSTRAP_USER_ID = 'bootstrap';

export const ADMIN_SESSION_STORAGE = {
	ACCESS_CONTROL_RULE_CREATION: 'access-control-rule-creation',
	LAST_VISITED_MCP_SERVER: 'last-visited-mcp-server'
} as const;
