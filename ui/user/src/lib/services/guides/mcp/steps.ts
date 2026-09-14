import { CATALOG_SERVER_FIELD_IDS } from '$lib/constants';
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
		title: 'Expand AI Resources',
		description: 'Expand AI Resources to continue.'
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
		content: ["To begin, let's head to the MCP Servers page under AI Resources."],
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
	const toCapitalize = (str: string) => str.charAt(0).toUpperCase() + str.slice(1);

	return {
		content: [
			`Create and manage your MCP servers here. We'll take you through creating a new ${type} MCP server.`
		],
		action: {
			highlight: {
				selector: {
					id: 'add-catalog-entry-button'
				},
				title: 'Add MCP Server',
				description:
					'This is where you can create and select what type of MCP server you want to add.',
				side: 'left'
			},
			listener: {
				id: 'add-catalog-entry-button',
				action: {
					highlight: {
						selector: {
							id: SECTION_ID
						},
						title: `Add ${toCapitalize(type)} Server`,
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
		content: ['These are the standard fields for an MCP server.'],
		action: {
			highlight: {
				selector: {
					id: `${CATALOG_SERVER_FIELD_IDS.serverFormDetails}`
				},
				side: 'top',
				align: 'center',
				title: 'Describe Your MCP',
				description:
					'This is where you provide user friendly details about your MCP server; the information here is displayed to users when previewing the MCP server.'
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
