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
		content: ['**What is a composite catalog entry?**', addCatalogEntryDescriptions.composite]
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
					'This list contains the catalog entries and deployed multi-user servers whose tools will be combined into the composite server.',
				noDescendantInteraction: true
			},
			listener: {
				id: CATALOG_SERVER_FIELD_IDS.compositeEntries,
				skipClickTargetOnNext: true,
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
									'Here you can search and select the component entries that you want to add to the composite server. Each component will have its own configuration and authentication requirements. Follow the wizard to complete the process.',
								noDescendantInteraction: true
							},
							listener: {
								id: `${CATALOG_SERVER_FIELD_IDS.compositeEntrySearchMcpServersDialog}`,
								action: {
									success: true,
									elementExists: CATALOG_SERVER_FIELD_IDS.compositeEntrySearchMcpServersDialog,
									closeExistingElement: true
								}
							}
						}
					}
				}
			}
		}
	},
	{
		content: [''],
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
];

export default {
	steps,
	title: 'Create a Composite MCP Server',
	description:
		'Combine multiple existing MCP servers & apply fine-grained access control on their tools.',
	id: 'mcp-create-composite-guide'
};
