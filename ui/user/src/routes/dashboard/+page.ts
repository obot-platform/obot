import { handleRouteError } from '$lib/errors';
import { AdminService, Group, UserService, type TotalTokenUsage } from '$lib/services';
import type { PageLoad } from './$types';
import { redirect } from '@sveltejs/kit';

function llmStatsExist(totals: TotalTokenUsage) {
	return (
		totals.totalTokens > 0 ||
		(totals.inputTokens ?? 0) > 0 ||
		(totals.outputTokens ?? 0) > 0 ||
		(totals.totalSpend ?? 0) > 0
	);
}

export const load: PageLoad = async ({ fetch, parent }) => {
	const { profile } = await parent();

	const isAtLeastPoweruser =
		profile?.groups.includes(Group.POWERUSER) || profile?.hasAdminAccess?.();
	if (!isAtLeastPoweruser) {
		throw redirect(302, '/vmcps');
	}

	const statsStart = new Date(0).toISOString();
	const statsEnd = new Date().toISOString();

	let hasDeviceScans = false;
	try {
		const response = await UserService.listDeviceScans({ limit: 1 }, { fetch });
		hasDeviceScans = response.total > 0;
	} catch (err) {
		handleRouteError(err, '/dashboard', profile);
	}

	let hasMcpStats = false;
	try {
		const stats = await UserService.listMcpAuditLogUsageStats(
			{ start_time: statsStart, end_time: statsEnd },
			{ fetch }
		);
		hasMcpStats = stats.totalCalls > 0;
	} catch (err) {
		handleRouteError(err, '/dashboard', profile);
	}

	let hasLlmStats = false;
	if (profile?.hasAdminAccess?.()) {
		try {
			const totals = await AdminService.listTotalTokenUsage(
				{ start: statsStart, end: statsEnd },
				{ fetch }
			);
			hasLlmStats = llmStatsExist(totals);
		} catch (err) {
			handleRouteError(err, '/dashboard', profile);
		}
	}

	return {
		hasDeviceScans,
		hasMcpStats,
		hasLlmStats
	};
};
