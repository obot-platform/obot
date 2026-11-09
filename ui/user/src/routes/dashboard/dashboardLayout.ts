import { MODEL_STAT_IDS, type ModelStatId } from '$lib/components/admin/token-usage/constants';
import { m } from '$lib/i18n';
import type { TotalTokenUsage } from '$lib/services';

export const DASHBOARD_LAYOUT_KEY = '@obot/dashboard/layout';

/** Saved layouts are per user, so accounts sharing a browser don't overwrite each other's layout. */
export function dashboardLayoutKey(userId: string): string {
	return `${DASHBOARD_LAYOUT_KEY}/${userId}`;
}

export const MAX_WIDGETS_PER_ROW = 4;

export const WIDGET_IDS = [
	'platform-stats',
	'device-scan-stats',
	'server-activity',
	'top-device-skills',
	'most-deployed',
	'device-clients',
	'top-device-mcps',
	'top-servers-used',
	'device-scan-timeline',
	'token-usage',
	'popular-tools',
	'tool-response-time',
	'model-stats',
	'llm-requests',
	'spend-by-provider',
	'top-models',
	'most-frequent-tool-calls',
	'tool-individual-response-time',
	'tool-call-errors',
	'tool-call-errors-by-server',
	'most-active-users'
] as const;

export type WidgetId = (typeof WIDGET_IDS)[number];

export const PLATFORM_STAT_IDS = [
	'total-users',
	'active-users',
	'total-tokens',
	'total-spend'
] as const;

export type PlatformStatId = (typeof PLATFORM_STAT_IDS)[number];

export const DEVICE_STAT_IDS = [
	'device-overview',
	'device-clients',
	'device-mcps',
	'device-skills'
] as const;

export type DeviceStatId = (typeof DEVICE_STAT_IDS)[number];

export { MODEL_STAT_IDS, type ModelStatId };

export type WidgetPlacement = {
	id: WidgetId;
	hideWhenEmpty: boolean;
};

export const HEADER_VISIBILITIES = ['always', 'device-scans', 'mcp-stats', 'llm-stats'] as const;

export type HeaderVisibility = (typeof HEADER_VISIBILITIES)[number];

const headerVisibilitySet = new Set<string>(HEADER_VISIBILITIES);

export type HeaderAvailability = {
	deviceScans: boolean;
	mcpStats: boolean;
	llmStats: boolean;
};

export type HeaderRow = {
	kind: 'header';
	id: string;
	text: string;
	visibility: HeaderVisibility;
};

export type WidgetRow = {
	kind: 'widgets';
	items: WidgetPlacement[];
};

export type BoxRow = {
	kind: 'box';
	id: string;
	rows: DashboardRow[];
};

export type DashboardRow = HeaderRow | WidgetRow | BoxRow;

export type DashboardConfig = {
	rows: DashboardRow[];
	platformStats: PlatformStatId[];
	deviceStats: DeviceStatId[];
	modelStats: ModelStatId[];
};

export type DashboardViewer = {
	isAdmin: boolean;
};

export type WidgetStatus = {
	loading: boolean;
	empty: boolean;
};

export type DropTarget =
	| { kind: 'slot'; path: number[]; index: number }
	| { kind: 'new-row'; path: number[]; index: number };

export type DashboardSignals = {
	platformTiles: { loading: boolean; value: number }[];
	deviceTiles: { loading: boolean; value: number }[];
	serverLoading: boolean;
	totalServers: number;
	toolUsageLoading: boolean;
	topServerCount: number;
	popularToolCount: number;
	responseTimeCount: number;
	serversLoading: boolean;
	deployedCount: number;
	deviceScanLoading: boolean;
	skillCount: number;
	clientCount: number;
	mcpCount: number;
	scanCount: number;
	tokenLoading: boolean;
	totalTokens: number;
	totalSpend: number;
	modelTiles: { loading: boolean; value: number }[];
	requestStatsLoading: boolean;
	requestCount: number;
	providerSpendCount: number;
	topModelCount: number;
	individualResponseCount: number;
	toolErrorCount: number;
	serverErrorCount: number;
	activeUserCount: number;
};

const ADMIN_ONLY = new Set<WidgetId>([
	'platform-stats',
	'device-scan-stats',
	'server-activity',
	'top-device-skills',
	'most-deployed',
	'device-clients',
	'top-device-mcps',
	'device-scan-timeline',
	'token-usage',
	'model-stats',
	'llm-requests',
	'spend-by-provider',
	'top-models',
	'most-active-users'
]);

const INVENTORY_WIDGETS = new Set<WidgetId>([
	'device-scan-stats',
	'device-clients',
	'top-device-mcps',
	'top-device-skills',
	'device-scan-timeline'
]);

const POWER_USER_ROWS: WidgetId[][] = [
	['top-servers-used'],
	['popular-tools', 'tool-response-time']
];

const widgetIdSet = new Set<string>(WIDGET_IDS);

export function isWidgetId(value: unknown): value is WidgetId {
	return typeof value === 'string' && widgetIdSet.has(value);
}

export function defaultHideWhenEmpty(id: WidgetId): boolean {
	return INVENTORY_WIDGETS.has(id);
}

