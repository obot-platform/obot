import { MCP_LIST_ORDER } from './constants';
import type { MCP } from './services';

export const sortByPreferredMcpOrder = (a: MCP, b: MCP) => {
	const aId = (a.id?.toLowerCase() ?? '').trim();
	const bId = (b.id?.toLowerCase() ?? '').trim();
	const aIndex = MCP_LIST_ORDER.indexOf(aId);
	const bIndex = MCP_LIST_ORDER.indexOf(bId);

	if (aIndex === -1 && bIndex === -1) return 0;
	if (aIndex === -1) return 1;
	if (bIndex === -1) return -1;
	return aIndex - bIndex;
};

export const sortShownToolsPriority = (a: string, b: string) => {
	const lastPriorityTools = new Set([
		'images-analyze-images',
		'images-generate-images',
		'obot-search'
	]);
	const aIsLastPriority = lastPriorityTools.has(a);
	const bIsLastPriority = lastPriorityTools.has(b);
	if (aIsLastPriority && !bIsLastPriority) return 1;
	if (!aIsLastPriority && bIsLastPriority) return -1;
	return 0;
};
