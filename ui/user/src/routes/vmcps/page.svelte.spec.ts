import type { MCPCatalogEntry, MCPCatalogEntryServerManifest, ToolOverride } from '$lib/services';
import { mcpServersAndEntries } from '$lib/stores';
import { createMCPCatalogEntry } from '../../tests/helpers/mcp';
import { preparePageData } from '../../tests/helpers/pageData';
import { worker } from '../../tests/mocks/worker';
import VMcpsPage from './+page.svelte';
import { http, HttpResponse } from 'msw';
import { tick } from 'svelte';
import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page, userEvent } from 'vitest/browser';

const componentEntry = createMCPCatalogEntry({
	id: 'entry-github',
	name: 'GitHub',
	manifest: {
		toolPreview: [
			{ id: 'create_issue', name: 'create_issue', description: 'Create an issue' },
			{ id: 'list_issues', name: 'list_issues', description: 'List issues' }
		]
	}
});

const toolOverrides: ToolOverride[] = [
	{ name: 'create_issue', description: 'Create an issue', enabled: true },
	{ name: 'list_issues', description: 'List issues', enabled: false }
];

function createVMcp(overrides?: ToolOverride[]) {
	return createMCPCatalogEntry({
		id: 'vmcp-1',
		name: 'Issue Tracker vMCP',
		runtime: 'composite',
		manifest: {
			compositeConfig: {
				componentServers: [
					{
						catalogEntryID: componentEntry.id,
						manifest: componentEntry.manifest,
						toolPrefix: 'github_',
						...(overrides ? { toolOverrides: overrides } : {})
					}
				]
			}
		}
	});
}

async function renderVMcpsPage(vmcp: MCPCatalogEntry, extraEntries: MCPCatalogEntry[] = []) {
	mcpServersAndEntries.current = {
		entries: [componentEntry, vmcp, ...extraEntries],
		servers: [],
		userInstances: [],
		userConfiguredServers: [],
		loading: false,
		lastFetched: null,
		isInitialized: true
	};
	await preparePageData();
	return render(VMcpsPage);
}

async function clickNative(locator: ReturnType<typeof page.getByCSS>) {
	await expect.element(locator).toBeInTheDocument();
	// Native DOM click: Playwright actionability fails on the popover's click-catch overlay.
	const el = await locator.element();
	if (!(el instanceof HTMLElement)) {
		throw new Error('Expected an HTMLElement');
	}
	el.click();
	await tick();
}

async function expandServers(name = 'Issue Tracker vMCP', count = 1) {
	const label = count === 1 ? 'server' : 'servers';
	await page.getByRole('button', { name: `Show ${count} ${label} in ${name}` }).click();
	await tick();
}

function componentBlock() {
	return page.getByRole('button', { name: componentEntry.manifest.name!, exact: true });
}

function mockUpdateEntry(vmcp: MCPCatalogEntry, onUpdate: (manifest: unknown) => void) {
	worker.use(
		http.get(`/api/mcp-catalogs/default/entries/${vmcp.id}`, () => HttpResponse.json(vmcp)),
		http.put(`/api/mcp-catalogs/default/entries/${vmcp.id}`, async ({ request }) => {
			const manifest = (await request.json()) as MCPCatalogEntryServerManifest;
			onUpdate(manifest);
			return HttpResponse.json({ ...vmcp, manifest });
		})
	);
}

function componentServersFrom(manifest: unknown) {
	return (manifest as MCPCatalogEntryServerManifest).compositeConfig?.componentServers ?? [];
}

function panelCard(name: string) {
	return page.getByRole('button', { name: new RegExp(`View ${name} details`) });
}

function centerOf(el: Element) {
	const rect = el.getBoundingClientRect();
	return { x: rect.left + rect.width / 2, y: rect.top + rect.height / 2 };
}

type Point = { x: number; y: number };

function pointer(el: HTMLElement, type: string, pointerId: number, at: Point) {
	el.dispatchEvent(
		new PointerEvent(type, {
			bubbles: true,
			cancelable: true,
			button: 0,
			pointerId,
			clientX: at.x,
			clientY: at.y
		})
	);
}

