import { DEFAULT_MCP_CATALOG_ID } from '$lib/constants';
import type {
	MCPCatalogEntry,
	MCPCatalogServer,
	MCPConfig,
	OrgUser,
	VMCP,
	VMCPComponent,
	VMCPManifest,
	VMCPProfile
} from '$lib/services';
import { AiClient } from '../user/constants';
import { getManifestConfiguration } from '../user/mcp';
import {
	COMPONENT_LABEL_SEPARATOR,
	MCP_SERVER_POPULARITY_ORDER,
	WIRE_MAX_ARCH_PX,
	WIRE_SEGMENTS
} from './constants';
import type {
	Point,
	RectLike,
	VMcpComponentView,
	VMcpFilterOption,
	VMcpFilters,
	VMcpSortBy
} from './types';

export function joinComponentLabels(parts: Array<string | undefined>) {
	return parts
		.map((part) => part?.trim() ?? '')
		.filter(Boolean)
		.join(COMPONENT_LABEL_SEPARATOR);
}

export function isJoinedComponentLabel(
	current: string | undefined,
	parts: Array<string | undefined>,
	maxLength?: number
) {
	const joined = joinComponentLabels(parts);
	if (!joined) return false;
	const expected = maxLength ? joined.slice(0, maxLength) : joined;
	const value = (current ?? '').trim();
	return value === expected || value === joined;
}

export function appendComponentLabel(
	current: string | undefined,
	existingParts: Array<string | undefined>,
	added: string | undefined,
	maxLength?: number
) {
	if (!added?.trim()) return current;
	if (!isJoinedComponentLabel(current, existingParts, maxLength)) return current;
	const next = joinComponentLabels([...existingParts, added]);
	return maxLength ? next.slice(0, maxLength) : next;
}

export type VMcpFormData = {
	displayName: string;
	description: string;
};

export const initVMcp = (): VMcpFormData => ({
	displayName: '',
	description: ''
});

export function vmcpConnectURL(vmcp: VMCP) {
	const link = vmcp.links?.connectURL || vmcp.links?.['mcp-connect'];
	if (link) return link;
	const origin = typeof window !== 'undefined' ? window.location.origin : '';
	return `${origin}/mcp-connect/${encodeURIComponent(vmcp.id)}`;
}

export function vmcpManifest(vmcp: VMCP): VMCPManifest {
	return {
		displayName: vmcp.displayName,
		description: vmcp.description,
		icon: vmcp.icon,
		components: vmcp.components ?? [],
		profiles: vmcp.profiles,
		forceSingleUser: vmcp.forceSingleUser
	};
}

export function catalogEntryToVMCPComponent(entry: MCPCatalogEntry): VMCPComponent {
	return {
		name: entry.manifest.name ?? entry.id,
		mcpCatalogID: DEFAULT_MCP_CATALOG_ID,
		mcpServerCatalogEntryID: entry.id,
		catalogEntry: {
			manifest: entry.manifest,
			unsupportedTools: entry.unsupportedTools
		}
	};
}

export function catalogConfigurationFields(entry: MCPCatalogEntry): MCPConfig[] {
	if (entry.manifest.config?.length) {
		return entry.manifest.config;
	}

	const { env, headers } = getManifestConfiguration(entry.manifest);
	return [
		...env.map((field) => ({
			...field,
			usage: (field.dynamicFile
				? 'dynamicFile'
				: field.file
					? 'file'
					: field.interpolated
						? 'interpolated'
						: 'env') as MCPConfig['usage']
		})),
		...headers.map((field) => ({
			...field,
			usage: 'header' as const
		}))
	];
}

export function vmcpComponentId(component: VMCPComponent) {
	return component.id || component.mcpServerCatalogEntryID || '';
}

/** Personal servers belong to a power user's workspace rather than the shared catalog. */
export function isWorkspaceOwned(entry: MCPCatalogEntry | VMCP) {
	return Boolean(
		('powerUserWorkspaceID' in entry && entry.powerUserWorkspaceID) ||
		('powerUserID' in entry && entry.powerUserID) ||
		('userID' in entry && entry.userID)
	);
}

export function parseSelectedFilterIds(selected: string) {
	return selected
		.split(',')
		.map((id) => id.trim())
		.filter(Boolean);
}

function componentServerIds(vmcp: VMCP) {
	return (vmcp.components ?? [])
		.map((component) => component.mcpServerCatalogEntryID || component.id)
		.filter((id): id is string => Boolean(id));
}

