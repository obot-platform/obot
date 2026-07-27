import type { GuideHighlight, GuideListener, GuideStep } from '../types';

const SIDEBAR_MCP_SERVERS_LINK = 'sidebar-link-mcp-servers';

const highlightMcpServersLink: GuideHighlight = {
	selector: {
		id: SIDEBAR_MCP_SERVERS_LINK
	},
	title: 'MCP Servers',
	description: 'Click here to view the MCP servers you have access to.'
};

const listenMcpServersLink: GuideListener = {
	id: SIDEBAR_MCP_SERVERS_LINK,
	action: {
		success: true
	}
};

export const steps: GuideStep[] = [
	{
		content: [
			'To get started, we need to connect to an MCP server. Go to the MCP servers page located in the left sidebar.'
		],
		action: [
			{
				elementExists: 'back-to-app-btn',
				highlight: {
					selector: {
						id: 'back-to-app-btn'
					},
					title: 'Return to User Consumption View',
					description: 'Click here to return to the user consumption view.'
				},
				listener: {
					id: 'back-to-app-btn',
					action: {
						highlight: highlightMcpServersLink,
						listener: listenMcpServersLink
					}
				}
			},
			{
				highlight: highlightMcpServersLink,
				listener: listenMcpServersLink
			}
		]
	},
	{
		content: ['For the purpose of this guide, let us connect with the AntV Charts MCP server.'],
		action: {
			highlight: {
				selector: {
					beginsWith: ['btn-connect-to-server-default-antv-charts']
				},
				title: 'Connect to AntV Charts MCP Server',
				description: 'Click here to begin connecting your AI client to the AntV Charts MCP server.',
				side: 'left',
				align: 'end'
			},
			listener: {
				beginsWith: ['btn-connect-to-server-default-antv-charts'],
				action: {
					success: true
				}
			}
		}
	},
	{
		content: [
			'To connect your AI client to the MCP server, follow the instructions on the connect dialog.'
		],
		action: {
			highlight: {
				selector: {
					id: 'connection-url-container'
				},
				title: 'Install via Connection URL',
				description:
					'Copy & paste the connection URL into your AI client to connect to the MCP server.'
			},
			listener: {
				id: 'connection-url-container',
				skipClickTargetOnNext: true,
				action: {
					highlight: {
						selector: {
							id: 'magic-links-container'
						},
						title: 'Install via Quick Install Link',
						description:
							'Click the quick install link to install the MCP server into your AI client.',
						noDescendantInteraction: true
					},
					listener: {
						id: 'magic-links-container',
						skipClickTargetOnNext: true,
						action: {
							highlight: {
								selector: {
									id: 'cli-commands-container'
								},
								title: 'Install via CLI Commands',
								description:
									'Copy & paste the CLI commands into your terminal to install the MCP server into your AI client.'
							},
							listener: {
								id: 'cli-commands-container',
								skipClickTargetOnNext: true,
								action: {
									highlight: {
										selector: {
											id: 'connect-to-server-dialog-content'
										},
										title: 'Connect to the MCP Server',
										description:
											'Try using one of the options here to connect the MCP server to your AI client.'
									},
									next: {
										action: {
											success: true,
											elementExists: 'connect-to-server-dialog',
											closeExistingElement: true
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
	title: 'Connecting to MCP Servers',
	description: 'Set up your AI client with an MCP server and begin using it.',
	id: 'mcp-connect-guide'
};