async function pressCard(locator: ReturnType<typeof page.getByRole>, pointerId: number) {
	const el = await locator.element();
	if (!(el instanceof HTMLElement)) throw new Error('Expected an HTMLElement');
	// A synthesized pointerId is not a live pointer, so real capture would throw.
	el.setPointerCapture = () => {};

	const from = centerOf(el);
	pointer(el, 'pointerdown', pointerId, from);
	return { el, from };
}

describe('vMCPs Page', () => {
	describe('component with stored tool overrides', () => {
		it('edits the stored overrides instead of running the tool setup flow', async () => {
			await renderVMcpsPage(createVMcp(toolOverrides));

			await componentBlock().click();

			await expect.element(page.getByText('Configure GitHub Tools')).toBeVisible();
			await expect.element(page.getByText('create_issue').first()).toBeVisible();
			await expect.element(page.getByText('list_issues').first()).toBeVisible();
			// The prefix field has no accessible name, and the setup flow keeps a second copy of
			// the same editor mounted, so scope the lookup to the dialog that is open.
			await expect
				.element(page.getByCSS('dialog[open] input[placeholder="No prefix"]'))
				.toHaveValue('github_');
			await expect.element(page.getByRole('button', { name: 'Delete MCP Server' })).toBeVisible();
			await expect.element(page.getByRole('button', { name: 'Refresh Tools' })).toBeVisible();
			await expect
				.element(page.getByRole('button', { name: 'Get Started', exact: true }))
				.not.toBeInTheDocument();
		});

		it('saves the edited overrides back onto the vMCP', async () => {
			const vmcp = createVMcp(toolOverrides);
			const update = vi.fn();
			mockUpdateEntry(vmcp, update);

			await renderVMcpsPage(vmcp);
			await componentBlock().click();

			// Enable the tool that is currently excluded from the composite.
			await page.getByRole('checkbox', { name: 'Enabled' }).nth(1).click();
			await page.getByRole('button', { name: 'Confirm' }).click();

			await vi.waitFor(() => expect(update).toHaveBeenCalled());
			expect(componentServersFrom(update.mock.calls[0][0])[0]).toMatchObject({
				catalogEntryID: componentEntry.id,
				toolPrefix: 'github_',
				toolOverrides: [
					{ name: 'create_issue', enabled: true },
					{ name: 'list_issues', enabled: true }
				]
			});
		});

		it('refreshes tools from the server through the setup flow', async () => {
			await renderVMcpsPage(createVMcp(toolOverrides));

			await componentBlock().click();
			await page.getByRole('button', { name: 'Refresh' }).click();

			await expect
				.element(page.getByRole('button', { name: 'Get Started', exact: true }))
				.toBeVisible();
		});
	});

	describe('component without stored tool overrides', () => {
		it('offers modifying tools or deleting the server', async () => {
			await renderVMcpsPage(createVMcp());

			await componentBlock().click();

			await expect.element(page.getByRole('button', { name: 'Modify Tools' })).toBeVisible();
			await expect.element(page.getByRole('button', { name: 'Delete MCP Server' })).toBeVisible();
			await expect
				.element(page.getByRole('button', { name: 'Get Started', exact: true }))
				.not.toBeInTheDocument();
		});

		it('starts the tool setup flow from Modify Tools', async () => {
			await renderVMcpsPage(createVMcp());

			await componentBlock().click();
			await page.getByRole('button', { name: 'Modify Tools' }).click();

			await expect
				.element(page.getByRole('button', { name: 'Get Started', exact: true }))
				.toBeVisible();
		});

		it('removes the server from the vMCP without visiting the setup flow', async () => {
			const vmcp = createVMcp();
			const update = vi.fn();
			mockUpdateEntry(vmcp, update);

			await renderVMcpsPage(vmcp);
			await componentBlock().click();
			await page.getByRole('button', { name: 'Delete MCP Server' }).click();

			await expect.element(page.getByText('Confirm Remove')).toBeVisible();
			await page.getByRole('button', { name: "Yes, I'm sure" }).click();

			await vi.waitFor(() => expect(update).toHaveBeenCalled());
			expect(componentServersFrom(update.mock.calls[0][0])).toEqual([]);
		});
	});

	describe('dragging a server from the panel onto the canvas', () => {
		const slack = createMCPCatalogEntry({ id: 'entry-slack', name: 'Slack' });

		function vmcpCard() {
			return page.getByRole('button', { name: 'Edit Issue Tracker vMCP' });
		}

		/** Presses the Slack panel card and drags it over the vMCP card, without releasing. */
		async function dragSlackOntoVMcp(pointerId: number) {
			const target = await vmcpCard().element();
			const { el } = await pressCard(panelCard('Slack'), pointerId);
			const to = centerOf(target);
			pointer(el, 'pointermove', pointerId, to);
			await tick();
			return { el, to };
		}

		it('marks both the dragged card and the vMCP it is linked to', async () => {
			await renderVMcpsPage(createVMcp(), [slack]);

			await dragSlackOntoVMcp(12);

			// The panel reads the drag state and the canvas reads the link state, so both
			// updating proves they share one source of truth. The drag ghost carries the same
			// class, so scope the canvas assertion to the vMCP card itself.
			await expect.element(panelCard('Slack')).toHaveClass(/opacity-30/);
			await expect.element(page.getByCSS('.vmcp-drop-target').first()).toBeInTheDocument();
		});

		it('adds the dropped server to the vMCP it landed on', async () => {
			const vmcp = createVMcp();
			const update = vi.fn();
			mockUpdateEntry(vmcp, update);
			await renderVMcpsPage(vmcp, [slack]);

			const { el, to } = await dragSlackOntoVMcp(13);
			pointer(el, 'pointerup', 13, to);

			await vi.waitFor(() => expect(update).toHaveBeenCalled());
			expect(componentServersFrom(update.mock.calls[0][0]).at(-1)).toEqual({
				catalogEntryID: slack.id,
				manifest: slack.manifest
			});
		});

		it('leaves the vMCP alone when Escape cancels the drag before release', async () => {
			const vmcp = createVMcp();
			const update = vi.fn();
			mockUpdateEntry(vmcp, update);
			await renderVMcpsPage(vmcp, [slack]);

			const { el, to } = await dragSlackOntoVMcp(14);
			await userEvent.keyboard('{Escape}');
			pointer(el, 'pointerup', 14, to);
			await tick();

			expect(update).not.toHaveBeenCalled();
			await expect.element(page.getByCSS('.vmcp-drop-target')).not.toBeInTheDocument();
		});

		it('opens the server details when the press never travels far enough to drag', async () => {
			const listServers = vi.fn();
			worker.use(
				http.get(`/api/mcp-catalogs/default/entries/${slack.id}`, () => HttpResponse.json(slack)),
				http.get(`/api/mcp-catalogs/default/entries/${slack.id}/servers`, () => {
					listServers();
					return HttpResponse.json({ items: [] });
				})
			);
			await renderVMcpsPage(createVMcp(), [slack]);

			const { el, from } = await pressCard(panelCard('Slack'), 15);
			pointer(el, 'pointerup', 15, from);

			await expect.element(page.getByRole('dialog').first()).toBeVisible();
			// The dialog loads the entry's servers, so let that settle inside the test rather
			// than leaking a request into the next one.
			await vi.waitFor(() => expect(listServers).toHaveBeenCalled());
		});
	});

	describe('dragging a server from the panel onto the table view', () => {
		const slack = createMCPCatalogEntry({ id: 'entry-slack', name: 'Slack' });

		async function showTableView() {
			await page.getByRole('button', { name: 'Table View' }).click({ force: true });
			await expect.element(vmcpRow()).toBeVisible();
		}

		function vmcpRow() {
			return page.getByRole('cell', { name: 'Issue Tracker vMCP' });
		}

		function dropZone() {
			return page.getByRole('region', { name: 'MCP Servers in Issue Tracker vMCP' });
		}

		/** Presses the Slack panel card and drags it over `target`, without releasing. */
		async function dragSlackOnto(target: ReturnType<typeof page.getByRole>, pointerId: number) {
			const targetEl = await target.element();
			const { el } = await pressCard(panelCard('Slack'), pointerId);
			const to = centerOf(targetEl);
			pointer(el, 'pointermove', pointerId, to);
			await tick();
			return { el, to };
		}

		it('marks the row the drag is linked to', async () => {
			await renderVMcpsPage(createVMcp(), [slack]);
			await showTableView();

			await dragSlackOnto(vmcpRow(), 20);

			await expect.element(page.getByCSS('tbody tr').first()).toHaveClass(/outline-primary/);
		});

		it('adds the dropped server to the row it landed on', async () => {
			const vmcp = createVMcp();
			const update = vi.fn();
			mockUpdateEntry(vmcp, update);
			await renderVMcpsPage(vmcp, [slack]);
			await showTableView();

			const { el, to } = await dragSlackOnto(vmcpRow(), 21);
			pointer(el, 'pointerup', 21, to);

			await vi.waitFor(() => expect(update).toHaveBeenCalled());
			expect(componentServersFrom(update.mock.calls[0][0]).at(-1)).toEqual({
				catalogEntryID: slack.id,
				manifest: slack.manifest
			});
		});

		it('lists the servers of the row that was clicked', async () => {
			await renderVMcpsPage(createVMcp(), [slack]);
			await showTableView();

			await vmcpRow().click();

			await expect.element(dropZone()).toBeVisible();
			await expect
				.element(
					page.getByCSS('dialog[open]').getByText(componentEntry.manifest.name!, { exact: true })
				)
				.toBeVisible();
			await expect.element(page.getByRole('button', { name: 'Edit tools' })).toBeVisible();
			await expect.element(page.getByRole('button', { name: 'Remove' })).toBeVisible();
		});

		it('opens the tool setup flow from Edit tools when no overrides are stored', async () => {
			await renderVMcpsPage(createVMcp(), [slack]);
			await showTableView();
			await vmcpRow().click();

			await page.getByRole('button', { name: 'Edit tools' }).click();

			await expect
				.element(page.getByRole('button', { name: 'Get Started', exact: true }))
				.toBeVisible();
			await expect
				.element(page.getByRole('button', { name: 'Modify Tools' }))
				.not.toBeInTheDocument();
		});

		it('opens the stored overrides editor from Edit tools', async () => {
			await renderVMcpsPage(createVMcp(toolOverrides), [slack]);
			await showTableView();
			await vmcpRow().click();

			await page.getByRole('button', { name: 'Edit tools' }).click();

			await expect.element(page.getByText('Configure GitHub Tools')).toBeVisible();
			await expect
				.element(page.getByRole('button', { name: 'Modify Tools' }))
				.not.toBeInTheDocument();
		});

		it('prompts to remove the server without an extra choice dialog', async () => {
			await renderVMcpsPage(createVMcp(), [slack]);
			await showTableView();
			await vmcpRow().click();

			await page.getByRole('button', { name: 'Remove' }).click();

			await expect.element(page.getByText('Confirm Remove')).toBeVisible();
			await expect
				.element(page.getByRole('button', { name: 'Modify Tools' }))
				.not.toBeInTheDocument();
			await page.getByRole('button', { name: 'Cancel' }).click();
			await expect.element(page.getByText('Confirm Remove')).not.toBeVisible();
		});

		it('paints the drag ghost above the open dialog', async () => {
			await renderVMcpsPage(createVMcp(), [slack]);
			await showTableView();
			await vmcpRow().click();
			await expect.element(dropZone()).toBeVisible();

			await dragSlackOnto(dropZone(), 23);

			const overlay = await page.getByCSS('[data-vmcp-drag-overlay]').element();
			const dialog = await page.getByCSS('dialog[open]').element();
			expect(Number(getComputedStyle(overlay).zIndex)).toBeGreaterThan(
				Number(getComputedStyle(dialog).zIndex)
			);
		});

		it('adds the dropped server to the vMCP whose dialog is open', async () => {
			const vmcp = createVMcp();
			const update = vi.fn();
			mockUpdateEntry(vmcp, update);
			await renderVMcpsPage(vmcp, [slack]);
			await showTableView();
			await vmcpRow().click();
			await expect.element(dropZone()).toBeVisible();

			const { el, to } = await dragSlackOnto(dropZone(), 22);
			pointer(el, 'pointerup', 22, to);

			await vi.waitFor(() => expect(update).toHaveBeenCalled());
			expect(componentServersFrom(update.mock.calls[0][0]).at(-1)).toEqual({
				catalogEntryID: slack.id,
				manifest: slack.manifest
			});
		});
	});

	describe('MCP servers sidebar', () => {
		it('hides the server list when the sidebar is collapsed', async () => {
			await renderVMcpsPage(createVMcp());

			await expect.element(page.getByPlaceholder('Search MCP servers...')).toBeVisible();

			await page.getByRole('button', { name: 'Hide MCP Servers' }).click();

			await expect.element(page.getByPlaceholder('Search MCP servers...')).not.toBeInTheDocument();
			await expect.element(page.getByRole('button', { name: 'Show MCP Servers' })).toBeVisible();

			await page.getByRole('button', { name: 'Show MCP Servers' }).click();

			await expect.element(page.getByPlaceholder('Search MCP servers...')).toBeVisible();
		});
	});

	describe('MCP server settings popover', () => {
		function settingsTrigger() {
			return page.getByCSS('#mcp-server-settings-button');
		}

		function settingsPanel() {
			return page.getByRole('dialog', { name: 'MCP server settings' });
		}

		function deprecatedToggle() {
			return page.getByRole('checkbox', { name: 'Show deprecated MCP servers' });
		}

		async function openSettings() {
			await clickNative(settingsTrigger());
		}

		it('announces the panel as a modal dialog owned by the trigger', async () => {
			await renderVMcpsPage(createVMcp());

			const trigger = settingsTrigger();
			await expect.element(trigger).toHaveAttribute('aria-haspopup', 'dialog');
			await expect.element(trigger).toHaveAttribute('aria-expanded', 'false');
			await expect.element(trigger).toHaveAttribute('aria-controls', 'mcp-server-settings-panel');

			await openSettings();

			await expect.element(trigger).toHaveAttribute('aria-expanded', 'true');
			await expect.element(settingsPanel()).toHaveAttribute('aria-modal', 'true');
		});

		it('moves focus into the panel and keeps tabbing inside it', async () => {
			await renderVMcpsPage(createVMcp());
			await openSettings();

			const toggle = await deprecatedToggle().element();
			await vi.waitFor(() => expect(document.activeElement).toBe(toggle));

			// Tab wraps onto the only control in the panel rather than the page behind it.
			await userEvent.keyboard('{Tab}');
			expect(document.activeElement).toBe(toggle);
		});

		it('closes on Escape and returns focus to the trigger', async () => {
			await renderVMcpsPage(createVMcp());
			await openSettings();

			const toggle = await deprecatedToggle().element();
			await vi.waitFor(() => expect(document.activeElement).toBe(toggle));

			await userEvent.keyboard('{Escape}');

			await expect.element(settingsTrigger()).toHaveAttribute('aria-expanded', 'false');
			expect(document.activeElement).toBe(await settingsTrigger().element());
		});
	});

	describe('vMCP settings popover', () => {
		const workspaceVMcp = createMCPCatalogEntry({
			id: 'vmcp-workspace',
			name: 'Workspace vMCP',
			runtime: 'composite',
			powerUserWorkspaceID: 'ws-1',
			powerUserID: 'user-1'
		});

		function settingsTrigger() {
			return page.getByCSS('#vmcp-settings-button');
		}

		function settingsPanel() {
			return page.getByRole('dialog', { name: 'vMCP settings' });
		}

		function showAllToggle() {
			return page.getByRole('checkbox', { name: 'Include User Connectors' });
		}

		async function openSettings() {
			await clickNative(settingsTrigger());
		}

		it('hides workspace-owned connectors until they are enabled in settings', async () => {
			const workspaceConnector = createMCPCatalogEntry({
				id: 'entry-workspace',
				name: 'Workspace Slack',
				powerUserWorkspaceID: 'ws-1',
				powerUserID: 'user-1'
			});
			await renderVMcpsPage(createVMcp(), [workspaceVMcp, workspaceConnector]);

			await expect
				.element(page.getByRole('button', { name: 'Edit Issue Tracker vMCP' }))
				.toBeVisible();
			await expect.element(page.getByRole('button', { name: /View GitHub/ })).toBeVisible();
			await expect
				.element(page.getByRole('button', { name: 'Edit Workspace vMCP' }))
				.not.toBeInTheDocument();
			await expect
				.element(page.getByRole('button', { name: /View Workspace Slack/ }))
				.not.toBeInTheDocument();

			await openSettings();
			const toggle = await showAllToggle().element();
			if (!(toggle instanceof HTMLElement)) {
				throw new Error('Expected the show-all toggle to be an HTMLElement');
			}
			toggle.click();
			await tick();

			await expect.element(page.getByRole('button', { name: 'Edit Workspace vMCP' })).toBeVisible();
			await expect
				.element(page.getByRole('button', { name: /View Workspace Slack/ }))
				.toBeVisible();
		});

		it('announces the panel as a modal dialog owned by the trigger', async () => {
			await renderVMcpsPage(createVMcp());

			const trigger = settingsTrigger();
			await expect.element(trigger).toHaveAttribute('aria-haspopup', 'dialog');
			await expect.element(trigger).toHaveAttribute('aria-expanded', 'false');
			await expect.element(trigger).toHaveAttribute('aria-controls', 'vmcp-settings-panel');

			await openSettings();

			await expect.element(trigger).toHaveAttribute('aria-expanded', 'true');
			await expect.element(settingsPanel()).toHaveAttribute('aria-modal', 'true');
		});

		it('moves focus into the panel and keeps tabbing inside it', async () => {
			await renderVMcpsPage(createVMcp());
			await openSettings();

			const toggle = await showAllToggle().element();
			await vi.waitFor(() => expect(document.activeElement).toBe(toggle));

			await userEvent.keyboard('{Tab}');
			expect(document.activeElement).toBe(
				await page.getByRole('combobox', { name: 'Sort by' }).element()
			);
		});

		it('closes on Escape and returns focus to the trigger', async () => {
			await renderVMcpsPage(createVMcp());
			await openSettings();

			const toggle = await showAllToggle().element();
			await vi.waitFor(() => expect(document.activeElement).toBe(toggle));

			await userEvent.keyboard('{Escape}');

			await expect.element(settingsTrigger()).toHaveAttribute('aria-expanded', 'false');
			expect(document.activeElement).toBe(await settingsTrigger().element());
		});

		it('reorders the canvas by name, created date, and server count', async () => {
			const bravo = createMCPCatalogEntry({
				id: 'vmcp-b',
				name: 'Bravo Gateway',
				runtime: 'composite',
				created: '2020-01-01T00:00:00.000Z'
			});
			const zulu = createMCPCatalogEntry({
				id: 'vmcp-z',
				name: 'Zulu Gateway',
				runtime: 'composite',
				created: '2024-06-01T00:00:00.000Z',
				manifest: {
					compositeConfig: { componentServers: [{ catalogEntryID: componentEntry.id }] }
				}
			});
			await renderVMcpsPage(createVMcp(), [zulu, bravo]);

			async function editOrder() {
				const els = await page.getByRole('button', { name: /^Edit .+ Gateway$/ }).elements();
				return els.map((el) => el.getAttribute('aria-label'));
			}

			await vi.waitFor(async () => {
				expect(await editOrder()).toEqual(['Edit Bravo Gateway', 'Edit Zulu Gateway']);
			});

			await openSettings();
			await page.getByRole('combobox', { name: 'Sort by' }).click();
			await page.getByRole('button', { name: 'Created', exact: true }).click();
			await tick();

			await vi.waitFor(async () => {
				expect(await editOrder()).toEqual(['Edit Zulu Gateway', 'Edit Bravo Gateway']);
			});

			await page.getByRole('combobox', { name: 'Sort by' }).click();
			await page.getByRole('button', { name: 'Number of servers', exact: true }).click();
			await tick();

			await vi.waitFor(async () => {
				expect(await editOrder()).toEqual(['Edit Zulu Gateway', 'Edit Bravo Gateway']);
			});
		});

		it('reduces the canvas to composites that match any selected filter', async () => {
			const owned = createMCPCatalogEntry({
				id: 'vmcp-owned',
				name: 'Owned Gateway',
				runtime: 'composite',
				powerUserID: 'user-1',
				powerUserWorkspaceID: 'ws-1'
			});
			const other = createMCPCatalogEntry({
				id: 'vmcp-other',
				name: 'Other Gateway',
				runtime: 'composite'
			});
			await renderVMcpsPage(createVMcp(), [owned, other]);

			function onCanvas(name: string) {
				return page.getByRole('button', { name: `Edit ${name}` });
			}

			await expect.element(onCanvas('Issue Tracker vMCP')).toBeVisible();
			await expect.element(onCanvas('Other Gateway')).toBeVisible();
			await expect.element(onCanvas('Owned Gateway')).not.toBeInTheDocument();

			await openSettings();
			await expect
				.element(page.getByRole('combobox', { name: 'By Owner' }))
				.not.toBeInTheDocument();

			const showAll = await showAllToggle().element();
			if (!(showAll instanceof HTMLElement)) {
				throw new Error('Expected the show-all toggle to be an HTMLElement');
			}
			showAll.click();
			await tick();

			await expect.element(onCanvas('Owned Gateway')).toBeVisible();

			await page.getByRole('combobox', { name: 'By Name' }).click();
			await page
				.getByCSS('#vmcp-filter-by-name-popover')
				.getByRole('button', { name: 'Issue Tracker vMCP', exact: true })
				.click();
			await tick();

			await expect
				.element(page.getByRole('button', { name: 'Remove Issue Tracker vMCP' }))
				.toBeVisible();
			await expect.element(onCanvas('Issue Tracker vMCP')).toBeVisible();
			await expect.element(onCanvas('Other Gateway')).not.toBeInTheDocument();
			await expect.element(onCanvas('Owned Gateway')).not.toBeInTheDocument();

			await page.getByRole('combobox', { name: 'By Owner' }).click();
			await page
				.getByCSS('#vmcp-filter-by-owner-popover')
				.getByRole('button', { name: 'user-1', exact: true })
				.click();
			await tick();

			await expect.element(page.getByRole('button', { name: 'Remove user-1' })).toBeVisible();
			await expect.element(onCanvas('Issue Tracker vMCP')).toBeVisible();
			await expect.element(onCanvas('Owned Gateway')).toBeVisible();
			await expect.element(onCanvas('Other Gateway')).not.toBeInTheDocument();

			await page.getByRole('button', { name: 'Remove Issue Tracker vMCP' }).click();
			await tick();

			await expect
				.element(page.getByRole('button', { name: 'Remove Issue Tracker vMCP' }))
				.not.toBeInTheDocument();
			await expect.element(onCanvas('Issue Tracker vMCP')).not.toBeInTheDocument();
			await expect.element(onCanvas('Owned Gateway')).toBeVisible();
		});
	});

	describe('vMCP card click target', () => {
		function editDialog() {
			return page.getByRole('heading', { name: 'Edit Issue Tracker vMCP' });
		}

		function editButton() {
			return page.getByRole('button', { name: 'Edit Issue Tracker vMCP' });
		}

		it('edits the vMCP when the click lands on the card body rather than its title', async () => {
			await renderVMcpsPage(createVMcp());

			const { x, y } = centerOf(await page.getByText(/2\/2 selected/).element());
			// The card-wide hit area is the Edit button stretched over the card, so a press on the
			// body belongs to that button. Playwright refuses to click the text it covers, so hit
			// test the point the user presses and click what is actually on top of it.
			const target = document.elementFromPoint(x, y);
			expect(target).toBe(await editButton().element());

			(target as HTMLElement).click();

			await expect.element(editDialog()).toBeVisible();
		});

		it('leaves the controls on the card to their own actions', async () => {
			await renderVMcpsPage(createVMcp());

			await page.getByRole('button', { name: 'Connect' }).click();

			await expect.element(editDialog()).not.toBeInTheDocument();
		});

		it('keeps the row actions inside the card when the name is too long to fit', async () => {
			const vmcp = createVMcp();
			vmcp.manifest.name = 'Brave Search, Asana, Calendar, Everything, and a few more servers';
			await renderVMcpsPage(vmcp);

			const edit = (await page
				.getByRole('button', { name: `Edit ${vmcp.manifest.name}` })
				.element()) as HTMLElement;
			const actions = await page.getByRole('button', { name: 'Row actions' }).first().element();
			// The hit area is pinned to the card, so the button's offset parent is the card box.
			const card = edit.offsetParent as HTMLElement;

			expect(actions.getBoundingClientRect().right).toBeLessThanOrEqual(
				card.getBoundingClientRect().right
			);
		});

		it('edits from the keyboard while keeping the card controls out of the edit button', async () => {
			await renderVMcpsPage(createVMcp());

			const button = await editButton().element();
			const connect = await page.getByRole('button', { name: 'Connect' }).element();
			expect(button.contains(connect)).toBe(false);

			(button as HTMLElement).focus();
			await userEvent.keyboard('{Enter}');

			await expect.element(editDialog()).toBeVisible();
		});
	});

	describe('whiteboard canvas', () => {
		it('expands the first five connectors by default', async () => {
			await renderVMcpsPage(createVMcp());

			await expect.element(componentBlock()).toBeVisible();
			await expect
				.element(page.getByRole('button', { name: 'Hide servers in Issue Tracker vMCP' }))
				.toHaveAttribute('aria-expanded', 'true');
		});

		it('lets the user expand more than one connector', async () => {
			const extras = [2, 3, 4, 5, 6].map((n) =>
				createMCPCatalogEntry({
					id: `vmcp-${n}`,
					name: `vMCP ${n}`,
					runtime: 'composite',
					manifest: {
						compositeConfig: {
							componentServers: [
								{
									catalogEntryID: componentEntry.id,
									manifest: componentEntry.manifest
								}
							]
						}
					}
				})
			);
			await renderVMcpsPage(createVMcp(), extras);

			await expect
				.element(page.getByRole('button', { name: 'Hide servers in Issue Tracker vMCP' }))
				.toBeVisible();
			await expect
				.element(page.getByRole('button', { name: 'Hide servers in vMCP 2' }))
				.toBeVisible();
			await expect
				.element(page.getByRole('button', { name: 'Hide servers in vMCP 3' }))
				.toBeVisible();
			await expect
				.element(page.getByRole('button', { name: 'Hide servers in vMCP 4' }))
				.toBeVisible();
			await expect
				.element(page.getByRole('button', { name: 'Hide servers in vMCP 5' }))
				.toBeVisible();
			await expect
				.element(page.getByRole('button', { name: 'Show 1 server in vMCP 6' }))
				.toBeVisible();

			await expandServers('vMCP 6');

			await expect
				.element(page.getByRole('button', { name: 'Hide servers in vMCP 6' }))
				.toBeVisible();
			await expect
				.element(page.getByRole('button', { name: 'Hide servers in Issue Tracker vMCP' }))
				.toBeVisible();
		});

		it('restores expanded connectors from localStorage', async () => {
			localStorage.setItem('vmcps.expandedIds', JSON.stringify(['vmcp-1', 'vmcp-6']));
			const extras = [2, 3, 4, 5, 6].map((n) =>
				createMCPCatalogEntry({
					id: `vmcp-${n}`,
					name: `vMCP ${n}`,
					runtime: 'composite',
					manifest: {
						compositeConfig: {
							componentServers: [
								{
									catalogEntryID: componentEntry.id,
									manifest: componentEntry.manifest
								}
							]
						}
					}
				})
			);
			await renderVMcpsPage(createVMcp(), extras);

			await expect
				.element(page.getByRole('button', { name: 'Hide servers in Issue Tracker vMCP' }))
				.toBeVisible();
			await expect
				.element(page.getByRole('button', { name: 'Show 1 server in vMCP 2' }))
				.toBeVisible();
			await expect
				.element(page.getByRole('button', { name: 'Hide servers in vMCP 6' }))
				.toBeVisible();
		});

		it('zooms the world from the toolbar', async () => {
			await renderVMcpsPage(createVMcp());
			const world = page.getByCSS('[data-vmcp-world]');
			await expect.element(world).toBeInTheDocument();
			const before = (await world.element()).getAttribute('style') ?? '';

			await page.getByRole('button', { name: 'Zoom in' }).click();
			await tick();

			const after = (await world.element()).getAttribute('style') ?? '';
			expect(after).not.toBe(before);
			expect(after).toContain('scale(');
		});

		it('does not pan when dragging from a vMCP card', async () => {
			await renderVMcpsPage(createVMcp());
			const world = await page.getByCSS('[data-vmcp-world]').element();
			const before = world.getAttribute('style');
			const card = await page.getByRole('button', { name: 'Edit Issue Tracker vMCP' }).element();
			card.dispatchEvent(
				new PointerEvent('pointerdown', {
					bubbles: true,
					cancelable: true,
					clientX: 40,
					clientY: 40,
					button: 0,
					pointerId: 7
				})
			);
			card.dispatchEvent(
				new PointerEvent('pointermove', {
					bubbles: true,
					cancelable: true,
					clientX: 120,
					clientY: 90,
					button: 0,
					pointerId: 7
				})
			);
			await tick();
			expect(world.getAttribute('style')).toBe(before);
		});
	});
});
