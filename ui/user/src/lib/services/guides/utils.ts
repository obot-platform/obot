import { browser } from '$app/environment';
import { page } from '$app/state';
import { OBOT_GUIDE_KEYS } from '$lib/constants';
import { Group } from '$lib/services/admin/types';
import { profile } from '$lib/stores';
import { SkillsInstallGuide, McpConnectGuide, McpCreateCustomGuide } from '.';
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
					label: McpCreateCustomGuide.title,
					description: McpCreateCustomGuide.description,
					guide: McpCreateCustomGuide
				}
			]
		: [
				{
					label: McpConnectGuide.title,
					description: McpConnectGuide.description,
					guide: McpConnectGuide
				},
				{
					label: SkillsInstallGuide.title,
					description: SkillsInstallGuide.description,
					guide: SkillsInstallGuide
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