function componentServerCount(vmcp: VMCP) {
	return vmcp.components?.length ?? 0;
}

function sortFilterOptions(options: VMcpFilterOption[]) {
	return options.sort((a, b) => a.label.localeCompare(b.label, undefined, { sensitivity: 'base' }));
}

function matchesOwnerQuery(vmcp: VMCP, query: string, owners: Map<string, OrgUser>) {
	const owner = vmcp.userID && owners.get(vmcp.userID);
	if (!owner) return false;
	return (
		owner.username.toLowerCase().includes(query) ||
		owner.email.toLowerCase().includes(query) ||
		Boolean(owner.displayName?.toLowerCase().includes(query))
	);
}

function matchesVMcpQuery(vmcp: VMCP, query: string, owners: Map<string, OrgUser>) {
	if (!query) return true;
	return matchesQuery(vmcp, query) || matchesOwnerQuery(vmcp, query.toLowerCase(), owners);
}

export function matchesVMcpFilters(vmcp: VMCP, filters: VMcpFilters, owners: Map<string, OrgUser>) {
	const query = (filters.query ?? '').trim();
	const componentIds = parseSelectedFilterIds(filters.components ?? '');
	if (!query && componentIds.length === 0) return true;
	const matchesComponents =
		componentIds.length === 0 || componentIds.some((id) => componentServerIds(vmcp).includes(id));
	return matchesVMcpQuery(vmcp, query, owners) && matchesComponents;
}

export function filterVMcps(vmcps: VMCP[], filters: VMcpFilters, owners: Map<string, OrgUser>) {
	const query = (filters.query ?? '').trim();
	const componentIds = parseSelectedFilterIds(filters.components ?? '');
	if (!query && componentIds.length === 0) return vmcps;
	return vmcps.filter((vmcp) => matchesVMcpFilters(vmcp, filters, owners));
}

export function buildVMcpComponentFilterOptions(
	vmcps: VMCP[],
	componentName?: (id: string) => string | undefined
): VMcpFilterOption[] {
	const options: VMcpFilterOption[] = [];
	const seen = new Set<string>();
	for (const vmcp of vmcps) {
		for (const component of vmcp.components ?? []) {
			const id = component.mcpServerCatalogEntryID || component.id;
			if (!id || seen.has(id)) continue;
			seen.add(id);
			options.push({
				id,
				label: componentName?.(id) || component.name || component.catalogEntry?.manifest?.name || id
			});
		}
	}
	return sortFilterOptions(options);
}

function compareNames(a: VMCP, b: VMCP) {
	return (a.displayName ?? '').localeCompare(b.displayName ?? '', undefined, {
		sensitivity: 'base'
	});
}

export function sortVMcps(vmcps: VMCP[], sortBy: VMcpSortBy) {
	return [...vmcps].sort((a, b) => {
		if (sortBy === 'created') {
			return (b.created ?? '').localeCompare(a.created ?? '') || compareNames(a, b);
		}
		if (sortBy === 'componentServers') {
			return componentServerCount(b) - componentServerCount(a) || compareNames(a, b);
		}
		return compareNames(a, b);
	});
}

export type McpServerSortBy = 'nameAsc' | 'nameDesc' | 'created' | 'popularity';

export const MCP_SERVER_SORT_OPTIONS: Array<{ id: McpServerSortBy; label: string }> = [
	{ id: 'nameAsc', label: 'Alphabetical (A-Z)' },
	{ id: 'nameDesc', label: 'Alphabetical (Z-A)' },
	{ id: 'created', label: 'Created Date' },
	{ id: 'popularity', label: 'Most Popular' }
];

function normalizeServerName(name: string) {
	return name.toLowerCase().replace(/[_-]+/g, ' ').replace(/\s+/g, ' ').trim();
}

const MCP_SERVER_POPULARITY_RANK = new Map(
	MCP_SERVER_POPULARITY_ORDER.map((name, index) => [name, index])
);

function popularityRank(entry: MCPCatalogEntry) {
	const key = normalizeServerName(entry.manifest.name ?? '');
	const exact = MCP_SERVER_POPULARITY_RANK.get(key as (typeof MCP_SERVER_POPULARITY_ORDER)[number]);
	if (exact !== undefined) return exact;

	for (const suffix of [' official', ' workspace', ' cloud']) {
		if (!key.endsWith(suffix)) continue;
		const base = MCP_SERVER_POPULARITY_RANK.get(
			key.slice(0, -suffix.length).trim() as (typeof MCP_SERVER_POPULARITY_ORDER)[number]
		);
		if (base !== undefined) return base;
	}

	return MCP_SERVER_POPULARITY_ORDER.length;
}

