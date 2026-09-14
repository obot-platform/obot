import type { GuideHighlight, GuideListener } from '../types';

export const SIDEBAR_AI_RESOURCES_COLLAPSE = 'sidebar-collapse-ai-resources';
export const SIDEBAR_OPERATIONS_COLLAPSE = 'sidebar-collapse-operations';

export const SIDEBAR_MCP_SERVERS_LINK = 'sidebar-link-mcp-servers';
export const SIDEBAR_SKILLS_LINK = 'sidebar-link-skills';
export const SIDEBAR_INVENTORY_LINK = 'sidebar-link-inventory';

export const MCP_SERVERS_TAB_ACCESS_POLICIES = 'tab-access-policies';
export const MCP_SERVERS_TAB_FILTERS = 'tab-filters';

export const highlightMcpServersLink: GuideHighlight = {
	selector: {
		id: SIDEBAR_MCP_SERVERS_LINK
	},
	title: 'MCP Servers',
	description: 'This is where you can manage MCP servers.'
};

export const listenMcpServersLink: GuideListener = {
	id: SIDEBAR_MCP_SERVERS_LINK,
	action: {
		success: true
	}
};

export function getMcpServersTabHighlight(
	tabId: string,
	title: string,
	description: string
): GuideHighlight {
	return {
		selector: { id: tabId },
		side: 'bottom',
		title,
		description
	};
}

export function getMcpServersTabListener(
	tabId: string,
	next?: GuideListener['action']
): GuideListener {
	return {
		id: tabId,
		action: next ?? { success: true }
	};
}

export const highlightMcpAccessPoliciesTab = getMcpServersTabHighlight(
	MCP_SERVERS_TAB_ACCESS_POLICIES,
	'Access Policies',
	'Click here to manage MCP access policies.'
);

export const listenMcpAccessPoliciesTab = getMcpServersTabListener(MCP_SERVERS_TAB_ACCESS_POLICIES);

export const highlightMcpFiltersTab = getMcpServersTabHighlight(
	MCP_SERVERS_TAB_FILTERS,
	'Filters',
	'Click here to view MCP filters.'
);

export const listenMcpFiltersTab = getMcpServersTabListener(MCP_SERVERS_TAB_FILTERS);

export const addCatalogEntryDescriptions = {
	hosted:
		'A hosted MCP server allows you to add a custom MCP server that is managed and hosted by Obot. By having Obot host your MCP server, you can take advantage of lifecycle management, configuration management, and access policy enforcement. Once deployed in Obot, the server is available as a remote MCP URL that your MCP clients can consume.',
	remote:
		"A remote MCP server allows you to proxy any remote MCP server through Obot, enabling you to take advantage of Obot's access policies, audit logging, and static OAuth integration."
};

export const obotCatalogEntryDescriptions = {
	hosted:
		"A hosted MCP server provides a simple way to deploy and host an MCP server on the Obot platform, where Obot manages its operation and lifecycle. Let's continue through here.",
	remote:
		"A remote MCP server lets you proxy all traffic to a remote MCP server through Obot, enabling you to take advantage of Obot's access policies and audit logging. Let's continue through here."
};
