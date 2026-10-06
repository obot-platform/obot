import { CATALOG_SERVER_FIELD_IDS } from '$lib/constants';
import { m } from '$lib/i18n';
import { getExpandAdvancedPaneAction } from '../actions';
import type { GuideAction, GuideStep } from '../types';
import {
	highlightMcpServersLink,
	listenMcpServersLink,
	obotCatalogEntryDescriptions,
	SIDEBAR_AI_RESOURCES_COLLAPSE,
	SIDEBAR_MCP_SERVERS_LINK
} from './constants';

function getExpandAiResourcesAction({
	elementMissing,
	highlight,
	listener
}: {
	elementMissing: string;
	highlight?: GuideAction['highlight'];
	listener?: GuideAction['listener'];
}): GuideAction {
	return getExpandAdvancedPaneAction({
		elementMissing,
		highlight,
		listener,
		parentID: SIDEBAR_AI_RESOURCES_COLLAPSE,
		title: m.core_guide_expand_ai_resources(),
		description: m.mcps_guides_expand_ai_resources_to_continue()
	});
}

function getNavigateToMcpServersLinkAction(): GuideAction[] {
	return [
		{
			elementExists: SIDEBAR_MCP_SERVERS_LINK,
			highlight: highlightMcpServersLink,
			listener: listenMcpServersLink
		},
		getExpandAiResourcesAction({
			elementMissing: SIDEBAR_MCP_SERVERS_LINK,
			highlight: highlightMcpServersLink,
			listener: listenMcpServersLink
		})
	];
}

// shared steps that are used in mcp specific guides
export function getNavigateToMCPCatalogStep(): GuideStep {
	return {
		content: [m.mcps_guides_to_begin_let_s_head_to()],
		action: getNavigateToMcpServersLinkAction()
	};
}

export function getNavigateToMcpServersTabStep(
	tabId: string,
	tabTitle: string,
	tabDescription: string,
	content: string
): GuideStep {
	const tabHighlight = {
		selector: { id: tabId },
		side: 'bottom' as const,
		title: tabTitle,
		description: tabDescription
	};
	const tabListener = {
		id: tabId,
		action: { success: true }
	};
	const afterMcpServersLink = {
		highlight: tabHighlight,
		listener: tabListener
	};

	return {
		content: [content],
		action: [
			{
				elementExists: tabId,
				highlight: tabHighlight,
				listener: tabListener
			},
			{
				elementExists: SIDEBAR_MCP_SERVERS_LINK,
				elementMissing: tabId,
				highlight: highlightMcpServersLink,
				listener: {
					id: SIDEBAR_MCP_SERVERS_LINK,
					action: afterMcpServersLink
				}
			},
			getExpandAiResourcesAction({
				elementMissing: SIDEBAR_MCP_SERVERS_LINK,
				highlight: highlightMcpServersLink,
				listener: {
					id: SIDEBAR_MCP_SERVERS_LINK,
					action: afterMcpServersLink
				}
			})
		]
	};
}

export function getHighlightAddCatalogEntryStep(type: 'hosted' | 'remote'): GuideStep {
	const SECTION_ID = `add-${type}-server-button`;

	return {
		content: [
			type === 'hosted'
				? m.mcps_guides_create_manage_hosted_servers()
				: m.mcps_guides_create_manage_remote_servers()
		],
		action: {
			highlight: {
				selector: {
					id: 'add-catalog-entry-button'
				},
				title: m.mcps_add_mcp_server(),
				description: m.mcps_guides_this_is_where_you_can_create(),
				side: 'left'
			},
			listener: {
				id: 'add-catalog-entry-button',
				action: {
					highlight: {
						selector: {
							id: SECTION_ID
						},
						title:
							type === 'hosted'
								? m.mcps_guides_add_hosted_server()
								: m.mcps_guides_add_remote_server(),
						description: obotCatalogEntryDescriptions[type]
					},
					listener: {
						id: SECTION_ID,
						action: {
							success: true
						}
					}
				}
			}
		}
	};
}

export function getNavigateBasicCatalogEntryFieldsStep(): GuideStep {
	return {
		content: [m.mcps_guides_these_are_the_standard_fields_for()],
		action: {
			highlight: {
				selector: {
					id: `${CATALOG_SERVER_FIELD_IDS.serverFormDetails}`
				},
				side: 'top',
				align: 'center',
				title: m.mcps_guides_describe_your_mcp(),
				description: m.mcps_guides_this_is_where_you_provide_user()
			},
			listener: {
				id: `${CATALOG_SERVER_FIELD_IDS.serverFormDetails}`,
				action: {
					success: true
				}
			}
		}
	};
}
