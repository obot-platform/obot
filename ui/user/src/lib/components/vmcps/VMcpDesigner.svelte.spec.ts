import { page as appPage } from '$app/state';
import {
	claimProfilesHintForVMcp,
	claimToolSetupForVMcp,
	finishVMcpCreateHandoff,
	queueProfilesHintForCreatedVMcp,
	queueToolSetupForCreatedVMcp,
	VMCP_PROFILES_HINT_STORAGE_KEY
} from '$lib/runes/vmcps/vmcpToolFlow.svelte';
import {
	Group,
	type MCPCatalogEntry,
	type ToolOverride,
	type VMCP,
	type VMCPManifest
} from '$lib/services';
import { catalogEntryToVMCPComponent } from '$lib/services/vmcps/utils';
import { mcpServersAndEntries } from '$lib/stores';
import { createMCPCatalogEntry, createVMCP, createVMCPComponent } from '../../../tests/helpers/mcp';
import { createMockProfile, preparePageData } from '../../../tests/helpers/pageData';
import { getProfileResponse } from '../../../tests/mocks/data';
import { worker } from '../../../tests/mocks/worker';
import VMcpDesigner from './VMcpDesigner.svelte';
import { http, HttpResponse } from 'msw';
import { tick } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
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

async function renderDesigner(
	entries: MCPCatalogEntry[],
	vmcp?: VMCP,
	options?: { groups?: string[]; onBack?: () => void }
) {
	mcpServersAndEntries.current = {
		entries,
		servers: [],
		userInstances: [],
		userConfiguredServers: [],
		loading: false,
		lastFetched: null,
		isInitialized: true
	};
	await preparePageData({
		profile: createMockProfile(options?.groups ?? [Group.ADMIN])
	});
	return render(VMcpDesigner, {
		...(vmcp ? { vmcp } : {}),
		...(options?.onBack ? { onBack: options.onBack } : {})
	});
}

function componentBlock() {
	return page.getByRole('button', { name: componentEntry.manifest.name!, exact: true });
}