export function availableWidgets(viewer: DashboardViewer): WidgetId[] {
	return WIDGET_IDS.filter((id) => viewer.isAdmin || !ADMIN_ONLY.has(id));
}

export function factoryConfig(viewer: DashboardViewer): DashboardConfig {
	const rows: DashboardRow[] = viewer.isAdmin
		? [
				widgetRow(['platform-stats']),
				boxRow('box-mcp-servers', [
					headerRow('header-mcp-servers', m.dashboard_section_mcp_servers(), 'always'),
					widgetRow(['server-activity', 'top-servers-used']),
					widgetRow(['most-deployed', 'popular-tools', 'tool-response-time']),
					widgetRow(['token-usage'])
				]),
				boxRow('box-inventory', [
					headerRow('header-inventory', m.dashboard_section_inventory(), 'device-scans'),
					widgetRow(['device-scan-stats']),
					widgetRow(['device-clients', 'top-device-mcps', 'top-device-skills']),
					widgetRow(['device-scan-timeline'])
				])
			]
		: POWER_USER_ROWS.map((row) => widgetRow(row));
	return {
		rows,
		platformStats: [...PLATFORM_STAT_IDS],
		deviceStats: [...DEVICE_STAT_IDS],
		modelStats: [...MODEL_STAT_IDS]
	};
}

export function visibleStatIds<T extends string>(
	selected: readonly T[],
	catalog: readonly T[]
): T[] {
	const chosen = new Set(selected);
	return catalog.filter((id) => chosen.has(id));
}

export function projectConfig(config: DashboardConfig, viewer: DashboardViewer): DashboardConfig {
	const available = new Set(availableWidgets(viewer));
	const seen = new Set<WidgetId>();
	return {
		rows: projectRows(config.rows, available, seen),
		platformStats: visibleStatIds(config.platformStats, PLATFORM_STAT_IDS),
		deviceStats: visibleStatIds(config.deviceStats, DEVICE_STAT_IDS),
		modelStats: visibleStatIds(config.modelStats, MODEL_STAT_IDS)
	};
}

export function parseDashboardConfig(value: string | null): DashboardConfig | null {
	if (!value) return null;

	try {
		const parsed: unknown = JSON.parse(value);
		if (!parsed || typeof parsed !== 'object') return null;
		const record = parsed as {
			rows?: unknown;
			platformStats?: unknown;
			deviceStats?: unknown;
			modelStats?: unknown;
		};
		const rows = normalizeRows(record.rows);
		if (!rows) return null;
		return {
			rows,
			platformStats: normalizeStatIds(record.platformStats, PLATFORM_STAT_IDS),
			deviceStats: normalizeStatIds(record.deviceStats, DEVICE_STAT_IDS),
			modelStats: normalizeStatIds(record.modelStats, MODEL_STAT_IDS)
		};
	} catch {
		return null;
	}
}

export function rowsForRender(
	config: DashboardConfig,
	statuses: Partial<Record<WidgetId, WidgetStatus>>,
	availability: HeaderAvailability = {
		deviceScans: true,
		mcpStats: true,
		llmStats: true
	}
): DashboardRow[] {
	return renderRows(config.rows, statuses, availability);
}

export function hasRenderedContent(rows: DashboardRow[]): boolean {
	return rows.some((row) => {
		if (row.kind === 'header') return row.text.trim().length > 0;
		if (row.kind === 'box') return hasRenderedContent(row.rows);
		return row.items.length > 0;
	});
}

/** Visibility modes used by any header in the layout, including inside boxes. */
export function headerVisibilities(
	rows: DashboardRow[],
	found = new Set<HeaderVisibility>()
): Set<HeaderVisibility> {
	for (const row of rows) {
		if (row.kind === 'header') found.add(row.visibility);
		else if (row.kind === 'box') headerVisibilities(row.rows, found);
	}
	return found;
}

/** Range-backed datasets the dashboard fetches; each is only fetched when a placed widget uses it. */
export type DashboardDataset = 'mcp-usage' | 'device-scans' | 'token-totals' | 'token-stats';

const DATASET_WIDGETS: Record<DashboardDataset, readonly WidgetId[]> = {
	'mcp-usage': [
		'top-servers-used',
		'popular-tools',
		'tool-response-time',
		'most-frequent-tool-calls',
		'tool-individual-response-time',
		'tool-call-errors',
		'tool-call-errors-by-server',
		'most-active-users'
	],
	'device-scans': [
		'device-scan-stats',
		'top-device-skills',
		'device-clients',
		'top-device-mcps',
		'device-scan-timeline'
	],
	// token-usage renders its own timeline but its hide-when-empty status reads these totals.
	'token-totals': ['model-stats', 'token-usage'],
	'token-stats': ['llm-requests', 'spend-by-provider', 'top-models']
};

const TOKEN_PLATFORM_STATS: readonly PlatformStatId[] = ['total-tokens', 'total-spend'];

