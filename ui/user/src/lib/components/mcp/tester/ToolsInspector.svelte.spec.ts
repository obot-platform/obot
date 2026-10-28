import type { MCPCatalogServer } from '$lib/services';
import { MCPTesterSession } from '$lib/services/mcp/tester.svelte';
import ToolsInspector from './ToolsInspector.svelte';
import { EditorView } from '@codemirror/view';
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
	it('shows the original tool response inside the full-error dialog', async () => {
		const { session } = renderInspector(1);
		const rawResponse = {
			jsonrpc: '2.0' as const,
			id: 3,
			result: { content: [{ type: 'text', text: 'no_subscription' }] }
		};
		vi.spyOn(session, 'callTool').mockResolvedValue({
			status: 'transport-error',
			durationMs: 10,
			message: 'Output schema validation failed',
			details: { name: 'McpError', code: -32600 },
			rawResponse
		});
		await page.getByRole('button', { name: 'tool_0', exact: false }).click();
		await page.getByLabelText('value').fill('test');
		await page.getByRole('button', { name: 'Call', exact: true }).click();
		await page.getByRole('button', { name: 'View full error' }).click();
		await page.getByRole('dialog').getByText('Raw response', { exact: true }).click();
		const preview = page.getByRole('dialog').getByLabelText('Raw MCP response');
		await expect.element(preview).toBeVisible();
		expect(EditorView.findFromDOM(preview.element() as HTMLElement)?.state.doc.toString()).toBe(
			JSON.stringify(rawResponse, null, 2)
		);
		session.close();
	});

	it('keeps cached rejected-tool warnings visible after a cancelled refresh', async () => {
		const { session } = renderInspector(2);
		session.toolDiscoveryIssues = [{ name: 'rejected', index: 2, issues: [] }];
		session.cache.tools.error = 'Loading was cancelled';
		session.cache.tools.errorStatus = 'cancelled';
		await expect.element(page.getByText('Malformed tools excluded: 1')).toBeVisible();
		await expect.element(page.getByRole('list', { name: 'Tools' })).toBeVisible();
		session.close();
	});

	it('keeps valid tools usable beside rejected-entry diagnostics', async () => {
		const { session } = renderInspector(2);
		session.toolDiscoveryIssues = [
			{
				name: 'add_issue_comment',
				index: 2,
				issues: [{ path: ['outputSchema', 'type'], message: 'Invalid input: expected object' }]
			}
		];
		await expect.element(page.getByText('Malformed tools excluded: 1')).toBeVisible();
		await page.getByRole('button', { name: 'tool_0', exact: false }).click();
		await expect.element(page.getByLabelText('value')).toBeVisible();
		await page.getByRole('button', { name: 'View full error' }).click();
		await expect.element(page.getByRole('dialog', { name: 'Error details' })).toBeVisible();
		await expect.element(page.getByLabelText('Full error')).toBeVisible();
		const errorText = EditorView.findFromDOM(
			page.getByLabelText('Full error').element() as HTMLElement
		)?.state.doc.toString();
		expect(JSON.parse(errorText ?? '')).toEqual(session.toolDiscoveryIssues);
		session.close();
	});

	it('keeps Retry available beside a bounded discovery error', async () => {
		const { session } = renderInspector(0);
		session.cache.tools.error = 'discovery failure '.repeat(200);
		const retry = vi.spyOn(session, 'loadTools').mockResolvedValue();
		await expect.element(page.getByRole('button', { name: 'View full error' })).toBeVisible();
		await page.getByRole('button', { name: 'Retry', exact: true }).click();
		expect(retry).toHaveBeenCalledWith(true);
		expect(page.getByRole('alert').element().textContent?.length).toBeLessThan(500);
		session.close();
	});

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
		await page.getByRole('button', { name: 'tool_0', exact: false }).click();

		await expect.element(page.getByRole('button', { name: 'Call' })).toBeVisible();
		expect(card.getBoundingClientRect().height).toBe(CARD_HEIGHT);
		expect(card.scrollHeight).toBeLessThanOrEqual(card.clientHeight);

		const details = detailsPane();
		expect(Math.round(details.getBoundingClientRect().bottom)).toBeLessThanOrEqual(
			Math.round(card.getBoundingClientRect().bottom)
		);

		session.close();
	});

	it('resets the argument form when Refresh changes a tool schema under the same name', async () => {
		const { session } = renderInspector(2);

		await page.getByRole('button', { name: 'tool_0', exact: false }).click();
		await page.getByLabelText('value').fill('typed');

		const [first, ...rest] = session.cache.tools.items;
		session.cache.tools.items = [
			{
				...first,
				inputSchema: {
					type: 'object' as const,
					properties: { replacement: { type: 'string', description: 'New field' } }
				}
			},
			...rest
		];

		await expect.element(page.getByLabelText('replacement')).toBeVisible();
		await expect.element(page.getByLabelText('value')).not.toBeInTheDocument();
		await page.getByRole('tab', { name: 'Raw JSON' }).click();
		await expect.element(page.getByLabelText('Arguments JSON')).toHaveValue('{}');
	});

	it('shows details for tools whose schema has object or array defaults', async () => {
		const { session } = renderInspector(0);
		// The tools land in the session's deep $state, so their schemas are proxies,
		// as they are when loaded from a server.
		session.cache.tools.items = [
			{
				name: 'tavily_research',
				description: 'Research a topic.',
				inputSchema: {
					type: 'object' as const,
					properties: { input: { type: 'string' } },
					required: ['input']
				}
			},
			{
				name: 'tavily_crawl',
				description: 'Crawl a site.',
				inputSchema: {
					type: 'object' as const,
					properties: {
						url: { type: 'string' },
						select_paths: { type: 'array', items: { type: 'string' }, default: [] }
					},
					required: ['url']
				}
			}
		];

		const details = page.getByRole('region', { name: 'Tool details' });
		await page.getByRole('button', { name: 'tavily_research', exact: false }).click();
		await expect.element(details.getByText('Research a topic.')).toBeVisible();

		await page.getByRole('button', { name: 'tavily_crawl', exact: false }).click();
		await expect
			.element(details.getByRole('heading', { name: 'tavily_crawl', level: 3 }))
			.toBeVisible();
		await expect.element(details.getByText('Crawl a site.')).toBeVisible();
		await expect.element(details.getByText('Research a topic.')).not.toBeInTheDocument();
		await expect.element(details.getByRole('group', { name: 'select_paths' })).toBeVisible();
		await expect.element(page.getByRole('button', { name: 'Call' })).toBeDisabled();

		await page.getByLabelText('url *').fill('https://example.com');
		await expect.element(page.getByRole('button', { name: 'Call' })).toBeEnabled();

		session.close();
	});

	it('resets the argument form when switching between tools with identical schemas', async () => {
		const { session } = renderInspector(0);
		const inputSchema = {
			type: 'object' as const,
			properties: { value: { type: 'string' } },
			required: ['value']
		};
		session.cache.tools.items = [
			{ name: 'first_tool', inputSchema },
			{ name: 'second_tool', inputSchema }
		];

		await page.getByRole('button', { name: 'first_tool', exact: false }).click();
		await page.getByLabelText('value *').fill('typed');
		await expect.element(page.getByRole('button', { name: 'Call' })).toBeEnabled();

		await page.getByRole('button', { name: 'second_tool', exact: false }).click();
		await expect
			.element(page.getByRole('heading', { name: 'second_tool', level: 3 }))
			.toBeVisible();
		await expect.element(page.getByLabelText('value *')).toHaveValue('');
		await expect.element(page.getByRole('button', { name: 'Call' })).toBeDisabled();

		session.close();
	});

	it('replaces a form that fails to render with an alert and clears its arguments', async () => {
		const { session } = renderInspector(1);
		await page.getByRole('button', { name: 'tool_0', exact: false }).click();
		await page.getByLabelText('value').fill('typed');
		await expect.element(page.getByRole('button', { name: 'Call' })).toBeEnabled();

		// Refresh returns a malformed schema (enum must be an array), which the form
		// throws on. selectTool is not called, so only the boundary can clear the arguments.
		const [tool] = session.cache.tools.items;
		session.cache.tools.items = [
			{
				...tool,
				inputSchema: {
					type: 'object' as const,
					properties: { mode: { type: 'string', enum: 'fast' as unknown as string[] } }
				}
			}
		];

		await expect
			.element(page.getByRole('alert').getByText('could not be displayed', { exact: false }))
			.toBeVisible();
		await expect.element(page.getByLabelText('value')).not.toBeInTheDocument();
		await expect.element(page.getByRole('button', { name: 'Call' })).toBeDisabled();

		session.close();
	});

	it('locks the tool list while a workflow runs so results stay with their tool', async () => {
		const { session } = renderInspector(3);
		await page.getByRole('button', { name: 'tool_0', exact: false }).click();
		await expect.element(page.getByRole('button', { name: 'Call' })).toBeVisible();

		const workflow = session.beginWorkflow('direct', 'tool call');
		await expect.element(page.getByRole('button', { name: 'tool_1', exact: false })).toBeDisabled();

		session.finishWorkflow(workflow);
		await expect.element(page.getByRole('button', { name: 'tool_1', exact: false })).toBeEnabled();

		session.close();
	});
});