async function chooseModifyTools() {
	await expect.element(page.getByRole('button', { name: 'Modify Tools' })).toBeVisible();
	await page.getByRole('button', { name: 'Modify Tools' }).click();
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
	afterEach(() => {
		appPage.url.searchParams.delete('view');
		finishVMcpCreateHandoff();
	});

	describe('component with stored tool overrides', () => {
		it('opens the actions dialog instead of the tool editor', async () => {
			const vmcp = createIssueTrackerVMcp(toolOverrides);
			await renderDesigner([componentEntry], vmcp);

			await componentBlock().click();

			const actions = page.getByCSS('dialog[open]');
			await expect.element(actions.getByRole('button', { name: 'Modify Tools' })).toBeVisible();
			await expect.element(actions.getByRole('button', { name: 'Remove GitHub' })).toBeVisible();
			await expect
				.element(actions.getByRole('heading', { name: 'Configure GitHub Tools' }))
				.not.toBeInTheDocument();
		});

		it('edits the stored overrides from Modify Tools instead of running setup', async () => {
			const vmcp = createIssueTrackerVMcp(toolOverrides);
			await renderDesigner([componentEntry], vmcp);

			await componentBlock().click();
			await chooseModifyTools();

			await expect
				.element(page.getByRole('heading', { name: 'Configure GitHub Tools' }))
				.toBeVisible();
			await expect.element(page.getByText('create_issue').first()).toBeVisible();
			await expect.element(page.getByText('list_issues').first()).toBeVisible();
			await expect
				.element(page.getByCSS('dialog[open] input[placeholder="No prefix"]'))
				.toHaveValue('github_');
			await expect.element(page.getByRole('button', { name: 'Refresh tools' })).toBeVisible();
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
			await chooseModifyTools();

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
			await chooseModifyTools();
			await page.getByRole('button', { name: 'Refresh tools' }).click();

			await expect
				.element(page.getByRole('button', { name: 'Configure Tools', exact: true }))
				.toBeVisible();
		});
	});

	describe('component without stored tool overrides', () => {
		it('offers modifying tools or deleting the server', async () => {
			const vmcp = createIssueTrackerVMcp();
			await renderDesigner([componentEntry], vmcp);

			await componentBlock().click();

			await expect.element(page.getByRole('button', { name: 'Modify Tools' })).toBeVisible();
			await expect.element(page.getByRole('button', { name: 'Remove GitHub' })).toBeVisible();
			await expect.element(page.getByRole('button', { name: 'Modify Tools' })).toBeEnabled();
			await expect
				.element(page.getByRole('button', { name: 'Change Configuration' }))
				.toBeVisible();
			await expect
				.element(page.getByRole('button', { name: 'Get Started', exact: true }))
				.not.toBeInTheDocument();
		});

		it('disables Modify Tools when the server has user-supplied configuration', async () => {
			const slack = createMCPCatalogEntry({ id: 'entry-slack', name: 'Slack' });
			const vmcp = createVMCP(
				{
					id: 'vmcp-1',
					displayName: 'Issue Tracker vMCP',
					components: [
						createVMCPComponent(componentEntry, {
							toolPrefix: 'github_',
							configuration: [{ key: 'API_TOKEN', policy: 'userAllowed' }]
						}),
						createVMCPComponent(slack)
					]
				},
				[componentEntry, slack]
			);
			await renderDesigner([componentEntry, slack], vmcp);

			await componentBlock().click();

			const modify = page.getByRole('button', { name: 'Modify Tools' });
			await expect.element(modify).toBeDisabled();
			await expect.element(page.getByRole('button', { name: 'Remove GitHub' })).toBeEnabled();

			const trigger = (await modify.element()).parentElement;
			trigger?.dispatchEvent(new MouseEvent('mouseenter', { bubbles: true }));
			await expect
				.element(page.getByRole('tooltip'))
				.toHaveTextContent(
					"Tools can't be modified because this server has user-supplied configuration."
				);
		});

		it('edits existing configuration policies from the actions dialog', async () => {
			const entry = createMCPCatalogEntry({
				id: 'entry-github',
				name: 'GitHub',
				manifest: {
					toolPreview: componentEntry.manifest.toolPreview,
					config: [
						{
							key: 'API_TOKEN',
							name: 'API token',
							description: 'Token',
							required: true,
							sensitive: false,
							value: '',
							usage: 'env'
						}
					]
				}
			});
			const vmcp = createVMCP(
				{
					id: 'vmcp-1',
					displayName: 'Issue Tracker vMCP',
					components: [
						{
							id: `component-${entry.id}`,
							name: entry.manifest.name ?? entry.id,
							mcpCatalogID: 'default',
							mcpServerCatalogEntryID: entry.id,
							catalogEntry: { manifest: entry.manifest },
							toolPrefix: 'github_',
							configuration: [{ key: 'API_TOKEN', policy: 'userAllowed' }]
						}
					]
				},
				[entry]
			);
			const update = vi.fn();
			mockUpdateVMcp(vmcp, update);
			await renderDesigner([entry], vmcp);

			await componentBlock().click();
			await page.getByRole('button', { name: 'Change Configuration' }).click();
			await expect
				.element(page.getByRole('combobox', { name: 'API token policy' }))
				.toHaveValue('userAllowed');

			await page.getByRole('button', { name: 'Cancel' }).click();
			await expect.element(page.getByRole('button', { name: 'Modify Tools' })).toBeVisible();

			await page.getByRole('button', { name: 'Change Configuration' }).click();
			await page.getByRole('combobox', { name: 'API token policy' }).selectOptions('Preconfigured');
			await page.getByCSS('#fixed-API_TOKEN').fill('secret');
			await page.getByRole('checkbox', { name: 'Force single-user' }).click();
			await page.getByRole('button', { name: 'Save' }).click();

			await vi.waitFor(() => expect(update).toHaveBeenCalled());
			expect(componentsFrom(update.mock.calls[0][0])[0]).toMatchObject({
				configuration: [{ key: 'API_TOKEN', policy: 'fixed', value: 'secret' }],
				forceSingleUser: true
			});
		});

		it('starts the tool setup flow from Modify Tools', async () => {
			const vmcp = createIssueTrackerVMcp();
			await renderDesigner([componentEntry], vmcp);

			await componentBlock().click();
			await page.getByRole('button', { name: 'Modify Tools' }).click();

			await expect
				.element(page.getByRole('button', { name: 'Configure Tools', exact: true }))
				.toBeVisible();
		});

		it('removes the server from the vMCP without visiting the setup flow', async () => {
			const slack = createMCPCatalogEntry({ id: 'entry-slack', name: 'Slack' });
			const vmcp = createVMCP(
				{
					id: 'vmcp-1',
					displayName: 'Issue Tracker vMCP',
					components: [
						createVMCPComponent(componentEntry, { toolPrefix: 'github_' }),
						createVMCPComponent(slack)
					]
				},
				[componentEntry, slack]
			);
			const update = vi.fn();
			mockUpdateVMcp(vmcp, update);

			await renderDesigner([componentEntry, slack], vmcp);
			await componentBlock().click();
			await page.getByRole('button', { name: 'Remove GitHub' }).click();

			await expect.element(page.getByText('Confirm Remove')).toBeVisible();
			await page.getByRole('button', { name: "Yes, I'm sure" }).click();

			await vi.waitFor(() => expect(update).toHaveBeenCalled());
			expect(componentsFrom(update.mock.calls[0][0])).toEqual([
				expect.objectContaining({ mcpServerCatalogEntryID: slack.id, name: 'Slack' })
			]);
		});

		it('disables Remove when the vMCP has only one server', async () => {
			const vmcp = createIssueTrackerVMcp();
			const update = vi.fn();
			mockUpdateVMcp(vmcp, update);

			await renderDesigner([componentEntry], vmcp);
			await componentBlock().click();

			const remove = page.getByRole('button', { name: 'Remove GitHub' });
			await expect.element(remove).toBeDisabled();
			await expect.element(page.getByText('Confirm Remove')).not.toBeVisible();

			const trigger = (await remove.element()).parentElement;
			trigger?.dispatchEvent(new MouseEvent('mouseenter', { bubbles: true }));
			await expect
				.element(page.getByRole('tooltip'))
				.toHaveTextContent('VMCP requires at least one component.');

			expect(update).not.toHaveBeenCalled();
			await expect.element(componentBlock()).toBeVisible();
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

			await expect
				.element(page.getByRole('heading', { name: /Configure Token Slack/ }))
				.toBeVisible();
			expect(update).not.toHaveBeenCalled();

			await page
				.getByRole('combobox', { name: 'API token policy' })
				.selectOptions('Provided at connection');
			await page.getByRole('button', { name: 'Next' }).click();

			await vi.waitFor(() => expect(update).toHaveBeenCalled());
			expect(componentsFrom(update.mock.calls[0][0]).at(-1)).toMatchObject({
				mcpServerCatalogEntryID: tokenSlack.id,
				name: tokenSlack.manifest.name,
				configuration: [{ key: 'API_TOKEN', policy: 'userAllowed' }]
			});
			await expect
				.element(page.getByRole('heading', { name: 'Add Tools' }))
				.not.toBeInTheDocument();
		});

		it('offers tool selection after configuration when no policy is user-supplied', async () => {
			const tokenSlack = createMCPCatalogEntry({
				id: 'entry-slack-fixed',
				name: 'Token Slack',
				manifest: {
					config: [
						{
							key: 'API_TOKEN',
							name: 'API token',
							description: 'Token',
							required: true,
							sensitive: false,
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

			const { el } = await pressCard(panelCard('Token Slack'), 19);
			const to = centerOf(await vmcpCard().element());
			pointer(el, 'pointermove', 19, to);
			await tick();
			pointer(el, 'pointerup', 19, to);

			await expect
				.element(page.getByRole('heading', { name: /Configure Token Slack/ }))
				.toBeVisible();
			await page.getByCSS('#fixed-API_TOKEN').fill('secret');
			await page.getByRole('button', { name: 'Next' }).click();

			await vi.waitFor(() => expect(update).toHaveBeenCalled());
			expect(componentsFrom(update.mock.calls[0][0]).at(-1)).toMatchObject({
				mcpServerCatalogEntryID: tokenSlack.id,
				configuration: [{ key: 'API_TOKEN', policy: 'fixed', value: 'secret' }]
			});
			await expect.element(page.getByRole('heading', { name: 'Add Tools' })).toBeVisible();
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

		it('opens create when a configurable server is dropped on the empty canvas', async () => {
			const tokenSlack = createMCPCatalogEntry({
				id: 'entry-slack-create',
				name: 'Token Slack',
				manifest: {
					config: [
						{
							key: 'API_TOKEN',
							name: 'API token',
							description: 'Token',
							required: true,
							sensitive: false,
							value: '',
							usage: 'env'
						}
					]
				}
			});
			mockEntryDetails(tokenSlack);
			await renderDesigner([componentEntry, tokenSlack]);

			const { el } = await pressCard(panelCard('Token Slack'), 20);
			const to = centerOf(await page.getByRole('button', { name: /Create New vMCP/ }).element());
			pointer(el, 'pointermove', 20, to);
			await tick();
			pointer(el, 'pointerup', 20, to);

			await expect.element(page.getByRole('dialog').getByText('Create vMCP').first()).toBeVisible();
			await expect
				.element(page.getByRole('heading', { name: /Configure Token Slack/ }))
				.not.toBeInTheDocument();
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
		it('draws the vMCP with its servers', async () => {
			const vmcp = createIssueTrackerVMcp();
			await renderDesigner([componentEntry], vmcp);

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

		it('keeps edit controls for a non-admin owner', async () => {
			const vmcp = createIssueTrackerVMcp();
			vmcp.userID = getProfileResponse.id;
			await renderDesigner([componentEntry], vmcp, { groups: [Group.USER] });

			await expect.element(page.getByRole('button', { name: 'Designer' })).not.toBeInTheDocument();
			await expect.element(page.getByRole('button', { name: 'Profiles' })).not.toBeInTheDocument();
			await expect.element(page.getByRole('button', { name: 'Delete vMCP' })).toBeVisible();
			await expect.element(page.getByRole('button', { name: 'Hide MCP Servers' })).toBeVisible();
			await expect
				.element(page.getByRole('button', { name: 'Edit Issue Tracker vMCP' }))
				.toBeVisible();
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

	describe('when the viewer is not an admin or owner', () => {
		const slack = createMCPCatalogEntry({ id: 'entry-slack', name: 'Slack' });

		function sharedVMcp(overrides?: ToolOverride[]) {
			const vmcp = createIssueTrackerVMcp(overrides);
			vmcp.userID = 'someone-else';
			return vmcp;
		}

		it('hides delete, profile tabs, and the MCP Servers sidebar', async () => {
			await renderDesigner([componentEntry, slack], sharedVMcp(), { groups: [Group.USER] });

			await expect
				.element(page.getByRole('button', { name: 'Delete vMCP' }))
				.not.toBeInTheDocument();
			await expect.element(page.getByRole('button', { name: 'Designer' })).not.toBeInTheDocument();
			await expect.element(page.getByRole('button', { name: 'Profiles' })).not.toBeInTheDocument();
			await expect
				.element(page.getByRole('button', { name: 'Hide MCP Servers' }))
				.not.toBeInTheDocument();
			await expect.element(page.getByPlaceholder('Search MCP servers...')).not.toBeInTheDocument();
			await expect
				.element(page.getByRole('button', { name: 'Connect', exact: true }))
				.toBeVisible();
		});

		it('does not open edit or component actions, and omits delete from the card menu', async () => {
			await renderDesigner([componentEntry], sharedVMcp(), { groups: [Group.USER] });

			// Viewers cannot select the card, so the title is not a button.
			await page
				.getByCSS('[data-vmcp-canvas]')
				.getByText('Issue Tracker vMCP', { exact: true })
				.click();
			await expect.element(page.getByText('Edit vMCP')).not.toBeInTheDocument();

			await expect.element(componentBlock()).not.toBeInTheDocument();
			await expect.element(page.getByText('GitHub').first()).toBeVisible();
			await expect
				.element(page.getByRole('button', { name: 'Modify Tools' }))
				.not.toBeInTheDocument();

			await page.getByRole('button', { name: 'Actions for Issue Tracker vMCP' }).click();
			await expect.element(page.getByRole('link', { name: 'View Audit Logs' })).toBeVisible();
			await expect.element(page.getByRole('link', { name: 'View Usage' })).toBeVisible();
			await expect
				.element(page.getByRole('button', { name: 'Delete', exact: true }))
				.not.toBeInTheDocument();
		});

		it('stays on the graph when the URL asks for profiles', async () => {
			appPage.url.searchParams.set('view', 'profiles');
			await renderDesigner([componentEntry], sharedVMcp(), { groups: [Group.USER] });

			await expect.element(page.getByText('GitHub').first()).toBeVisible();
			await expect
				.element(page.getByRole('button', { name: 'Create profile', exact: true }))
				.not.toBeInTheDocument();
			await expect.element(page.getByRole('button', { name: 'Designer' })).not.toBeInTheDocument();
		});
	});

	describe('back navigation', () => {
		function layoutBackButton() {
			return page.getByRole('button', { name: 'Back', exact: true });
		}

		function profilesBackButton() {
			return page.getByRole('button', { name: 'Back to profiles' });
		}

		it('leaves the designer from the graph', async () => {
			const onBack = vi.fn();
			await renderDesigner([componentEntry], createIssueTrackerVMcp(), { onBack });

			await layoutBackButton().click();
			expect(onBack).toHaveBeenCalledOnce();
		});

		it('leaves the designer from the profiles list', async () => {
			const onBack = vi.fn();
			appPage.url.searchParams.set('view', 'profiles');
			await renderDesigner([componentEntry], createIssueTrackerVMcp(), { onBack });

			await expect.element(page.getByRole('button', { name: 'Create profile' })).toBeVisible();
			await layoutBackButton().click();
			expect(onBack).toHaveBeenCalledOnce();
		});

		it('returns to the profiles list when creating a profile', async () => {
			const onBack = vi.fn();
			appPage.url.searchParams.set('view', 'profiles');
			await renderDesigner([componentEntry], createIssueTrackerVMcp(), { onBack });

			await page.getByRole('button', { name: 'Create profile', exact: true }).click();
			await expect.element(page.getByRole('heading', { name: 'Create profile' })).toBeVisible();

			await profilesBackButton().click();
			await expect
				.element(page.getByRole('heading', { name: 'Create profile' }))
				.not.toBeInTheDocument();
			await expect.element(page.getByRole('button', { name: 'Create profile' })).toBeVisible();
			expect(onBack).not.toHaveBeenCalled();

			await layoutBackButton().click();
			expect(onBack).toHaveBeenCalledOnce();
		});

		it('returns to the profiles list when a profile is selected', async () => {
			const onBack = vi.fn();
			appPage.url.searchParams.set('view', 'profiles');
			await renderDesigner([componentEntry], createIssueTrackerVMcp(), { onBack });

			await page.getByRole('button', { name: 'Edit default' }).click();
			await expect.element(page.getByRole('heading', { name: 'Edit profile' })).toBeVisible();

			await profilesBackButton().click();
			await expect
				.element(page.getByRole('heading', { name: 'Edit profile' }))
				.not.toBeInTheDocument();
			await expect.element(page.getByRole('button', { name: 'Edit default' })).toBeVisible();
			expect(onBack).not.toHaveBeenCalled();

			await layoutBackButton().click();
			expect(onBack).toHaveBeenCalledOnce();
		});
	});

	describe('tool setup handed over from vMCP creation', () => {
		function addToolsDialog() {
			return page.getByRole('heading', { name: 'Add Tools' });
		}

		function configurableGithub() {
			return createMCPCatalogEntry({
				id: 'entry-github',
				name: 'GitHub',
				manifest: {
					toolPreview: componentEntry.manifest.toolPreview,
					config: [
						{
							key: 'API_TOKEN',
							name: 'API token',
							description: 'Token',
							required: true,
							sensitive: false,
							value: '',
							usage: 'env'
						}
					]
				}
			});
		}

		it('opens the tool dialog on the page the new vMCP navigated to', async () => {
			const vmcp = createIssueTrackerVMcp();
			queueToolSetupForCreatedVMcp(vmcp.id);

			await renderDesigner([componentEntry], vmcp);

			await expect.element(addToolsDialog()).toBeVisible();
			await expect.element(page.getByRole('button', { name: /As-is/ })).toBeVisible();
			await expect.element(page.getByRole('button', { name: /Managed/ })).toBeVisible();
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

		it('opens required configuration on the page the new vMCP navigated to', async () => {
			const entry = configurableGithub();
			const vmcp = createVMCP(
				{
					id: 'vmcp-1',
					displayName: 'Issue Tracker vMCP',
					components: [
						{
							id: `component-${entry.id}`,
							name: entry.manifest.name ?? entry.id,
							mcpCatalogID: 'default',
							mcpServerCatalogEntryID: entry.id,
							catalogEntry: { manifest: entry.manifest }
						}
					]
				},
				[entry]
			);
			queueToolSetupForCreatedVMcp(vmcp.id);

			await renderDesigner([entry], vmcp);

			await expect.element(page.getByRole('heading', { name: /Configure GitHub/ })).toBeVisible();
			await expect.element(page.getByRole('button', { name: 'Next' })).toBeVisible();
			await expect
				.element(page.getByRole('heading', { name: 'Add Tools' }))
				.not.toBeInTheDocument();
		});

		it('opens configuration after creation even when required policies are already fixed', async () => {
			const entry = configurableGithub();
			const vmcp = createVMCP({
				components: [catalogEntryToVMCPComponent(entry)]
			});
			const update = vi.fn();
			mockUpdateVMcp(vmcp, update);
			queueToolSetupForCreatedVMcp(vmcp.id);

			await renderDesigner([entry], vmcp);

			await expect.element(page.getByRole('heading', { name: /Configure GitHub/ })).toBeVisible();
			await expect.element(addToolsDialog()).not.toBeInTheDocument();
			await page.getByCSS('#fixed-API_TOKEN').fill('secret');
			await page.getByRole('button', { name: 'Next' }).click();

			await vi.waitFor(() => expect(update).toHaveBeenCalledOnce());
			expect(componentsFrom(update.mock.calls[0][0])[0]).toMatchObject({
				configuration: [{ key: 'API_TOKEN', policy: 'fixed', value: 'secret' }]
			});
			await expect.element(addToolsDialog()).toBeVisible();
		});

		it('offers tool selection after post-create configuration when no policy is user-supplied', async () => {
			const entry = configurableGithub();
			const vmcp = createVMCP(
				{
					id: 'vmcp-1',
					displayName: 'Issue Tracker vMCP',
					components: [
						{
							id: `component-${entry.id}`,
							name: entry.manifest.name ?? entry.id,
							mcpCatalogID: 'default',
							mcpServerCatalogEntryID: entry.id,
							catalogEntry: { manifest: entry.manifest }
						}
					]
				},
				[entry]
			);
			const update = vi.fn();
			mockUpdateVMcp(vmcp, update);
			queueToolSetupForCreatedVMcp(vmcp.id);

			await renderDesigner([entry], vmcp);
			await page.getByCSS('#fixed-API_TOKEN').fill('secret');
			await page.getByRole('button', { name: 'Next' }).click();

			await vi.waitFor(() => expect(update).toHaveBeenCalled());
			expect(componentsFrom(update.mock.calls[0][0])[0]).toMatchObject({
				configuration: [{ key: 'API_TOKEN', policy: 'fixed', value: 'secret' }]
			});
			await expect.element(page.getByRole('heading', { name: 'Add Tools' })).toBeVisible();
		});

		it('skips tool selection after post-create configuration when a policy is user-supplied', async () => {
			const entry = configurableGithub();
			const vmcp = createVMCP(
				{
					id: 'vmcp-1',
					displayName: 'Issue Tracker vMCP',
					components: [
						{
							id: `component-${entry.id}`,
							name: entry.manifest.name ?? entry.id,
							mcpCatalogID: 'default',
							mcpServerCatalogEntryID: entry.id,
							catalogEntry: { manifest: entry.manifest }
						}
					]
				},
				[entry]
			);
			const update = vi.fn();
			mockUpdateVMcp(vmcp, update);
			queueToolSetupForCreatedVMcp(vmcp.id);

			await renderDesigner([entry], vmcp);
			await page
				.getByRole('combobox', { name: 'API token policy' })
				.selectOptions('Provided at connection');
			await page.getByRole('button', { name: 'Next' }).click();

			await vi.waitFor(() => expect(update).toHaveBeenCalled());
			expect(componentsFrom(update.mock.calls[0][0])[0]).toMatchObject({
				configuration: [{ key: 'API_TOKEN', policy: 'userAllowed' }]
			});
			await expect
				.element(page.getByRole('heading', { name: 'Add Tools' }))
				.not.toBeInTheDocument();
		});
	});

	describe('profiles hint handed over from vMCP creation', () => {
		const HINT_TEXT = 'Click here to begin tailoring access and tools this VMCP.';

		it('shows the profiles tip on the page the new vMCP navigated to', async () => {
			const vmcp = createIssueTrackerVMcp();
			queueProfilesHintForCreatedVMcp(vmcp.id);

			await renderDesigner([componentEntry], vmcp);

			await expect.element(page.getByText(HINT_TEXT, { exact: false })).toBeVisible();
		});

		it('leaves an existing vMCP alone when nothing was queued', async () => {
			const vmcp = createIssueTrackerVMcp();
			await renderDesigner([componentEntry], vmcp);

			await expect.element(page.getByText(HINT_TEXT, { exact: false })).not.toBeInTheDocument();
		});

		it('hands the queued hint over only once, so later visits stay quiet', async () => {
			const vmcp = createIssueTrackerVMcp();
			queueProfilesHintForCreatedVMcp(vmcp.id);

			await renderDesigner([componentEntry], vmcp);
			await expect.element(page.getByText(HINT_TEXT, { exact: false })).toBeVisible();

			expect(claimProfilesHintForVMcp(vmcp.id)).toBe(false);
		});

		it('does not show the hint for viewers without profile tabs', async () => {
			const vmcp = createIssueTrackerVMcp();
			vmcp.userID = getProfileResponse.id;
			queueProfilesHintForCreatedVMcp(vmcp.id);

			await renderDesigner([componentEntry], vmcp, { groups: [Group.USER] });

			await expect.element(page.getByText(HINT_TEXT, { exact: false })).not.toBeInTheDocument();
		});

		it('does not show the hint again after it has been seen', async () => {
			localStorage.setItem(VMCP_PROFILES_HINT_STORAGE_KEY, new Date().toISOString());
			const vmcp = createIssueTrackerVMcp();
			queueProfilesHintForCreatedVMcp(vmcp.id);

			await renderDesigner([componentEntry], vmcp);

			await expect.element(page.getByText(HINT_TEXT, { exact: false })).not.toBeInTheDocument();
			expect(claimProfilesHintForVMcp(vmcp.id)).toBe(false);
		});
	});
});
