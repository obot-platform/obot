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
			'Great! To connect your AI client to the MCP server, follow the instructions on the connect dialog.',
			'Close the connect window once you have set up your MCP server with your AI client or feel free to click below to continue.'
		],
		action: {
			highlight: {
				selector: {
					id: 'connect-to-server-dialog-content'
				},
				title: 'Add the MCP Server to your AI Client',
				description:
					'To connect to your client, use the appropriate option; add via connection URL, install via quick link, or copy & paste the CLI commands listed here.'
			},
			listener: {
				id: 'connect-to-server-dialog-content',
				action: {
					success: true,
					elementExists: 'connect-to-server-dialog',
					closeExistingElement: true
				}
			}
		}
	},
	{
		content: ['Once you’ve connected, you can begin using the MCP server on your AI Client!']
	}
];

export default {
	steps,
	title: 'Connecting to MCP Servers',
	id: 'mcp-connect-guide'
};
