export const DEFAULT_PROJECT_NAME = 'My Obot';
export const DEFAULT_PROJECT_DESCRIPTION = 'Do more with AI';

export const ABORTED_THREAD_MESSAGE = 'thread was aborted, cancelling run';
export const ABORTED_BY_USER_MESSAGE = 'aborted by user';

export const IGNORED_BUILTIN_TOOLS = new Set([
	'workspace-files',
	'tasks',
	'knowledge',
	'database',
	'time',
	'threads',
	'github-com-obot-platform-tools-search-tavily-websiteknowl-d2d96'
]);

export const MCP_LIST_ORDER = [
	'google-search-bundle',
	'outlook-mail-bundle',
	'wordpress-bundle',
	'github-bundle'
];

export const FEATURED_AGENT_PREFERRED_ORDER = [
	'google productivity assistant',
	'microsoft productivity assistant',
	'github productivity assistant',
	'wordpress blog assistant',
	'smartthings helper'
];
