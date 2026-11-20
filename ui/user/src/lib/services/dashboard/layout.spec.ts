import {
	MODEL_STAT_IDS,
	PLATFORM_STAT_IDS,
	appendBox,
	appendHeader,
	insertBox,
	insertHeader,
	appendWidget,
	availableWidgets,
	colSpanClass,
	buildGridCells,
	commitLayout,
	dashboardLayoutKey,
	factoryConfig,
	hasRenderedContent,
	headerVisibilities,
	hasLlmUsage,
	moveHeader,
	neededDatasets,
	parseDashboardConfig,
	placeWidget,
	placedWidgetIds,
	projectConfig,
	removeWidget,
	rowVisible,
	rowsForRender,
	setHeaderText,
	setHideWhenEmpty,
	statCardStatus,
	toggleModelStat,
	togglePlatformStat,
	visibleItems,
	widgetStatuses,
	type DashboardConfig,
	type DashboardRow,
	type DashboardSignals,
	type WidgetId,
	type WidgetPlacement
} from './layout';
import { describe, expect, it } from 'vitest';

function ids(rows: DashboardRow[]): unknown[] {
	return rows.map((row) => {
		if (row.kind === 'header') return [row.id];
		if (row.kind === 'box') return { id: row.id, rows: ids(row.rows) };
		return row.items.map((placement) => placement.id);
	});
}

function widgetRow(widgetIds: WidgetId[]): DashboardRow {
	return {
		kind: 'widgets',
		items: widgetIds.map((id) => ({ id, hideWhenEmpty: false }))
	};
}

function placements(rows: DashboardRow[]): WidgetPlacement[] {
	return rows.flatMap((row): WidgetPlacement[] => {
		if (row.kind === 'widgets') return row.items;
		if (row.kind === 'box') return placements(row.rows);
		return [];
	});
}

function hasRow(rows: DashboardRow[], match: (row: DashboardRow) => boolean): boolean {
	return rows.some((row) => match(row) || (row.kind === 'box' && hasRow(row.rows, match)));
}

function findWidgetRow(rows: DashboardRow[], id: string): DashboardRow | undefined {
	for (const row of rows) {
		if (row.kind === 'widgets' && row.items.some((item) => item.id === id)) return row;
		if (row.kind === 'box') {
			const nested = findWidgetRow(row.rows, id);
			if (nested) return nested;
		}
	}
	return undefined;
}

function signals(overrides: Partial<DashboardSignals> = {}): DashboardSignals {
	return {
		platformTiles: [{ loading: false, value: 1 }],
		deviceTiles: [{ loading: false, value: 1 }],
		serverLoading: false,
		totalServers: 1,
		toolUsageLoading: false,
		topServerCount: 1,
		popularToolCount: 1,
		responseTimeCount: 1,
		serversLoading: false,
		deployedCount: 1,
		deviceScanLoading: false,
		skillCount: 1,
		clientCount: 1,
		mcpCount: 1,
		scanCount: 1,
		tokenLoading: false,
		totalTokens: 1,
		totalSpend: 1,
		modelTiles: [{ loading: false, value: 1 }],
		requestStatsLoading: false,
		requestCount: 1,
		providerSpendCount: 1,
		topModelCount: 1,
		individualResponseCount: 1,
		toolErrorCount: 1,
		serverErrorCount: 1,
		activeUserCount: 1,
		...overrides
	};
}

