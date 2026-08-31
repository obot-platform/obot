import type { MCPCatalogServer } from '$lib/services';
import { MCPTesterSession } from '$lib/services/mcp/tester.svelte';
import ToolsInspector from './ToolsInspector.svelte';
import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

const server = {
	id: 'ms1tools',
	configured: true,
	deploymentStatus: 'Available',
	manifest: {
		name: 'Tester Server',
		runtime: 'remote',
		serverUserType: 'singleUser'
	}
} as unknown as MCPCatalogServer;

const CARD_HEIGHT = 420;

function makeTools(count: number) {
	return Array.from({ length: count }, (_, index) => ({
		name: `tool_${index}`,
		description: `Tool number ${index} with a description long enough to wrap onto a second line.`,
		inputSchema: {
			type: 'object' as const,
			properties: { value: { type: 'string', description: `Value for tool ${index}` } }
		}
	}));
}

function renderInspector(toolCount = 40) {
	const session = new MCPTesterSession(
		server,
		{ name: 'obot-mcp-tester', version: 'test' },
		vi.fn<typeof fetch>(async () => new Response(null, { status: 202 }))
	);
	session.status = 'ready';
	session.cache.tools = {
		loaded: true,
		loading: false,
		unsupported: false,
		items: makeTools(toolCount),
		pages: []
	};

	render(ToolsInspector, { session });
	// Stand in for the tester page's card: a fixed-height flex column.
	const root = page
		.getByRole('heading', { name: 'Tools', level: 2 })
		.element()
		.closest('div') as HTMLElement;
	const card = document.createElement('div');
	card.style.cssText = `height:${CARD_HEIGHT}px;display:flex;flex-direction:column;overflow:hidden;padding:1rem;`;
	root.replaceWith(card);
	card.append(root);
	return { session, card };
}

function toolList(): HTMLElement {
	return page.getByRole('list', { name: 'Tools' }).element() as HTMLElement;
}

function detailsPane(): HTMLElement {
	return page.getByCSS('[aria-label="Tool details"]').element() as HTMLElement;
}

describe('ToolsInspector', () => {
	it('keeps the card fixed and scrolls the tool list inside it', async () => {
		const { session, card } = renderInspector();
		await expect.element(page.getByRole('list', { name: 'Tools' })).toBeVisible();

		// The card never grows past the height it was given, so the page itself
		// has nothing to scroll.
		expect(card.scrollHeight).toBeLessThanOrEqual(card.clientHeight);
		expect(card.getBoundingClientRect().height).toBe(CARD_HEIGHT);

		// The overflow lives in the list instead.
		const list = toolList();
		expect(list.scrollHeight).toBeGreaterThan(list.clientHeight);
		expect(Math.round(list.getBoundingClientRect().bottom)).toBeLessThanOrEqual(
			Math.round(card.getBoundingClientRect().bottom)
		);

		list.scrollTop = list.scrollHeight;
		expect(list.scrollTop).toBeGreaterThan(0);
		expect(card.scrollTop).toBe(0);

		session.close();
	});

	it('scrolls a long tool detail on its own without resizing the card', async () => {
		const { session, card } = renderInspector();
		await page.getByRole('button', { name: 'tool_0' }).click();

		await expect.element(page.getByRole('button', { name: 'Call' })).toBeVisible();
		expect(card.getBoundingClientRect().height).toBe(CARD_HEIGHT);
		expect(card.scrollHeight).toBeLessThanOrEqual(card.clientHeight);

		const details = detailsPane();
		expect(Math.round(details.getBoundingClientRect().bottom)).toBeLessThanOrEqual(
			Math.round(card.getBoundingClientRect().bottom)
		);

		session.close();
	});
});