export function neededDatasets(config: DashboardConfig): Set<DashboardDataset> {
	const placed = placedWidgetIds(config.rows);
	const needed = new Set<DashboardDataset>();
	for (const [dataset, widgets] of Object.entries(DATASET_WIDGETS) as [
		DashboardDataset,
		readonly WidgetId[]
	][]) {
		if (widgets.some((id) => placed.has(id))) needed.add(dataset);
	}
	if (
		placed.has('platform-stats') &&
		config.platformStats.some((id) => TOKEN_PLATFORM_STATS.includes(id))
	) {
		needed.add('token-totals');
	}
	return needed;
}

export function placedWidgetIds(rows: DashboardRow[], ids = new Set<WidgetId>()): Set<WidgetId> {
	for (const row of rows) {
		if (row.kind === 'widgets') for (const placement of row.items) ids.add(placement.id);
		else if (row.kind === 'box') placedWidgetIds(row.rows, ids);
	}
	return ids;
}

export function headerVisible(row: HeaderRow, availability: HeaderAvailability): boolean {
	switch (row.visibility) {
		case 'device-scans':
			return availability.deviceScans;
		case 'mcp-stats':
			return availability.mcpStats;
		case 'llm-stats':
			return availability.llmStats;
		default:
			return true;
	}
}

function headerVisibilityFrom(record: {
	visibility?: unknown;
	onlyWithDeviceScans?: unknown;
}): HeaderVisibility {
	if (typeof record.visibility === 'string' && headerVisibilitySet.has(record.visibility)) {
		return record.visibility as HeaderVisibility;
	}
	return record.onlyWithDeviceScans === true ? 'device-scans' : 'always';
}

/**
 * Whether a row renders on the dashboard. The page grid (buildGridCells) and rowsForRender both
 * use it, so the rendered rows agree. A box with no children still renders as a drop target, but
 * hasRenderedContent does not count it: a dashboard of only empty boxes shows the empty state.
 */
export function rowVisible(
	row: DashboardRow,
	statuses: Partial<Record<WidgetId, WidgetStatus>>,
	availability: HeaderAvailability
): boolean {
	if (row.kind === 'header') {
		return row.text.trim().length > 0 && headerVisible(row, availability);
	}
	if (row.kind === 'box') {
		return (
			row.rows.length === 0 || row.rows.some((child) => rowVisible(child, statuses, availability))
		);
	}
	return visibleItems(row.items, statuses).length > 0;
}

export type GridCell =
	| { kind: 'box'; key: string; rowIndex: number; row: BoxRow; children: GridCell[] }
	| { kind: 'header'; key: string; rowIndex: number; row: HeaderRow }
	| {
			kind: 'widget';
			key: string;
			rowIndex: number;
			index: number;
			rowSize: number;
			placement: WidgetPlacement;
	  };

/**
 * The dashboard grid as flat cells per grid level, keyed by widget, header, or box id. Widget rows
 * have no wrapper element, so cards keep their component when a drag changes their row. Built in
 * one pass: a box child is visible exactly when it produces cells, matching rowVisible.
 */
export function buildGridCells(
	rows: DashboardRow[],
	statuses: Partial<Record<WidgetId, WidgetStatus>>,
	availability: HeaderAvailability
): GridCell[] {
	return rows.flatMap((row, rowIndex): GridCell[] => {
		if (row.kind === 'box') {
			const children = buildGridCells(row.rows, statuses, availability);
			if (row.rows.length > 0 && children.length === 0) return [];
			return [{ kind: 'box', key: `box:${row.id}`, rowIndex, row, children }];
		}
		if (row.kind === 'header') {
			if (!rowVisible(row, statuses, availability)) return [];
			return [{ kind: 'header', key: `header:${row.id}`, rowIndex, row }];
		}
		const visible = visibleItems(row.items, statuses);
		return visible.map(({ item, index }) => ({
			kind: 'widget',
			key: `widget:${item.id}`,
			rowIndex,
			index,
			rowSize: visible.length,
			placement: item
		}));
	});
}

/** True when an all-time LLM total shows any usage; drives 'llm-stats' header visibility. */
export function hasLlmUsage(totals: TotalTokenUsage): boolean {
	return (
		totals.totalTokens > 0 ||
		(totals.inputTokens ?? 0) > 0 ||
		(totals.outputTokens ?? 0) > 0 ||
		(totals.totalSpend ?? 0) > 0
	);
}

/** Placements that show in a widget row, with their index in the stored row (for drop targets). */
export function visibleItems(
	items: WidgetPlacement[],
	statuses: Partial<Record<WidgetId, WidgetStatus>>
): { item: WidgetPlacement; index: number }[] {
	return items.flatMap((item, index) =>
		isHiddenBecauseEmpty(item, statuses[item.id]) ? [] : [{ item, index }]
	);
}

export function isHiddenBecauseEmpty(
	placement: WidgetPlacement,
	status: WidgetStatus | undefined
): boolean {
	if (!status || status.loading) return false;
	return status.empty && placement.hideWhenEmpty;
}

export function colSpanClass(count: number): string {
	switch (count) {
		case 1:
			return 'col-span-12 @3xl:col-span-12';
		case 2:
			return 'col-span-12 @3xl:col-span-6';
		case 3:
			return 'col-span-12 @3xl:col-span-4';
		default:
			return 'col-span-12 @3xl:col-span-3';
	}
}

