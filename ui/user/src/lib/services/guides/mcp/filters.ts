import { MCP_FILTERS_FIELD_IDS } from '$lib/constants';
import { m } from '$lib/i18n';
import type { GuideStep } from '../types';
import { MCP_SERVERS_TAB_FILTERS } from './constants';
import { getNavigateToMcpServersTabStep } from './steps';

export const steps: GuideStep[] = [
	{
		content: [
			m.mcps_filters_guide_what_is_an_mcp_filter(),
			m.mcps_filters_guide_an_mcp_filter_is_a_way()
		]
	},
	getNavigateToMcpServersTabStep(
		MCP_SERVERS_TAB_FILTERS,
		m.core_filters_title(),
		m.mcps_guide_click_here_to_view_mcp_filters(),
		m.mcps_filters_guide_let_s_head_to_the_filters()
	),
	{
		content: [m.mcps_filters_guide_create_and_manage_your_mcp_filters()],
		action: {
			highlight: {
				selector: {
					id: MCP_FILTERS_FIELD_IDS.addFilterBtn
				},
				title: m.mcps_add_new_filter(),
				description: m.mcps_filters_guide_this_is_where_you_can_add(),
				side: 'left'
			},
			listener: {
				id: MCP_FILTERS_FIELD_IDS.addFilterBtn,
				action: {
					highlight: {
						selector: {
							id: MCP_FILTERS_FIELD_IDS.createCustomBtn
						},
						title: m.mcps_filters_guide_create_custom_filter(),
						description: m.mcps_filters_guide_obot_also_supports_out_of_the(),
						side: 'left'
					},
					listener: {
						id: MCP_FILTERS_FIELD_IDS.createCustomBtn,
						action: {
							success: true
						}
					}
				}
			}
		}
	},
	{
		content: [m.mcps_filters_guide_now_let_s_go_over_the_3()],
		action: {
			highlight: {
				selector: {
					id: MCP_FILTERS_FIELD_IDS.basicDetails
				},
				side: 'top',
				align: 'center',
				title: m.mcps_filters_guide_basic_details(),
				description: m.mcps_filters_guide_this_is_where_you_can_enter()
			},
			listener: {
				id: MCP_FILTERS_FIELD_IDS.basicDetails,
				action: {
					highlight: {
						selector: {
							id: MCP_FILTERS_FIELD_IDS.runtimeSelector
						},
						side: 'top',
						align: 'center',
						title: m.mcps_filters_guide_runtime_type(),
						description: m.mcps_filters_guide_filters_can_be_implemented_via_an()
					},
					listener: {
						id: MCP_FILTERS_FIELD_IDS.runtimeSelector,
						skipClickTargetOnNext: true,
						action: {
							highlight: {
								selector: {
									id: MCP_FILTERS_FIELD_IDS.runtimeFormDetails
								},
								side: 'top',
								align: 'center',
								title: m.mcps_filters_guide_runtime_form_details(),
								description: m.mcps_filters_guide_depending_on_the_runtime_type_selected(),
								noDescendantInteraction: true
							},
							listener: {
								id: MCP_FILTERS_FIELD_IDS.runtimeFormDetails,
								skipClickTargetOnNext: true,
								action: {
									highlight: {
										selector: {
											id: MCP_FILTERS_FIELD_IDS.filterSelectors
										},
										side: 'top',
										align: 'center',
										title: m.mcps_filters_guide_selectors(),
										description: m.mcps_filters_guide_this_is_where_you_specify_which(),
										noDescendantInteraction: true
									},
									listener: {
										id: MCP_FILTERS_FIELD_IDS.filterSelectors,
										skipClickTargetOnNext: true,
										action: {
											highlight: {
												selector: {
													id: MCP_FILTERS_FIELD_IDS.filterMcpServers
												},
												side: 'top',
												align: 'center',
												title: m.mcps_filters_mcp_servers(),
												description: m.mcps_filters_guide_select_the_mcp_servers_that_will(),
												noDescendantInteraction: true
											},
											listener: {
												id: MCP_FILTERS_FIELD_IDS.filterMcpServers,
												skipClickTargetOnNext: true,
												action: {
													highlight: {
														selector: {
															id: MCP_FILTERS_FIELD_IDS.saveBtn
														},
														side: 'left',
														title: m.mcps_filters_guide_save_the_filter(),
														description: m.mcps_filters_guide_once_you_ve_filled_out_all_2(),
														noDescendantInteraction: true
													},
													listener: {
														id: MCP_FILTERS_FIELD_IDS.saveBtn,
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
					}
				}
			}
		}
	}
];

export default {
	steps,
	title: m.mcps_filters_guide_filtering_and_controlling_mcp_tool_calls(),
	description: m.mcps_filters_guide_add_additional_security_policies_to_your(),
	id: 'mcp-create-filter-guide'
};
