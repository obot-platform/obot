import { MDM_DEVICES_CONFIGURATION_FIELD_IDS } from '$lib/constants';
import { m } from '$lib/i18n';
import { getExpandAdvancedPaneAction } from '../actions';
import { SIDEBAR_OPERATIONS_COLLAPSE } from '../mcp/constants';
import type { GuideAction, GuideStep } from '../types';

function getInventoryTabAction(
	tabId: string,
	title: string,
	description: string,
	next: GuideAction | GuideAction[]
): GuideAction {
	return {
		highlight: {
			selector: { id: tabId },
			side: 'left',
			title,
			description
		},
		listener: {
			id: tabId,
			skipClickTargetOnNext: true,
			action: next
		}
	};
}

function getEnforcementEventsAction(): GuideAction[] {
	const highlight = {
		selector: { id: MDM_DEVICES_CONFIGURATION_FIELD_IDS.enforcementEventsLink },
		side: 'right' as const,
		title: m.inventory_enforcement_enforcement_events_enforcement_events(),
		description: m.inventory_enforcement_guides_when_enforcement_is_enabled_and_tool()
	};
	const listener = {
		id: MDM_DEVICES_CONFIGURATION_FIELD_IDS.enforcementEventsLink,
		skipClickTargetOnNext: true,
		action: { success: true }
	};

	return [
		{
			elementExists: MDM_DEVICES_CONFIGURATION_FIELD_IDS.enforcementEventsLink,
			highlight,
			listener
		},
		getExpandAdvancedPaneAction({
			elementMissing: MDM_DEVICES_CONFIGURATION_FIELD_IDS.enforcementEventsLink,
			highlight,
			listener,
			parentID: SIDEBAR_OPERATIONS_COLLAPSE,
			title: m.inventory_enforcement_guides_expand_operations(),
			description: m.inventory_enforcement_guides_expand_operations_to_access_enforcement_events()
		})
	];
}

const highlightInventoryLink = {
	selector: {
		id: MDM_DEVICES_CONFIGURATION_FIELD_IDS.devicesLink
	},
	title: m.inventory_enforcement_guides_inventory(),
	description: m.inventory_enforcement_guides_this_is_where_you_can_manage_2()
};

const listenInventoryLink = {
	id: MDM_DEVICES_CONFIGURATION_FIELD_IDS.devicesLink,
	action: {
		success: true
	}
};