export function statCardStatus(tiles: { loading: boolean; value: number }[]): WidgetStatus {
	if (tiles.some((tile) => tile.loading)) return { loading: true, empty: false };
	// No stats selected is the user's choice, not missing data: keep showing the card.
	if (tiles.length === 0) return { loading: false, empty: false };
	return { loading: false, empty: tiles.every((tile) => tile.value === 0) };
}

export function widgetStatuses(signals: DashboardSignals): Partial<Record<WidgetId, WidgetStatus>> {
	return {
		'platform-stats': statCardStatus(signals.platformTiles),
		'device-scan-stats': statCardStatus(signals.deviceTiles),
		'server-activity': { loading: signals.serverLoading, empty: signals.totalServers === 0 },
		'top-device-skills': {
			loading: signals.deviceScanLoading,
			empty: signals.skillCount === 0
		},
		'most-deployed': { loading: signals.serversLoading, empty: signals.deployedCount === 0 },
		'device-clients': { loading: signals.deviceScanLoading, empty: signals.clientCount === 0 },
		'top-device-mcps': { loading: signals.deviceScanLoading, empty: signals.mcpCount === 0 },
		'top-servers-used': {
			loading: signals.toolUsageLoading,
			empty: signals.topServerCount === 0
		},
		'device-scan-timeline': {
			loading: signals.deviceScanLoading,
			empty: signals.scanCount === 0
		},
		'token-usage': {
			loading: signals.tokenLoading,
			empty: signals.totalTokens === 0 && signals.totalSpend === 0
		},
		'popular-tools': {
			loading: signals.toolUsageLoading,
			empty: signals.popularToolCount === 0
		},
		'tool-response-time': {
			loading: signals.toolUsageLoading,
			empty: signals.responseTimeCount === 0
		},
		'model-stats': statCardStatus(signals.modelTiles),
		'llm-requests': {
			loading: signals.requestStatsLoading,
			empty: signals.requestCount === 0
		},
		'spend-by-provider': {
			loading: signals.requestStatsLoading,
			empty: signals.providerSpendCount === 0
		},
		'top-models': {
			loading: signals.requestStatsLoading,
			empty: signals.topModelCount === 0
		},
		'most-frequent-tool-calls': {
			loading: signals.toolUsageLoading,
			empty: signals.popularToolCount === 0
		},
		'tool-individual-response-time': {
			loading: signals.toolUsageLoading,
			empty: signals.individualResponseCount === 0
		},
		'tool-call-errors': {
			loading: signals.toolUsageLoading,
			empty: signals.toolErrorCount === 0
		},
		'tool-call-errors-by-server': {
			loading: signals.toolUsageLoading,
			empty: signals.serverErrorCount === 0
		},
		'most-active-users': {
			loading: signals.toolUsageLoading,
			empty: signals.activeUserCount === 0
		}
	};
}

export function removeWidget(config: DashboardConfig, id: WidgetId): DashboardConfig {
	return { ...config, rows: stripWidget(config.rows, id) };
}

export function appendWidget(config: DashboardConfig, id: WidgetId): DashboardConfig {
	if (findPosition(config, id)) return config;
	const placement = placementFor(id);
	const last = config.rows.at(-1);
	if (last?.kind === 'widgets' && last.items.length < MAX_WIDGETS_PER_ROW) {
		return {
			...config,
			rows: config.rows.map((row, index) =>
				index === config.rows.length - 1 && row.kind === 'widgets'
					? { ...row, items: [...row.items, placement] }
					: row
			)
		};
	}
	return { ...config, rows: [...config.rows, { kind: 'widgets', items: [placement] }] };
}

export function insertHeader(
	config: DashboardConfig,
	index: number,
	text = '',
	parentPath: number[] = []
): DashboardConfig {
	return insertRow(config, parentPath, index, {
		kind: 'header',
		id: createHeaderId(),
		text,
		visibility: 'always'
	});
}

export function appendHeader(config: DashboardConfig, text = ''): DashboardConfig {
	return insertHeader(config, config.rows.length, text);
}

export function removeHeader(config: DashboardConfig, id: string): DashboardConfig {
	return {
		...config,
		rows: stripRow(config.rows, (row) => row.kind === 'header' && row.id === id)
	};
}

export function insertBox(
	config: DashboardConfig,
	index: number,
	parentPath: number[] = []
): DashboardConfig {
	return insertRow(config, parentPath, index, { kind: 'box', id: createBoxId(), rows: [] });
}

export function appendBox(config: DashboardConfig): DashboardConfig {
	return insertBox(config, config.rows.length);
}

export function removeBox(config: DashboardConfig, id: string): DashboardConfig {
	return { ...config, rows: stripRow(config.rows, (row) => row.kind === 'box' && row.id === id) };
}

export function setHeaderVisibility(
	config: DashboardConfig,
	id: string,
	visibility: HeaderVisibility
): DashboardConfig {
	return {
		...config,
		rows: mapRows(config.rows, (row) =>
			row.kind === 'header' && row.id === id ? { ...row, visibility } : row
		)
	};
}

