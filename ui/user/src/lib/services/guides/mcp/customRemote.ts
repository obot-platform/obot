import { CATALOG_SERVER_FIELD_IDS } from '$lib/constants';
import { m } from '$lib/i18n';
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
			title: m.mcps_servers_guide_save_the_entry_2(),
			description: m.mcps_servers_guide_once_you_have_finished_configuring_the()
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
			title: m.mcps_catalog_remote_remote_static_oauth(),
			description: m.mcps_servers_guide_enable_this_only_when_the_remote(),
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
				title: m.mcps_servers_guide_connection_restriction(),
				description: m.mcps_servers_guide_choose_an_exact_url_allow_a(),
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
			m.mcps_servers_guide_what_is_a_remote_mcp_server(),
			m.mcps_servers_guide_a_remote_mcp_server_is_great()
		]
	},
	getNavigateToMCPCatalogStep(),
	getHighlightAddCatalogEntryStep('remote'),
	getNavigateBasicCatalogEntryFieldsStep(),
	{
		content: [m.mcps_servers_guide_now_let_s_go_over_the_2()],
		action: {
			highlight: {
				selector: { id: CATALOG_SERVER_FIELD_IDS.remoteURL },
				side: 'top',
				align: 'center',
				title: m.mcps_servers_guide_remote_server_url(),
				description: m.mcps_servers_guide_enter_the_full_url_of_the()
			},
			listener: {
				id: CATALOG_SERVER_FIELD_IDS.remoteURL,
				action: {
					highlight: {
						selector: { id: CATALOG_SERVER_FIELD_IDS.remoteAdvancedBtn },
						side: 'top',
						title: m.mcps_catalog_remote_remote_advanced(),
						description: m.mcps_servers_guide_open_this_to_restrict_connections_by()
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
	title: m.mcps_servers_guide_reroute_a_remote_mcp_through_obot(),
	description: m.mcps_servers_guide_add_auditing_governance_to_an_existing(),
	id: 'mcp-create-remote-guide'
};
