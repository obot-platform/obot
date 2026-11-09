import { Group } from '$lib/services';
import { errors } from '$lib/stores';
import { createMockProfile, preparePageData } from '../../tests/helpers/pageData';
import { getProfileResponse } from '../../tests/mocks/data';
import { worker } from '../../tests/mocks/worker';
import type { PageData } from './$types';
import DashboardPage from './+page.svelte';
import {
	appendHeader,
	dashboardLayoutKey,
	factoryConfig,
	setHeaderVisibility
} from './dashboardLayout';
import { http, HttpResponse } from 'msw';
import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

const emptyUsageStats = {
	items: [],
	timeStart: '',
	timeEnd: '',
	totalCalls: 0,
	uniqueUsers: 0
};

const emptyScanStats = {
	timeStart: '',
	timeEnd: '',
	deviceCount: 0,
	userCount: 0,
	clients: [],
	mcpServers: [],
	skills: [],
	scanTimestamps: []
};

function hoursBetween(url: string, startKey: string, endKey: string) {
	const params = new URL(url).searchParams;
	const start = new Date(params.get(startKey) ?? '');
	const end = new Date(params.get(endKey) ?? '');
	return (end.getTime() - start.getTime()) / (1000 * 60 * 60);
}

function isAllTimeRequest(url: string) {
	const start = new Date(new URL(url).searchParams.get('start') ?? '');
	return start.getTime() < 24 * 60 * 60 * 1000;
}

async function renderDashboard(
	groups: string[],
	hasDeviceScans = false,
	stats: { hasMcpStats?: boolean; hasLlmStats?: boolean } = {}
) {
	// The page probes MCP/LLM stats lazily, only when a header is set to show conditionally.
	// Passing stats overrides the probe responses; otherwise a test's own handlers apply.
	if (stats.hasMcpStats !== undefined || stats.hasLlmStats !== undefined) {
		worker.use(
			http.get('/api/mcp-audit-logs', () =>
				HttpResponse.json({ items: [], total: stats.hasMcpStats ? 1 : 0, limit: 1, offset: 0 })
			),
			http.get('/api/total-token-usage', ({ request }) => {
				// Only answer the all-time probe; range requests fall through to the other handlers.
				if (!isAllTimeRequest(request.url)) return;
				return HttpResponse.json({ totalTokens: stats.hasLlmStats ? 1 : 0, totalSpend: 0 });
			})
		);
	}
	const data = await preparePageData<PageData>({
		hasDeviceScans,
		profile: createMockProfile(groups)
	});
	return render(DashboardPage, { data });
}

describe('Dashboard date range', () => {
	it('refetches range-backed datasets when the calendar range changes', async () => {
		const mcpStats = vi.fn();
		const scanStats = vi.fn();
		const totalTokens = vi.fn();
		const tokenUsage = vi.fn();
		const catalogServers = vi.fn();

		worker.use(
			http.get('/api/mcp-stats', ({ request }) => {
				mcpStats(request.url);
				return HttpResponse.json(emptyUsageStats);
			}),
			http.get('/api/devices/scan-stats', ({ request }) => {
				scanStats(request.url);
				return HttpResponse.json(emptyScanStats);
			}),
			http.get('/api/total-token-usage', ({ request }) => {
				totalTokens(request.url);
				const hours = hoursBetween(request.url, 'start', 'end');
				return HttpResponse.json({
					totalTokens: hours < 2 ? 42 : 1000,
					totalSpend: 0
				});
			}),
			http.get('/api/token-usage', ({ request }) => {
				tokenUsage(request.url);
				return HttpResponse.json({ items: [] });
			}),
			http.get('/api/mcp-catalogs/default/entries/all-servers', () => {
				catalogServers();
				return HttpResponse.json({ items: [] });
			}),
			http.get('/api/workspaces/all-entries/all-servers', () => {
				catalogServers();
				return HttpResponse.json({ items: [] });
			})
		);

		await renderDashboard([Group.ADMIN], true);

		await vi.waitFor(() => {
			expect(mcpStats).toHaveBeenCalled();
			expect(scanStats).toHaveBeenCalled();
			expect(totalTokens).toHaveBeenCalled();
			expect(tokenUsage).toHaveBeenCalled();
			expect(catalogServers).toHaveBeenCalled();
		});

		expect(hoursBetween(mcpStats.mock.calls[0][0], 'start_time', 'end_time')).toBeGreaterThan(
			24 * 27
		);
		expect(hoursBetween(scanStats.mock.calls[0][0], 'start', 'end')).toBeGreaterThan(24 * 27);
		expect(hoursBetween(totalTokens.mock.calls[0][0], 'start', 'end')).toBeGreaterThan(24 * 27);
		expect(hoursBetween(tokenUsage.mock.calls[0][0], 'start', 'end')).toBeGreaterThan(24 * 27);

		const catalogCalls = catalogServers.mock.calls.length;
		const rangeButton = page.getByRole('button', { name: /Last 30 Days/ });
		await expect.element(rangeButton).toBeEnabled();
		await rangeButton.click();
		await page.getByRole('button', { name: 'Last Hour', exact: true }).click();

		await expect.element(page.getByRole('button', { name: /Last Hour/ })).toBeVisible();
		await vi.waitFor(() => {
			expect(mcpStats.mock.calls.length).toBeGreaterThan(1);
			expect(hoursBetween(mcpStats.mock.lastCall?.[0], 'start_time', 'end_time')).toBeCloseTo(1, 1);
		});

		expect(hoursBetween(scanStats.mock.lastCall?.[0], 'start', 'end')).toBeCloseTo(1, 1);
		expect(hoursBetween(totalTokens.mock.lastCall?.[0], 'start', 'end')).toBeCloseTo(1, 1);
		expect(hoursBetween(tokenUsage.mock.lastCall?.[0], 'start', 'end')).toBeCloseTo(1, 1);
		expect(catalogServers.mock.calls.length).toBe(catalogCalls);
		await expect.element(page.getByText('42', { exact: true })).toBeVisible();
	});

	it('refetches tool usage for power users and skips admin datasets', async () => {
		const mcpStats = vi.fn();
		const scanStats = vi.fn();
		const totalTokens = vi.fn();

		worker.use(
			http.get('/api/mcp-stats', ({ request }) => {
				mcpStats(request.url);
				return HttpResponse.json(emptyUsageStats);
			}),
			http.get('/api/devices/scan-stats', () => {
				scanStats();
				return HttpResponse.json(emptyScanStats);
			}),
			http.get('/api/total-token-usage', () => {
				totalTokens();
				return HttpResponse.json({ totalTokens: 0, totalSpend: 0 });
			})
		);

		await renderDashboard([Group.POWERUSER]);

		await vi.waitFor(() => expect(mcpStats).toHaveBeenCalled());
		const rangeButton = page.getByRole('button', { name: /Last 30 Days/ });
		await expect.element(rangeButton).toBeEnabled();
		await rangeButton.click();
		await page.getByRole('button', { name: 'Last Hour', exact: true }).click();

		await vi.waitFor(() => {
			expect(mcpStats.mock.calls.length).toBeGreaterThan(1);
			expect(hoursBetween(mcpStats.mock.lastCall?.[0], 'start_time', 'end_time')).toBeCloseTo(1, 1);
		});
		expect(scanStats).not.toHaveBeenCalled();
		expect(totalTokens).not.toHaveBeenCalled();
	});
});

