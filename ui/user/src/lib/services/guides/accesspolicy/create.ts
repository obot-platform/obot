import { MCP_ACCESS_POLICY_FIELD_IDS } from '$lib/constants';
import { getExpandAdvancedPaneAction } from '../actions';
import {
	highlightMcpAccessPoliciesLink,
	listenMcpAccessPoliciesLink,
	SIDEBAR_MCP_ACCESS_POLICIES_LINK
} from '../mcp/constants';
import type { GuideStep } from '../types';

export const steps: GuideStep[] = [
	{
		content: [
			'**What is an MCP access policy?**',
			'An MCP access policy allows you to grant one or more users or user groups access to one or more MCP servers.'
		],
		action: [
			{
				elementExists: SIDEBAR_MCP_ACCESS_POLICIES_LINK,
				highlight: highlightMcpAccessPoliciesLink,
				listener: listenMcpAccessPoliciesLink
			},
			getExpandAdvancedPaneAction({
				elementMissing: SIDEBAR_MCP_ACCESS_POLICIES_LINK,
				highlight: highlightMcpAccessPoliciesLink,
				listener: listenMcpAccessPoliciesLink
			})
		]
	},
	{
		content: [
			'Create and manage your MCP access policies here. To start creating a new access policy, click the "Add Access Policy" button.'
		],
		action: {
			routeContains: 'mcp-access-policies',
			highlight: {
				selector: { id: MCP_ACCESS_POLICY_FIELD_IDS.addPolicyBtn },
				side: 'left',
				title: 'Add Access Policy',
				description: 'Click here to create a new MCP access policy.'
			},
			listener: {
				id: MCP_ACCESS_POLICY_FIELD_IDS.addPolicyBtn,
				action: { success: true }
			}
		}
	},
	{
		content: ["Let's go over the basic fields for the access policy."],
		action: {
			highlight: {
				selector: { id: MCP_ACCESS_POLICY_FIELD_IDS.name },
				side: 'top',
				title: 'Policy Name',
				description: 'Enter a recognizable name for this access policy.'
			},
			listener: {
				id: MCP_ACCESS_POLICY_FIELD_IDS.name,
				action: {
					highlight: {
						selector: { id: MCP_ACCESS_POLICY_FIELD_IDS.usersGroupsSection },
						side: 'top',
						title: 'Users & Groups',
						description:
							'Add the users and groups that should have access to the selected MCP servers.',
						noDescendantInteraction: true
					},
					listener: {
						id: MCP_ACCESS_POLICY_FIELD_IDS.usersGroupsSection,
						skipClickTargetOnNext: true,
						action: {
							highlight: {
								selector: { id: MCP_ACCESS_POLICY_FIELD_IDS.addUserGroupBtn },
								side: 'left',
								title: 'Add User/Group',
								description: 'Click here to add to User & Groups.'
							},
							listener: {
								id: MCP_ACCESS_POLICY_FIELD_IDS.addUserGroupBtn,
								action: {
									highlight: {
										selector: { id: 'add-user-group-dialog-content' },
										title: 'Adding Users/Groups',
										description:
											'This is where you can add who will be able to access the MCP servers.',
										noDescendantInteraction: true
									},
									listener: {
										id: 'add-user-group-dialog-content',
										skipClickTargetOnNext: true,
										action: {
											highlight: {
												selector: { id: MCP_ACCESS_POLICY_FIELD_IDS.allUsersOption },
												side: 'right',
												title: 'Select A User/Group',
												description:
													"For now, let's select All Obot Users. This will grant access to everyone."
											},
											listener: {
												id: MCP_ACCESS_POLICY_FIELD_IDS.allUsersOption,
												action: {
													highlight: {
														selector: { id: MCP_ACCESS_POLICY_FIELD_IDS.userGroupConfirmBtn },
														side: 'top',
														title: 'Confirm Selection',
														description: 'Click here to apply your changes.'
													},
													listener: {
														id: MCP_ACCESS_POLICY_FIELD_IDS.userGroupConfirmBtn,
														action: {
															highlight: {
																selector: { id: MCP_ACCESS_POLICY_FIELD_IDS.serversSection },
																side: 'top',
																title: 'Servers',
																description:
																	'Select the servers that will be available to the selected users and groups.',
																noDescendantInteraction: true
															},
															listener: {
																id: MCP_ACCESS_POLICY_FIELD_IDS.serversSection,
																skipClickTargetOnNext: true,
																action: {
																	highlight: {
																		selector: { id: MCP_ACCESS_POLICY_FIELD_IDS.addServerBtn },
																		side: 'left',
																		title: 'Add Server',
																		description: 'Click here to begin adding a server.'
																	},
																	listener: {
																		id: MCP_ACCESS_POLICY_FIELD_IDS.addServerBtn,
																		action: {
																			highlight: {
																				selector: { id: 'search-mcp-servers-dialog-content' },
																				title: 'Adding A Server',
																				description:
																					'From here, you can search and add any servers that you want to make available to the selected users and groups.',
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
																						title: 'Add a Server',
																						description:
																							"For this guide, let's go ahead and add everything. You can choose to modify this later."
																					},
																					listener: {
																						id: MCP_ACCESS_POLICY_FIELD_IDS.everythingOption,
																						action: {
																							highlight: {
																								selector: {
																									id: MCP_ACCESS_POLICY_FIELD_IDS.serverConfirmBtn
																								},
																								side: 'top',
																								title: 'Confirm Changes',
																								description: 'Click here to apply your changes.'
																							},
																							listener: {
																								id: MCP_ACCESS_POLICY_FIELD_IDS.serverConfirmBtn,
																								action: {
																									highlight: {
																										selector: {
																											id: MCP_ACCESS_POLICY_FIELD_IDS.saveBtn
																										},
																										side: 'left',
																										title: 'Save Access Policy',
																										description:
																											"Once you've finished configuring the access policy, you can save it here."
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
	title: 'Create MCP Access Policy',
	description: 'Grant users and groups access to MCP servers.',
	id: 'mcp-create-access-policy-guide'
};
