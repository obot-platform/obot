import {
	PLATFORM_STAT_IDS,
	appendBox,
	appendHeader,
	insertBox,
	insertHeader,
	appendWidget,
	availableWidgets,
	colSpanClass,
	commitLayout,
	factoryConfig,
	moveHeader,
	parseDashboardConfig,
	placeWidget,
	projectConfig,
	removeWidget,
	rowsForRender,
	setHeaderText,
	setHideWhenEmpty,
	togglePlatformStat,
	widgetStatuses,
	type DashboardRow,
	type DashboardSignals,
	type WidgetPlacement
} from './dashboardLayout';
import { describe, expect, it } from 'vitest';

function ids(rows: DashboardRow[]): unknown[] {
	return rows.map((row) => {
		if (row.kind === 'header') return [row.id];
		if (row.kind === 'box') return { id: row.id, rows: ids(row.rows) };
		return row.items.map((placement) => placement.id);
	});
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
			onlyWithDeviceScans: false
		});
		expect(layout.rows[2].kind === 'box' ? layout.rows[2].rows[0] : null).toMatchObject({
			text: 'Inventory',
			onlyWithDeviceScans: true
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
	});

	it('gives power users the shared usage widgets', () => {
		expect(ids(factoryConfig({ isAdmin: false }).rows)).toEqual([
			['top-servers-used'],
			['popular-tools', 'tool-response-time']
		]);
		expect(availableWidgets({ isAdmin: false })).toEqual([
			'top-servers-used',
			'popular-tools',
			'tool-response-time'
		]);
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
			true
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
			false
		);
		expect(
			hasRow(withoutScans, (row) => row.kind === 'header' && row.id === 'header-inventory')
		).toBe(false);
		expect(
			hasRow(withoutScans, (row) => row.kind === 'header' && row.id === 'header-mcp-servers')
		).toBe(true);
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
		expect(hidden.platformStats).toEqual(['monthly-active-users', 'total-tokens', 'total-spend']);
		expect(togglePlatformStat(hidden, 'total-users').platformStats).toEqual([...PLATFORM_STAT_IDS]);
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
			{ kind: 'header', id: headerIds[0], text: 'Usage', onlyWithDeviceScans: false },
			{ kind: 'header', id: headerIds[1], text: 'Tools', onlyWithDeviceScans: false }
		]);
	});
});
