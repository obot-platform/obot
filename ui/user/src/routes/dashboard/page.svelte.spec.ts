import { Group } from '$lib/services';
import { createMockProfile, preparePageData } from '../../tests/helpers/pageData';
import { worker } from '../../tests/mocks/worker';
import type { PageData } from './$types';
import DashboardPage from './+page.svelte';
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

async function renderDashboard(groups: string[], hasDeviceScans = false) {
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
		http.get('/api/token-usage', () => HttpResponse.json({ items: [] }))
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
			.element(page.getByRole('checkbox', { name: 'Only show when device scans exist: Inventory' }))
			.toBeChecked();
		await expect
			.element(
				page.getByRole('checkbox', { name: 'Only show when device scans exist: MCP Servers' })
			)
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
		const scansOnly = page.getByRole('checkbox', {
			name: 'Only show when device scans exist: MCP Servers'
		});
		await expect.element(scansOnly).not.toBeChecked();
		await scansOnly.click();

		await expect
			.element(main.getByRole('heading', { name: 'MCP Servers', exact: true }))
			.not.toBeInTheDocument();
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
		await expect
			.element(page.getByRole('heading', { name: /Recently Popular Tools/ }))
			.toBeVisible();
	});
});
