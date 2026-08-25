import { requireHostedAgentsEnabled } from '$lib/hostedAgents';
import type { LayoutLoad } from './$types';

export const load: LayoutLoad = async ({ parent }) => {
	const { version } = await parent();
	requireHostedAgentsEnabled(version, '/mcp-servers');
};
