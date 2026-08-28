import type { MCPCatalogEntry, MCPCatalogServer, OrgUser, RuntimeFormData } from '$lib/services';
import { AiClient } from '../user/constants';
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

export const initVMcp = () => {
	const formData: RuntimeFormData = {
		categories: [''],
		metadata: {},
		name: '',
		icon: '',
		shortDescription: '',
		env: [],
		description: '',
		serverUserType: 'singleUser',
		runtime: 'composite',
		resources: undefined,
		npxConfig: undefined,
		uvxConfig: undefined,
		containerizedConfig: undefined,
		remoteConfig: undefined,
		remoteServerConfig: undefined,
		multiUserConfig: undefined,
		compositeConfig: { componentServers: [] }
	};
	return formData;
};

/** Personal servers belong to a power user's workspace rather than the shared catalog. */
export function isWorkspaceOwned(entry: MCPCatalogEntry) {
	return Boolean(entry.powerUserWorkspaceID || entry.powerUserID);
}

export function parseSelectedFilterIds(selected: string) {
	return selected
		.split(',')
		.map((id) => id.trim())
		.filter(Boolean);
}

function componentServerIds(entry: MCPCatalogEntry) {
	return (entry.manifest.compositeConfig?.componentServers ?? [])
		.map((component) => component.catalogEntryID ?? component.mcpServerID)
		.filter((id): id is string => Boolean(id));
}

function componentServerCount(entry: MCPCatalogEntry) {
	return entry.manifest.compositeConfig?.componentServers?.length ?? 0;
}

function sortFilterOptions(options: VMcpFilterOption[]) {
	return options.sort((a, b) => a.label.localeCompare(b.label, undefined, { sensitivity: 'base' }));
}

function matchesOwnerFilter(
	entry: MCPCatalogEntry,
	ownerString: string,
	owners: Map<string, OrgUser>
) {
	const query = ownerString.trim().toLowerCase();
	if (!query) return false;
	const hasMatch = (user: OrgUser) =>
		user.username.toLowerCase().includes(query) ||
		user.email.toLowerCase().includes(query) ||
		Boolean(user.displayName?.toLowerCase().includes(query));
	const poweruser = entry.powerUserID && owners.get(entry.powerUserID);
	const owner = entry.userID && owners.get(entry.userID);
	return Boolean((poweruser && hasMatch(poweruser)) || (owner && hasMatch(owner)));
}

function selectedVMcpFilterIds(filters: VMcpFilters) {
	return {
		nameIds: parseSelectedFilterIds(filters.names ?? ''),
		ownerString: (filters.owners ?? '').trim(),
		componentIds: parseSelectedFilterIds(filters.components ?? '')
	};
}

/** True when no filters are selected, or when the entry matches any selected filter (OR). */
export function matchesVMcpFilters(
	entry: MCPCatalogEntry,
	filters: VMcpFilters,
	owners: Map<string, OrgUser>
) {
	const { nameIds, ownerString, componentIds } = selectedVMcpFilterIds(filters);
	if (nameIds.length === 0 && !ownerString && componentIds.length === 0) return true;
	return (
		nameIds.includes(entry.id) ||
		matchesOwnerFilter(entry, ownerString, owners) ||
		componentIds.some((id) => componentServerIds(entry).includes(id))
	);
}

export function filterVMcps(
	entries: MCPCatalogEntry[],
	filters: VMcpFilters,
	owners: Map<string, OrgUser>
) {
	const { nameIds, ownerString, componentIds } = selectedVMcpFilterIds(filters);
	if (nameIds.length === 0 && !ownerString && componentIds.length === 0) return entries;
	return entries.filter((entry) => matchesVMcpFilters(entry, filters, owners));
}

export function buildVMcpComponentFilterOptions(
	entries: MCPCatalogEntry[],
	componentName?: (id: string) => string | undefined
): VMcpFilterOption[] {
	const options: VMcpFilterOption[] = [];
	const seen = new Set<string>();
	for (const entry of entries) {
		for (const component of entry.manifest.compositeConfig?.componentServers ?? []) {
			const id = component.catalogEntryID ?? component.mcpServerID;
			if (!id || seen.has(id)) continue;
			seen.add(id);
			options.push({
				id,
				label: componentName?.(id) || component.manifest?.name || id
			});
		}
	}
	return sortFilterOptions(options);
}

function compareNames(a: MCPCatalogEntry, b: MCPCatalogEntry) {
	return (a.manifest.name ?? '').localeCompare(b.manifest.name ?? '', undefined, {
		sensitivity: 'base'
	});
}

export function sortVMcps(entries: MCPCatalogEntry[], sortBy: VMcpSortBy) {
	return [...entries].sort((a, b) => {
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

export function sortMcpServers(entries: MCPCatalogEntry[], sortBy: McpServerSortBy) {
	return [...entries].sort((a, b) => {
		if (sortBy === 'created') {
			return (b.created ?? '').localeCompare(a.created ?? '') || compareNames(a, b);
		}
		if (sortBy === 'nameDesc') {
			return compareNames(b, a);
		}
		if (sortBy === 'popularity') {
			return popularityRank(a) - popularityRank(b) || compareNames(a, b);
		}
		return compareNames(a, b);
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

export function matchesQuery(item: MCPCatalogEntry | MCPCatalogServer, query: string) {
	const needle = query.toLowerCase();
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

export function resolveVMcpComponents(
	vmcp: MCPCatalogEntry,
	entries: MCPCatalogEntry[],
	servers: MCPCatalogServer[]
): VMcpComponentView[] {
	return (vmcp.manifest.compositeConfig?.componentServers ?? []).map((component, index) => {
		const entry = entries.find((candidate) => candidate.id === component.catalogEntryID);
		const server = servers.find((candidate) => candidate.id === component.mcpServerID);
		const manifest = entry?.manifest ?? server?.manifest ?? component.manifest;
		const reference = component.catalogEntryID ?? component.mcpServerID;
		return {
			key: reference ?? `component-${index}`,
			name: manifest?.name || reference || 'Unknown server',
			icon: manifest?.icon,
			description: manifest?.shortDescription,
			id: reference,
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

function httpMcpServers(vmcps: MCPCatalogEntry[]) {
	const used = new Set<string>();
	const servers: Record<string, { type: 'http'; url: string }> = {};
	for (const vmcp of vmcps) {
		const url = vmcp.connectURL;
		if (!url) continue;
		servers[mcpConfigKey(vmcp.manifest.name ?? vmcp.id, vmcp.id, used)] = {
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
	vmcps: MCPCatalogEntry[],
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