export function setHeaderText(config: DashboardConfig, id: string, text: string): DashboardConfig {
	return {
		...config,
		rows: mapRows(config.rows, (row) =>
			row.kind === 'header' && row.id === id ? { ...row, text } : row
		)
	};
}

export function moveHeader(
	config: DashboardConfig,
	id: string,
	toIndex: number,
	parentPath: number[] = []
): DashboardConfig {
	return moveMatchedRow(
		config,
		(row) => row.kind === 'header' && row.id === id,
		parentPath,
		toIndex
	);
}

export function moveBox(
	config: DashboardConfig,
	id: string,
	toIndex: number,
	parentPath: number[] = []
): DashboardConfig {
	return moveMatchedRow(config, (row) => row.kind === 'box' && row.id === id, parentPath, toIndex);
}

export function placeWidget(
	config: DashboardConfig,
	id: WidgetId,
	target: DropTarget
): DashboardConfig {
	const source = findPosition(config, id);
	const sourceRow = source ? rowAt(config.rows, source.path) : undefined;
	const placement: WidgetPlacement =
		source && sourceRow?.kind === 'widgets'
			? { ...sourceRow.items[source.index] }
			: placementFor(id);

	if (target.kind === 'slot') {
		const row = rowAt(config.rows, target.path);
		if (!row || row.kind !== 'widgets') return config;
		const alreadyInRow = source ? pathsEqual(source.path, target.path) : false;
		if (!alreadyInRow && row.items.length >= MAX_WIDGETS_PER_ROW) return config;
	}

	let rows = config.rows;
	let removedEmptyRow = false;
	if (source && sourceRow?.kind === 'widgets') {
		const items = sourceRow.items.filter((item) => item.id !== id);
		if (items.length === 0) {
			rows = removeAt(rows, source.path);
			removedEmptyRow = true;
		} else {
			rows = replaceRow(rows, source.path, { kind: 'widgets', items });
		}
	}

	if (target.kind === 'new-row') {
		const adjusted =
			source && removedEmptyRow
				? shiftDestination(source.path, target.path, target.index)
				: { parent: target.path, index: target.index };
		return {
			...config,
			rows: insertRowAt(rows, adjusted.parent, adjusted.index, widgetRowFrom(placement))
		};
	}

	let rowPath = target.path;
	let itemIndex = target.index;
	if (source && removedEmptyRow) {
		const adjusted = shiftDestination(
			source.path,
			target.path.slice(0, -1),
			target.path.at(-1) ?? 0
		);
		if (pathsEqual(source.path, target.path)) {
			return {
				...config,
				rows: insertRowAt(rows, adjusted.parent, adjusted.index, widgetRowFrom(placement))
			};
		}
		rowPath = [...adjusted.parent, adjusted.index];
	} else if (source && pathsEqual(source.path, target.path) && source.index < target.index) {
		itemIndex -= 1;
	}

	const destination = rowAt(rows, rowPath);
	if (!destination || destination.kind !== 'widgets') {
		return {
			...config,
			rows: insertRowAt(rows, rowPath.slice(0, -1), rowPath.at(-1) ?? 0, widgetRowFrom(placement))
		};
	}

	const nextItems = [...destination.items];
	nextItems.splice(clamp(itemIndex, 0, nextItems.length), 0, placement);
	return { ...config, rows: replaceRow(rows, rowPath, { kind: 'widgets', items: nextItems }) };
}

export function setHideWhenEmpty(
	config: DashboardConfig,
	id: WidgetId,
	hideWhenEmpty: boolean
): DashboardConfig {
	return {
		...config,
		rows: mapRows(config.rows, (row) =>
			row.kind === 'widgets'
				? {
						...row,
						items: row.items.map((placement) =>
							placement.id === id ? { ...placement, hideWhenEmpty } : placement
						)
					}
				: row
		)
	};
}

export function togglePlatformStat(config: DashboardConfig, id: PlatformStatId): DashboardConfig {
	const next = config.platformStats.includes(id)
		? config.platformStats.filter((statId) => statId !== id)
		: [...config.platformStats, id];
	return { ...config, platformStats: visibleStatIds(next, PLATFORM_STAT_IDS) };
}

export function toggleModelStat(config: DashboardConfig, id: ModelStatId): DashboardConfig {
	const next = config.modelStats.includes(id)
		? config.modelStats.filter((statId) => statId !== id)
		: [...config.modelStats, id];
	return { ...config, modelStats: visibleStatIds(next, MODEL_STAT_IDS) };
}

export function toggleDeviceStat(config: DashboardConfig, id: DeviceStatId): DashboardConfig {
	const next = config.deviceStats.includes(id)
		? config.deviceStats.filter((statId) => statId !== id)
		: [...config.deviceStats, id];
	return { ...config, deviceStats: visibleStatIds(next, DEVICE_STAT_IDS) };
}

