import type { AuditLogUsageStats } from '$lib/services';

export function transformTopToolCalls(stats: AuditLogUsageStats | undefined) {
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

export function transformTopServerUsage(stats: AuditLogUsageStats | undefined) {
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

export function transformAvgToolCallResponseTime(stats: AuditLogUsageStats | undefined) {
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
