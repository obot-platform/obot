import {
	transformAvgToolCallResponseTime,
	transformTopServerUsage,
	transformTopToolCalls
} from '$lib/components/admin/usage/utils';
import type { DonutDatum } from '$lib/components/graph/DonutGraph.svelte';
import type { AuditLogUsageStats, MCPCatalogEntry, MCPCatalogServer } from '$lib/services';
import { DEPLOYMENT_STATUS_ORDER, ENTRY_TYPE_GRAPH_META } from './constants';
import type { TopServerUsageRow, TopToolCallRow } from './types';

export function topToolCallsFromStats(stats: AuditLogUsageStats | undefined): TopToolCallRow[] {
	return transformTopToolCalls(stats).map((t) => ({
		compositeKey: t.toolName,
		toolLabel: t.toolName,
		count: t.count,
		serverDisplayName: t.serverDisplayName
	}));
}

export function topServersFromStats(stats: AuditLogUsageStats | undefined): TopServerUsageRow[] {
	return transformTopServerUsage(stats);
}

export function avgToolCallResponseTimeFromStats(stats: AuditLogUsageStats | undefined) {
	return transformAvgToolCallResponseTime(stats);
}

export function mixHex(base: string, toward: string, t: number): string {
	const parse = (hex: string) => {
		const s = hex.replace('#', '');
		const full = s.length === 3 ? [...s].map((c) => c + c).join('') : s;
		return [0, 2, 4].map((i) => parseInt(full.slice(i, i + 2), 16));
	};
	const [r1, g1, b1] = parse(base);
	const [r2, g2, b2] = parse(toward);
	const blend = (a: number, b: number) => Math.round(a + (b - a) * t);
	const r = blend(r1, r2);
	const g = blend(g1, g2);
	const b = blend(b1, b2);
	return `#${[r, g, b].map((x) => x.toString(16).padStart(2, '0')).join('')}`;
}

export function deploymentStatusSortKey(status: string): number {
	const i = DEPLOYMENT_STATUS_ORDER.indexOf(status as (typeof DEPLOYMENT_STATUS_ORDER)[number]);
	return i >= 0 ? i : DEPLOYMENT_STATUS_ORDER.length;
}

export function catalogServerEntryKind(
	server: MCPCatalogServer
): 'multi' | 'single' | 'remote' | 'composite' {
	if (!server.catalogEntryID) return 'multi';
	if (server.manifest.runtime === 'composite') return 'composite';
	if (server.manifest.runtime === 'remote') return 'remote';
	return 'single';
}

export function normalizeServerDeploymentStatus(server: MCPCatalogServer): string {
	const raw = server.deploymentStatus?.trim();
	if (raw && DEPLOYMENT_STATUS_ORDER.includes(raw as (typeof DEPLOYMENT_STATUS_ORDER)[number]))
		return raw;
	if (raw) return raw;
	return 'Unknown';
}

/** 12-column grid: 3× col-span-4 per full row; last row fills width (6+6 or 12). */
export function deploymentStatusRowLayout(total: number): {
	itemsInLastRow: number;
	lastRowStart: number;
} {
	const rem = total % 3;
	const itemsInLastRow = rem === 0 ? 3 : rem;
	const lastRowStart = total - itemsInLastRow;
	return { itemsInLastRow, lastRowStart };
}

export function deploymentStatusGridColClass(i: number, total: number): string {
	const { itemsInLastRow, lastRowStart } = deploymentStatusRowLayout(total);
	if (i < lastRowStart) return 'col-span-4';
	if (itemsInLastRow === 1) return 'col-span-12';
	if (itemsInLastRow === 2) return 'col-span-6';
	return 'col-span-4';
}

export function deploymentStatusGridShowBorderRight(i: number, total: number): boolean {
	const { itemsInLastRow, lastRowStart } = deploymentStatusRowLayout(total);
	if (i >= lastRowStart) {
		if (itemsInLastRow === 1) return false;
		if (itemsInLastRow === 2) return i === lastRowStart;
		return i < lastRowStart + 2;
	}
	return i % 3 !== 2;
}

