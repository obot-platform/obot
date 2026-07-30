import { MCP_FILTERS_FIELD_IDS } from '$lib/constants';
import { getExpandAdvancedPaneAction } from '../actions';
import type { GuideStep } from '../types';
import { SIDEBAR_MCP_FILTERS_LINK } from './constants';

const highlightMcpFiltersLink = {
	selector: {
		id: SIDEBAR_MCP_FILTERS_LINK
	},
	side: 'right' as const,
	title: 'MCP Filters',
	description: 'Click here to view MCP filters.'
};

const listenMcpFiltersLink = {
	id: SIDEBAR_MCP_FILTERS_LINK,
	action: {
		success: true
	}
};

export const steps: GuideStep[] = [
	{
		content: [
			'**What is an MCP filter?**',
			'An MCP filter is a way to add additional security policies to your MCP. They can be used for inspecting and controlling tool calls and their results in the MCP Gateway. They provide administrators with the ability to implement custom validation, logging, security checks, or other business logic by intercepting tool requests and responses before they are processed.'
		]
	},
	{
		content: ["Let's head to the MCP Filters page."],
		action: [
			{
				elementExists: SIDEBAR_MCP_FILTERS_LINK,
				highlight: highlightMcpFiltersLink,
				listener: listenMcpFiltersLink
			},
			getExpandAdvancedPaneAction({
				elementMissing: SIDEBAR_MCP_FILTERS_LINK,
				highlight: highlightMcpFiltersLink,
				listener: listenMcpFiltersLink,
				parentID: 'sidebar-collapse-mcp-server-management'
			})
		]
	},
	{
		content: ['Create and manage your MCP filters here. To start, click the "Add Filter" button.'],
		action: {
			highlight: {
				selector: {
					id: MCP_FILTERS_FIELD_IDS.addFilterBtn
				},
				title: 'Add New Filter',
				description: 'Click here to start adding a new MCP filter.',
				side: 'left'
			},
			listener: {
				id: MCP_FILTERS_FIELD_IDS.addFilterBtn,
				action: {
					highlight: {
						selector: {
							id: MCP_FILTERS_FIELD_IDS.createCustomBtn
						},
						title: 'Create Custom Filter',
						description:
							"Obot also supports out-of-the-box PII filtering, but for the sake of this guide, let's create a custom filter. Click here to get started.",
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
		content: ["Now let's go over the MCP filter form."],
		action: {
			highlight: {
				selector: {
					id: MCP_FILTERS_FIELD_IDS.basicDetails
				},
				side: 'top',
				align: 'center',
				title: 'Basic Details',
				description: 'Enter the basic details of the filter.'
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
						title: 'Runtime Type',
						description:
							'Filters can be implemented via an MCP that exposes a filter tool or through an HTTP webhook. Select the appropriate runtime here.'
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
								title: 'Runtime Form Details',
								description:
									'Depending on the runtime type selected, you will need to configure the appropriate fields here.',
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
										title: 'Selectors',
										description:
											'Specify which requests should be matched by this filter. These can be specified by methods or identifiers such as tool names.',
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
												title: 'MCP Servers',
												description:
													'Select the MCP servers that will be used to filter the tool calls.',
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
														title: 'Save the filter.',
														description:
															"Once you've filled out all necessary fields, you can save the filter here. The filter will be enabled by default but you can choose to disable or re-enable it at any point.",
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
	title: 'Filtering and Controlling MCP Tool Calls',
	description: 'Add additional security policies to your MCP via Filters.',
	id: 'mcp-create-filter-guide'
};
