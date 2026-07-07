import type { GuideStep } from '../types';
import { getExpandAdvancedPaneAction } from '../utils';
import {
	highlightMcpCatalogLink,
	listenMcpCatalogLink,
	SIDEBAR_MCP_CATALOG_LINK
} from './constants';

export const steps: GuideStep[] = [
	{
		content: ["To begin, let's head to the MCP Catalog page!"],
		button: {
			text: 'Where is MCP Catalog?',
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
		}
	},
	{
		content: [
			'Click the Add Catalog Entry button to start creating a new entry.',
			'For the purpose of this guide, let us create a hosted server entry.'
		],
		button: {
			text: 'Where do I go?',
			action: {
				highlight: {
					selector: {
						id: 'add-catalog-entry-button'
					},
					title: 'Add Catalog Entry',
					description: 'Click here to add a new catalog entry.'
				},
				listener: {
					id: 'add-catalog-entry-button',
					action: {
						highlight: {
							selector: {
								id: 'add-hosted-server-button'
							},
							title: 'Add Hosted Server',
							description: 'Click here to add a hosted server entry.'
						},
						listener: {
							id: 'add-hosted-server-button',
							action: {
								success: true
							}
						}
					}
				}
			}
		}
	},
	{
		content: [
			"We're going to use the Everything server as an example. Name and package are required, but feel free to fill out the rest of the form.",
			"For 'Package', use the following: @modelcontextprotocol/server-everything",
			'Go ahead and click Save.'
		],
		button: {
			text: "I'm done, what next?",
			action: {
				success: true
			}
		}
	},
	{
		content: [
			'Great! You should see more options available to you now. If your server is deployed, check out the deployment details in Server Details.',
			'For single-tenant catalog entries, it is recommended to populate the tools with an example set for users to see. This can be done in Tools.',
			"And that's it! You've completed the guide on creating a custom MCP server."
		]
	}
];

export default {
	steps,
	title: 'Create Custom MCP Server',
	id: 'mcp-create-custom-guide'
};
