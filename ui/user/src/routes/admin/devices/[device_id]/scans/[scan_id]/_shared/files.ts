import type { DeviceScan, DeviceScanFile } from '$lib/services/admin/types';

export function lookupFiles(
	scanFiles: DeviceScanFile[] | undefined,
	paths: string[] | undefined
): { path: string; file?: DeviceScanFile }[] {
	const byPath = new Map<string, DeviceScanFile>();
	for (const f of scanFiles ?? []) byPath.set(f.path, f);
	return (paths ?? []).map((path) => ({ path, file: byPath.get(path) }));
}

export function formatBytes(n: number): string {
	if (n < 1024) return `${n} B`;
	if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KiB`;
	return `${(n / 1024 / 1024).toFixed(1)} MiB`;
}

export function shortHash(h?: string): string {
	if (!h) return '—';
	return h.length > 12 ? `${h.slice(0, 8)}…${h.slice(-4)}` : h;
}

// findParentPlugin returns the plugin observation whose defining file or
// supporting files contain `file`. Used to cross-link MCP servers and
// skills back to the plugin that emitted them. The returned id is the
// plugin row's PK, suitable for the {id} segment of scan-scoped detail
// URLs.
export function findParentPlugin(
	scan: DeviceScan | undefined,
	file: string | undefined
): { id: number; name: string } | undefined {
	if (!scan || !file) return undefined;
	const match = scan.plugins?.find((p) => p.configPath === file || p.files?.includes(file));
	if (!match) return undefined;
	return { id: match.id, name: match.name };
}
