import { CATALOG_SERVER_FIELD_IDS } from '$lib/constants';
import { m } from '$lib/i18n';
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
		title: m.mcps_servers_guide_custom_configuration(),
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
				description: m.mcps_servers_guide_if_the_mcp_server_requires_any()
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
				title: m.mcps_servers_guide_runtime_configuration(),
				description: m.mcps_servers_guide_depending_on_which_runtime_you_choose(),
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
			title: m.mcps_catalog_runtime_heading(),
			description: m.mcps_servers_guide_this_is_where_you_choose_the(),
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
			title: m.mcps_servers_guide_save_the_entry(),
			description: m.mcps_servers_guide_once_you_ve_filled_out_all()
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
		content: [
			m.mcps_servers_guide_what_is_a_hosted_catalog_entry(),
			addCatalogEntryDescriptions.hosted
		]
	},
	getNavigateToMCPCatalogStep(),
	getHighlightAddCatalogEntryStep('hosted'),
	getNavigateBasicCatalogEntryFieldsStep(),
	{
		content: [m.mcps_servers_guide_now_let_s_go_over_the()],
		action: getHostedFieldsAction()
	},
	{
		content: [
			m.mcps_servers_guide_once_you_ve_properly_filled_out(),
			m.mcps_servers_guide_server_details_this_is_where_you(),
			m.mcps_servers_guide_tools_this_is_where_you_can(),
			m.mcps_servers_guide_audit_logs_this_is_where_you(),
			m.mcps_servers_guide_usage_this_is_where_you_can(),
			m.mcps_servers_guide_access_policies_this_is_where_you(),
			m.mcps_servers_guide_filters_this_is_where_you_can()
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
					title: m.mcps_catalog_headers_title(),
					description: m.mcps_servers_guide_these_allow_you_to_collect_configuration(),
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
	title: m.mcps_servers_guide_host_mcp_server_w_obot(),
	description: m.mcps_servers_guide_add_a_hosted_mcp_server_to(),
	id: 'mcp-create-hosted-guide'
};