export function commitLayout(
	previous: DashboardConfig,
	nextVisible: DashboardConfig,
	viewer: DashboardViewer
): DashboardConfig {
	const available = new Set(availableWidgets(viewer));
	return {
		rows: commitRows(previous.rows, nextVisible.rows, available),
		platformStats: visibleStatIds(nextVisible.platformStats, PLATFORM_STAT_IDS),
		deviceStats: visibleStatIds(nextVisible.deviceStats, DEVICE_STAT_IDS),
		modelStats: visibleStatIds(nextVisible.modelStats, MODEL_STAT_IDS)
	};
}

function placementFor(id: WidgetId): WidgetPlacement {
	return { id, hideWhenEmpty: defaultHideWhenEmpty(id) };
}

function widgetRow(ids: readonly WidgetId[]): WidgetRow {
	return { kind: 'widgets', items: ids.map(placementFor) };
}

function headerRow(id: string, text: string, visibility: HeaderVisibility): HeaderRow {
	return { kind: 'header', id, text, visibility };
}

function boxRow(id: string, rows: DashboardRow[]): BoxRow {
	return { kind: 'box', id, rows };
}

function widgetRowFrom(placement: WidgetPlacement): WidgetRow {
	return { kind: 'widgets', items: [placement] };
}

let headerSequence = 0;

function createHeaderId(): string {
	headerSequence += 1;
	return `header-${headerSequence.toString(36)}-${Math.random().toString(36).slice(2, 8)}`;
}

function findPosition(
	config: DashboardConfig,
	id: WidgetId
): { path: number[]; index: number } | undefined {
	return findWidget(config.rows, id);
}

function clamp(value: number, min: number, max: number): number {
	return Math.min(Math.max(value, min), max);
}

function pathsEqual(left: number[], right: number[]): boolean {
	return left.length === right.length && left.every((part, index) => part === right[index]);
}

function startsWithPath(path: number[], prefix: number[]): boolean {
	return prefix.length <= path.length && prefix.every((part, index) => path[index] === part);
}

function projectRows(
	rows: DashboardRow[],
	available: Set<WidgetId>,
	seen: Set<WidgetId>
): DashboardRow[] {
	return rows.flatMap((row): DashboardRow[] => {
		if (row.kind === 'header') return [row];
		if (row.kind === 'box') return [{ ...row, rows: projectRows(row.rows, available, seen) }];
		const items = row.items.filter((placement) => {
			if (!available.has(placement.id) || seen.has(placement.id)) return false;
			seen.add(placement.id);
			return true;
		});
		return items.length > 0 ? [{ kind: 'widgets' as const, items }] : [];
	});
}

function renderRows(
	rows: DashboardRow[],
	statuses: Partial<Record<WidgetId, WidgetStatus>>,
	availability: HeaderAvailability
): DashboardRow[] {
	return rows.flatMap((row): DashboardRow[] => {
		if (!rowVisible(row, statuses, availability)) return [];
		if (row.kind === 'header') return [row];
		if (row.kind === 'box') {
			return [{ ...row, rows: renderRows(row.rows, statuses, availability) }];
		}
		return [
			{ kind: 'widgets' as const, items: visibleItems(row.items, statuses).map((v) => v.item) }
		];
	});
}

function stripWidget(rows: DashboardRow[], id: WidgetId): DashboardRow[] {
	return rows.flatMap((row): DashboardRow[] => {
		if (row.kind === 'header') return [row];
		if (row.kind === 'box') return [{ ...row, rows: stripWidget(row.rows, id) }];
		const items = row.items.filter((placement) => placement.id !== id);
		return items.length > 0 ? [{ kind: 'widgets' as const, items }] : [];
	});
}

function stripRow(rows: DashboardRow[], match: (row: DashboardRow) => boolean): DashboardRow[] {
	return rows.flatMap((row): DashboardRow[] => {
		if (match(row)) return [];
		if (row.kind === 'box') return [{ ...row, rows: stripRow(row.rows, match) }];
		return [row];
	});
}

function mapRows(
	rows: DashboardRow[],
	update: (row: DashboardRow) => DashboardRow
): DashboardRow[] {
	return rows.map((row) => {
		const next = update(row);
		if (next.kind !== 'box') return next;
		return { ...next, rows: mapRows(next.rows, update) };
	});
}

function rowsAt(rows: DashboardRow[], path: number[]): DashboardRow[] | undefined {
	let current = rows;
	for (const index of path) {
		const row = current[index];
		if (!row || row.kind !== 'box') return undefined;
		current = row.rows;
	}
	return current;
}

function rowAt(rows: DashboardRow[], path: number[]): DashboardRow | undefined {
	if (path.length === 0) return undefined;
	return rowsAt(rows, path.slice(0, -1))?.[path[path.length - 1]];
}

function updateList(
	rows: DashboardRow[],
	path: number[],
	update: (list: DashboardRow[]) => DashboardRow[]
): DashboardRow[] {
	if (path.length === 0) return update(rows);
	const [index, ...rest] = path;
	return rows.map((row, rowIndex) => {
		if (rowIndex !== index || row.kind !== 'box') return row;
		return { ...row, rows: updateList(row.rows, rest, update) };
	});
}

function removeAt(rows: DashboardRow[], path: number[]): DashboardRow[] {
	const index = path[path.length - 1];
	return updateList(rows, path.slice(0, -1), (list) => list.filter((_, item) => item !== index));
}

