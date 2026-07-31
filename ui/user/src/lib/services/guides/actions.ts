import type { GuideAction, GuideHighlight, GuideListener } from './types';

// shared action that can be used in multiple guides
export function getExpandAdvancedPaneAction({
	elementMissing,
	highlight,
	listener,
	title,
	description,
	parentID
}: {
	elementMissing: string;
	highlight?: GuideHighlight;
	listener?: GuideListener;
	title?: string;
	description?: string;
	parentID: string;
}): GuideAction {
	return {
		elementExists: parentID,
		elementMissing,
		highlight: {
			selector: {
				id: parentID
			},
			title: title || 'Expand MCP Management',
			description: description || "Let's expand this section to continue."
		},
		listener: {
			id: parentID,
			action: {
				highlight: highlight,
				listener: listener
			}
		}
	};
}