function mockDashboardApis() {
	worker.use(
		http.get('/api/mcp-stats', () => HttpResponse.json(emptyUsageStats)),
		http.get('/api/devices/scan-stats', () => HttpResponse.json(emptyScanStats)),
		http.get('/api/total-token-usage', () => HttpResponse.json({ totalTokens: 0, totalSpend: 0 })),
		http.get('/api/token-usage', () => HttpResponse.json({ items: [] })),
		http.get('/api/mcp-audit-logs', () =>
			HttpResponse.json({ items: [], total: 0, limit: 1, offset: 0 })
		)
	);
}

describe('Dashboard layout', () => {
	it('hides a widget and a stat cell, then keeps that choice after remount', async () => {
		mockDashboardApis();
		const first = await renderDashboard([Group.ADMIN], false);

		await expect.element(page.getByRole('heading', { name: 'On Platform' })).toBeVisible();
		const main = page.getByRole('main');
		await page.getByRole('button', { name: 'Customize Dashboard' }).click();
		await page.getByRole('checkbox', { name: 'Total Users' }).click();
		await page.getByRole('button', { name: 'Remove Recently Popular Tools' }).click();
		await expect.element(main.getByText('Total Users', { exact: true })).toBeVisible();
		await expect
			.element(main.getByRole('heading', { name: /Recently Popular Tools/ }))
			.toBeVisible();
		await page.getByRole('button', { name: 'Cancel' }).click();

		await expect.element(main.getByText('Total Users', { exact: true })).toBeVisible();
		await expect
			.element(main.getByRole('heading', { name: /Recently Popular Tools/ }))
			.toBeVisible();

		await page.getByRole('button', { name: 'Customize Dashboard' }).click();
		await page.getByRole('checkbox', { name: 'Total Users' }).click();
		await page.getByRole('button', { name: 'Remove Recently Popular Tools' }).click();
		await page.getByRole('button', { name: 'Save' }).click();

		await expect.element(page.getByText('Total Users', { exact: true })).not.toBeVisible();
		await expect
			.element(page.getByRole('heading', { name: /Recently Popular Tools/ }))
			.not.toBeInTheDocument();

		first.unmount();
		await renderDashboard([Group.ADMIN], false);

		await expect.element(page.getByRole('heading', { name: /Top Servers Used/ })).toBeVisible();
		await expect.element(page.getByText('Total Users', { exact: true })).not.toBeVisible();
		await expect
			.element(page.getByRole('heading', { name: /Recently Popular Tools/ }))
			.not.toBeInTheDocument();
	});

	it('hides the inventory header until the org has device scans', async () => {
		mockDashboardApis();
		const withoutScans = await renderDashboard([Group.ADMIN], false);
		const main = page.getByRole('main');

		await expect.element(main.getByRole('heading', { name: 'MCP Servers' })).toBeVisible();
		await expect.element(main.getByRole('heading', { name: 'Inventory' })).not.toBeInTheDocument();

		await page.getByRole('button', { name: 'Customize Dashboard' }).click();
		await expect
			.element(page.getByRole('radio', { name: 'Only show when device scans exist: Inventory' }))
			.toBeChecked();
		await expect
			.element(page.getByRole('radio', { name: 'Always show: Inventory' }))
			.not.toBeChecked();
		await expect
			.element(page.getByRole('radio', { name: 'Only show when MCP stats exist: Inventory' }))
			.not.toBeChecked();
		await expect
			.element(page.getByRole('radio', { name: 'Only show when LLM stats exist: Inventory' }))
			.not.toBeChecked();
		await expect
			.element(page.getByRole('radio', { name: 'Always show: MCP Servers' }))
			.toBeChecked();
		await expect
			.element(page.getByRole('radio', { name: 'Only show when device scans exist: MCP Servers' }))
			.not.toBeChecked();

		withoutScans.unmount();
		await renderDashboard([Group.ADMIN], true);
		await expect
			.element(page.getByRole('main').getByRole('heading', { name: 'Inventory' }))
			.toBeVisible();
		expect(await page.getByCSS('[data-dashboard-box]').elements()).toHaveLength(2);
	});

	it('shows empty device-scan stats when hide on empty data is turned off', async () => {
		mockDashboardApis();
		await renderDashboard([Group.ADMIN], false);
		const main = page.getByRole('main');

		await expect.element(main.getByRole('heading', { name: 'On Platform' })).toBeVisible();
		await expect
			.element(main.getByRole('heading', { name: 'Device Scans' }))
			.not.toBeInTheDocument();

		await page.getByRole('button', { name: 'Customize Dashboard' }).click();
		const hideWhenEmpty = page.getByRole('checkbox', {
			name: 'Hide on empty data: Device Scans'
		});
		await expect.element(hideWhenEmpty).toBeChecked();
		await hideWhenEmpty.click();
		await expect
			.element(main.getByRole('heading', { name: 'Device Scans' }))
			.not.toBeInTheDocument();
		await page.getByRole('button', { name: 'Save' }).click();

		await expect.element(main.getByRole('heading', { name: 'Device Scans' })).toBeVisible();
		await expect.element(main.getByText('Unique Devices')).toBeVisible();

		await page.getByRole('button', { name: 'Customize Dashboard' }).click();
		await page.getByRole('button', { name: 'Reset layout' }).click();
		await expect.element(main.getByRole('heading', { name: 'Device Scans' })).toBeVisible();
		await page.getByRole('button', { name: 'Save' }).click();

		await expect.element(main.getByRole('heading', { name: 'On Platform' })).toBeVisible();
		await expect
			.element(main.getByRole('heading', { name: 'Device Scans' }))
			.not.toBeInTheDocument();
	});

	it('drags the handle with its title and highlights only the slot under the pointer', async () => {
		mockDashboardApis();
		await renderDashboard([Group.ADMIN], false);
		await page.getByRole('button', { name: 'Customize Dashboard' }).click();

		const dialog = page.getByRole('dialog');
		const handle = dialog.getByRole('button', { name: 'Drag On Platform' });
		await expect.element(handle).toBeVisible();
		const handleEl = await handle.element();
		expect(handleEl.textContent).toContain('On Platform');

		const body = await page.getByCSS('body').element();
		handleEl.dispatchEvent(
			new PointerEvent('pointerdown', { bubbles: true, cancelable: true, button: 0 })
		);

		const preview = await page.getByCSS('[data-drag-clone]').element();
		expect(preview.closest('dialog')).toBe((await dialog.element()) as Element);
		expect(preview.textContent).toContain('On Platform');
		expect(preview.childElementCount).toBeGreaterThan(1);
		expect(getComputedStyle(document.documentElement).cursor).toBe('grabbing');

		const firstCard = page.getByCSS('[data-layout-row="0"] [data-layout-card]').first();
		const firstRect = (await firstCard.element()).getBoundingClientRect();
		body.dispatchEvent(
			new PointerEvent('pointermove', {
				bubbles: true,
				cancelable: true,
				clientX: firstRect.right - 4,
				clientY: firstRect.top + firstRect.height / 2
			})
		);

		await expect
			.element(page.getByCSS('[data-drop-slot="0-1"][data-drop-active="true"]'))
			.toBeInTheDocument();
		await expect
			.element(page.getByCSS('[data-drop-slot="0-0"][data-drop-active="true"]'))
			.not.toBeInTheDocument();
		await expect
			.element(page.getByCSS('[data-drop-slot="0-2"][data-drop-active="true"]'))
			.not.toBeInTheDocument();

		body.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }));
		expect(getComputedStyle(document.documentElement).cursor).not.toBe('grabbing');

		const dragged = dialog.getByRole('button', { name: 'Drag Device Scans' });
		const draggedEl = await dragged.element();
		draggedEl.dispatchEvent(
			new PointerEvent('pointerdown', { bubbles: true, cancelable: true, button: 0 })
		);
		body.dispatchEvent(
			new PointerEvent('pointermove', {
				bubbles: true,
				cancelable: true,
				clientX: firstRect.left + 4,
				clientY: firstRect.top + firstRect.height / 2
			})
		);
		body.dispatchEvent(
			new PointerEvent('pointerup', {
				bubbles: true,
				cancelable: true,
				clientX: firstRect.left + 4,
				clientY: firstRect.top + firstRect.height / 2
			})
		);

		const previews = page.getByCSS('[data-layout-row="0"] [data-drag-preview]');
		await expect.element(previews.first()).toHaveTextContent('Device Scans');
		await expect.element(previews.nth(1)).toHaveTextContent('On Platform');

		const paletteItem = await dialog.getByText('Model Stats', { exact: true }).element();
		paletteItem.dispatchEvent(
			new PointerEvent('pointerdown', { bubbles: true, cancelable: true, button: 0 })
		);
		const paletteClone = await page.getByCSS('[data-drag-clone]').element();
		expect(paletteClone.textContent).toContain('Model Stats');
		expect(paletteClone.textContent).not.toContain('Add');
		expect(paletteClone.querySelector('button')).toBeNull();
		body.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }));
	});

	it('shrinks the drag clone and shows a half-opacity preview in the drop slot', async () => {
		mockDashboardApis();
		await renderDashboard([Group.ADMIN], false);
		await page.getByRole('button', { name: 'Customize Dashboard' }).click();
		const dialog = page.getByRole('dialog');
		const body = await page.getByCSS('body').element();
		const item = await dialog.getByText('Model Stats', { exact: true }).element();
		item.dispatchEvent(
			new PointerEvent('pointerdown', { bubbles: true, cancelable: true, button: 0 })
		);

		const row = await dialog.getByCSS('[data-layout-row="0"]').element();
		const rect = row.getBoundingClientRect();
		body.dispatchEvent(
			new PointerEvent('pointermove', {
				bubbles: true,
				cancelable: true,
				clientX: rect.left + rect.width / 2,
				clientY: rect.top + rect.height / 2
			})
		);

		await vi.waitFor(
			async () => {
				const ghost = await page.getByCSS('[data-drag-settled]').element();
				const slot = await dialog.getByCSS('[data-drop-active="true"]').element();
				expect(ghost.textContent).toContain('Model Stats');
				expect(ghost.classList.contains('mr-3')).toBe(true);
				expect(getComputedStyle(ghost).opacity).toBe('0.65');
				expect(getComputedStyle(slot).backgroundColor).toBe('rgba(0, 0, 0, 0)');
			},
			{ timeout: 80, interval: 10 }
		);
		const between = await dialog.getByCSS('[data-drop-slot="1"]').element();
		between.scrollIntoView({ block: 'center' });
		const betweenRect = between.getBoundingClientRect();
		body.dispatchEvent(
			new PointerEvent('pointermove', {
				bubbles: true,
				cancelable: true,
				clientX: betweenRect.left + betweenRect.width / 2,
				clientY: betweenRect.top + betweenRect.height / 2
			})
		);
		await vi.waitFor(async () => {
			const ghost = await page.getByCSS('[data-drag-settled]').element();
			expect(ghost.classList.contains('my-3')).toBe(true);
		});
		const boxStart = dialog.getByCSS('[data-layout-box]').first();
		const startSlot = (await boxStart.element()).querySelector(':scope > [data-drop-slot]');
		if (!(startSlot instanceof HTMLElement)) throw new Error('missing box start slot');
		startSlot.scrollIntoView({ block: 'center' });
		const startRect = startSlot.getBoundingClientRect();
		body.dispatchEvent(
			new PointerEvent('pointermove', {
				bubbles: true,
				cancelable: true,
				clientX: startRect.left + startRect.width / 2,
				clientY: startRect.top + startRect.height / 2
			})
		);
		await vi.waitFor(async () => {
			const ghost = await page.getByCSS('[data-drag-settled]').element();
			expect(ghost.classList.contains('my-3')).toBe(true);
		});
		const box = await dialog.getByCSS('[data-layout-box]').first().element();
		const endSlot = box.lastElementChild;
		if (!(endSlot instanceof HTMLElement)) throw new Error('missing box end slot');
		endSlot.scrollIntoView({ block: 'center' });
		const endRect = endSlot.getBoundingClientRect();
		body.dispatchEvent(
			new PointerEvent('pointermove', {
				bubbles: true,
				cancelable: true,
				clientX: endRect.left + endRect.width / 2,
				clientY: endRect.top + endRect.height / 2
			})
		);
		await vi.waitFor(async () => {
			const ghost = await page.getByCSS('[data-drag-settled]').element();
			expect(ghost.classList.contains('mt-3')).toBe(true);
		});
		const layoutEnd = await dialog
			.getByCSS('[data-layout-scroll] > [data-drop-slot]')
			.last()
			.element();
		layoutEnd.scrollIntoView({ block: 'center' });
		const layoutEndRect = layoutEnd.getBoundingClientRect();
		body.dispatchEvent(
			new PointerEvent('pointermove', {
				bubbles: true,
				cancelable: true,
				clientX: layoutEndRect.left + layoutEndRect.width / 2,
				clientY: layoutEndRect.top + layoutEndRect.height / 2
			})
		);
		await vi.waitFor(async () => {
			const ghost = await page.getByCSS('[data-drag-settled]').element();
			expect(ghost.classList.contains('mt-3')).toBe(true);
		});
		await vi.waitFor(async () => {
			const clone = await page.getByCSS('[data-drag-clone]').element();
			expect(getComputedStyle(clone).transform).toBe('matrix(0, 0, 0, 0, 0, 0)');
		});

		body.dispatchEvent(
			new PointerEvent('pointermove', { bubbles: true, cancelable: true, clientX: 4, clientY: 4 })
		);
		await expect.element(page.getByCSS('[data-drag-settled]')).not.toBeInTheDocument();
		await vi.waitFor(async () => {
			const clone = await page.getByCSS('[data-drag-clone]').element();
			expect(getComputedStyle(clone).opacity).toBe('1');
		});
		body.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }));
	});

	async function dropPaletteWidgetAt(name: string, clientX: number, clientY: number) {
		const dialog = page.getByRole('dialog');
		const item = await dialog.getByText(name, { exact: true }).element();
		const body = await page.getByCSS('body').element();
		item.dispatchEvent(
			new PointerEvent('pointerdown', { bubbles: true, cancelable: true, button: 0 })
		);
		body.dispatchEvent(
			new PointerEvent('pointermove', { bubbles: true, cancelable: true, clientX, clientY })
		);
		body.dispatchEvent(
			new PointerEvent('pointerup', { bubbles: true, cancelable: true, clientX, clientY })
		);
	}

	it('renders a drop slot before and after each box', async () => {
		mockDashboardApis();
		await renderDashboard([Group.ADMIN], false);
		await page.getByRole('button', { name: 'Customize Dashboard' }).click();
		const dialog = page.getByRole('dialog');
		const boxes = await dialog.getByCSS('[data-layout-box]').elements();
		expect(boxes.length).toBeGreaterThan(1);

		for (const box of boxes) {
			const location = box.getAttribute('data-box-location');
			expect(box.previousElementSibling?.getAttribute('data-drop-slot')).toBeTruthy();
			expect(box.nextElementSibling?.getAttribute('data-drop-slot')).toBeTruthy();
			expect(box.querySelector(':scope > [data-drop-slot]')?.getAttribute('data-drop-slot')).toBe(
				`${location}-0`
			);
			expect(box.lastElementChild?.getAttribute('data-drop-slot')).toBe(
				`${location}-${box.getAttribute('data-child-count')}`
			);
		}

		const platform = await dialog.getByCSS('[data-layout-row="0"]').element();
		expect(platform.nextElementSibling?.getAttribute('data-drop-slot')).toBe('1');
		expect(platform.nextElementSibling?.nextElementSibling?.hasAttribute('data-layout-box')).toBe(
			true
		);
		expect(boxes[0].nextElementSibling?.nextElementSibling).toBe(boxes[1]);
	});

	it('drops a widget between On Platform and the first box', async () => {
		mockDashboardApis();
		await renderDashboard([Group.ADMIN], false);
		await page.getByRole('button', { name: 'Customize Dashboard' }).click();
		const dialog = page.getByRole('dialog');
		const row = await dialog.getByCSS('[data-layout-row="0"]').element();
		const rect = row.getBoundingClientRect();

		await dropPaletteWidgetAt('Model Stats', rect.left + rect.width / 2, rect.bottom - 4);

		await expect
			.element(dialog.getByCSS('[data-layout-row="0"]').getByText('Model Stats'))
			.not.toBeInTheDocument();
		await expect
			.element(dialog.getByCSS('[data-layout-row="1"]').getByText('Model Stats'))
			.toBeVisible();
		await expect
			.element(dialog.getByCSS('[data-layout-box]').first())
			.toHaveAttribute('data-box-location', '2');
	});

	it('drops a widget at the beginning and end of a box', async () => {
		mockDashboardApis();
		await renderDashboard([Group.ADMIN], false);
		await page.getByRole('button', { name: 'Customize Dashboard' }).click();
		const dialog = page.getByRole('dialog');
		const header = await dialog.getByCSS('[data-layout-header]').first().element();
		header.scrollIntoView({ block: 'center' });
		const headerRect = header.getBoundingClientRect();

		await dropPaletteWidgetAt('Model Stats', headerRect.left + 24, headerRect.top + 4);

		await expect
			.element(dialog.getByCSS('[data-layout-row="1-0"]').getByText('Model Stats'))
			.toBeVisible();

		const box = await dialog.getByCSS('[data-layout-box]').last().element();
		const location = box.getAttribute('data-box-location');
		const count = box.getAttribute('data-child-count');
		box.scrollIntoView({ block: 'end' });
		const boxRect = box.getBoundingClientRect();
		await dropPaletteWidgetAt('Top Models', boxRect.left + boxRect.width / 2, boxRect.bottom - 6);

		await expect
			.element(dialog.getByCSS(`[data-layout-row="${location}-${count}"]`).getByText('Top Models'))
			.toBeVisible();
		await expect
			.element(dialog.getByCSS('[data-layout-box]').last())
			.toHaveAttribute('data-box-location', location ?? '');
	});

	it('drops a widget between two boxes', async () => {
		mockDashboardApis();
		await renderDashboard([Group.ADMIN], false);
		await page.getByRole('button', { name: 'Customize Dashboard' }).click();
		const dialog = page.getByRole('dialog');
		const title = dialog.getByRole('button', { name: 'Drag Box' }).nth(1);
		const titleEl = await title.element();
		titleEl.scrollIntoView({ block: 'center' });
		const rect = titleEl.getBoundingClientRect();

		await dropPaletteWidgetAt(
			'Model Stats',
			rect.left + rect.width / 2,
			rect.top + rect.height / 2
		);

		await expect
			.element(dialog.getByCSS('[data-layout-row="2"]').getByText('Model Stats'))
			.toBeVisible();
		await expect
			.element(dialog.getByCSS('[data-layout-box]').nth(0))
			.toHaveAttribute('data-box-location', '1');
		await expect
			.element(dialog.getByCSS('[data-layout-box]').nth(1))
			.toHaveAttribute('data-box-location', '3');
	});

	it('scrolls a configure column up and down while a drag is held at its edge', async () => {
		mockDashboardApis();
		await renderDashboard([Group.ADMIN], false);
		await page.getByRole('button', { name: 'Customize Dashboard' }).click();

		const dialog = page.getByRole('dialog');
		const scroller = await dialog.getByCSS('[data-layout-scroll]').element();
		scroller.style.minHeight = '0';
		scroller.style.height = '220px';
		scroller.style.maxHeight = '220px';
		scroller.style.flexGrow = '0';
		const maxScroll = scroller.scrollHeight - scroller.clientHeight;
		expect(maxScroll).toBeGreaterThan(40);

		const handle = dialog.getByRole('button', { name: 'Drag On Platform' });
		await expect.element(handle).toBeVisible();
		const body = await page.getByCSS('body').element();
		(await handle.element()).dispatchEvent(
			new PointerEvent('pointerdown', { bubbles: true, cancelable: true, button: 0 })
		);

		scroller.scrollTop = Math.round(maxScroll / 2);
		const rect = scroller.getBoundingClientRect();
		const held = scroller.scrollTop;
		body.dispatchEvent(
			new PointerEvent('pointermove', {
				bubbles: true,
				cancelable: true,
				clientX: rect.left + rect.width / 2,
				clientY: rect.bottom - 2
			})
		);
		await vi.waitFor(() => {
			expect(scroller.scrollTop).toBeGreaterThan(held + 24);
		});
		const downward = scroller.scrollTop;
		await new Promise((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve)));
		expect(scroller.scrollTop).toBeGreaterThan(downward);

		body.dispatchEvent(
			new PointerEvent('pointermove', {
				bubbles: true,
				cancelable: true,
				clientX: rect.left + rect.width / 2,
				clientY: rect.top + 4
			})
		);
		await vi.waitFor(() => {
			expect(scroller.scrollTop).toBeLessThan(downward - 24);
		});
		const upward = scroller.scrollTop;
		await new Promise((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve)));
		expect(scroller.scrollTop).toBeLessThan(upward);

		const stoppedAt = scroller.scrollTop;
		body.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }));
		await new Promise((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve)));
		expect(scroller.scrollTop).toBe(stoppedAt);
		await expect.element(page.getByCSS('[data-drag-clone]')).not.toBeInTheDocument();
	});

	it('moves a widget in the page grid while dragging and keeps that place', async () => {
		mockDashboardApis();
		const first = await renderDashboard([Group.ADMIN], false);
		const main = page.getByRole('main');
		await main.getByCSS('[data-widget-id="top-servers-used"]').hover();
		const grip = main.getByRole('button', { name: 'Drag Top Servers Used' });
		await expect.element(grip).toBeVisible();

		const activity = main.getByCSS('[data-widget-id="server-activity"]');
		const rect = (await activity.element()).getBoundingClientRect();
		const body = await page.getByCSS('body').element();
		(await grip.element()).dispatchEvent(
			new PointerEvent('pointerdown', { bubbles: true, cancelable: true, button: 0 })
		);
		const pointerX = rect.left + 12;
		const pointerY = rect.top + rect.height / 2;
		body.dispatchEvent(
			new PointerEvent('pointermove', {
				bubbles: true,
				cancelable: true,
				clientX: pointerX,
				clientY: pointerY
			})
		);

		const clone = await page.getByCSS('[data-drag-clone]').element();
		const cloneRect = clone.getBoundingClientRect();
		expect(clone.textContent).toBe('Top Servers Used');
		expect(cloneRect.height).toBeLessThan(80);
		expect(pointerX).toBeGreaterThanOrEqual(cloneRect.left - 1);
		expect(pointerX).toBeLessThanOrEqual(cloneRect.right + 1);
		expect(pointerY).toBeGreaterThanOrEqual(cloneRect.top - 1);
		expect(pointerY).toBeLessThanOrEqual(cloneRect.bottom + 1);

		body.dispatchEvent(new PointerEvent('pointerup', { bubbles: true, cancelable: true }));
		await expect.element(page.getByCSS('[data-drag-clone]')).not.toBeInTheDocument();

		const row = main.getByCSS('[data-dashboard-card][data-parent="1"][data-row="1"]');
		await expect.element(row.nth(0)).toHaveAttribute('data-widget-id', 'top-servers-used');
		await expect.element(row.nth(1)).toHaveAttribute('data-widget-id', 'server-activity');

		first.unmount();
		await renderDashboard([Group.ADMIN], false);
		const again = page
			.getByRole('main')
			.getByCSS('[data-dashboard-card][data-parent="1"][data-row="1"]');
		await expect.element(again.nth(0)).toHaveAttribute('data-widget-id', 'top-servers-used');
		await expect.element(again.nth(1)).toHaveAttribute('data-widget-id', 'server-activity');
	});

	it('scrolls downward while a drag is held at the bottom of the overflow', async () => {
		mockDashboardApis();
		await renderDashboard([Group.ADMIN], false);
		const main = page.getByRole('main');
		const mainEl = await main.element();
		mainEl.style.height = '320px';
		mainEl.style.maxHeight = '320px';
		expect(mainEl.scrollHeight).toBeGreaterThan(mainEl.clientHeight + 40);

		await main.getByCSS('[data-widget-id="platform-stats"]').hover();
		const grip = main.getByRole('button', { name: 'Drag On Platform' });
		await expect.element(grip).toBeVisible();
		const body = await page.getByCSS('body').element();
		(await grip.element()).dispatchEvent(
			new PointerEvent('pointerdown', { bubbles: true, cancelable: true, button: 0 })
		);

		const rect = mainEl.getBoundingClientRect();
		const held = mainEl.scrollTop;
		body.dispatchEvent(
			new PointerEvent('pointermove', {
				bubbles: true,
				cancelable: true,
				clientX: rect.left + rect.width / 2,
				clientY: rect.top + rect.height / 2
			})
		);
		await new Promise((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve)));
		expect(mainEl.scrollTop).toBe(held);

		body.dispatchEvent(
			new PointerEvent('pointermove', {
				bubbles: true,
				cancelable: true,
				clientX: rect.left + rect.width / 2,
				clientY: rect.bottom - 2
			})
		);
		await vi.waitFor(() => {
			expect(mainEl.scrollTop).toBeGreaterThan(held + 24);
		});

		const scrolling = mainEl.scrollTop;
		body.dispatchEvent(new PointerEvent('pointerup', { bubbles: true, cancelable: true }));
		await new Promise((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve)));
		expect(mainEl.scrollTop).toBe(scrolling);
		await expect.element(page.getByCSS('[data-drag-clone]')).not.toBeInTheDocument();
	});

	it('scrolls upward while a drag is held at the top of the overflow', async () => {
		mockDashboardApis();
		await renderDashboard([Group.ADMIN], false);
		const main = page.getByRole('main');
		const mainEl = await main.element();
		mainEl.style.height = '320px';
		mainEl.style.maxHeight = '320px';
		const maxScroll = mainEl.scrollHeight - mainEl.clientHeight;
		expect(maxScroll).toBeGreaterThan(40);

		await main.getByCSS('[data-widget-id="platform-stats"]').hover();
		const grip = main.getByRole('button', { name: 'Drag On Platform' });
		await expect.element(grip).toBeVisible();
		const body = await page.getByCSS('body').element();
		(await grip.element()).dispatchEvent(
			new PointerEvent('pointerdown', { bubbles: true, cancelable: true, button: 0 })
		);

		mainEl.scrollTop = Math.round(maxScroll / 2);
		const rect = mainEl.getBoundingClientRect();
		const held = mainEl.scrollTop;
		const headerBottom = mainEl.firstElementChild?.getBoundingClientRect().bottom ?? rect.top;
		body.dispatchEvent(
			new PointerEvent('pointermove', {
				bubbles: true,
				cancelable: true,
				clientX: rect.left + rect.width / 2,
				clientY: headerBottom + 4
			})
		);
		await vi.waitFor(() => {
			expect(mainEl.scrollTop).toBeLessThan(held - 24);
		});

		const scrolling = mainEl.scrollTop;
		await new Promise((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve)));
		expect(mainEl.scrollTop).toBeLessThan(scrolling);
		const stoppedAt = mainEl.scrollTop;
		body.dispatchEvent(new PointerEvent('pointerup', { bubbles: true, cancelable: true }));
		await new Promise((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve)));
		expect(mainEl.scrollTop).toBe(stoppedAt);
		await expect.element(page.getByCSS('[data-drag-clone]')).not.toBeInTheDocument();
	});

	it('drops a widget beside the only widget in a row', async () => {
		mockDashboardApis();
		await renderDashboard([Group.ADMIN], false);
		const main = page.getByRole('main');
		await main.getByCSS('[data-widget-id="popular-tools"]').hover();
		const grip = main.getByRole('button', { name: 'Drag Recently Popular Tools' });
		await expect.element(grip).toBeVisible();

		const tokenUsage = main.getByCSS('[data-widget-id="token-usage"]');
		const rect = (await tokenUsage.element()).getBoundingClientRect();
		const body = await page.getByCSS('body').element();
		(await grip.element()).dispatchEvent(
			new PointerEvent('pointerdown', { bubbles: true, cancelable: true, button: 0 })
		);
		body.dispatchEvent(
			new PointerEvent('pointermove', {
				bubbles: true,
				cancelable: true,
				clientX: rect.left + rect.width * 0.7,
				clientY: rect.top + rect.height * 0.18
			})
		);
		body.dispatchEvent(new PointerEvent('pointerup', { bubbles: true, cancelable: true }));

		const tokenRow = await main.getByCSS('[data-widget-id="token-usage"]').element();
		const popularRow = await main.getByCSS('[data-widget-id="popular-tools"]').element();
		expect(tokenRow.getAttribute('data-row')).toBe(popularRow.getAttribute('data-row'));
		expect(Number(popularRow.getAttribute('data-index'))).toBeGreaterThan(
			Number(tokenRow.getAttribute('data-index'))
		);
	});

	it('hides an empty widget from its title menu', async () => {
		mockDashboardApis();
		await renderDashboard([Group.POWERUSER]);
		const main = page.getByRole('main');
		await main.getByCSS('[data-widget-id="popular-tools"]').hover();
		const grip = main.getByRole('button', { name: 'Drag Recently Popular Tools' });

		await main.getByRole('button', { name: 'Row actions: Recently Popular Tools' }).click();
		await expect.element(grip).toBeVisible();
		await expect
			.element(main.getByRole('button', { name: 'Row actions: Recently Popular Tools' }))
			.toBeVisible();
		await page
			.getByRole('checkbox', { name: 'Hide on empty data: Recently Popular Tools' })
			.click();

		await expect
			.element(main.getByRole('heading', { name: /Recently Popular Tools/ }))
			.not.toBeInTheDocument();
	});

	it('removes a widget from its title menu', async () => {
		mockDashboardApis();
		await renderDashboard([Group.POWERUSER]);
		const main = page.getByRole('main');
		await main.getByCSS('[data-widget-id="popular-tools"]').hover();

		await main.getByRole('button', { name: 'Row actions: Recently Popular Tools' }).click();
		await page.getByRole('button', { name: 'Remove Recently Popular Tools' }).click();

		await expect
			.element(main.getByRole('heading', { name: /Recently Popular Tools/ }))
			.not.toBeInTheDocument();

		await page.getByRole('button', { name: 'Customize Dashboard' }).click();
		await expect
			.element(page.getByRole('dialog').getByRole('button', { name: 'Add Recently Popular Tools' }))
			.toBeVisible();
	});

	it('hides a header from its title menu when device scans are required', async () => {
		mockDashboardApis();
		await renderDashboard([Group.ADMIN], false);
		const main = page.getByRole('main');
		await main.getByRole('heading', { name: 'MCP Servers', exact: true }).hover();
		const grip = main.getByRole('button', { name: 'Drag MCP Servers' });

		await main.getByRole('button', { name: 'Row actions: MCP Servers' }).click();
		await expect.element(grip).toBeVisible();
		await expect
			.element(main.getByRole('button', { name: 'Row actions: MCP Servers' }))
			.toBeVisible();
		const scansOnly = page.getByRole('radio', {
			name: 'Only show when device scans exist: MCP Servers'
		});
		await expect.element(scansOnly).not.toBeChecked();
		await expect
			.element(page.getByRole('radio', { name: 'Always show: MCP Servers' }))
			.toBeChecked();
		await scansOnly.click();

		await expect
			.element(main.getByRole('heading', { name: 'MCP Servers', exact: true }))
			.not.toBeInTheDocument();
	});

	it('shows a header for one stats source at a time', async () => {
		mockDashboardApis();
		const withoutStats = await renderDashboard([Group.ADMIN], true);
		const main = page.getByRole('main');

		await main.getByRole('heading', { name: 'MCP Servers', exact: true }).hover();
		await main.getByRole('button', { name: 'Row actions: MCP Servers' }).click();
		await page.getByRole('radio', { name: 'Only show when MCP stats exist: MCP Servers' }).click();
		await expect
			.element(main.getByRole('heading', { name: 'MCP Servers', exact: true }))
			.not.toBeInTheDocument();

		withoutStats.unmount();
		const withMcp = await renderDashboard([Group.ADMIN], true, { hasMcpStats: true });
		await expect
			.element(page.getByRole('main').getByRole('heading', { name: 'MCP Servers', exact: true }))
			.toBeVisible();

		await page.getByRole('main').getByRole('heading', { name: 'MCP Servers', exact: true }).hover();
		await page.getByRole('button', { name: 'Row actions: MCP Servers' }).click();
		await page.getByRole('radio', { name: 'Only show when LLM stats exist: MCP Servers' }).click();
		await expect
			.element(page.getByRole('main').getByRole('heading', { name: 'MCP Servers', exact: true }))
			.not.toBeInTheDocument();

		withMcp.unmount();
		const withLlm = await renderDashboard([Group.ADMIN], true, { hasLlmStats: true });
		await expect
			.element(page.getByRole('main').getByRole('heading', { name: 'MCP Servers', exact: true }))
			.toBeVisible();

		withLlm.unmount();
		await renderDashboard([Group.ADMIN], true, { hasMcpStats: true });
		await expect
			.element(page.getByRole('main').getByRole('heading', { name: 'MCP Servers', exact: true }))
			.not.toBeInTheDocument();

		await page.getByRole('button', { name: 'Customize Dashboard' }).click();
		await page.getByRole('radio', { name: 'Always show: MCP Servers' }).click();
		await expect
			.element(page.getByRole('radio', { name: 'Only show when LLM stats exist: MCP Servers' }))
			.not.toBeChecked();
		await page.getByRole('button', { name: 'Save' }).click();
		await expect
			.element(page.getByRole('main').getByRole('heading', { name: 'MCP Servers', exact: true }))
			.toBeVisible();
	});

	it('adds more than one header, each on its own row', async () => {
		mockDashboardApis();
		await renderDashboard([Group.POWERUSER]);
		const main = page.getByRole('main');

		await expect.element(main.getByRole('heading', { name: /Top Servers Used/ })).toBeVisible();
		await page.getByRole('button', { name: 'Customize Dashboard' }).click();
		const addHeader = page.getByRole('button', { name: 'Add Header' });
		await addHeader.click();
		await addHeader.click();

		const headings = page.getByRole('textbox', { name: 'Header' });
		await headings.nth(0).fill('Usage');
		await headings.nth(1).fill('Tools');
		await expect
			.element(main.getByRole('heading', { name: 'Usage', exact: true }))
			.not.toBeInTheDocument();
		await page.getByRole('button', { name: 'Save' }).click();

		await expect.element(main.getByRole('heading', { name: 'Usage', exact: true })).toBeVisible();
		await expect.element(main.getByRole('heading', { name: 'Tools', exact: true })).toBeVisible();
		expect(await page.getByCSS('[data-layout-header]').elements()).toHaveLength(2);
		expect(await page.getByCSS('[data-layout-header] [data-drop-slot]').elements()).toHaveLength(0);
	});

	it('drops a header from the available list into its own row', async () => {
		mockDashboardApis();
		await renderDashboard([Group.POWERUSER]);
		await page.getByRole('button', { name: 'Customize Dashboard' }).click();

		const item = await page.getByCSS('[data-palette-header]').element();
		const body = await page.getByCSS('body').element();
		item.dispatchEvent(
			new PointerEvent('pointerdown', { bubbles: true, cancelable: true, button: 0 })
		);
		const gap = await page.getByCSS('[data-layout-gap="0"]').element();
		const rect = gap.getBoundingClientRect();
		const clientX = rect.left + rect.width / 2;
		const clientY = rect.top + rect.height / 2;
		body.dispatchEvent(
			new PointerEvent('pointermove', { bubbles: true, cancelable: true, clientX, clientY })
		);
		body.dispatchEvent(
			new PointerEvent('pointerup', { bubbles: true, cancelable: true, clientX, clientY })
		);

		await expect.element(page.getByCSS('[data-layout-header]')).toBeInTheDocument();
		expect(await page.getByCSS('[data-layout-row] [data-layout-header]').elements()).toHaveLength(
			0
		);
		expect(await page.getByCSS('[data-layout-row="1"] [data-layout-card]').elements()).toHaveLength(
			1
		);
	});

	it('does not offer admin widgets to power users', async () => {
		mockDashboardApis();
		await renderDashboard([Group.POWERUSER]);

		await expect.element(page.getByRole('heading', { name: /Top Servers Used/ })).toBeVisible();
		await page.getByRole('button', { name: 'Customize Dashboard' }).click();

		await expect.element(page.getByText('Server Activity')).not.toBeInTheDocument();
		await expect.element(page.getByText('Token Usage')).not.toBeInTheDocument();
		await expect.element(page.getByText('Model Stats')).not.toBeInTheDocument();
		await expect.element(page.getByText('Most Active Users')).not.toBeInTheDocument();
		await expect
			.element(page.getByRole('button', { name: 'Add Most Frequent Tool Calls' }))
			.toBeVisible();
		await expect
			.element(page.getByRole('heading', { name: /Recently Popular Tools/ }))
			.toBeVisible();
	});

	it('keeps usage graphs out of the default layout and adds them from the catalog', async () => {
		mockDashboardApis();
		worker.use(
			http.get('/api/mcp-stats', () =>
				HttpResponse.json({
					items: [
						{
							mcpID: 'srv-1',
							mcpServerInstanceName: 'inst',
							mcpServerName: 'github',
							mcpServerDisplayName: 'GitHub',
							toolCalls: [
								{
									toolName: 'search',
									callCount: 4,
									items: [
										{
											createdAt: '2026-10-01T00:00:00Z',
											userID: 'user-1',
											processingTimeMs: 120.4,
											responseStatus: 500,
											error: 'boom'
										}
									]
								}
							]
						}
					],
					timeStart: '',
					timeEnd: '',
					totalCalls: 4,
					uniqueUsers: 1
				})
			),
			http.get('/api/total-token-usage', () =>
				HttpResponse.json({
					totalTokens: 1000,
					inputTokens: 600,
					outputTokens: 300,
					cacheReadTokens: 40,
					cacheWriteTokens: 60,
					totalSpend: 1.25
				})
			),
			http.get('/api/token-usage-stats', () =>
				HttpResponse.json({
					start: '2026-10-01T00:00:00Z',
					end: '2026-10-08T00:00:00Z',
					bucketSeconds: 3600,
					totals: {
						requests: 4,
						successes: 3,
						fallbacks: 1,
						avgDurationMs: 250,
						inputTokens: 10,
						outputTokens: 5,
						totalTokens: 15,
						totalSpend: 1.5
					},
					buckets: [
						{
							time: '2026-10-01T00:00:00Z',
							requests: 4,
							successes: 3,
							fallbacks: 1,
							avgDurationMs: 250,
							inputTokens: 10,
							outputTokens: 5,
							totalTokens: 15,
							totalSpend: 1.5
						}
					],
					providers: [
						{
							provider: 'anthropic',
							requests: 4,
							successes: 3,
							fallbacks: 1,
							avgDurationMs: 250,
							inputTokens: 10,
							outputTokens: 5,
							totalTokens: 15,
							totalSpend: 1.5
						}
					],
					topModelsByRequests: [
						{
							model: 'openai/gpt-5',
							provider: 'openai',
							requests: 4,
							successes: 3,
							fallbacks: 0,
							avgDurationMs: 250,
							inputTokens: 10,
							outputTokens: 5,
							totalTokens: 15,
							totalSpend: 0.25
						}
					],
					topModelsBySpend: []
				})
			)
		);

		await renderDashboard([Group.ADMIN], false);
		const main = page.getByRole('main');

		await expect.element(main.getByRole('heading', { name: 'On Platform' })).toBeVisible();
		await expect
			.element(main.getByRole('heading', { name: 'Model Stats' }))
			.not.toBeInTheDocument();
		await expect
			.element(main.getByRole('heading', { name: 'Most Frequent Tool Calls' }))
			.not.toBeInTheDocument();

		await page.getByRole('button', { name: 'Customize Dashboard' }).click();
		await expect.element(page.getByRole('button', { name: 'Add Model Stats' })).toBeVisible();
		await expect.element(page.getByRole('button', { name: 'Add Requests' })).toBeVisible();
		await expect.element(page.getByRole('button', { name: 'Add Spend by Provider' })).toBeVisible();
		await expect.element(page.getByRole('button', { name: 'Add Top Models' })).toBeVisible();
		await expect
			.element(page.getByRole('button', { name: 'Add Tool Call Individual Response Time' }))
			.toBeVisible();
		await expect.element(page.getByRole('button', { name: 'Add Tool Call Errors' })).toBeVisible();
		await expect
			.element(page.getByRole('button', { name: 'Add Tool Call Errors by Server' }))
			.toBeVisible();
		await expect.element(page.getByRole('button', { name: 'Add Most Active Users' })).toBeVisible();
		await expect
			.element(page.getByRole('button', { name: 'Add Most Frequently Used Servers' }))
			.not.toBeInTheDocument();

		await page.getByRole('button', { name: 'Add Model Stats' }).click();
		await page.getByRole('button', { name: 'Add Most Frequent Tool Calls' }).click();
		await page.getByRole('button', { name: 'Add Spend by Provider' }).click();
		await page.getByRole('button', { name: 'Add Top Models' }).click();
		await page.getByRole('button', { name: 'Save' }).click();

		await expect.element(main.getByRole('heading', { name: 'Model Stats' })).toBeVisible();
		await expect.element(main.getByText('Cached Input')).toBeVisible();
		await expect
			.element(main.getByRole('heading', { name: 'Most Frequent Tool Calls' }))
			.toBeVisible();
		await expect
			.element(
				main.getByCSS('[data-widget-id="most-frequent-tool-calls"]').getByText('search', {
					exact: true
				})
			)
			.toBeVisible();
		await expect.element(main.getByRole('heading', { name: 'Spend by Provider' })).toBeVisible();
		await expect.element(main.getByText('Anthropic')).toBeVisible();
		await expect.element(main.getByRole('heading', { name: 'Top Models' })).toBeVisible();
		await expect.element(main.getByRole('cell', { name: 'openai/gpt-5' })).toBeVisible();
	});

	it('lets an admin choose which model stats to show', async () => {
		mockDashboardApis();
		worker.use(
			http.get('/api/total-token-usage', () =>
				HttpResponse.json({
					totalTokens: 1000,
					inputTokens: 600,
					outputTokens: 300,
					cacheReadTokens: 40,
					cacheWriteTokens: 60,
					totalSpend: 1.25
				})
			)
		);

		const first = await renderDashboard([Group.ADMIN], false);
		const main = page.getByRole('main');

		await page.getByRole('button', { name: 'Customize Dashboard' }).click();
		await page.getByRole('button', { name: 'Add Model Stats' }).click();
		await page.getByRole('checkbox', { name: 'Input', exact: true }).click();
		await page.getByRole('button', { name: 'Save' }).click();

		const card = main.getByCSS('[data-widget-id="model-stats"]');
		await expect.element(card.getByText('Total', { exact: true })).toBeVisible();
		await expect.element(card.getByText('Output', { exact: true })).toBeVisible();
		await expect.element(card.getByText('Cached Input')).toBeVisible();
		await expect.element(card.getByText('Spend', { exact: true })).toBeVisible();
		await expect.element(card.getByText('Input', { exact: true })).not.toBeInTheDocument();

		await card.hover();
		await main.getByRole('button', { name: 'Row actions: Model Stats' }).click();
		await page.getByRole('checkbox', { name: 'Input', exact: true }).click();
		await expect.element(card.getByText('Input', { exact: true })).toBeVisible();

		first.unmount();
		await renderDashboard([Group.ADMIN], false);
		const again = page.getByRole('main').getByCSS('[data-widget-id="model-stats"]');
		await expect.element(again.getByText('Input', { exact: true })).toBeVisible();
		await expect.element(again.getByText('1,000')).toBeVisible();
	});
});

