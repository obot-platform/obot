import {
	claimToolSetupForVMcp,
	queueToolSetupForCreatedVMcp
} from '$lib/runes/vmcps/vmcpToolFlow.svelte';
import type { MCPCatalogEntry, ToolOverride, VMCP, VMCPManifest } from '$lib/services';
import { mcpServersAndEntries } from '$lib/stores';
import { createMCPCatalogEntry, createVMCP } from '../../../tests/helpers/mcp';
import { preparePageData } from '../../../tests/helpers/pageData';
import { worker } from '../../../tests/mocks/worker';
import VMcpDesigner from './VMcpDesigner.svelte';
import { http, HttpResponse } from 'msw';
import { tick } from 'svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
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

function createIssueTrackerVMcp(overrides?: ToolOverride[]) {
	return createVMCP(
		{
			id: 'vmcp-1',
			displayName: 'Issue Tracker vMCP',
			components: [
				{
					id: `component-${componentEntry.id}`,
					name: componentEntry.manifest.name ?? componentEntry.id,
					mcpCatalogID: 'default',
					mcpServerCatalogEntryID: componentEntry.id,
					catalogEntry: { manifest: componentEntry.manifest },
					toolPrefix: 'github_',
					...(overrides ? { toolOverrides: overrides } : {})
				}
			]
		},
		[componentEntry]
	);
}

async function renderDesigner(entries: MCPCatalogEntry[], vmcp?: VMCP) {
	mcpServersAndEntries.current = {
		entries,
		servers: [],
		userInstances: [],
		userConfiguredServers: [],
		loading: false,
		lastFetched: null,
		isInitialized: true
	};
	await preparePageData();
	return render(VMcpDesigner, vmcp ? { vmcp } : {});
}

function componentBlock() {
	return page.getByRole('button', { name: componentEntry.manifest.name!, exact: true });
}

function mockUpdateVMcp(vmcp: VMCP, onUpdate: (manifest: unknown) => void) {
	worker.use(
		http.get(`/api/vmcps/${vmcp.id}`, () => HttpResponse.json(vmcp)),
		http.put(`/api/vmcps/${vmcp.id}`, async ({ request }) => {
			const manifest = (await request.json()) as VMCPManifest;
			onUpdate(manifest);
			return HttpResponse.json({ ...vmcp, ...manifest });
		})
	);
}

function componentsFrom(manifest: unknown) {
	return (manifest as VMCPManifest).components ?? [];
}

function mockEntryDetails(entry: MCPCatalogEntry) {
	const listServers = vi.fn();
	worker.use(
		http.get(`/api/mcp-catalogs/default/entries/${entry.id}`, () => HttpResponse.json(entry)),
		http.get(`/api/mcp-catalogs/default/entries/${entry.id}/servers`, () => {
			listServers();
			return HttpResponse.json({ items: [] });
		})
	);
	return listServers;
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
	el.setPointerCapture = () => {};

	const from = centerOf(el);
	pointer(el, 'pointerdown', pointerId, from);
	return { el, from };
}

