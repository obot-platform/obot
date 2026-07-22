import { CATALOG_SERVER_FIELD_IDS } from '$lib/constants';
import type { GuideStep } from '../types';
import { addCatalogEntryDescriptions } from './constants';
import {
	getHighlightAddCatalogEntryStep,
	getNavigateBasicCatalogEntryFieldsStep,
	getNavigateToMCPCatalogStep
} from './steps';

export const steps: GuideStep[] = [
	{
		content: ['What is a composite catalog entry?', addCatalogEntryDescriptions.composite]
	},
	getNavigateToMCPCatalogStep(),
	getHighlightAddCatalogEntryStep('composite'),
	getNavigateBasicCatalogEntryFieldsStep(),
	{
		content: [
			'Composite servers present tools from multiple existing MCP servers through one catalog entry.',
			"Let's go through the process of adding a component entry to the composite server."
		],
		action: {
			highlight: {
				selector: { id: CATALOG_SERVER_FIELD_IDS.compositeEntries },
				side: 'top',
				align: 'center',
				title: 'Component Entries',
				description:
					'This list contains the catalog entries and deployed multi-user servers whose tools will be combined into the composite server.'
			},
			next: {
				action: {
					highlight: {
						selector: { id: CATALOG_SERVER_FIELD_IDS.addCompositeEntryBtn },
						side: 'top',
						align: 'center',
						title: 'Add Component Entry',
						description: 'Click here to begin adding a component entry to the composite server.'
					},
					listener: {
						id: CATALOG_SERVER_FIELD_IDS.addCompositeEntryBtn,
						action: {
							highlight: {
								selector: {
									id: `${CATALOG_SERVER_FIELD_IDS.compositeEntrySearchMcpServersDialog}-content`
								},
								side: 'right',
								title: 'Adding Component Entries',
								description:
									'Here you can add the component entries that will be combined into the composite server.'
							},
							listener: {
								id: `${CATALOG_SERVER_FIELD_IDS.compositeEntrySearchMcpServersDialog}-content`,
								skipClickTargetOnNext: true,
								action: {
									highlight: {
										selector: {
											beginsWith: ['search-mcp-server-default-antv-charts']
										},
										title: 'Add a Component Entry',
										description:
											"For this example, let's go ahead and select AntV Charts as a component entry."
									},
									listener: {
										beginsWith: ['search-mcp-server-default-antv-charts'],
										action: {
											highlight: {
												selector: {
													id: CATALOG_SERVER_FIELD_IDS.compositeEntryConfigureToolsBtn
												},
												side: 'right',
												title: 'Configure Tools',
												description:
													"Let's go ahead and configure the tools for this component entry."
											},
											listener: {
												id: CATALOG_SERVER_FIELD_IDS.compositeEntryConfigureToolsBtn,
												action: {
													highlight: {
														selector: {
															id: CATALOG_SERVER_FIELD_IDS.compositeEntryConfigureToolsGetStartedBtn
														},
														title: 'Get Started',
														description:
															"In order to get the tools, we'll need to supply any required configuration to deploy the entry. Similarly, when a user deploys this composite server, they'll also have to provide the required configuration details for each component entry."
													},
													listener: {
														id: CATALOG_SERVER_FIELD_IDS.compositeEntryConfigureToolsGetStartedBtn,
														action: {
															highlight: {
																selector: {
																	id: 'mcp-catalog-configure-submit-btn'
																},
																side: 'left',
																title: 'Configure the Entry',
																description:
																	'Luckily, this entry does not have any required configuration. Go ahead and click Continue.'
															},
															listener: {
																id: 'mcp-catalog-configure-submit-btn',
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
		}
	},
	{
		content: ['__Waiting for tools to be fetched...__'],
		action: {
			waitFor: CATALOG_SERVER_FIELD_IDS.compositeEntryEditToolsDialog
		}
	},
	{
		content: [
			'Each component keeps its own configuration and authentication requirements. You can also choose which tools to expose and customize their names or descriptions.'
		],
		action: {
			highlight: {
				selector: { id: CATALOG_SERVER_FIELD_IDS.compositeEntryEditToolsDialog },
				title: 'Configuring Entry Tools',
				description:
					'This is the tool configuration dialog for the component entry that was just added. From here, you can choose which tools to expose and customize their names or descriptions.'
			},
			listener: {
				id: CATALOG_SERVER_FIELD_IDS.compositeEntryEditToolsDialog,
				skipClickTargetOnNext: true,
				action: {
					highlight: {
						selector: { id: CATALOG_SERVER_FIELD_IDS.compositeEntryConfigureToolsToggleAll },
						side: 'bottom',
						align: 'center',
						title: 'Toggle All Tools',
						description:
							'If you want to enable or disable all tools at once, you can use this toggle.'
					},
					listener: {
						id: CATALOG_SERVER_FIELD_IDS.compositeEntryConfigureToolsToggleAll,
						skipClickTargetOnNext: true,
						action: {
							highlight: {
								selector: {
									beginsWith: ['edit-tool-']
								},
								side: 'right',
								title: 'Toggle a Tool',
								description:
									'You choose individually which tools to expose and customize their names or descriptions.'
							},
							listener: {
								beginsWith: ['edit-tool-'],
								skipClickTargetOnNext: true,
								action: {
									highlight: {
										selector: {
											id: CATALOG_SERVER_FIELD_IDS.compositeEntryConfigureToolsConfirmBtn
										},
										side: 'left',
										title: 'Save Tools',
										description:
											"Once you have configured the tools, you can confirm the changes by clicking the button here. For now, let's just confirm the changes."
									},
									listener: {
										id: CATALOG_SERVER_FIELD_IDS.compositeEntryConfigureToolsConfirmBtn,
										action: {
											highlight: {
												selector: {
													beginsWith: [`${CATALOG_SERVER_FIELD_IDS.compositeEntryToolCollapseBtn}-`]
												},
												side: 'left',
												title: 'See Added Tools & Update',
												description:
													'If you want to make additional changes, you can expand the tools you just added and edit them from here. Go ahead and expand this to see.'
											},
											listener: {
												beginsWith: [`${CATALOG_SERVER_FIELD_IDS.compositeEntryToolCollapseBtn}-`],
												skipClickTargetOnNext: true,
												action: {
													highlight: {
														selector: { id: CATALOG_SERVER_FIELD_IDS.submitBtn },
														side: 'left',
														title: 'Save the entry',
														description:
															'Once you have finished configuring the composite catalog entry, you can save here.'
													},
													listener: {
														id: CATALOG_SERVER_FIELD_IDS.submitBtn,
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
];

export default {
	steps,
	title: 'Create a Composite MCP Server',
	description:
		'Combine multiple existing MCP servers & apply fine-grained access control on their tools.',
	id: 'mcp-create-composite-guide'
};
