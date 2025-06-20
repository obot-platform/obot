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

export const UNAUTHORIZED_PATHS = new Set(['/', '/privacy-policy', '/terms-of-service']);
