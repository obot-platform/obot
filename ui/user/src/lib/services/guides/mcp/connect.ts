import { getExpandAdvancedPaneAction } from '../actions';
import type { GuideHighlight, GuideListener, GuideStep } from '../types';

const SIDEBAR_MCP_SERVERS_LINK = 'sidebar-link-mcp-servers';
const SIDEBAR_MCP_AUDIT_LOGS_LINK = 'sidebar-link-audit-logs';

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

const highlightMcpAuditLogsLink: GuideHighlight = {
	selector: {
		id: SIDEBAR_MCP_AUDIT_LOGS_LINK
	},
	title: 'Audit Logs',
	description: 'Click here to view MCP server audit logs.'
};

const listenMcpAuditLogsLink: GuideListener = {
	id: SIDEBAR_MCP_AUDIT_LOGS_LINK,
	action: {
		success: true
	}
};

export const steps: GuideStep[] = [
	{
		content: [
			'Before connecting to an MCP server, let us know what is your preferred AI client by clicking the button below.'
		],
		button: {
			text: 'Choose preferred client',
			action: {
				setPreferredClient: true
			}
		}
	},
	{
		content: [
			'Great! This will help us tailor your initial connection experience.',
			'Next, we need to connect to an MCP server. Go to the MCP servers page located in the left sidebar.'
		],
		button: {
			text: 'Where is MCP Servers?',
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
		}
	},
	{
		content: ['For the purpose of this guide, let us connect with the AntV Charts MCP server.'],
		button: {
			text: 'Where is this?',
			action: {
				highlight: {
					selector: {
						beginsWith: ['btn-connect-to-server-default-antv-charts']
					},
					title: 'Connect to AntV Charts MCP Server',
					description:
						'Click here to begin connecting your AI client to the AntV Charts MCP server.',
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
		}
	},
	{
		content: [
			'Great! To connect your AI client to the MCP server, follow the instructions on the connect dialog.',
			'Close the connect window once you have set up your MCP server with your AI client. Feel free to click below to continue.'
		],
		button: {
			text: "I've Connected!",
			action: {
				success: true
			}
		}
	},
	{
		content: [
			"Now that you've connected, you can begin using the MCP server on your AI client!",
			"You can continue to connect to other MCP servers from here. Or, if you'd like to gain more insight & visibility into your MCP servers, head to MCP Audit Logs!"
		],
		button: {
			text: 'Where is Audit Logs?',
			action: [
				{
					elementExists: SIDEBAR_MCP_AUDIT_LOGS_LINK,
					highlight: highlightMcpAuditLogsLink,
					listener: listenMcpAuditLogsLink
				},
				getExpandAdvancedPaneAction({
					elementMissing: SIDEBAR_MCP_AUDIT_LOGS_LINK,
					highlight: highlightMcpAuditLogsLink,
					listener: listenMcpAuditLogsLink
				}),
				{
					highlight: {
						selector: {
							id: 'advanced-pane-btn'
						},
						title: 'Advanced Pane',
						description:
							'Click here to open the advanced pane; this section contains more advanced settings and management capabilities such as audit logs.'
					},
					listener: {
						id: 'advanced-pane-btn',
						action: [
							{
								elementExists: SIDEBAR_MCP_AUDIT_LOGS_LINK,
								highlight: highlightMcpAuditLogsLink,
								listener: listenMcpAuditLogsLink
							},
							getExpandAdvancedPaneAction({
								elementMissing: SIDEBAR_MCP_AUDIT_LOGS_LINK,
								highlight: highlightMcpAuditLogsLink,
								listener: listenMcpAuditLogsLink
							})
						]
					}
				}
			]
		}
	},
	{
		content: [
			'From here, you can view the audit logs of MCP servers being used by everyone in your organization.',
			'If you are not seeing any audit logs currently, try asking your AI client to generate a bar chart using the AntV Charts MCP server.',
			'Click the button below to see a video example.'
		],
		button: {
			text: 'Tool Call Video',
			action: {
				dialog: {
					title: 'Tool Call',
					content: [
						{
							title: 'Obot - Tool Call Example',
							videoUrl: 'https://youtu.be/Vv5bxk64QOw'
						}
					]
				}
			}
		}
	},
	{
		content: ["And that's it! You've completed the connecting to MCP servers guide."]
	}
];

export default {
	steps,
	title: 'Connecting to MCP Servers',
	id: 'mcp-connect-guide'
};
