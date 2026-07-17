import { browser } from '$app/environment';
import { page } from '$app/state';
import { OBOT_GUIDE_KEYS } from '$lib/constants';
import { Group } from '$lib/services/admin/types';
import { profile } from '$lib/stores';
import McpConnectGuide from './mcp/connect';
import McpCreateCustomGuide from './mcp/customMcp';
import { isValid } from 'date-fns';

export function generateLessonItems() {
	const isAdvancedRoute =
		page.url.pathname.startsWith('/admin') ||
		page.url.pathname.includes('mcp-catalog') ||
		page.url.pathname.includes('access-policies');
	const isAtLeastPoweruser = profile.current.groups.includes(Group.POWERUSER);
	return isAdvancedRoute && isAtLeastPoweruser
		? [
				{
					label: 'Add custom MCP Server to the catalog',
					description: 'Create a custom MCP server.',
					guide: McpCreateCustomGuide
				}
			]
		: [
				{
					label: 'Connect to an MCP Server',
					description: 'Set up your AI client with an MCP server and begin using it.',
					guide: McpConnectGuide
				}
			];
}

export function getGuideSeen(): Date | undefined {
	if (!browser) return undefined;
	const userId = profile.current?.id;
	const key = userId ? `${OBOT_GUIDE_KEYS.GUIDE}:${userId}` : OBOT_GUIDE_KEYS.GUIDE;

	const dateString = localStorage.getItem(key) ?? '';
	if (!dateString) return undefined;

	const validDate = new Date(dateString);
	return isValid(validDate) ? validDate : undefined;
}

export function setGuideSeen() {
	if (!browser) return undefined;
	const userId = profile.current?.id;
	const key = userId ? `${OBOT_GUIDE_KEYS.GUIDE}:${userId}` : OBOT_GUIDE_KEYS.GUIDE;
	localStorage.setItem(key, new Date().toISOString());
}

export function resetGuide() {
	if (!browser) return;
	const userId = profile.current?.id;
	const seenGuideKey = userId ? `${OBOT_GUIDE_KEYS.GUIDE}:${userId}` : OBOT_GUIDE_KEYS.GUIDE;
	const completedGuidesKey = userId
		? `${OBOT_GUIDE_KEYS.COMPLETED}:${userId}`
		: OBOT_GUIDE_KEYS.COMPLETED;

	localStorage.removeItem(seenGuideKey);
	localStorage.removeItem(completedGuidesKey);
}
