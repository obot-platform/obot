import { CATALOG_SERVER_FIELD_IDS } from '$lib/constants';
import type { GuideAction, GuideStep } from '../types';
import {
	getHighlightAddCatalogEntryStep,
	getNavigateBasicCatalogEntryFieldsStep,
	getNavigateToMCPCatalogStep
} from './steps';

function getSubmitAction(): GuideAction {
	return {
		highlight: {
			selector: { id: CATALOG_SERVER_FIELD_IDS.submitBtn },
			side: 'left',
			title: 'Save the entry',
			description: 'Once you have finished configuring the remote MCP server, you can save here.'
		},
		listener: {
			id: CATALOG_SERVER_FIELD_IDS.submitBtn,
			skipClickTargetOnNext: true,
			action: { success: true }
		}
	};
}

function getStaticOAuthAction(): GuideAction {
	return {
		highlight: {
			selector: { id: CATALOG_SERVER_FIELD_IDS.remoteStaticOAuth },
			side: 'top',
			align: 'center',
			title: 'Static OAuth',
			description:
				'Enable this only when the remote MCP server requires a pre-registered OAuth app. The entry will need to be saved first, then the shared client ID and secret can be configured; each user will still need to complete their own OAuth login.',
			noDescendantInteraction: true
		},
		listener: {
			id: CATALOG_SERVER_FIELD_IDS.remoteStaticOAuth,
			skipClickTargetOnNext: true,
			action: getSubmitAction()
		}
	};
}

function getAdvancedFieldsAction(): GuideAction[] {
	return [
		{
			elementExists: CATALOG_SERVER_FIELD_IDS.remoteConnection,
			highlight: {
				selector: { id: CATALOG_SERVER_FIELD_IDS.remoteConnection },
				side: 'top',
				align: 'center',
				title: 'Connection Restriction',
				description:
					'Choose an exact URL, allow a user-configured URL on one hostname, or build a URL from a template. You can also route requests through an MCP tunnel when tunnels are available.',
				noDescendantInteraction: true
			},
			listener: {
				id: CATALOG_SERVER_FIELD_IDS.remoteConnection,
				skipClickTargetOnNext: true,
				action: [
					{
						elementExists: CATALOG_SERVER_FIELD_IDS.remoteStaticOAuth,
						...getStaticOAuthAction()
					},
					{
						elementMissing: CATALOG_SERVER_FIELD_IDS.remoteStaticOAuth,
						...getSubmitAction()
					}
				]
			}
		},
		{
			elementMissing: CATALOG_SERVER_FIELD_IDS.remoteConnection,
			...getSubmitAction()
		}
	];
}

export const steps: GuideStep[] = [
	{
		content: [
			'**What is a remote MCP server?**',
			'A remote MCP server is great for allowing users to connect to MCP servers that are already elsewhere. When they deploy from Obot, the MCP server will go through the gateway.'
		]
	},
	getNavigateToMCPCatalogStep(),
	getHighlightAddCatalogEntryStep('remote'),
	getNavigateBasicCatalogEntryFieldsStep(),
	{
		content: ["Now let's go over the remote specific fields."],
		action: {
			highlight: {
				selector: { id: CATALOG_SERVER_FIELD_IDS.remoteURL },
				side: 'top',
				align: 'center',
				title: 'Remote Server URL',
				description:
					'Enter the full URL of the remote MCP server. Use Advanced Configuration below if you need hostname restrictions, URL templates, tunnels, or static OAuth.'
			},
			listener: {
				id: CATALOG_SERVER_FIELD_IDS.remoteURL,
				action: {
					highlight: {
						selector: { id: CATALOG_SERVER_FIELD_IDS.remoteAdvancedBtn },
						side: 'top',
						title: 'Advanced Configuration',
						description:
							'Open this to restrict connections by hostname or URL template, route through an MCP tunnel, or enable static OAuth. Skip this and save if a fixed URL is all you need.'
					},
					listener: {
						id: CATALOG_SERVER_FIELD_IDS.remoteAdvancedBtn,
						skipClickTargetOnNext: true,
						action: getAdvancedFieldsAction()
					}
				}
			}
		}
	}
];

export default {
	steps,
	title: 'Reroute a Remote MCP Through Obot',
	description: 'Add auditing & governance to an existing MCP server.',
	id: 'mcp-create-remote-guide'
};