describe('VMcpDesigner.svelte', () => {
	describe('component with stored tool overrides', () => {
		it('edits the stored overrides instead of running the tool setup flow', async () => {
			const vmcp = createIssueTrackerVMcp(toolOverrides);
			await renderDesigner([componentEntry], vmcp);

			await componentBlock().click();

			await expect.element(page.getByText('Configure GitHub Tools')).toBeVisible();
			await expect.element(page.getByText('create_issue').first()).toBeVisible();
			await expect.element(page.getByText('list_issues').first()).toBeVisible();
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
			const vmcp = createIssueTrackerVMcp(toolOverrides);
			const update = vi.fn();
			mockUpdateVMcp(vmcp, update);

			await renderDesigner([componentEntry], vmcp);
			await componentBlock().click();

			await page.getByRole('checkbox', { name: 'Enabled' }).nth(1).click();
			await page.getByRole('button', { name: 'Confirm' }).click();

			await vi.waitFor(() => expect(update).toHaveBeenCalled());
			expect(componentsFrom(update.mock.calls[0][0])[0]).toMatchObject({
				mcpServerCatalogEntryID: componentEntry.id,
				toolPrefix: 'github_',
				toolOverrides: [
					{ name: 'create_issue', enabled: true },
					{ name: 'list_issues', enabled: true }
				]
			});
		});

		it('refreshes tools from the server through the setup flow', async () => {
			const vmcp = createIssueTrackerVMcp(toolOverrides);
			await renderDesigner([componentEntry], vmcp);

			await componentBlock().click();
			await page.getByRole('button', { name: 'Refresh' }).click();

			await expect
				.element(page.getByRole('button', { name: 'Get Started', exact: true }))
				.toBeVisible();
		});
	});

	describe('component without stored tool overrides', () => {
		it('offers modifying tools or deleting the server', async () => {
			const vmcp = createIssueTrackerVMcp();
			await renderDesigner([componentEntry], vmcp);

			await componentBlock().click();

			await expect.element(page.getByRole('button', { name: 'Modify Tools' })).toBeVisible();
			await expect.element(page.getByRole('button', { name: 'Delete MCP Server' })).toBeVisible();
			await expect
				.element(page.getByRole('button', { name: 'Get Started', exact: true }))
				.not.toBeInTheDocument();
		});

		it('starts the tool setup flow from Modify Tools', async () => {
			const vmcp = createIssueTrackerVMcp();
			await renderDesigner([componentEntry], vmcp);

			await componentBlock().click();
			await page.getByRole('button', { name: 'Modify Tools' }).click();

			await expect
				.element(page.getByRole('button', { name: 'Get Started', exact: true }))
				.toBeVisible();
		});

		it('removes the server from the vMCP without visiting the setup flow', async () => {
			const vmcp = createIssueTrackerVMcp();
			const update = vi.fn();
			mockUpdateVMcp(vmcp, update);

			await renderDesigner([componentEntry], vmcp);
			await componentBlock().click();
			await page.getByRole('button', { name: 'Delete MCP Server' }).click();

			await expect.element(page.getByText('Confirm Remove')).toBeVisible();
			await page.getByRole('button', { name: "Yes, I'm sure" }).click();

			await vi.waitFor(() => expect(update).toHaveBeenCalled());
			expect(componentsFrom(update.mock.calls[0][0])).toEqual([]);
		});
	});

	describe('dragging a server from the panel onto the canvas', () => {
		const slack = createMCPCatalogEntry({ id: 'entry-slack', name: 'Slack' });
		let listSlackServers: ReturnType<typeof mockEntryDetails>;

		beforeEach(() => {
			listSlackServers = mockEntryDetails(slack);
		});

		function vmcpCard() {
			return page.getByRole('button', { name: 'Edit Issue Tracker vMCP' });
		}

		async function dragSlackOnto(target: Element, pointerId: number) {
			const { el } = await pressCard(panelCard('Slack'), pointerId);
			const to = centerOf(target);
			pointer(el, 'pointermove', pointerId, to);
			await tick();
			return { el, to };
		}

		async function dragSlackOntoVMcp(pointerId: number) {
			return dragSlackOnto(await vmcpCard().element(), pointerId);
		}

		it('marks both the dragged card and the vMCP it is linked to', async () => {
			const vmcp = createIssueTrackerVMcp();
			await renderDesigner([componentEntry, slack], vmcp);

			await dragSlackOntoVMcp(12);

			await expect.element(panelCard('Slack')).toHaveClass(/opacity-30/);
			await expect.element(page.getByCSS('.vmcp-drop-target').first()).toBeInTheDocument();
		});

		it('adds the dropped server to the vMCP it landed on', async () => {
			const vmcp = createIssueTrackerVMcp();
			const update = vi.fn();
			mockUpdateVMcp(vmcp, update);
			await renderDesigner([componentEntry, slack], vmcp);

			const { el, to } = await dragSlackOntoVMcp(13);
			pointer(el, 'pointerup', 13, to);

			await vi.waitFor(() => expect(update).toHaveBeenCalled());
			expect(componentsFrom(update.mock.calls[0][0]).at(-1)).toMatchObject({
				mcpServerCatalogEntryID: slack.id,
				name: slack.manifest.name
			});
		});

		it('collects configuration policies before adding a server with config', async () => {
			const tokenSlack = createMCPCatalogEntry({
				id: 'entry-slack-token',
				name: 'Token Slack',
				manifest: {
					config: [
						{
							key: 'API_TOKEN',
							name: 'API token',
							description: 'Token',
							required: true,
							sensitive: true,
							value: '',
							usage: 'env'
						}
					]
				}
			});
			mockEntryDetails(tokenSlack);
			const vmcp = createIssueTrackerVMcp();
			const update = vi.fn();
			mockUpdateVMcp(vmcp, update);
			await renderDesigner([componentEntry, tokenSlack], vmcp);

			const { el } = await pressCard(panelCard('Token Slack'), 18);
			const to = centerOf(await vmcpCard().element());
			pointer(el, 'pointermove', 18, to);
			await tick();
			pointer(el, 'pointerup', 18, to);

			await expect.element(page.getByText('Set configuration policy')).toBeVisible();
			expect(update).not.toHaveBeenCalled();

			await page.getByRole('radio', { name: 'User-supplied' }).click();
			await page.getByRole('button', { name: 'Next' }).click();

			await vi.waitFor(() => expect(update).toHaveBeenCalled());
			expect(componentsFrom(update.mock.calls[0][0]).at(-1)).toMatchObject({
				mcpServerCatalogEntryID: tokenSlack.id,
				name: tokenSlack.manifest.name,
				configuration: [{ key: 'API_TOKEN', policy: 'userAllowed' }]
			});
		});

		it('adds the dropped server when the pointer is anywhere on the canvas', async () => {
			const vmcp = createIssueTrackerVMcp();
			const update = vi.fn();
			mockUpdateVMcp(vmcp, update);
			await renderDesigner([componentEntry, slack], vmcp);

			const canvas = await page.getByCSS('[data-vmcp-canvas]').element();
			const rect = canvas.getBoundingClientRect();
			const { el } = await pressCard(panelCard('Slack'), 16);
			const to = { x: rect.right - 16, y: rect.bottom - 16 };
			pointer(el, 'pointermove', 16, to);
			await tick();
			pointer(el, 'pointerup', 16, to);

			await vi.waitFor(() => expect(update).toHaveBeenCalled());
			expect(componentsFrom(update.mock.calls[0][0]).at(-1)).toMatchObject({
				mcpServerCatalogEntryID: slack.id,
				name: slack.manifest.name
			});
		});

		it('creates a vMCP from a drop anywhere on the empty canvas', async () => {
			await renderDesigner([componentEntry, slack]);

			const canvas = await page.getByCSS('[data-vmcp-canvas]').element();
			const rect = canvas.getBoundingClientRect();
			const { el } = await pressCard(panelCard('Slack'), 17);
			const to = { x: rect.left + 16, y: rect.top + 16 };
			pointer(el, 'pointermove', 17, to);
			await tick();
			pointer(el, 'pointerup', 17, to);

			await expect.element(page.getByRole('dialog').getByText('Create vMCP').first()).toBeVisible();
		});

		it('leaves the vMCP alone when Escape cancels the drag before release', async () => {
			const vmcp = createIssueTrackerVMcp();
			const update = vi.fn();
			mockUpdateVMcp(vmcp, update);
			await renderDesigner([componentEntry, slack], vmcp);

			const { el, to } = await dragSlackOntoVMcp(14);
			await userEvent.keyboard('{Escape}');
			pointer(el, 'pointerup', 14, to);
			await tick();

			expect(update).not.toHaveBeenCalled();
			await expect.element(page.getByCSS('.vmcp-drop-target')).not.toBeInTheDocument();
		});

		it('opens the server details when the press never travels far enough to drag', async () => {
			const vmcp = createIssueTrackerVMcp();
			await renderDesigner([componentEntry, slack], vmcp);

			const { el, from } = await pressCard(panelCard('Slack'), 15);
			pointer(el, 'pointerup', 15, from);

			await expect.element(page.getByRole('dialog').first()).toBeVisible();
			await vi.waitFor(() => expect(listSlackServers).toHaveBeenCalled());
		});
	});

	describe('graph canvas', () => {
		it('draws the vMCP with its servers expanded', async () => {
			const vmcp = createIssueTrackerVMcp();
			await renderDesigner([componentEntry], vmcp);

			await expect.element(componentBlock()).toBeVisible();
			await expect
				.element(page.getByRole('button', { name: 'Hide servers in Issue Tracker vMCP' }))
				.toHaveAttribute('aria-expanded', 'true');
		});

		it('collapses and re-expands the servers of the selected vMCP', async () => {
			const vmcp = createIssueTrackerVMcp();
			await renderDesigner([componentEntry], vmcp);

			await page.getByRole('button', { name: 'Hide servers in Issue Tracker vMCP' }).click();
			await tick();

			await expect.element(componentBlock()).not.toBeInTheDocument();

			await page.getByRole('button', { name: 'Show 1 server in Issue Tracker vMCP' }).click();
			await tick();

			await expect.element(componentBlock()).toBeVisible();
		});

		it('offers vMCP creation on the canvas while nothing is selected', async () => {
			await renderDesigner([componentEntry]);

			await expect.element(page.getByRole('button', { name: /Create New vMCP/ })).toBeVisible();
			await expect.element(page.getByCSS('[data-vmcp-world]')).not.toBeInTheDocument();
			await expect
				.element(page.getByRole('button', { name: 'Edit Issue Tracker vMCP' }))
				.not.toBeInTheDocument();
		});

		it('offers deleting the selected vMCP from the canvas toolbar', async () => {
			const vmcp = createIssueTrackerVMcp();
			await renderDesigner([componentEntry], vmcp);

			await page.getByRole('button', { name: 'Delete vMCP' }).click();

			await expect.element(page.getByText(/Are you sure you want to delete/)).toBeVisible();
		});

		it('offers audit, usage, and delete actions from the vMCP card menu', async () => {
			const vmcp = createIssueTrackerVMcp();
			await renderDesigner([componentEntry], vmcp);

			await page.getByRole('button', { name: 'Actions for Issue Tracker vMCP' }).click();

			const auditLogs = page.getByRole('link', { name: 'View Audit Logs' });
			const usage = page.getByRole('link', { name: 'View Usage' });
			await expect.element(auditLogs).toBeVisible();
			await expect.element(usage).toBeVisible();
			await expect.element(auditLogs).toHaveAttribute('href', `/audit-logs?mcp_id=${vmcp.id}`);
			await expect.element(usage).toHaveAttribute('href', `/usage?mcp_id=${vmcp.id}`);

			await page.getByRole('button', { name: 'Delete', exact: true }).click();
			await expect.element(page.getByText(/Are you sure you want to delete/)).toBeVisible();
		});

		it('zooms the world from the toolbar', async () => {
			const vmcp = createIssueTrackerVMcp();
			await renderDesigner([componentEntry], vmcp);
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
			const vmcp = createIssueTrackerVMcp();
			await renderDesigner([componentEntry], vmcp);
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

	describe('tool setup handed over from vMCP creation', () => {
		function addToolsDialog() {
			return page.getByRole('button', { name: 'Select Which Tools To Enable' });
		}

		it('opens the tool dialog on the page the new vMCP navigated to', async () => {
			const vmcp = createIssueTrackerVMcp();
			queueToolSetupForCreatedVMcp(vmcp.id);

			await renderDesigner([componentEntry], vmcp);

			await expect.element(addToolsDialog()).toBeVisible();
			await expect.element(page.getByRole('button', { name: 'Add All Tools' })).toBeVisible();
		});

		it('leaves an existing vMCP alone when nothing was queued', async () => {
			const vmcp = createIssueTrackerVMcp();
			await renderDesigner([componentEntry], vmcp);

			await expect.element(addToolsDialog()).not.toBeInTheDocument();
		});

		it('hands the queued setup over only once, so later visits stay quiet', async () => {
			const vmcp = createIssueTrackerVMcp();
			queueToolSetupForCreatedVMcp(vmcp.id);

			await renderDesigner([componentEntry], vmcp);
			await expect.element(addToolsDialog()).toBeVisible();

			expect(claimToolSetupForVMcp(vmcp.id)).toBe(false);
		});
	});
});
