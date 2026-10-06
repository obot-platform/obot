import { m } from '$lib/i18n';
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
			title: title || m.core_guides_expand_mcp_management(),
			description: description || m.core_guides_let_s_expand_this_section_to()
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
