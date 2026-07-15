import type { GuideAction, GuideHighlight, GuideListener } from './types';

export function getExpandAdvancedPaneAction({
	elementMissing,
	highlight,
	listener,
	title,
	description
}: {
	elementMissing: string;
	highlight?: GuideHighlight;
	listener?: GuideListener;
	title?: string;
	description?: string;
}): GuideAction {
	return {
		elementExists: 'sidebar-collapse-mcp-server-management',
		elementMissing,
		highlight: {
			selector: {
				id: 'sidebar-collapse-mcp-server-management'
			},
			title: title || 'Expand MCP Management',
			description: description || 'MCP Management is collapsed. Click here to expand it.'
		},
		listener: {
			id: 'sidebar-collapse-mcp-server-management',
			action: {
				highlight: highlight,
				listener: listener
			}
		}
	};
}
