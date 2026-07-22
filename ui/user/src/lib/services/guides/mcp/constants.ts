import type { GuideHighlight, GuideListener } from '../types';

export const SIDEBAR_MCP_CATALOG_LINK = 'sidebar-link-mcp-catalog';
export const SIDEBAR_MCP_ACCESS_POLICIES_LINK = 'sidebar-link-mcp-access-policies';

export const highlightMcpCatalogLink: GuideHighlight = {
	selector: {
		id: SIDEBAR_MCP_CATALOG_LINK
	},
	title: 'MCP Catalog',
	description: 'Click here to view MCP server catalog.'
};

export const listenMcpCatalogLink: GuideListener = {
	id: SIDEBAR_MCP_CATALOG_LINK,
	action: {
		success: true
	}
};

export const highlightMcpAccessPoliciesLink: GuideHighlight = {
	selector: {
		id: SIDEBAR_MCP_ACCESS_POLICIES_LINK
	},
	title: 'MCP Access Policies',
	description: 'Click here to view MCP access policies.'
};

export const listenMcpAccessPoliciesLink: GuideListener = {
	id: SIDEBAR_MCP_ACCESS_POLICIES_LINK,
	action: {
		success: true
	}
};

export const addCatalogEntryDescriptions = {
	hosted:
		'A hosted catalog entry is great for setting up an MCP server hosted under the Obot platform.',
	remote:
		'A remote catalog entry is great for allowing users to connect to MCP servers that are already elsewhere. When they deploy from Obot, the MCP server will go through the gateway.',
	composite:
		'A composite catalog entry is great for combining tools from multiple existing MCP servers into single deployment. You can also configure which tools to expose and customize their names or descriptions.'
};
