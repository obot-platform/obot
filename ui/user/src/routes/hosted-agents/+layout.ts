import { requireHostedAgentsEnabled } from '$lib/hostedAgents';
import type { LayoutLoad } from './$types';

export const load: LayoutLoad = async ({ parent }) => {
	const { version, profile } = await parent();
	requireHostedAgentsEnabled(version, profile.hasAdminAccess?.() ? '/dashboard' : '/mcp-servers');
};