export const steps: GuideStep[] = [
	{
		content: [
			m.inventory_enforcement_guides_in_order_to_discover_shadow_ai(),
			m.inventory_enforcement_guides_what_is_obot_sentry(),
			m.inventory_enforcement_guides_to_get_set_up_let_s()
		],
		action: [
			{
				elementExists: MDM_DEVICES_CONFIGURATION_FIELD_IDS.devicesLink,
				highlight: highlightInventoryLink,
				listener: listenInventoryLink
			},
			getExpandAdvancedPaneAction({
				elementMissing: MDM_DEVICES_CONFIGURATION_FIELD_IDS.devicesLink,
				highlight: highlightInventoryLink,
				listener: listenInventoryLink,
				parentID: SIDEBAR_OPERATIONS_COLLAPSE,
				title: m.inventory_enforcement_guides_expand_operations(),
				description: m.inventory_enforcement_guides_expand_operations_to_access_inventory()
			})
		]
	},
	{
		content: [m.inventory_enforcement_guides_the_configuration_tab_contains_all_the()],
		action: [
			{
				routeContains: '/inventory',
				elementExists: MDM_DEVICES_CONFIGURATION_FIELD_IDS.getStartedButton,
				highlight: {
					selector: {
						id: MDM_DEVICES_CONFIGURATION_FIELD_IDS.getStartedButton
					},
					side: 'left' as const,
					title: m.inventory_enforcement_guides_configure_managed_devices(),
					description: m.inventory_enforcement_guides_begin_creating_the_initial_managed_device()
				},
				listener: {
					id: MDM_DEVICES_CONFIGURATION_FIELD_IDS.getStartedButton,
					action: {
						success: true
					}
				}
			},
			{
				routeContains: '/inventory',
				elementMissing: MDM_DEVICES_CONFIGURATION_FIELD_IDS.getStartedButton,
				elementExists: MDM_DEVICES_CONFIGURATION_FIELD_IDS.configurationTab,
				highlight: {
					selector: {
						id: MDM_DEVICES_CONFIGURATION_FIELD_IDS.configurationTab
					},
					side: 'right' as const,
					title: m.inventory_enforcement_guides_open_configuration(),
					description: m.inventory_enforcement_guides_select_the_configuration_tab_to_install()
				},
				listener: {
					id: MDM_DEVICES_CONFIGURATION_FIELD_IDS.configurationTab,
					action: {
						success: true
					}
				}
			}
		]
	},
	{
		content: [m.inventory_enforcement_guides_we_ll_take_you_through_the()],
		action: {
			highlight: {
				selector: {
					id: MDM_DEVICES_CONFIGURATION_FIELD_IDS.enrollmentConfigSetup
				},
				side: 'top',
				title: m.inventory_enforcement_guides_installation_setup(),
				description: m.inventory_enforcement_guides_follow_the_steps_here_to_install(),
				noDescendantInteraction: true
			},
			listener: {
				id: MDM_DEVICES_CONFIGURATION_FIELD_IDS.enrollmentConfigSetup,
				skipClickTargetOnNext: true,
				action: {
					highlight: {
						selector: {
							id: `${MDM_DEVICES_CONFIGURATION_FIELD_IDS.enrollmentConfigSetupStep}-1`
						},
						side: 'top',
						title: m.inventory_enforcement_guides_generate_enrollment_key(),
						description: m.inventory_enforcement_guides_first_things_first_you_ll_need()
					},
					listener: {
						id: `${MDM_DEVICES_CONFIGURATION_FIELD_IDS.enrollmentConfigSetupStep}-1`,
						skipClickTargetOnNext: true,

						action: {
							highlight: {
								selector: {
									id: `${MDM_DEVICES_CONFIGURATION_FIELD_IDS.enrollmentConfigSetupStep}-2`
								},
								side: 'top',
								title: m.inventory_enforcement_guides_installation_method(),
								description:
									m.inventory_enforcement_guides_select_the_appropriate_installation_method_for()
							},
							listener: {
								id: `${MDM_DEVICES_CONFIGURATION_FIELD_IDS.enrollmentConfigSetupStep}-2`,
								skipClickTargetOnNext: true,
								action: {
									highlight: {
										selector: {
											id: `${MDM_DEVICES_CONFIGURATION_FIELD_IDS.enrollmentConfigSetupStep}-3`
										},
										side: 'top',
										title: m.inventory_enforcement_guides_operating_system(),
										description: m.inventory_enforcement_guides_make_sure_to_select_the_proper()
									},
									listener: {
										id: `${MDM_DEVICES_CONFIGURATION_FIELD_IDS.enrollmentConfigSetupStep}-3`,
										skipClickTargetOnNext: true,
										action: {
											success: true
										}
									}
								}
							}
						}
					}
				}
			}
		}
	},
	{
		content: [
			m.inventory_enforcement_guides_for_more_detailed_instructions_here_s(),
			{
				videoUrl: 'https://youtu.be/NwuQlU5WpK0',
				title: m.inventory_enforcement_guides_installing_obot_sentry_w_intune()
			}
		],
		action: {
			highlight: {
				selector: {
					id: `${MDM_DEVICES_CONFIGURATION_FIELD_IDS.enrollmentConfigSetupStep}-5`
				},
				side: 'top',
				title: m.inventory_enforcement_guides_os_type_specific_instructions(),
				description: m.inventory_enforcement_guides_depending_on_the_installation_type_and()
			},
			listener: {
				id: `${MDM_DEVICES_CONFIGURATION_FIELD_IDS.enrollmentConfigSetupStep}-5`,
				action: {
					highlight: {
						selector: {
							id: MDM_DEVICES_CONFIGURATION_FIELD_IDS.enrollmentKeysSection
						},
						side: 'top',
						title: m.inventory_enforcement_configuration_enrollment_keys(),
						description: m.inventory_enforcement_guides_once_you_ve_set_up_an(),
						noDescendantInteraction: true
					},
					listener: {
						id: MDM_DEVICES_CONFIGURATION_FIELD_IDS.enrollmentKeysSection,
						skipClickTargetOnNext: true,
						action: {
							highlight: {
								selector: {
									id: MDM_DEVICES_CONFIGURATION_FIELD_IDS.toolCallEnforcementSection
								},
								side: 'top',
								title: m.inventory_enforcement_configuration_tool_call_enforcement(),
								description: m.inventory_enforcement_guides_here_you_can_control_which_tool(),
								experimental: true
							},
							listener: {
								id: MDM_DEVICES_CONFIGURATION_FIELD_IDS.toolCallEnforcementSection,
								action: {
									success: true
								}
							}
						}
					}
				}
			}
		}
	},
	{
		content: [m.inventory_enforcement_guides_once_obot_sentry_has_been_installed()],
		action: getInventoryTabAction(
			MDM_DEVICES_CONFIGURATION_FIELD_IDS.devicesTabOverview,
			m.inventory_enforcement_overview_tab(),
			m.inventory_enforcement_guides_view_an_overall_summary_of_scans(),
			getInventoryTabAction(
				MDM_DEVICES_CONFIGURATION_FIELD_IDS.devicesTabDevices,
				m.inventory_enforcement_devices_tab(),
				m.inventory_enforcement_guides_view_results_for_an_individual_device(),
				getInventoryTabAction(
					MDM_DEVICES_CONFIGURATION_FIELD_IDS.inventoryTabDeviceMcpServers,
					m.inventory_enforcement_device_mcp_servers_tab(),
					m.inventory_enforcement_guides_browse_mcp_servers_discovered_across_your(),
					getEnforcementEventsAction()
				)
			)
		)
	}
];

export default {
	steps,
	title: m.inventory_enforcement_guides_discover_shadow_ai_enforce_policies_for(),
	description: m.inventory_enforcement_guides_install_obot_sentry_on_devices_to(),
	id: 'devices-install-sentry-guide'
};
