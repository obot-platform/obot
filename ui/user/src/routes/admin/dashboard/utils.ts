import {
	transformAvgToolCallResponseTime,
	transformTopServerUsage,
	transformTopToolCalls
} from '$lib/components/admin/usage/utils';
import type { AuditLogUsageStats, MCPCatalogServer } from '$lib/services';
import { DEPLOYMENT_STATUS_ORDER } from './constants';
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
