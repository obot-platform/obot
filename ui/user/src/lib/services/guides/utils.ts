import { browser } from '$app/environment';
import { OBOT_GUIDE_KEYS } from '$lib/constants';
import { Group } from '$lib/services/admin/types';
import { profile } from '$lib/stores';
import CliInstallGuide from './cli/install';
import McpConnectGuide from './mcp/connect';
import McpCreateCustomGuide from './mcp/customMcp';
import McpCreateWithAccessPolicyCustomGuide from './mcp/customMcpAndAccessPolicy';
import { isValid } from 'date-fns';

export function generateLessonItems(lessonsCompleted: Record<string, boolean>) {
	const isAtLeastPoweruser = profile.current.groups.includes(Group.POWERUSER);
	const isAtLeastPoweruserPlus = profile.current.groups.includes(Group.POWERUSER_PLUS);
	return [
		{
			completed: lessonsCompleted[McpConnectGuide.id],
			label: 'Connect to an MCP Server',
			description:
				'Set up your AI client with an MCP server and begin using it. Gain visibility into your data and begin using the app.',
			guide: McpConnectGuide
		},
		...(isAtLeastPoweruser
			? [
					{
						completed: isAtLeastPoweruserPlus
							? lessonsCompleted[McpCreateWithAccessPolicyCustomGuide.id]
							: lessonsCompleted[McpCreateCustomGuide.id],
						label: 'Add custom MCP Server to the catalog',
						description: isAtLeastPoweruserPlus
							? 'Run a step-by-step guide to add a custom MCP server to the catalog & set up who can access it via access control policies.'
							: 'Run a step-by-step guide to add a custom MCP server to the catalog.',
						guide: isAtLeastPoweruserPlus
							? McpCreateWithAccessPolicyCustomGuide
							: McpCreateCustomGuide
					}
				]
			: []),
		{
			completed: lessonsCompleted[CliInstallGuide.id],
			label: 'Use Obot/Obot Sentry CLI',
			description:
				"Install the CLI to assist in installing MCP servers & skills or perform scans to provide visibility into your userbase's devices.",
			guide: CliInstallGuide
		}
	];
}

export function getLessonsCompleted() {
	const defaultEmptyState: Record<string, boolean> = {};

	if (!browser) return defaultEmptyState;
	const userId = profile.current?.id;
	const key = userId ? `${OBOT_GUIDE_KEYS.COMPLETED}:${userId}` : OBOT_GUIDE_KEYS.COMPLETED;

	const json = localStorage.getItem(key);
	if (json) {
		try {
			const parsed: unknown = JSON.parse(json);
			if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
				return parsed as Record<string, boolean>;
			}
			return defaultEmptyState;
		} catch (error) {
			console.error('Error parsing OBOT guide completed status:', error);
		}
	}

	return defaultEmptyState;
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

export function setLessonCompleted(guideId?: string) {
	if (!guideId || !browser) return;
	const completed = getLessonsCompleted();
	completed[guideId] = true;

	const userId = profile.current?.id;
	const key = userId ? `${OBOT_GUIDE_KEYS.COMPLETED}:${userId}` : OBOT_GUIDE_KEYS.COMPLETED;
	localStorage.setItem(key, JSON.stringify(completed));
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
