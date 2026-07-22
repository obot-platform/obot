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
			routeContains: 'admin',
			highlight: {
				...configurationHighlight,
				description:
					"If the MCP server requires any custom configuration such as API keys or secrets, you'll want to add them here. For multi-tenancy, you can supply configuration that'll be used by all users or set up it so each user can supply their own. In single-tenancy, the user will have to provide their custom configuration when deploying the server."
			},
			listener: configurationListener
		},
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
		id: CATALOG_SERVER_FIELD_IDS.tenancy,
		action: {
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
			listener: {
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
			}
		}
	};
}

function getHostedFieldsActions(admin: boolean): GuideAction {
	return {
		...(admin ? { routeContains: 'admin' } : {}),
		highlight: {
			selector: {
				id: CATALOG_SERVER_FIELD_IDS.tenancy
			},
			side: 'top',
			align: 'center',
			title: 'Server Tenancy',
			description: admin
				? 'This is where you choose the tenancy type for your MCP server. The default is multi-tenant, which allows multiple users to access the same MCP server. If your MCP server is intended for a single user or isolated deployment, select single-tenant instead.'
				: 'For any catalog entry you create, a user will deploy their own instance of the MCP server.',
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
		action: [getHostedFieldsActions(true), getHostedFieldsActions(false)]
	},
	{
		content: [
			"Once you've properly filled out the form, you'll get access to additional tabs such as:",
			'**Server Details**: This is where you see the deployments related to the catalog entry.',
			'**Tools**: This is where you can preview/set up the list of previewable tools for a catalog entry that a user can see before deploying the server.',
			'**Audit Logs**: This is where you can see logs pertaining to the usage of the catalog entry.',
			'**Usage**: This is where you can see usage metrics for the catalog entry.',
			'**Access Policies**: This is where you can access policies pertaining to the catalog entry.',
			'**Filters**: This is where you can see filters tied to the catalog entry.'
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
					title: 'Headers',
					description: 'Add any headers that the MCP server requires.',
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