function compareCatalogNames(a: MCPCatalogEntry, b: MCPCatalogEntry) {
	return (a.manifest.name ?? '').localeCompare(b.manifest.name ?? '', undefined, {
		sensitivity: 'base'
	});
}

export function sortMcpServers(entries: MCPCatalogEntry[], sortBy: McpServerSortBy) {
	return [...entries].sort((a, b) => {
		if (sortBy === 'created') {
			return (b.created ?? '').localeCompare(a.created ?? '') || compareCatalogNames(a, b);
		}
		if (sortBy === 'nameDesc') {
			return compareCatalogNames(b, a);
		}
		if (sortBy === 'popularity') {
			return popularityRank(a) - popularityRank(b) || compareCatalogNames(a, b);
		}
		return compareCatalogNames(a, b);
	});
}

export function entryCategories(entry: MCPCatalogEntry) {
	return (entry.manifest.metadata?.categories ?? '')
		.split(',')
		.map((category) => category.trim())
		.filter(Boolean);
}

/** True when no categories are selected, or when the entry has any selected category (OR). */
export function matchesMcpServerCategoryFilters(entry: MCPCatalogEntry, selected: string[]) {
	if (selected.length === 0) return true;
	const categories = entryCategories(entry);
	return selected.some((category) => categories.includes(category));
}

export function filterMcpServersByCategories(entries: MCPCatalogEntry[], selected: string) {
	const ids = parseSelectedFilterIds(selected);
	if (ids.length === 0) return entries;
	return entries.filter((entry) => matchesMcpServerCategoryFilters(entry, ids));
}

export function buildMcpServerFilterOptions(entries: MCPCatalogEntry[]): VMcpFilterOption[] {
	const options: VMcpFilterOption[] = [];
	const seen = new Set<string>();

	for (const entry of entries) {
		for (const category of entryCategories(entry)) {
			if (seen.has(category)) continue;
			seen.add(category);
			options.push({ id: category, label: category });
		}
	}

	return options.sort((a, b) => a.label.localeCompare(b.label, undefined, { sensitivity: 'base' }));
}

export function matchesQuery(item: MCPCatalogEntry | MCPCatalogServer | VMCP, query: string) {
	const needle = query.toLowerCase();
	if ('displayName' in item) {
		return Boolean(
			item.displayName?.toLowerCase().includes(needle) ||
			item.description?.toLowerCase().includes(needle)
		);
	}
	const { name, description, shortDescription } = item.manifest;
	return Boolean(
		name?.toLowerCase().includes(needle) ||
		description?.toLowerCase().includes(needle) ||
		shortDescription?.toLowerCase().includes(needle)
	);
}

export function distanceToRect(rect: RectLike, point: Point) {
	const dx = Math.max(rect.left - point.x, 0, point.x - rect.right);
	const dy = Math.max(rect.top - point.y, 0, point.y - rect.bottom);
	return Math.hypot(dx, dy);
}

export function borderAnchor(rect: RectLike, point: Point): Point {
	const centerX = rect.left + rect.width / 2;
	const centerY = rect.top + rect.height / 2;
	const dx = point.x - centerX;
	const dy = point.y - centerY;
	if (dx === 0 && dy === 0) return { x: centerX, y: centerY };

	const scaleX = dx === 0 ? Infinity : rect.width / 2 / Math.abs(dx);
	const scaleY = dy === 0 ? Infinity : rect.height / 2 / Math.abs(dy);
	const scale = Math.min(1, scaleX, scaleY);
	return { x: centerX + dx * scale, y: centerY + dy * scale };
}

/**
 * Polyline approximating a sine arch between two points, sampled along the straight
 * segment and displaced along its normal. `sin(pi * u)` pins both ends to the anchors,
 * and a single traveling harmonic driven by `phase` makes the wire shimmer while it is held.
 */