export function compileServerAndEntries(
	data: MCPCatalogServer[],
	entries: MCPCatalogEntry[],
	doesSupportK8sUpdates: boolean
) {
	const entriesMap = new Map(entries.map((e) => [e.id, e]));
	const catalogEntriesCount = data.reduce<
		Record<
			string,
			{
				entry?: MCPCatalogEntry | undefined;
				server?: MCPCatalogServer | undefined;
				count: number;
				id: string;
			}
		>
	>((acc, server) => {
		if (!server.catalogEntryID) {
			acc[server.id] = {
				server,
				count: 1,
				id: server.id
			};
			return acc;
		}
		if (!acc[server.catalogEntryID]) {
			const entry = entriesMap.get(server.catalogEntryID);
			if (!entry) return acc;
			acc[server.catalogEntryID] = {
				entry,
				count: 0,
				id: entry.id
			};
		}
		acc[server.catalogEntryID].count++;
		return acc;
	}, {});
	const sortByCountDescending = Object.values(catalogEntriesCount).sort(
		(a, b) => b.count - a.count
	);

	const entryTypes = data.reduce(
		(acc, server) => {
			if (server.manifest.runtime === 'composite') acc.composite++;
			else if (server.manifest.runtime === 'remote') acc.remote++;
			else if (server.serverUserType === 'singleUser') acc.single++;
			else if (server.serverUserType === 'multiUser') acc.multi++;
			return acc;
		},
		{
			single: 0,
			multi: 0,
			local: 0,
			remote: 0,
			composite: 0
		}
	);

	let graphData: DonutDatum[] = [];
	let deploymentStatusBreakdown: { status: string; count: number }[] = [];
	if (doesSupportK8sUpdates) {
		const overallByStatus: Record<string, number> = {};
		for (const server of data) {
			const s = normalizeServerDeploymentStatus(server);
			overallByStatus[s] = (overallByStatus[s] ?? 0) + 1;
		}
		deploymentStatusBreakdown = Object.entries(overallByStatus)
			.filter(([, count]) => count > 0)
			.sort(([a], [b]) => {
				const d = deploymentStatusSortKey(a) - deploymentStatusSortKey(b);
				return d !== 0 ? d : a.localeCompare(b);
			})
			.map(([status, count]) => ({ status, count }));

		const countsByKindAndStatus: Record<string, number> = {};
		for (const server of data) {
			const kind = catalogServerEntryKind(server);
			const status = normalizeServerDeploymentStatus(server);
			const key = `${kind}\0${status}`;
			countsByKindAndStatus[key] = (countsByKindAndStatus[key] ?? 0) + 1;
		}

		for (const { key: kind, label: typeLabel, baseColor } of ENTRY_TYPE_GRAPH_META) {
			const prefix = `${kind}\0`;
			const statusEntries = Object.entries(countsByKindAndStatus)
				.filter(([k]) => k.startsWith(prefix))
				.map(([k, value]) => [k.slice(prefix.length), value] as [string, number])
				.filter(([, value]) => value > 0)
				.sort(([a], [b]) => {
					const d = deploymentStatusSortKey(a) - deploymentStatusSortKey(b);
					return d !== 0 ? d : a.localeCompare(b);
				});

			const n = statusEntries.length;
			const maxTint = 0.25;
			statusEntries.forEach(([status, value], i) => {
				const t = n <= 1 ? 0 : i / (n - 1);
				graphData.push({
					label: `${typeLabel} · ${status}`,
					value,
					color: mixHex(baseColor, '#ffffff', t * maxTint),
					groupKey: kind
				});
			});
		}
	} else {
		graphData = ENTRY_TYPE_GRAPH_META.map(({ key, label, baseColor }) => ({
			label,
			value: entryTypes[key],
			color: baseColor
		}));
	}

	return {
		graphData,
		popularServers: sortByCountDescending.filter((s) => s.count > 0).slice(0, 5),
		totalServers: data.length,
		deploymentStatusBreakdown
	};
}