describe('Dashboard data loading', () => {
	it('only fetches range datasets that a placed widget uses', async () => {
		const scanStats = vi.fn();
		const tokenStats = vi.fn();
		mockDashboardApis();
		worker.use(
			http.get('/api/devices/scan-stats', () => {
				scanStats();
				return HttpResponse.json(emptyScanStats);
			}),
			http.get('/api/token-usage-stats', () => {
				tokenStats();
				return HttpResponse.json({});
			})
		);

		await renderDashboard([Group.ADMIN], true);

		// The admin default layout places device-scan widgets but no LLM request widgets.
		await vi.waitFor(() => expect(scanStats).toHaveBeenCalled());
		await expect.element(page.getByRole('button', { name: /Last 30 Days/ })).toBeEnabled();
		expect(tokenStats).not.toHaveBeenCalled();
	});

	it('disables drag and the widget menu until the widget finishes loading', async () => {
		mockDashboardApis();
		let release: () => void = () => {};
		const released = new Promise<void>((resolve) => (release = resolve));
		worker.use(
			http.get('/api/mcp-stats', async () => {
				await released;
				return HttpResponse.json(emptyUsageStats);
			})
		);

		await renderDashboard([Group.POWERUSER]);
		const main = page.getByRole('main');
		const menu = main.getByRole('button', { name: 'Row actions: Recently Popular Tools' });
		const grip = main.getByRole('button', { name: 'Drag Recently Popular Tools' });

		// Popular Tools keeps its title (and list skeleton) while loading.
		await expect.element(menu).toBeDisabled();
		await expect.element(grip).toBeDisabled();

		release();
		await expect.element(menu).toBeEnabled();
		await expect.element(grip).toBeEnabled();
	});

	it('does not apply a layout saved by another user', async () => {
		mockDashboardApis();
		const otherLayout = appendHeader(factoryConfig({ isAdmin: true }), 'Someone else');
		localStorage.setItem(dashboardLayoutKey('another-user'), JSON.stringify(otherLayout));

		const other = await renderDashboard([Group.ADMIN], true);
		await expect
			.element(page.getByRole('main').getByRole('heading', { name: 'MCP Servers', exact: true }))
			.toBeVisible();
		await expect
			.element(page.getByRole('heading', { name: 'Someone else', exact: true }))
			.not.toBeInTheDocument();

		other.unmount();
		localStorage.setItem(dashboardLayoutKey(getProfileResponse.id), JSON.stringify(otherLayout));
		await renderDashboard([Group.ADMIN], true);
		await expect
			.element(page.getByRole('heading', { name: 'Someone else', exact: true }))
			.toBeVisible();
	});

	it('retries a failed header check on the next range change', async () => {
		mockDashboardApis();
		let probeCalls = 0;
		worker.use(
			http.get('/api/mcp-audit-logs', () => {
				probeCalls += 1;
				if (probeCalls === 1) return new HttpResponse(null, { status: 500 });
				return HttpResponse.json({ items: [], total: 1, limit: 1, offset: 0 });
			})
		);
		const layout = setHeaderVisibility(
			factoryConfig({ isAdmin: true }),
			'header-mcp-servers',
			'mcp-stats'
		);
		localStorage.setItem(dashboardLayoutKey(getProfileResponse.id), JSON.stringify(layout));

		await renderDashboard([Group.ADMIN], true);
		const main = page.getByRole('main');
		await vi.waitFor(() => expect(probeCalls).toBe(1));
		await expect
			.element(main.getByRole('heading', { name: 'MCP Servers', exact: true }))
			.not.toBeInTheDocument();

		const rangeButton = page.getByRole('button', { name: /Last 30 Days/ });
		await expect.element(rangeButton).toBeEnabled();
		await rangeButton.click();
		await page.getByRole('button', { name: 'Last Hour', exact: true }).click();

		await expect
			.element(main.getByRole('heading', { name: 'MCP Servers', exact: true }))
			.toBeVisible();
		expect(probeCalls).toBe(2);
	});

	it('clears the previous range when a range fetch fails', async () => {
		mockDashboardApis();
		let statsCalls = 0;
		worker.use(
			http.get('/api/mcp-stats', () => {
				statsCalls += 1;
				if (statsCalls > 1) return new HttpResponse(null, { status: 500 });
				return HttpResponse.json({
					...emptyUsageStats,
					totalCalls: 3,
					items: [
						{
							mcpID: 'ms1stale',
							mcpServerInstanceName: '',
							mcpServerName: 'stale',
							mcpServerDisplayName: 'Stale Server',
							toolCalls: [{ toolName: 'search', callCount: 3, items: [] }]
						}
					]
				});
			})
		);

		await renderDashboard([Group.POWERUSER]);
		const main = page.getByRole('main');
		await expect.element(main.getByText('Stale Server').first()).toBeVisible();

		const rangeButton = page.getByRole('button', { name: /Last 30 Days/ });
		await expect.element(rangeButton).toBeEnabled();
		await rangeButton.click();
		await page.getByRole('button', { name: 'Last Hour', exact: true }).click();

		await vi.waitFor(() => expect(statsCalls).toBe(2));
		await expect.element(main.getByText('Stale Server')).not.toBeInTheDocument();
		// The HTTP client reports the failure; the page must not report it a second time.
		expect(errors.items.filter((error) => error.message.includes('/mcp-stats'))).toHaveLength(1);
	});

	it('reuses data for the current range when a widget is removed and added back', async () => {
		mockDashboardApis();
		const statsCalls = vi.fn();
		worker.use(
			http.get('/api/mcp-stats', () => {
				statsCalls();
				return HttpResponse.json(emptyUsageStats);
			})
		);
		const layout = { ...factoryConfig({ isAdmin: false }), rows: [] };
		localStorage.setItem(
			dashboardLayoutKey(getProfileResponse.id),
			JSON.stringify({
				...layout,
				rows: [{ kind: 'widgets', items: [{ id: 'popular-tools', hideWhenEmpty: false }] }]
			})
		);

		await renderDashboard([Group.POWERUSER]);
		const main = page.getByRole('main');
		await vi.waitFor(() => expect(statsCalls).toHaveBeenCalledTimes(1));

		await main.getByCSS('[data-widget-id="popular-tools"]').hover();
		await main.getByRole('button', { name: 'Row actions: Recently Popular Tools' }).click();
		await page.getByRole('button', { name: 'Remove Recently Popular Tools' }).click();
		// Removing the only widget shows the empty state, which has its own customize button.
		await page.getByRole('button', { name: 'Customize Dashboard' }).first().click();
		await page.getByRole('button', { name: 'Add Recently Popular Tools' }).click();
		await page.getByRole('button', { name: 'Save' }).click();

		await expect
			.element(main.getByRole('heading', { name: /Recently Popular Tools/ }))
			.toBeVisible();
		expect(statsCalls).toHaveBeenCalledTimes(1);
	});

	it('waits for the saved layout before fetching range data', async () => {
		mockDashboardApis();
		const scanStats = vi.fn();
		const tokenTotals = vi.fn();
		const mcpStats = vi.fn();
		worker.use(
			http.get('/api/devices/scan-stats', () => {
				scanStats();
				return HttpResponse.json(emptyScanStats);
			}),
			http.get('/api/total-token-usage', () => {
				tokenTotals();
				return HttpResponse.json({ totalTokens: 0, totalSpend: 0 });
			}),
			http.get('/api/mcp-stats', () => {
				mcpStats();
				return HttpResponse.json(emptyUsageStats);
			})
		);
		localStorage.setItem(
			dashboardLayoutKey(getProfileResponse.id),
			JSON.stringify({
				...factoryConfig({ isAdmin: true }),
				rows: [{ kind: 'widgets', items: [{ id: 'most-active-users', hideWhenEmpty: false }] }]
			})
		);

		await renderDashboard([Group.ADMIN], true);

		await vi.waitFor(() => expect(mcpStats).toHaveBeenCalledTimes(1));
		await expect.element(page.getByRole('button', { name: /Last 30 Days/ })).toBeEnabled();
		// The admin default layout would need device scans and token totals; the saved one doesn't.
		expect(scanStats).not.toHaveBeenCalled();
		expect(tokenTotals).not.toHaveBeenCalled();
	});
});
