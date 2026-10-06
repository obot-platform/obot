import { m } from '$lib/i18n';
import type { VMcpSortBy, VMcpStatusFilter } from './types';

const SETTINGS_BUTTON_ID = 'mcp-server-settings-button';
const SETTINGS_PANEL_ID = 'mcp-server-settings-panel';
const FILTER_LABEL_ID = 'mcp-server-filter-by-label';

export const VMCP_IDS = {
	SETTINGS_BUTTON_ID,
	SETTINGS_PANEL_ID,
	FILTER_LABEL_ID
};

export const WIRE_SEGMENTS = 26;
export const WIRE_MAX_ARCH_PX = 72;

export const COMPONENT_LABEL_SEPARATOR = ', ';
export const SHORT_DESCRIPTION_MAX_LENGTH = 160;

// camera constants
export const MIN_ZOOM = 0.25;
export const MAX_ZOOM = 1.5;
export const ZOOM_STEP = 1.2;
export const VMCP_CARD_HEIGHT = 236;
export const VMCP_COMPONENT_HEIGHT = 148;
export const VMCP_COMPONENT_WINDOW_THRESHOLD = 15;
export const VMCP_OVERSCAN_ROWS = 2;

// virtualized vmcp list constants
export const VIRTUAL_LIST_THRESHOLD = 40;
export const VIRTUAL_LIST_OVERSCAN = 6;
export const ESTIMATED_ROW_HEIGHT = 68;
export const MIN_VIEWPORT_HEIGHT = 800;

/**
 * Temporary popularity ranking derived from
 * https://github.com/obot-platform/mcp-catalog (obot-remotes, remotes, obot-images).
 * First-party Obot remotes first, then widely used third-party remotes, then Obot images.
 * Unlisted servers sort after these, alphabetically. Will want to replace by downloads.
 */
export const MCP_SERVER_POPULARITY_ORDER = [
	'gmail',
	'outlook',
	'calendar',
	'google calendar',
	'google drive',
	'onedrive',
	'google docs',
	'google sheets',
	'excel',
	'word',
	'contact',
	'google search console',
	'slack',
	'github',
	'github enterprise cloud',
	'atlassian',
	'notion',
	'linear',
	'hubspot',
	'salesforce',
	'stripe',
	'datadog',
	'grafana cloud',
	'elasticsearch',
	'firecrawl',
	'monday.com',
	'asana',
	'clickup',
	'zapier',
	'todoist',
	'pagerduty',
	'context7',
	'exa search',
	'tavily search',
	'neon',
	'supabase',
	'snowflake',
	'bigquery',
	'cloudflare',
	'sentry',
	'intercom',
	'paypal',
	'dropbox',
	'box',
	'airtable',
	'canva',
	'zoom',
	'playwright',
	'gitlab',
	'grafana',
	'azure',
	'aws',
	'redis',
	'postgresql',
	'mongodb atlas management',
	'mongodb database',
	'brave search',
	'duckduckgo search',
	'browserbase',
	'wordpress',
	'terraform',
	'github enterprise'
] as const;

export const VMCP_FILTER_OWNER_NONE = '__none__';

export const VMCP_SORT_OPTIONS: Array<{ id: VMcpSortBy; label: string }> = [
	{ id: 'name', label: m.core_name() },
	{ id: 'created', label: m.core_col_created() },
	{ id: 'componentServers', label: m.vmcps_mcp_servers() }
];

export const VMCP_STATUS_FILTER_OPTIONS: Array<{ id: VMcpStatusFilter; label: string }> = [
	{ id: 'needs-update', label: m.vmcps_status_needs_update() },
	{ id: 'not-configured', label: m.core_mcp_value_not_configured() },
	{ id: 'connected', label: m.core_mcp_value_connected() },
	{ id: 'not-connected', label: m.vmcps_status_not_connected() }
];