function replaceRow(rows: DashboardRow[], path: number[], next: DashboardRow): DashboardRow[] {
	const index = path[path.length - 1];
	return updateList(rows, path.slice(0, -1), (list) =>
		list.map((row, rowIndex) => (rowIndex === index ? next : row))
	);
}

function insertRowAt(
	rows: DashboardRow[],
	parentPath: number[],
	index: number,
	row: DashboardRow
): DashboardRow[] {
	return updateList(rows, parentPath, (list) => {
		const next = [...list];
		next.splice(clamp(index, 0, next.length), 0, row);
		return next;
	});
}

function insertRow(
	config: DashboardConfig,
	parentPath: number[],
	index: number,
	row: DashboardRow
): DashboardConfig {
	return { ...config, rows: insertRowAt(config.rows, parentPath, index, row) };
}

function shiftDestination(
	sourcePath: number[],
	destParent: number[],
	destIndex: number
): { parent: number[]; index: number } {
	const parent = [...destParent];
	let index = destIndex;
	const sourceParent = sourcePath.slice(0, -1);
	const sourceIndex = sourcePath[sourcePath.length - 1];
	if (pathsEqual(sourceParent, parent)) {
		if (sourceIndex < index) index -= 1;
		return { parent, index };
	}
	if (
		sourceParent.length < parent.length &&
		pathsEqual(sourceParent, parent.slice(0, sourceParent.length))
	) {
		const depth = sourceParent.length;
		if (sourceIndex < parent[depth]) parent[depth] -= 1;
	}
	return { parent, index };
}

function findPath(
	rows: DashboardRow[],
	match: (row: DashboardRow) => boolean,
	prefix: number[] = []
): number[] | undefined {
	for (let index = 0; index < rows.length; index += 1) {
		const path = [...prefix, index];
		if (match(rows[index])) return path;
		if (rows[index].kind === 'box') {
			const nested = findPath((rows[index] as BoxRow).rows, match, path);
			if (nested) return nested;
		}
	}
	return undefined;
}

function findWidget(
	rows: DashboardRow[],
	id: WidgetId,
	prefix: number[] = []
): { path: number[]; index: number } | undefined {
	for (let index = 0; index < rows.length; index += 1) {
		const row = rows[index];
		const path = [...prefix, index];
		if (row.kind === 'widgets') {
			const item = row.items.findIndex((placement) => placement.id === id);
			if (item >= 0) return { path, index: item };
		}
		if (row.kind === 'box') {
			const nested = findWidget(row.rows, id, path);
			if (nested) return nested;
		}
	}
	return undefined;
}

function moveMatchedRow(
	config: DashboardConfig,
	match: (row: DashboardRow) => boolean,
	parentPath: number[],
	toIndex: number
): DashboardConfig {
	const sourcePath = findPath(config.rows, match);
	if (!sourcePath) return config;
	if (startsWithPath(parentPath, sourcePath)) return config;
	const row = rowAt(config.rows, sourcePath);
	if (!row) return config;
	const removed = removeAt(config.rows, sourcePath);
	const adjusted = shiftDestination(sourcePath, parentPath, toIndex);
	return { ...config, rows: insertRowAt(removed, adjusted.parent, adjusted.index, row) };
}

type HiddenAnchors = {
	// Hidden widgets that shared a row with this visible widget.
	joined: Map<WidgetId, WidgetPlacement[]>;
	// Hidden-only rows that came right after this key (scope start, header, box, or widget).
	after: Map<string, WidgetPlacement[][]>;
};

const scopeStartKey = (boxId?: string) => (boxId ? `start:${boxId}` : 'start:');
const headerKey = (id: string) => `header:${id}`;
const boxKey = (id: string) => `box:${id}`;
const widgetKey = (id: WidgetId) => `widget:${id}`;

function collectHiddenAnchors(
	rows: DashboardRow[],
	available: Set<WidgetId>,
	anchors: HiddenAnchors,
	scopeKey: string
): void {
	let lastKey = scopeKey;
	for (const row of rows) {
		if (row.kind === 'header') {
			lastKey = headerKey(row.id);
		} else if (row.kind === 'box') {
			collectHiddenAnchors(row.rows, available, anchors, scopeStartKey(row.id));
			lastKey = boxKey(row.id);
		} else {
			const visible = row.items.filter((placement) => available.has(placement.id));
			const hidden = row.items.filter((placement) => !available.has(placement.id));
			if (visible.length > 0) {
				if (hidden.length > 0) anchors.joined.set(visible[0].id, hidden);
				lastKey = widgetKey(visible[0].id);
			} else if (hidden.length > 0) {
				const list = anchors.after.get(lastKey) ?? [];
				list.push(hidden);
				anchors.after.set(lastKey, list);
			}
		}
	}
}

