import type { EntryDrag } from '$lib/runes/vmcps/entryDrag.svelte';
import { mcpServersAndEntries } from '$lib/stores';
import { createMCPCatalogEntry } from '../../../tests/helpers/mcp';
import { preparePageData } from '../../../tests/helpers/pageData';
import McpServersSidebar from './McpServersSidebar.svelte';
import { tick } from 'svelte';
import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

const github = createMCPCatalogEntry({ id: 'entry-github', name: 'GitHub' });
const slack = createMCPCatalogEntry({ id: 'entry-slack', name: 'Slack' });

function createDragStub(overrides: Partial<EntryDrag> = {}) {
	return {
		active: false,
		started: false,
		entry: undefined,
		x: 0,
		y: 0,
		wire: undefined,
		isLinked: () => false,
		isComponentLinked: () => false,
		isDragging: () => false,
		isDraggingNewEntry: false,
		activate: vi.fn(),
		cancel: vi.fn(),
		pointerDown: vi.fn(),
		pointerMove: vi.fn(),
		pointerUp: vi.fn(),
		createTarget: () => undefined,
		vmcpTarget: () => undefined,
		componentTarget: () => undefined,
		...overrides
	} as unknown as EntryDrag;
}

async function renderSidebar(
	props: Partial<{
		drag: EntryDrag;
		entries: ReturnType<typeof createMCPCatalogEntry>[];
		canCreateEntry: boolean;
		showAllConnectors: boolean;
		query: string;
		onSearch: (value: string) => void;
	}> = {}
) {
	const { entries = [github, slack], drag = createDragStub(), ...rest } = props;
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
	const result = render(McpServersSidebar, {
		drag,
		onSearch: rest.onSearch ?? vi.fn(),
		...rest
	});
	return { result, drag };
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

function card(name: string) {
	return page.getByRole('button', { name: new RegExp(`View ${name} details`) });
}

describe('McpServersSidebar.svelte', () => {
	it('filters the list by the query it is given', async () => {
		await renderSidebar({ query: 'slack' });

		await expect.element(card('Slack')).toBeVisible();
		await expect.element(card('GitHub')).not.toBeInTheDocument();
	});

	it('reports searches back to the page rather than filtering on its own', async () => {
		const onSearch = vi.fn();
		await renderSidebar({ onSearch });

		await page.getByPlaceholder('Search MCP servers...').fill('git');

		await vi.waitFor(() => expect(onSearch).toHaveBeenCalledWith('git'));
	});

	it('hides workspace-owned servers unless the page allows them', async () => {
		const workspaceEntry = createMCPCatalogEntry({
			id: 'entry-workspace',
			name: 'Workspace Slack',
			powerUserWorkspaceID: 'ws-1'
		});
		await renderSidebar({ entries: [github, workspaceEntry] });

		await expect.element(card('Workspace Slack')).not.toBeInTheDocument();

		await renderSidebar({ entries: [github, workspaceEntry], showAllConnectors: true });

		await expect.element(card('Workspace Slack')).toBeVisible();
	});

	it('leaves out composite and multi-user servers, which cannot be dragged in', async () => {
		const composite = createMCPCatalogEntry({
			id: 'entry-composite',
			name: 'Composite',
			runtime: 'composite'
		});
		const multiUser = createMCPCatalogEntry({
			id: 'entry-multi',
			name: 'Multi User',
			serverUserType: 'multiUser'
		});
		await renderSidebar({ entries: [github, composite, multiUser] });

		await expect.element(card('GitHub')).toBeVisible();
		await expect.element(card('Composite')).not.toBeInTheDocument();
		await expect.element(card('Multi User')).not.toBeInTheDocument();
	});

	it('says so when nothing is left to show', async () => {
		await renderSidebar({ entries: [] });

		await expect.element(page.getByText('No MCP servers available.')).toBeVisible();
	});

	it('reorders the list by name and created date', async () => {
		const bravo = createMCPCatalogEntry({
			id: 'entry-bravo',
			name: 'Bravo',
			created: '2020-01-01T00:00:00.000Z'
		});
		const zulu = createMCPCatalogEntry({
			id: 'entry-zulu',
			name: 'Zulu',
			created: '2024-06-01T00:00:00.000Z'
		});
		await renderSidebar({ entries: [zulu, bravo] });

		async function cardOrder() {
			const els = await page.getByRole('button', { name: /View .+ details/ }).elements();
			return els.map((el) => el.getAttribute('aria-label'));
		}

		await vi.waitFor(async () => {
			expect(await cardOrder()).toEqual([
				'View Bravo details, or drag it onto a vMCP or Create vMCP',
				'View Zulu details, or drag it onto a vMCP or Create vMCP'
			]);
		});

		await page.getByRole('combobox', { name: 'Sort by' }).click();
		await page.getByRole('button', { name: 'Created Date', exact: true }).click();
		await tick();

		await vi.waitFor(async () => {
			expect(await cardOrder()).toEqual([
				'View Zulu details, or drag it onto a vMCP or Create vMCP',
				'View Bravo details, or drag it onto a vMCP or Create vMCP'
			]);
		});
	});

	describe('create entry button', () => {
		function createButton() {
			return page.getByCSS('#mcp-create-catalog-entry-button');
		}

		it('appears only for users allowed to create entries', async () => {
			await renderSidebar({ canCreateEntry: false });
			await expect.element(createButton()).not.toBeInTheDocument();

			await renderSidebar({ canCreateEntry: true });
			await expect.element(createButton()).toBeVisible();
		});
	});

	describe('drag sources', () => {
		it('hands a press on a server card to the drag state', async () => {
			const { drag } = await renderSidebar();

			const el = await card('GitHub').element();
			el.dispatchEvent(
				new PointerEvent('pointerdown', { bubbles: true, cancelable: true, pointerId: 3 })
			);

			expect(drag.pointerDown).toHaveBeenCalled();
			expect(vi.mocked(drag.pointerDown).mock.calls[0][1]).toMatchObject({ id: github.id });
		});

		it('treats Enter on a server card as a plain activation', async () => {
			const { drag } = await renderSidebar();

			const el = await card('GitHub').element();
			el.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true }));

			expect(drag.activate).toHaveBeenCalled();
			expect(vi.mocked(drag.activate).mock.calls[0][0]).toMatchObject({ id: github.id });
		});

		it('activates without an entry from the create card, which has none yet', async () => {
			const { drag } = await renderSidebar({ canCreateEntry: true });

			const el = await page.getByCSS('#mcp-create-catalog-entry-button').element();
			el.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true }));

			expect(drag.activate).toHaveBeenCalledWith();
		});

		it('dims the card that is being dragged', async () => {
			await renderSidebar({
				drag: createDragStub({ active: true, isDragging: (entry) => entry.id === github.id })
			});

			await expect.element(card('GitHub')).toHaveClass(/opacity-30/);
			await expect.element(card('Slack')).not.toHaveClass(/opacity-30/);
		});
	});

	describe('server settings popover', () => {
		const deprecated = createMCPCatalogEntry({
			id: 'entry-old',
			name: 'Old Server',
			manifest: { metadata: { deprecated: 'true' } }
		});

		it('hides deprecated servers until they are switched on', async () => {
			await renderSidebar({ entries: [github, deprecated] });

			await expect.element(card('Old Server')).not.toBeInTheDocument();

			await clickNative(page.getByCSS('#mcp-server-settings-button'));
			await clickNative(page.getByCSS('input[type="checkbox"].checkbox'));

			await expect.element(card('Old Server')).toBeVisible();
		});

		it('keeps servers that match any selected category', async () => {
			const githubDev = createMCPCatalogEntry({
				id: 'entry-github',
				name: 'GitHub',
				manifest: { metadata: { categories: 'devtools' } }
			});
			const slackChat = createMCPCatalogEntry({
				id: 'entry-slack',
				name: 'Slack',
				manifest: { metadata: { categories: 'communication' } }
			});
			const plain = createMCPCatalogEntry({ id: 'entry-plain', name: 'Plain' });
			await renderSidebar({ entries: [githubDev, slackChat, plain] });

			await clickNative(page.getByCSS('#mcp-server-settings-button'));
			await page.getByRole('combobox', { name: 'Filter By' }).click();
			await page.getByRole('button', { name: 'devtools', exact: true }).click();
			await tick();

			await expect.element(card('GitHub')).toBeVisible();
			await expect.element(card('Slack')).not.toBeInTheDocument();
			await expect.element(card('Plain')).not.toBeInTheDocument();

			await page.getByRole('combobox', { name: 'Filter By' }).click();
			await page.getByRole('button', { name: 'communication', exact: true }).click();
			await tick();

			await expect.element(card('GitHub')).toBeVisible();
			await expect.element(card('Slack')).toBeVisible();
			await expect.element(card('Plain')).not.toBeInTheDocument();

			await expect.element(page.getByRole('button', { name: 'Remove devtools' })).toBeVisible();
			await expect
				.element(page.getByRole('button', { name: 'Remove communication' }))
				.toBeVisible();

			await page.getByRole('button', { name: 'Remove communication' }).click();
			await tick();

			await expect.element(card('GitHub')).toBeVisible();
			await expect.element(card('Slack')).not.toBeInTheDocument();
			await expect.element(card('Plain')).not.toBeInTheDocument();
			await expect
				.element(page.getByRole('button', { name: 'Remove communication' }))
				.not.toBeInTheDocument();
		});
	});
});
