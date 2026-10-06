import { m } from '$lib/i18n';
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
	title: m.mcps_filters_mcp_servers(),
	description: m.mcps_guides_this_is_where_you_can_manage()
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
	m.mcps_access_policies_tab(),
	m.mcps_guide_click_here_to_manage_mcp_access()
);

export const listenMcpAccessPoliciesTab = getMcpServersTabListener(MCP_SERVERS_TAB_ACCESS_POLICIES);

export const highlightMcpFiltersTab = getMcpServersTabHighlight(
	MCP_SERVERS_TAB_FILTERS,
	m.core_filters_title(),
	m.mcps_guide_click_here_to_view_mcp_filters()
);

export const listenMcpFiltersTab = getMcpServersTabListener(MCP_SERVERS_TAB_FILTERS);

export const addCatalogEntryDescriptions = {
	hosted: m.mcps_guides_a_hosted_mcp_server_allows_you(),
	remote: m.mcps_guides_a_remote_mcp_server_allows_you()
};

export const obotCatalogEntryDescriptions = {
	hosted: m.mcps_guides_a_hosted_mcp_server_provides_a(),
	remote: m.mcps_guides_a_remote_mcp_server_lets_you()
};
