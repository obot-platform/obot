import { CATALOG_SERVER_FIELD_IDS } from '$lib/constants';
import { getExpandAdvancedPaneAction } from '../actions';
import type { GuideStep } from '../types';
import {
	highlightMcpCatalogLink,
	listenMcpCatalogLink,
	SIDEBAR_MCP_CATALOG_LINK,
	addCatalogEntryDescriptions
} from './constants';

// shared steps that are used in mcp specific guides
export function getNavigateToMCPCatalogStep(): GuideStep {
	return {
		content: ["To begin, let's head to the MCP Catalog page!"],
		action: [
			{
				elementExists: SIDEBAR_MCP_CATALOG_LINK,
				highlight: highlightMcpCatalogLink,
				listener: listenMcpCatalogLink
			},
			getExpandAdvancedPaneAction({
				elementMissing: SIDEBAR_MCP_CATALOG_LINK,
				highlight: highlightMcpCatalogLink,
				listener: listenMcpCatalogLink
			}),
			{
				highlight: {
					selector: {
						id: 'advanced-pane-btn'
					},
					title: 'Advanced Pane',
					description:
						'Click here to open the advanced pane; this section contains more advanced settings and management capabilities such as audit logs.'
				},
				listener: {
					id: 'advanced-pane-btn',
					action: [
						{
							elementExists: SIDEBAR_MCP_CATALOG_LINK,
							highlight: highlightMcpCatalogLink,
							listener: listenMcpCatalogLink
						},
						getExpandAdvancedPaneAction({
							elementMissing: SIDEBAR_MCP_CATALOG_LINK,
							highlight: highlightMcpCatalogLink,
							listener: listenMcpCatalogLink
						})
					]
				}
			}
		]
	};
}

export function getHighlightAddCatalogEntryStep(
	type: 'hosted' | 'remote' | 'composite'
): GuideStep {
	const SECTION_ID = `add-${type}-server-button`;
	const toCapitalize = (str: string) => str.charAt(0).toUpperCase() + str.slice(1);

	return {
		content: [
			'Create and manage your MCP catalog entries here. To start creating a custom entry, click the "Add Catalog Entry" button.'
		],
		action: {
			highlight: {
				selector: {
					id: 'add-catalog-entry-button'
				},
				title: 'Add Catalog Entry',
				description: 'Click here to start adding a new catalog entry.',
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
						description: `${addCatalogEntryDescriptions[type]} Click here and let's go through creating one now.`
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
		content: ['These are the standard fields for a catalog entry.'],
		action: {
			highlight: {
				selector: {
					id: `${CATALOG_SERVER_FIELD_IDS.serverFormDetails}`
				},
				side: 'top',
				align: 'center',
				title: 'Describe Your MCP',
				description:
					'This is where you provide user friendly details about your MCP server; the information here is displayed to users when previewing the catalog entry.'
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
