import { CATALOG_SERVER_FIELD_IDS } from '$lib/constants';
import type { GuideAction, GuideListener, GuideStep } from '../types';
import { addCatalogEntryDescriptions } from './constants';
import {
	getHighlightAddCatalogEntryStep,
	getNavigateBasicCatalogEntryFieldsStep,
	getNavigateToMCPCatalogStep
} from './steps';

function getCustomConfigurationAction(): GuideAction[] {
	const configurationHighlight = {
		selector: {
			id: CATALOG_SERVER_FIELD_IDS.configuration
		},
		side: 'top' as const,
		align: 'center' as const,
		title: 'Custom Configuration',
		noDescendantInteraction: true
	};

	const configurationListener = {
		id: CATALOG_SERVER_FIELD_IDS.configuration,
		action: {
			success: true
		}
	};

	return [
		{
			highlight: {
				...configurationHighlight,
				description:
					"If the MCP server requires any custom configuration such as API keys or secrets, you'll want to add them here. The user will have to provide their custom configuration when deploying the server."
			},
			listener: configurationListener
		}
	];
}

function getHostedFieldsListener(): GuideListener {
	return {
		id: CATALOG_SERVER_FIELD_IDS.runtime,
		action: {
			highlight: {
				selector: {
					id: CATALOG_SERVER_FIELD_IDS.runtimeConfiguration
				},
				side: 'top',
				align: 'center',
				title: 'Runtime Configuration',
				description:
					'Depending on which runtime you choose, you will see the appropriate form for that runtime here to fill out.',
				noDescendantInteraction: true
			},
			listener: {
				id: CATALOG_SERVER_FIELD_IDS.runtimeConfiguration,
				action: getCustomConfigurationAction()
			}
		}
	};
}

function getHostedFieldsAction(): GuideAction {
	return {
		highlight: {
			selector: {
				id: CATALOG_SERVER_FIELD_IDS.runtime
			},
			side: 'top',
			align: 'center',
			title: 'Runtime',
			description: 'This is where you choose the runtime configuration for your MCP server.',
			noDescendantInteraction: true
		},
		listener: getHostedFieldsListener()
	};
}

function getSubmitAction(): GuideAction {
	return {
		highlight: {
			selector: {
				id: CATALOG_SERVER_FIELD_IDS.submitBtn
			},
			side: 'left',
			title: 'Save the entry.',
			description: "Once you've filled out all necessary fields, you can save the entry here."
		},
		listener: {
			skipClickTargetOnNext: true,
			id: CATALOG_SERVER_FIELD_IDS.submitBtn,
			action: {
				success: true
			}
		}
	};
}

export const steps: GuideStep[] = [
	{
		content: ['**What is a hosted catalog entry?**', addCatalogEntryDescriptions.hosted]
	},
	getNavigateToMCPCatalogStep(),
	getHighlightAddCatalogEntryStep('hosted'),
	getNavigateBasicCatalogEntryFieldsStep(),
	{
		content: ["Now let's go over the hosted specific fields."],
		action: getHostedFieldsAction()
	},
	{
		content: [
			"Once you've properly filled out the form, you'll get access to additional tabs such as:",
			'**Server Details**: This is where you see the deployments related to the MCP server.',
			'**Tools**: This is where you can preview/set up the list of previewable tools for an MCP server that a user can see before deploying the server.',
			'**Audit Logs**: This is where you can see logs pertaining to the usage of the MCP server.',
			'**Usage**: This is where you can see usage metrics for the MCP server.',
			'**Access Policies**: This is where you can access policies pertaining to the MCP server.',
			'**Filters**: This is where you can see filters tied to the MCP server.'
		],
		action: [
			{
				elementExists: CATALOG_SERVER_FIELD_IDS.headers,
				highlight: {
					selector: {
						id: CATALOG_SERVER_FIELD_IDS.headers
					},
					side: 'top',
					align: 'center',
					title: 'User-Defined Headers',
					description:
						'These allow you to collect configuration information from users connecting to your server and inject them as HTTP headers. You can use this to, for example, inject an API key as a bearer token header.',
					noDescendantInteraction: true
				},
				listener: {
					id: CATALOG_SERVER_FIELD_IDS.headers,
					action: getSubmitAction()
				}
			},
			{
				elementMissing: CATALOG_SERVER_FIELD_IDS.headers,
				...getSubmitAction()
			}
		]
	}
];

export default {
	steps,
	title: 'Host MCP Server w/ Obot',
	description: 'Add a hosted MCP server to the catalog.',
	id: 'mcp-create-hosted-guide'
};