function restoreHiddenRows(
	rows: DashboardRow[],
	anchors: HiddenAnchors,
	scopeKey: string
): DashboardRow[] {
	const takeAfter = (key: string): DashboardRow[] => {
		const list = anchors.after.get(key);
		if (!list) return [];
		anchors.after.delete(key);
		return list.map((items) => ({ kind: 'widgets' as const, items }));
	};

	const out: DashboardRow[] = takeAfter(scopeKey);
	for (const row of rows) {
		if (row.kind === 'header') {
			out.push(row, ...takeAfter(headerKey(row.id)));
			continue;
		}
		if (row.kind === 'box') {
			out.push(
				{ ...row, rows: restoreHiddenRows(row.rows, anchors, scopeStartKey(row.id)) },
				...takeAfter(boxKey(row.id))
			);
			continue;
		}
		const joined = row.items.flatMap((placement) => {
			const hidden = anchors.joined.get(placement.id) ?? [];
			anchors.joined.delete(placement.id);
			return hidden;
		});
		if (row.items.length + joined.length <= MAX_WIDGETS_PER_ROW) {
			out.push({ ...row, items: [...row.items, ...joined] });
		} else {
			out.push(row);
			for (let index = 0; index < joined.length; index += MAX_WIDGETS_PER_ROW) {
				out.push({ kind: 'widgets', items: joined.slice(index, index + MAX_WIDGETS_PER_ROW) });
			}
		}
		for (const placement of row.items) out.push(...takeAfter(widgetKey(placement.id)));
	}
	return out;
}

function commitRows(
	previous: DashboardRow[],
	next: DashboardRow[],
	available: Set<WidgetId>
): DashboardRow[] {
	const anchors: HiddenAnchors = { joined: new Map(), after: new Map() };
	collectHiddenAnchors(previous, available, anchors, scopeStartKey());
	const rows = restoreHiddenRows(next, anchors, scopeStartKey());
	for (const hidden of anchors.joined.values()) rows.push({ kind: 'widgets', items: hidden });
	for (const list of anchors.after.values()) {
		for (const items of list) rows.push({ kind: 'widgets', items });
	}
	return rows.filter((row) => row.kind !== 'widgets' || row.items.length > 0);
}

let boxSequence = 0;

function createBoxId(): string {
	boxSequence += 1;
	return `box-${boxSequence.toString(36)}-${Math.random().toString(36).slice(2, 8)}`;
}

function normalizeRows(
	value: unknown,
	seenWidgets = new Set<WidgetId>(),
	seenHeaders = new Set<string>(),
	seenBoxes = new Set<string>()
): DashboardRow[] | null {
	if (!Array.isArray(value)) return null;
	const rows: DashboardRow[] = [];
	for (const row of value) {
		if (Array.isArray(row)) {
			const items = normalizePlacements(row, seenWidgets);
			if (items.length > 0) rows.push({ kind: 'widgets', items });
			continue;
		}
		if (!row || typeof row !== 'object') return null;
		const record = row as {
			kind?: unknown;
			id?: unknown;
			text?: unknown;
			items?: unknown;
			rows?: unknown;
			visibility?: unknown;
			onlyWithDeviceScans?: unknown;
		};
		if (record.kind === 'box') {
			const id = typeof record.id === 'string' && record.id ? record.id : createBoxId();
			if (seenBoxes.has(id)) continue;
			seenBoxes.add(id);
			const nested = normalizeRows(record.rows, seenWidgets, seenHeaders, seenBoxes);
			if (!nested) return null;
			rows.push({ kind: 'box', id, rows: nested });
			continue;
		}
		if (record.kind === 'header') {
			const id = typeof record.id === 'string' && record.id ? record.id : createHeaderId();
			if (seenHeaders.has(id)) continue;
			seenHeaders.add(id);
			rows.push({
				kind: 'header',
				id,
				text: typeof record.text === 'string' ? record.text : '',
				visibility: headerVisibilityFrom(record)
			});
			continue;
		}
		if (record.kind === 'widgets' && Array.isArray(record.items)) {
			const items = normalizePlacements(record.items, seenWidgets);
			if (items.length > 0) rows.push({ kind: 'widgets', items });
			continue;
		}
		return null;
	}
	return rows;
}

function hideWhenEmptyFlag(
	record: { hideWhenEmpty?: unknown; showWhenEmpty?: unknown },
	id: WidgetId
): boolean {
	if (typeof record.hideWhenEmpty === 'boolean') return record.hideWhenEmpty;
	if (typeof record.showWhenEmpty === 'boolean') return !record.showWhenEmpty;
	return defaultHideWhenEmpty(id);
}

function normalizePlacements(value: unknown[], seen: Set<WidgetId>): WidgetPlacement[] {
	const placements: WidgetPlacement[] = [];
	for (const item of value) {
		if (!item || typeof item !== 'object') continue;
		const id = (item as { id?: unknown }).id;
		if (!isWidgetId(id) || seen.has(id)) continue;
		seen.add(id);
		const record = item as { hideWhenEmpty?: unknown; showWhenEmpty?: unknown };
		placements.push({
			id,
			hideWhenEmpty: hideWhenEmptyFlag(record, id)
		});
	}
	return placements;
}

function normalizeStatIds<T extends string>(value: unknown, catalog: readonly T[]): T[] {
	if (!Array.isArray(value)) return [...catalog];
	return visibleStatIds(
		value.filter((id): id is T => catalog.includes(id as T)),
		catalog
	);
}
