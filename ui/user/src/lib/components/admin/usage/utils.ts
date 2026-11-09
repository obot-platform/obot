import type { McpAuditLogUsageStats } from '$lib/services';

export function transformTopToolCalls(stats: McpAuditLogUsageStats | undefined) {
	const counts = new Map<string, { count: number; serverDisplayName: string }>();
	for (const s of stats?.items ?? []) {
		for (const call of s.toolCalls ?? []) {
			const key = `${s.mcpServerDisplayName}.${call.toolName}`;
			const existing = counts.get(key) ?? {
				count: 0,
				serverDisplayName: s.mcpServerDisplayName
			};
			existing.count += call.callCount;
			counts.set(key, existing);
		}
	}
	return Array.from(counts.entries())
		.map(([toolName, { count, serverDisplayName }]) => ({
			toolName,
			count,
			serverDisplayName
		}))
		.sort((a, b) => b.count - a.count);
}

export function transformTopServerUsage(stats: McpAuditLogUsageStats | undefined) {
	const counts = new Map<string, number>();
	for (const s of stats?.items ?? []) {
		const total = (s.toolCalls ?? []).reduce((sum, t) => sum + t.callCount, 0);
		if (total > 0) {
			counts.set(s.mcpServerDisplayName, (counts.get(s.mcpServerDisplayName) ?? 0) + total);
		}
	}
	return Array.from(counts.entries())
		.map(([serverName, count]) => ({ serverName, count }))
		.sort((a, b) => b.count - a.count);
}

export function transformAvgToolCallResponseTime(stats: McpAuditLogUsageStats | undefined) {
	const responseTimes = new Map<
		string,
		{ total: number; count: number; serverDisplayName: string }
	>();

	for (const s of stats?.items ?? []) {
		for (const call of s.toolCalls ?? []) {
			const key = `${s.mcpServerDisplayName}.${call.toolName}`;
			for (const item of call.items ?? []) {
				const entry = responseTimes.get(key) ?? {
					total: 0,
					count: 0,
					serverDisplayName: s.mcpServerDisplayName
				};
				entry.total += item.processingTimeMs;
				entry.count += 1;
				responseTimes.set(key, entry);
			}
		}
	}

	return Array.from(responseTimes.entries())
		.map(([toolName, { total, count, serverDisplayName }]) => ({
			toolName,
			averageResponseTimeMs: count > 0 ? total / count : 0,
			serverDisplayName
		}))
		.sort((a, b) => b.averageResponseTimeMs - a.averageResponseTimeMs);
}

export type IndividualToolResponseRow = {
	toolName: string;
	processingTimeMs: number;
	serverDisplayName: string;
};

export function transformIndividualToolResponseTimes(
	stats: McpAuditLogUsageStats | undefined
): IndividualToolResponseRow[] {
	const rows: IndividualToolResponseRow[] = [];
	for (const server of stats?.items ?? []) {
		for (const call of server.toolCalls ?? []) {
			for (const [itemIndex, item] of (call.items ?? []).entries()) {
				rows.push({
					toolName: `${server.mcpServerDisplayName}.${server.mcpID}.${itemIndex}.${call.toolName}`,
					processingTimeMs: item.processingTimeMs,
					serverDisplayName: server.mcpServerDisplayName
				});
			}
		}
	}
	return rows.sort((a, b) => b.processingTimeMs - a.processingTimeMs);
}

export type ToolCallErrorRow = {
	toolName: string;
	errorCount: number;
	serverDisplayName: string;
};

export function transformToolCallErrors(
	stats: McpAuditLogUsageStats | undefined
): ToolCallErrorRow[] {
	const errorCounts = new Map<string, { errorCount: number; serverDisplayName: string }>();
	for (const server of stats?.items ?? []) {
		for (const call of server.toolCalls ?? []) {
			const key = `${server.mcpServerDisplayName}.${call.toolName}`;
			let count = 0;
			for (const item of call.items ?? []) {
				if (item.error || item.responseStatus >= 400) count++;
			}
			if (count > 0) {
				const existing = errorCounts.get(key) ?? {
					errorCount: 0,
					serverDisplayName: server.mcpServerDisplayName
				};
				existing.errorCount += count;
				errorCounts.set(key, existing);
			}
		}
	}
	return Array.from(errorCounts.entries())
		.filter(([, entry]) => entry.errorCount > 0)
		.map(([toolName, { errorCount, serverDisplayName }]) => ({
			toolName,
			errorCount,
			serverDisplayName
		}))
		.sort((a, b) => b.errorCount - a.errorCount);
}

export type ToolCallErrorsByServerRow = {
	serverName: string;
	errorCount: number;
};

export function transformToolCallErrorsByServer(
	stats: McpAuditLogUsageStats | undefined
): ToolCallErrorsByServerRow[] {
	const errorCounts = new Map<string, number>();
	for (const server of stats?.items ?? []) {
		let count = 0;
		for (const call of server.toolCalls ?? []) {
			for (const item of call.items ?? []) {
				if (item.error || item.responseStatus >= 400) count++;
			}
		}
		if (count > 0) {
			errorCounts.set(
				server.mcpServerDisplayName,
				(errorCounts.get(server.mcpServerDisplayName) ?? 0) + count
			);
		}
	}
	return Array.from(errorCounts.entries())
		.map(([serverName, errorCount]) => ({ serverName, errorCount }))
		.sort((a, b) => b.errorCount - a.errorCount);
}

export type MostActiveUserRow = {
	userId: string;
	callCount: number;
};

export function transformMostActiveUsers(
	stats: McpAuditLogUsageStats | undefined
): MostActiveUserRow[] {
	const userCounts = new Map<string, number>();
	for (const server of stats?.items ?? []) {
		for (const call of server.toolCalls ?? []) {
			for (const item of call.items ?? []) {
				userCounts.set(item.userID, (userCounts.get(item.userID) ?? 0) + 1);
			}
		}
	}
	return Array.from(userCounts.entries())
		.map(([userId, callCount]) => ({ userId, callCount }))
		.sort((a, b) => b.callCount - a.callCount);
}