describe('dashboard layout', () => {
	it('uses the sectioned arrangement for every admin', () => {
		const layout = factoryConfig({ isAdmin: true });
		const inventoryWidgets = [
			'device-scan-stats',
			'device-clients',
			'top-device-mcps',
			'top-device-skills',
			'device-scan-timeline'
		];

		expect(ids(layout.rows)).toEqual([
			['platform-stats'],
			{
				id: 'box-mcp-servers',
				rows: [
					['header-mcp-servers'],
					['server-activity', 'top-servers-used'],
					['most-deployed', 'popular-tools', 'tool-response-time'],
					['token-usage']
				]
			},
			{
				id: 'box-inventory',
				rows: [
					['header-inventory'],
					['device-scan-stats'],
					['device-clients', 'top-device-mcps', 'top-device-skills'],
					['device-scan-timeline']
				]
			}
		]);
		expect(layout.rows[1]).toMatchObject({ kind: 'box' });
		expect(layout.rows[2]).toMatchObject({ kind: 'box' });
		expect(layout.rows[1].kind === 'box' ? layout.rows[1].rows[0] : null).toMatchObject({
			text: 'MCP Servers',
			visibility: 'always'
		});
		expect(layout.rows[2].kind === 'box' ? layout.rows[2].rows[0] : null).toMatchObject({
			text: 'Inventory',
			visibility: 'device-scans'
		});
		expect(
			placements(layout.rows)
				.filter((placement) => inventoryWidgets.includes(placement.id))
				.every((placement) => placement.hideWhenEmpty)
		).toBe(true);
		expect(
			placements(layout.rows)
				.filter((placement) => !inventoryWidgets.includes(placement.id))
				.every((placement) => !placement.hideWhenEmpty)
		).toBe(true);
		expect(layout.platformStats).toEqual([...PLATFORM_STAT_IDS]);
		expect(layout.modelStats).toEqual([...MODEL_STAT_IDS]);
	});

	it('gives power users the shared usage widgets', () => {
		expect(ids(factoryConfig({ isAdmin: false }).rows)).toEqual([
			['top-servers-used'],
			['popular-tools', 'tool-response-time']
		]);
		expect(availableWidgets({ isAdmin: false })).toEqual([
			'top-servers-used',
			'popular-tools',
			'tool-response-time',
			'most-frequent-tool-calls',
			'tool-individual-response-time',
			'tool-call-errors',
			'tool-call-errors-by-server'
		]);
	});

	it('offers usage graphs without placing them in the factory layout', () => {
		const usageWidgets = [
			'model-stats',
			'llm-requests',
			'spend-by-provider',
			'top-models',
			'most-frequent-tool-calls',
			'tool-individual-response-time',
			'tool-call-errors',
			'tool-call-errors-by-server',
			'most-active-users'
		];
		const placed = placements(factoryConfig({ isAdmin: true }).rows).map(
			(placement) => placement.id
		);

		for (const id of usageWidgets) expect(placed).not.toContain(id);
		expect(availableWidgets({ isAdmin: true })).toEqual(expect.arrayContaining(usageWidgets));
		expect(availableWidgets({ isAdmin: false })).not.toContain('model-stats');
		expect(availableWidgets({ isAdmin: false })).not.toContain('most-active-users');
		const powerPlaced = placements(factoryConfig({ isAdmin: false }).rows).map(
			(placement) => placement.id
		);
		for (const id of usageWidgets) expect(powerPlaced).not.toContain(id);
	});

	it('falls back when stored layout cannot be read', () => {
		expect(parseDashboardConfig(null)).toBeNull();
		expect(parseDashboardConfig('not-json')).toBeNull();
		expect(parseDashboardConfig(JSON.stringify({ rows: 'nope' }))).toBeNull();
	});

	it('fills a missing hide-on-empty flag from the catalog default', () => {
		const parsed = parseDashboardConfig(
			JSON.stringify({
				rows: [[{ id: 'device-scan-stats' }, { id: 'platform-stats' }, { id: 'unknown' }]]
			})
		);

		expect(parsed?.rows).toEqual([
			{
				kind: 'widgets',
				items: [
					{ id: 'device-scan-stats', hideWhenEmpty: true },
					{ id: 'platform-stats', hideWhenEmpty: false }
				]
			}
		]);
		expect(parsed?.platformStats).toEqual([...PLATFORM_STAT_IDS]);
		expect(parsed?.modelStats).toEqual([...MODEL_STAT_IDS]);
	});

	it('keeps an explicit hide-on-empty flag and reads the previous show-when-empty flag', () => {
		const parsed = parseDashboardConfig(
			JSON.stringify({
				rows: [[{ id: 'popular-tools', hideWhenEmpty: true }]],
				platformStats: ['nope', 'total-spend'],
				deviceStats: []
			})
		);

		expect(parsed?.platformStats).toEqual(['total-spend']);
		expect(parsed?.deviceStats).toEqual([]);
		expect(parsed?.modelStats).toEqual([...MODEL_STAT_IDS]);
		expect(parsed?.rows[0]).toMatchObject({
			kind: 'widgets',
			items: [{ id: 'popular-tools', hideWhenEmpty: true }]
		});

		const legacy = parseDashboardConfig(
			JSON.stringify({
				rows: [[{ id: 'popular-tools', showWhenEmpty: false }]]
			})
		);
		expect(legacy?.rows[0]).toMatchObject({
			kind: 'widgets',
			items: [{ id: 'popular-tools', hideWhenEmpty: true }]
		});
	});

	it('drops widgets the viewer cannot use', () => {
		const projected = projectConfig(factoryConfig({ isAdmin: true }), { isAdmin: false });

		expect(ids(projected.rows)).toEqual([
			{
				id: 'box-mcp-servers',
				rows: [
					['header-mcp-servers'],
					['top-servers-used'],
					['popular-tools', 'tool-response-time']
				]
			},
			{ id: 'box-inventory', rows: [['header-inventory']] }
		]);
	});

	it('hides empty inventory widgets and the inventory header without scans', () => {
		const layout = factoryConfig({ isAdmin: true });
		const rendered = rowsForRender(
			layout,
			widgetStatuses(
				signals({
					deviceTiles: [{ loading: false, value: 0 }],
					skillCount: 0
				})
			),
			{ deviceScans: true, mcpStats: true, llmStats: true }
		);
		const inventoryRow = findWidgetRow(rendered, 'device-clients');

		expect(
			hasRow(
				rendered,
				(row) => row.kind === 'widgets' && row.items.some((item) => item.id === 'device-scan-stats')
			)
		).toBe(false);
		expect(inventoryRow).toMatchObject({
			kind: 'widgets',
			items: [{ id: 'device-clients' }, { id: 'top-device-mcps' }]
		});
		expect(colSpanClass(inventoryRow?.kind === 'widgets' ? inventoryRow.items.length : 0)).toBe(
			'col-span-12 @3xl:col-span-6'
		);

		const withoutScans = rowsForRender(
			layout,
			widgetStatuses(signals({ clientCount: 1, mcpCount: 1, skillCount: 1, scanCount: 1 })),
			{ deviceScans: false, mcpStats: true, llmStats: true }
		);
		expect(
			hasRow(withoutScans, (row) => row.kind === 'header' && row.id === 'header-inventory')
		).toBe(false);
		expect(
			hasRow(withoutScans, (row) => row.kind === 'header' && row.id === 'header-mcp-servers')
		).toBe(true);
	});

	it('reads a stored device-scan header and shows a header for one condition', () => {
		const parsed = parseDashboardConfig(
			JSON.stringify({
				rows: [
					{ kind: 'header', id: 'legacy', text: 'Inventory', onlyWithDeviceScans: true },
					{ kind: 'header', id: 'open', text: 'Always' },
					{ kind: 'header', id: 'models', text: 'Models', visibility: 'llm-stats' },
					{ kind: 'header', id: 'tools', text: 'Tools', visibility: 'mcp-stats' },
					{
						kind: 'header',
						id: 'unknown',
						text: 'Unknown',
						visibility: 'nope',
						onlyWithDeviceScans: true
					}
				]
			})
		);

		expect(parsed?.rows).toMatchObject([
			{ id: 'legacy', visibility: 'device-scans' },
			{ id: 'open', visibility: 'always' },
			{ id: 'models', visibility: 'llm-stats' },
			{ id: 'tools', visibility: 'mcp-stats' },
			{ id: 'unknown', visibility: 'device-scans' }
		]);

		const rendered = rowsForRender(parsed!, widgetStatuses(signals()), {
			deviceScans: false,
			mcpStats: true,
			llmStats: false
		});
		expect(rendered.flatMap((row) => (row.kind === 'header' ? [row.id] : []))).toEqual([
			'open',
			'tools'
		]);
	});

	it('keeps a loading widget mounted', () => {
		const rendered = rowsForRender(
			factoryConfig({ isAdmin: true }),
			widgetStatuses(signals({ deviceTiles: [{ loading: true, value: 0 }] }))
		);

		expect(findWidgetRow(rendered, 'device-scan-stats')).toMatchObject({
			kind: 'widgets',
			items: [{ id: 'device-scan-stats' }]
		});
	});

	it('moves a widget without dropping its hide-on-empty flag', () => {
		const layout = setHideWhenEmpty(factoryConfig({ isAdmin: true }), 'popular-tools', true);
		const moved = placeWidget(layout, 'popular-tools', { kind: 'new-row', path: [], index: 0 });

		expect(moved.rows[0]).toEqual({
			kind: 'widgets',
			items: [{ id: 'popular-tools', hideWhenEmpty: true }]
		});
		expect(
			placements(moved.rows).filter((placement) => placement.id === 'popular-tools')
		).toHaveLength(1);
	});

	it('reorders within a row and refuses a fifth widget', () => {
		const layout = factoryConfig({ isAdmin: true });
		const box = layout.rows[2];
		if (box.kind !== 'box') throw new Error('expected inventory box');
		const rowIndex = box.rows.findIndex(
			(row) => row.kind === 'widgets' && row.items.some((item) => item.id === 'top-device-mcps')
		);
		const reordered = placeWidget(layout, 'top-device-mcps', {
			kind: 'slot',
			path: [2, rowIndex],
			index: 0
		});
		const reorderedBox = reordered.rows[2];

		expect(reorderedBox.kind === 'box' ? ids(reorderedBox.rows)[rowIndex] : []).toEqual([
			'top-device-mcps',
			'device-clients',
			'top-device-skills'
		]);

		const full = appendWidget(
			{
				...layout,
				rows: [
					{
						kind: 'widgets',
						items: [
							{ id: 'platform-stats', hideWhenEmpty: false },
							{ id: 'server-activity', hideWhenEmpty: false },
							{ id: 'most-deployed', hideWhenEmpty: false },
							{ id: 'token-usage', hideWhenEmpty: false }
						]
					}
				]
			},
			'top-servers-used'
		);
		expect(full.rows).toHaveLength(2);

		const blocked = placeWidget(full, 'popular-tools', { kind: 'slot', path: [0], index: 0 });
		expect(blocked).toBe(full);
	});

	it('removes a widget and toggles stat cells in catalog order', () => {
		const removed = removeWidget(factoryConfig({ isAdmin: true }), 'token-usage');
		expect(placements(removed.rows).map((placement) => placement.id)).not.toContain('token-usage');

		const hidden = togglePlatformStat(factoryConfig({ isAdmin: true }), 'total-users');
		expect(hidden.platformStats).toEqual(['active-users', 'total-tokens', 'total-spend']);
		expect(togglePlatformStat(hidden, 'total-users').platformStats).toEqual([...PLATFORM_STAT_IDS]);

		const hiddenModel = toggleModelStat(factoryConfig({ isAdmin: true }), 'input');
		expect(hiddenModel.modelStats).toEqual(['total', 'output', 'cached-input', 'spend']);
		expect(toggleModelStat(hiddenModel, 'input').modelStats).toEqual([...MODEL_STAT_IDS]);
	});

	it('preserves widgets the viewer cannot see when saving', () => {
		const previous = factoryConfig({ isAdmin: true });
		const visible = projectConfig(previous, { isAdmin: false });
		const edited = placeWidget(visible, 'popular-tools', { kind: 'new-row', path: [], index: 0 });
		const committed = commitLayout(previous, edited, { isAdmin: false });
		const committedIds = placements(committed.rows).map((placement) => placement.id);

		expect(committedIds).toContain('platform-stats');
		expect(committedIds).toContain('device-scan-stats');
		expect(committedIds.filter((id) => id === 'popular-tools')).toEqual(['popular-tools']);
		expect(
			placements(committed.rows).find((placement) => placement.id === 'device-scan-stats')
		).toMatchObject({ hideWhenEmpty: true });
	});

	it('keeps each header on its own row and refuses widgets in that row', () => {
		const withHeaders = appendHeader(appendHeader(factoryConfig({ isAdmin: false }), 'Usage'), '');
		const headerIds = withHeaders.rows.flatMap((row) => (row.kind === 'header' ? [row.id] : []));

		expect(headerIds).toHaveLength(2);
		expect(withHeaders.rows.at(-2)).toMatchObject({ kind: 'header', text: 'Usage' });
		expect(withHeaders.rows.at(-1)).toMatchObject({ kind: 'header', text: '' });

		const named = setHeaderText(withHeaders, headerIds[1], 'Tools');
		expect(named.rows.at(-1)).toMatchObject({ kind: 'header', text: 'Tools' });

		const blocked = placeWidget(named, 'popular-tools', {
			kind: 'slot',
			path: [named.rows.findIndex((row) => row.kind === 'header')],
			index: 0
		});
		expect(blocked).toBe(named);

		const appended = appendWidget(
			{ ...named, rows: named.rows.filter((row) => row.kind === 'header') },
			'top-servers-used'
		);
		expect(appended.rows.at(-1)).toMatchObject({
			kind: 'widgets',
			items: [{ id: 'top-servers-used' }]
		});
		expect(appended.rows.filter((row) => row.kind === 'header')).toHaveLength(2);

		const moved = moveHeader(named, headerIds[0], named.rows.length);
		expect(moved.rows.at(-1)).toMatchObject({ kind: 'header', id: headerIds[0], text: 'Usage' });

		const inserted = insertHeader(factoryConfig({ isAdmin: false }), 1, 'Usage');
		expect(inserted.rows[1]).toMatchObject({ kind: 'header', text: 'Usage' });
		expect(inserted.rows.filter((row) => row.kind === 'widgets')).toHaveLength(2);
		expect(placeWidget(inserted, 'popular-tools', { kind: 'slot', path: [1], index: 0 })).toBe(
			inserted
		);

		const outer = appendBox(factoryConfig({ isAdmin: false }));
		const outerBox = outer.rows.at(-1);
		if (outerBox?.kind !== 'box') throw new Error('expected box');
		const nested = insertBox(outer, 0, [outer.rows.length - 1]);
		const withWidget = placeWidget(nested, 'top-servers-used', {
			kind: 'new-row',
			path: [outer.rows.length - 1, 0],
			index: 0
		});
		const nestedBox = withWidget.rows.at(-1);
		expect(nestedBox?.kind === 'box' && nestedBox.rows[0]?.kind === 'box').toBe(true);
		expect(
			nestedBox?.kind === 'box' && nestedBox.rows[0]?.kind === 'box' ? nestedBox.rows[0].rows : []
		).toMatchObject([{ kind: 'widgets', items: [{ id: 'top-servers-used' }] }]);
		expect(
			placeWidget(withWidget, 'popular-tools', {
				kind: 'slot',
				path: [withWidget.rows.length - 1],
				index: 0
			})
		).toBe(withWidget);

		const stored = parseDashboardConfig(JSON.stringify(named));
		expect(stored?.rows.filter((row) => row.kind === 'header')).toEqual([
			{ kind: 'header', id: headerIds[0], text: 'Usage', visibility: 'always' },
			{ kind: 'header', id: headerIds[1], text: 'Tools', visibility: 'always' }
		]);
	});
	it('reattaches hidden widgets by anchor when a hidden-only row is dropped from the view', () => {
		const viewer = { isAdmin: false };
		const previous: DashboardConfig = {
			...factoryConfig({ isAdmin: true }),
			rows: [
				widgetRow(['platform-stats']),
				widgetRow([
					'top-servers-used',
					'popular-tools',
					'tool-response-time',
					'most-frequent-tool-calls'
				]),
				widgetRow(['tool-call-errors', 'token-usage'])
			]
		};
		const visible = projectConfig(previous, viewer);
		expect(ids(visible.rows)).toEqual([
			['top-servers-used', 'popular-tools', 'tool-response-time', 'most-frequent-tool-calls'],
			['tool-call-errors']
		]);

		const committed = commitLayout(previous, visible, viewer);

		expect(ids(committed.rows)).toEqual([
			['platform-stats'],
			['top-servers-used', 'popular-tools', 'tool-response-time', 'most-frequent-tool-calls'],
			['tool-call-errors', 'token-usage']
		]);
	});

	it('keeps hidden widgets with their row anchor after the viewer reorders rows', () => {
		const viewer = { isAdmin: false };
		const previous: DashboardConfig = {
			...factoryConfig({ isAdmin: true }),
			rows: [widgetRow(['top-servers-used', 'token-usage']), widgetRow(['popular-tools'])]
		};
		const visible = projectConfig(previous, viewer);
		const reordered: DashboardConfig = { ...visible, rows: [visible.rows[1], visible.rows[0]] };

		const committed = commitLayout(previous, reordered, viewer);

		expect(ids(committed.rows)).toEqual([['popular-tools'], ['top-servers-used', 'token-usage']]);
		for (const row of committed.rows) {
			if (row.kind === 'widgets') expect(row.items.length).toBeLessThanOrEqual(4);
		}
	});

	it('treats blank headers and empty boxes as no content', () => {
		const emptyBox: DashboardRow = { kind: 'box', id: 'box-1', rows: [] };
		const blankHeader: DashboardRow = {
			kind: 'header',
			id: 'header-1',
			text: '  ',
			visibility: 'always'
		};
		expect(hasRenderedContent([])).toBe(false);
		expect(hasRenderedContent([blankHeader, emptyBox])).toBe(false);
		expect(hasRenderedContent([{ ...blankHeader, text: 'Usage' }])).toBe(true);
		expect(hasRenderedContent([{ ...emptyBox, rows: [widgetRow(['top-servers-used'])] }])).toBe(
			true
		);
	});
	it('lists placed widgets including those inside boxes', () => {
		const rows: DashboardRow[] = [
			widgetRow(['top-servers-used']),
			{ kind: 'header', id: 'header-1', text: 'Tools', visibility: 'always' },
			{ kind: 'box', id: 'box-1', rows: [widgetRow(['tool-call-errors', 'popular-tools'])] }
		];
		expect([...placedWidgetIds(rows)].sort()).toEqual([
			'popular-tools',
			'tool-call-errors',
			'top-servers-used'
		]);
	});
	it('fetches range datasets only for placed widgets', () => {
		const adminDefaults = factoryConfig({ isAdmin: true });
		expect([...neededDatasets(adminDefaults)].sort()).toEqual([
			'device-scans',
			'mcp-usage',
			'token-totals'
		]);

		const withoutTokenStats: DashboardConfig = {
			...adminDefaults,
			rows: [widgetRow(['platform-stats'])],
			platformStats: ['total-users', 'active-users']
		};
		expect(neededDatasets(withoutTokenStats).size).toBe(0);

		const llmOnly: DashboardConfig = { ...adminDefaults, rows: [widgetRow(['top-models'])] };
		expect([...neededDatasets(llmOnly)]).toEqual(['token-stats']);
	});

	it('lists header visibilities including nested headers', () => {
		const rows: DashboardRow[] = [
			{ kind: 'header', id: 'h1', text: 'A', visibility: 'always' },
			{
				kind: 'box',
				id: 'b1',
				rows: [{ kind: 'header', id: 'h2', text: 'B', visibility: 'llm-stats' }]
			}
		];
		expect([...headerVisibilities(rows)].sort()).toEqual(['always', 'llm-stats']);
	});

	it('keeps showing a stat card when the user selects no stats', () => {
		expect(statCardStatus([])).toEqual({ loading: false, empty: false });
		expect(statCardStatus([{ loading: false, value: 0 }])).toEqual({
			loading: false,
			empty: true
		});
	});

	it('shares one visibility rule between rendering and rowsForRender', () => {
		const statuses = { 'top-servers-used': { loading: false, empty: true } };
		const availability = { deviceScans: false, mcpStats: true, llmStats: true };
		const hiddenRow = {
			kind: 'widgets' as const,
			items: [{ id: 'top-servers-used' as const, hideWhenEmpty: true }]
		};
		const mixedRow: DashboardRow = {
			kind: 'widgets',
			items: [...hiddenRow.items, { id: 'popular-tools', hideWhenEmpty: false }]
		};
		const rows: DashboardRow[] = [
			{ kind: 'header', id: 'blank', text: ' ', visibility: 'always' },
			{ kind: 'header', id: 'scans', text: 'Scans', visibility: 'device-scans' },
			{ kind: 'box', id: 'empty-box', rows: [] },
			{ kind: 'box', id: 'hidden-box', rows: [hiddenRow] },
			mixedRow
		];

		const shown = rows.filter((row) => rowVisible(row, statuses, availability));
		const rendered = rowsForRender(
			{ ...factoryConfig({ isAdmin: false }), rows },
			statuses,
			availability
		);
		expect(ids(rendered)).toEqual([{ id: 'empty-box', rows: [] }, ['popular-tools']]);
		expect(shown).toHaveLength(rendered.length);
		expect(visibleItems(mixedRow.items, statuses)).toEqual([
			{ item: { id: 'popular-tools', hideWhenEmpty: false }, index: 1 }
		]);
	});

	it('stores layouts under a per-user key', () => {
		expect(dashboardLayoutKey('user-a')).not.toEqual(dashboardLayoutKey('user-b'));
	});
	it('builds grid cells that match rowsForRender', () => {
		const statuses = { 'top-servers-used': { loading: false, empty: true } };
		const availability = { deviceScans: false, mcpStats: true, llmStats: true };
		const rows: DashboardRow[] = [
			{ kind: 'header', id: 'blank', text: ' ', visibility: 'always' },
			{ kind: 'header', id: 'scans', text: 'Scans', visibility: 'device-scans' },
			{ kind: 'box', id: 'empty-box', rows: [] },
			{
				kind: 'box',
				id: 'hidden-box',
				rows: [{ kind: 'widgets', items: [{ id: 'top-servers-used', hideWhenEmpty: true }] }]
			},
			{
				kind: 'widgets',
				items: [
					{ id: 'top-servers-used', hideWhenEmpty: true },
					{ id: 'popular-tools', hideWhenEmpty: false },
					{ id: 'tool-call-errors', hideWhenEmpty: false }
				]
			}
		];

		const cells = buildGridCells(rows, statuses, availability);
		expect(cells.map((cell) => cell.key)).toEqual([
			'box:empty-box',
			'widget:popular-tools',
			'widget:tool-call-errors'
		]);
		expect(cells[1]).toMatchObject({ rowIndex: 4, index: 1, rowSize: 2 });

		const rendered = rowsForRender(
			{ ...factoryConfig({ isAdmin: false }), rows },
			statuses,
			availability
		);
		expect(ids(rendered)).toEqual([
			{ id: 'empty-box', rows: [] },
			['popular-tools', 'tool-call-errors']
		]);
	});

	it('detects any all-time LLM usage', () => {
		expect(hasLlmUsage({ totalTokens: 0, totalSpend: 0 } as never)).toBe(false);
		expect(hasLlmUsage({ totalTokens: 0, outputTokens: 3 } as never)).toBe(true);
		expect(hasLlmUsage({ totalTokens: 0, totalSpend: 0.01 } as never)).toBe(true);
	});
});
