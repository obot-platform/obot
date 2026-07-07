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
