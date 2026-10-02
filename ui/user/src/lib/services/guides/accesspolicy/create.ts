import { MCP_ACCESS_POLICY_FIELD_IDS } from '$lib/constants';
import { m } from '$lib/i18n';
import { MCP_SERVERS_TAB_ACCESS_POLICIES } from '../mcp/constants';
import { getNavigateToMcpServersTabStep } from '../mcp/steps';
import type { GuideStep } from '../types';

export const steps: GuideStep[] = [
	{
		content: [
			m.core_guide_what_is_an_mcp_access_policy(),
			m.core_guide_an_mcp_access_policy_allows_you()
		]
	},
	getNavigateToMcpServersTabStep(
		MCP_SERVERS_TAB_ACCESS_POLICIES,
		m.core_guide_access_policies(),
		m.core_guide_click_here_to_manage_mcp_access(),
		m.core_guide_let_s_head_to_the_access()
	),
	{
		content: [m.core_guide_create_and_manage_your_mcp_access()],
		action: {
			routeContains: '/mcp-servers',
			highlight: {
				selector: { id: MCP_ACCESS_POLICY_FIELD_IDS.addPolicyBtn },
				side: 'left',
				title: m.core_guide_add_access_policy(),
				description: m.core_guide_this_is_where_you_go_to()
			},
			listener: {
				id: MCP_ACCESS_POLICY_FIELD_IDS.addPolicyBtn,
				action: { success: true }
			}
		}
	},
	{
		content: [m.core_guide_let_s_go_over_the_basic()],
		action: {
			highlight: {
				selector: { id: MCP_ACCESS_POLICY_FIELD_IDS.name },
				side: 'top',
				title: m.core_guide_policy_name(),
				description: m.core_guide_this_is_where_you_enter_a()
			},
			listener: {
				id: MCP_ACCESS_POLICY_FIELD_IDS.name,
				action: {
					highlight: {
						selector: { id: MCP_ACCESS_POLICY_FIELD_IDS.usersGroupsSection },
						side: 'top',
						title: m.core_guide_users_groups(),
						description: m.core_guide_here_is_where_you_can_add(),
						noDescendantInteraction: true
					},
					listener: {
						id: MCP_ACCESS_POLICY_FIELD_IDS.usersGroupsSection,
						skipClickTargetOnNext: true,
						action: {
							highlight: {
								selector: { id: MCP_ACCESS_POLICY_FIELD_IDS.addUserGroupBtn },
								side: 'left',
								title: m.core_guide_add_user_group(),
								description: m.core_guide_you_can_add_user_and_groups()
							},
							listener: {
								id: MCP_ACCESS_POLICY_FIELD_IDS.addUserGroupBtn,
								action: {
									highlight: {
										selector: { id: 'add-user-group-dialog-content' },
										title: m.core_guide_adding_users_groups(),
										description: m.core_guide_clicking_it_will_open_this_dialog(),
										noDescendantInteraction: true
									},
									listener: {
										id: 'add-user-group-dialog-content',
										skipClickTargetOnNext: true,
										action: {
											highlight: {
												selector: { id: MCP_ACCESS_POLICY_FIELD_IDS.allUsersOption },
												side: 'right',
												title: m.core_guide_select_a_user_group(),
												description: m.core_guide_for_now_we_ll_select_all()
											},
											listener: {
												id: MCP_ACCESS_POLICY_FIELD_IDS.allUsersOption,
												action: {
													highlight: {
														selector: { id: MCP_ACCESS_POLICY_FIELD_IDS.userGroupConfirmBtn },
														side: 'top',
														title: m.core_guide_confirm_selection(),
														description: m.core_guide_then_you_can_apply_your_changes()
													},
													listener: {
														id: MCP_ACCESS_POLICY_FIELD_IDS.userGroupConfirmBtn,
														action: {
															highlight: {
																selector: { id: MCP_ACCESS_POLICY_FIELD_IDS.serversSection },
																side: 'top',
																title: m.core_guide_servers(),
																description: m.core_guide_this_is_where_you_can_select(),
																noDescendantInteraction: true
															},
															listener: {
																id: MCP_ACCESS_POLICY_FIELD_IDS.serversSection,
																skipClickTargetOnNext: true,
																action: {
																	highlight: {
																		selector: { id: MCP_ACCESS_POLICY_FIELD_IDS.addServerBtn },
																		side: 'left',
																		title: m.core_guide_add_server(),
																		description: m.core_guide_you_can_add_a_server_from()
																	},
																	listener: {
																		id: MCP_ACCESS_POLICY_FIELD_IDS.addServerBtn,
																		action: {
																			highlight: {
																				selector: { id: 'search-mcp-servers-dialog-content' },
																				title: m.core_guide_adding_a_server(),
																				description:
																					m.core_guide_clicking_it_will_open_this_dialog_2(),
																				noDescendantInteraction: true
																			},
																			listener: {
																				id: 'search-mcp-servers-dialog-content',
																				skipClickTargetOnNext: true,
																				action: {
																					highlight: {
																						selector: {
																							id: MCP_ACCESS_POLICY_FIELD_IDS.everythingOption
																						},
																						side: 'right',
																						title: m.core_guide_add_a_server(),
																						description: m.core_guide_for_this_guide_we_ll_go()
																					},
																					listener: {
																						id: MCP_ACCESS_POLICY_FIELD_IDS.everythingOption,
																						action: {
																							highlight: {
																								selector: {
																									id: MCP_ACCESS_POLICY_FIELD_IDS.serverConfirmBtn
																								},
																								side: 'top',
																								title: m.core_guide_confirm_changes(),
																								description:
																									m.core_guide_then_you_can_apply_your_changes()
																							},
																							listener: {
																								id: MCP_ACCESS_POLICY_FIELD_IDS.serverConfirmBtn,
																								action: {
																									highlight: {
																										selector: {
																											id: MCP_ACCESS_POLICY_FIELD_IDS.saveBtn
																										},
																										side: 'left',
																										title: m.core_guide_save_access_policy(),
																										description:
																											m.core_guide_once_you_ve_finished_configuring_the()
																									},
																									listener: {
																										id: MCP_ACCESS_POLICY_FIELD_IDS.saveBtn,
																										skipClickTargetOnNext: true,
																										action: { success: true }
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
	title: m.core_guide_create_mcp_access_policy(),
	description: m.core_guide_grant_users_and_groups_access_to(),
	id: 'mcp-create-access-policy-guide'
};
