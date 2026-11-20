import type { UsageStatsFilters } from '$lib/services';

export type SupportedStateFilter = keyof UsageStatsFilters;

export type McpUsageCounts = {
	individualResponses: number;
	toolErrors: number;
	serverErrors: number;
	activeUsers: number;
};
