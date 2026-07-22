import { CATALOG_SERVER_FIELD_IDS } from '$lib/constants';
import type { GuideStep } from '../types';
import {
	getHighlightAddCatalogEntryStep,
	getNavigateBasicCatalogEntryFieldsStep,
	getNavigateToMCPCatalogStep
} from './steps';

export const steps: GuideStep[] = [
	{
		content: [
			'What is a remote catalog entry?',
			'A remote catalog entry is great for allowing users to connect to MCP servers that are already elsewhere. When they deploy from Obot, the MCP server will go through the gateway.'
		]
	},
	getNavigateToMCPCatalogStep(),
	getHighlightAddCatalogEntryStep('remote'),
	getNavigateBasicCatalogEntryFieldsStep(),
	{
		content: ["Now let's go over the remote specific fields."],
		action: {
			highlight: {
				selector: { id: CATALOG_SERVER_FIELD_IDS.remoteURL },
				side: 'top',
				align: 'center',
				title: 'Remote Server URL',
				description:
					'Enter the complete MCP endpoint when every user should connect to the same exact URL.'
			},
			listener: {
				id: CATALOG_SERVER_FIELD_IDS.remoteURL,
				action: {
					highlight: {
						selector: { id: CATALOG_SERVER_FIELD_IDS.remoteAdvancedBtn },
						side: 'top',
						title: 'Advanced Configuration',
						description:
							"Open this section when the connection needs a hostname or URL template, custom headers, or static OAuth. Let's go ahead and open it."
					},
					listener: {
						id: CATALOG_SERVER_FIELD_IDS.remoteAdvancedBtn,
						action: {
							elementExists: CATALOG_SERVER_FIELD_IDS.remoteConnection,
							highlight: {
								selector: { id: CATALOG_SERVER_FIELD_IDS.remoteConnection },
								side: 'top',
								align: 'center',
								title: 'Connection Restriction',
								description:
									'Choose an exact URL, allow a user-configured URL on one hostname, or build a URL from a template. Template variables come from user-supplied header values.'
							},
							listener: {
								id: CATALOG_SERVER_FIELD_IDS.remoteConnection,
								action: {
									highlight: {
										selector: { id: CATALOG_SERVER_FIELD_IDS.remoteHeaders },
										side: 'top',
										align: 'center',
										title: 'Request Headers',
										description:
											'Add HTTP headers required by the upstream server. Values can be fixed for everyone or requested from each user during setup; keep secrets in headers instead of URLs.'
									},
									listener: {
										id: CATALOG_SERVER_FIELD_IDS.remoteHeaders,
										skipClickTargetOnNext: true,
										action: {
											highlight: {
												selector: { id: CATALOG_SERVER_FIELD_IDS.remoteStaticOAuth },
												side: 'top',
												align: 'center',
												title: 'Static OAuth',
												description:
													'Enable this only when the provider requires a pre-registered OAuth app. The entry will need to be saved first, then the shared client ID and secret can be configured; each user will still need to complete their own OAuth login.'
											},
											listener: {
												id: CATALOG_SERVER_FIELD_IDS.remoteStaticOAuth,
												skipClickTargetOnNext: true,
												action: {
													highlight: {
														selector: { id: CATALOG_SERVER_FIELD_IDS.submitBtn },
														side: 'left',
														title: 'Save the entry',
														description:
															'Once you have finished configuring the remote catalog entry, you can save here.'
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
	title: 'Reroute a Remote MCP Through Obot',
	description: 'Add auditing & governance to an existing MCP server.',
	id: 'mcp-create-remote-guide'
};