export function buildWirePath(from: Point, to: Point, phase = 0) {
	const dx = to.x - from.x;
	const dy = to.y - from.y;
	const distance = Math.hypot(dx, dy);
	if (distance < 1) {
		return `M${from.x.toFixed(1)},${from.y.toFixed(1)}`;
	}

	const angle = Math.atan2(dy, dx);
	const normalX = Math.cos(angle + Math.PI / 2);
	const normalY = Math.sin(angle + Math.PI / 2);
	const arch = Math.min(distance * 0.18, WIRE_MAX_ARCH_PX);

	const points: string[] = [];
	for (let step = 0; step <= WIRE_SEGMENTS; step++) {
		const u = step / WIRE_SEGMENTS;
		const displacement =
			arch * Math.sin(Math.PI * u) * (1 + 0.15 * Math.sin(2 * Math.PI * u - phase));
		const x = from.x + dx * u + normalX * displacement;
		const y = from.y + dy * u + normalY * displacement;
		points.push(`${step === 0 ? 'M' : 'L'}${x.toFixed(1)},${y.toFixed(1)}`);
	}
	return points.join(' ');
}

export function resolveVMcpComponents(vmcp: VMCP): VMcpComponentView[] {
	return (vmcp.components ?? []).map((component, index) => {
		const manifest = component.catalogEntry?.manifest;
		const id = vmcpComponentId(component) || `component-${index}`;
		return {
			key: id,
			name: component.name || manifest?.name || id || 'Unknown server',
			icon: manifest?.icon,
			description: manifest?.shortDescription || manifest?.description,
			id,
			toolOverrides: component.toolOverrides,
			toolPreview: manifest?.toolPreview
		};
	});
}

function mcpConfigKey(name: string, id: string, used: Set<string>) {
	let key = name.trim() || id;
	if (used.has(key)) {
		key = `${key} (${id})`;
	}
	used.add(key);
	return key;
}

function httpMcpServers(vmcps: VMCP[]) {
	const used = new Set<string>();
	const servers: Record<string, { type: 'http'; url: string }> = {};
	for (const vmcp of vmcps) {
		const url = vmcpConnectURL(vmcp);
		if ((vmcp.components ?? []).length === 0) continue;
		if (!url) continue;
		servers[mcpConfigKey(vmcp.displayName || vmcp.id, vmcp.id, used)] = {
			type: 'http',
			url
		};
	}
	return servers;
}

function toTomlQuotedKey(name: string) {
	if (/^[A-Za-z0-9_-]+$/.test(name)) return name;
	return JSON.stringify(name);
}

export function buildConnectAllSnippets(
	clientId: AiClient,
	vmcps: VMCP[],
	admin: boolean
): { id: string; label: string; value: string }[] {
	const servers = httpMcpServers(vmcps);
	if (clientId === AiClient.Codex) {
		const value = Object.entries(servers)
			.map(
				([name, server]) =>
					`[mcp_servers.${toTomlQuotedKey(name)}]\nurl = ${JSON.stringify(server.url)}`
			)
			.join('\n\n');
		return [{ id: 'codex-config-toml', label: 'config.toml', value }];
	}

	if (clientId === AiClient.VSCode) {
		return [
			{
				id: 'vscode-mcp-json',
				label: 'mcp.json',
				value: JSON.stringify({ servers }, null, 2)
			}
		];
	}

	if (clientId === AiClient.Claude && admin) {
		const config = {
			allowedMcpServers: Object.values(servers).map((server) => ({ serverUrl: server.url }))
		};
		return [
			{
				id: 'claude-settings-json',
				label: 'Claude Enterprise',
				value: JSON.stringify(config, null, 2)
			},
			{
				id: `${clientId}-mcp-json`,
				label: 'mcp.json',
				value: JSON.stringify({ mcpServers: servers }, null, 2)
			}
		];
	}

	return [
		{
			id: `${clientId}-mcp-json`,
			label: 'mcp.json',
			value: JSON.stringify({ mcpServers: servers }, null, 2)
		}
	];
}

/** Servers without tool overrides only report a preview, so those totals are approximate. */
export function getToolCounts(componentServers: VMcpComponentView[]) {
	let enabled = 0;
	let total = 0;
	let approximate = false;
	for (const component of componentServers) {
		if (component.toolOverrides) {
			enabled += component.toolOverrides.filter((tool) => tool.enabled === true).length;
			total += component.toolOverrides.length;
			continue;
		}
		approximate = true;
		const previewCount = component.toolPreview?.length ?? 0;
		enabled += previewCount;
		total += previewCount;
	}
	return { enabled, total, approximate };
}

export function getProfilesDisplayText(profiles?: VMCPProfile[]) {
	if (!profiles) return '';

	const names = profiles.map((profile) => profile.name);
	if (names.length <= 1) return names[0] ?? '';

	const rest = names.slice(0, names.length > 5 ? 4 : -1);
	const last = names.length > 5 ? `${names.length - 4} others` : names.at(-1);
	return `${rest.join(', ')} and ${last}`;
}
