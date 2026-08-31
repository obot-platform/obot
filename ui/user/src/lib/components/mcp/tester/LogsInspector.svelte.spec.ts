import type { MCPCatalogServer } from '$lib/services';
import type { MCPTesterSession, TesterStatus } from '$lib/services/mcp/tester.svelte';
import { MCPTesterSession as Session } from '$lib/services/mcp/tester.svelte';
import LogsInspector from './LogsInspector.svelte';
import type { JSONRPCMessage } from '@modelcontextprotocol/sdk/types.js';
import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

const server = {
	id: 'ms1logs',
	configured: true,
	deploymentStatus: 'Available',
	manifest: {
		name: 'Tester Server',
		runtime: 'remote',
		serverUserType: 'singleUser'
	}
} as unknown as MCPCatalogServer;

const CARD_HEIGHT = 320;

function request(id: number, method: string, params?: Record<string, unknown>) {
	return { jsonrpc: '2.0', id, method, ...(params ? { params } : {}) } as JSONRPCMessage;
}

function response(id: number, result: Record<string, unknown> = {}) {
	return { jsonrpc: '2.0', id, result } as JSONRPCMessage;
}

interface RenderOptions {
	status?: TesterStatus;
	error?: string;
	seed?: (session: MCPTesterSession) => void;
	fixedHeight?: boolean;
}

function renderLogs({ status = 'ready', error, seed, fixedHeight = false }: RenderOptions = {}) {
	const session = new Session(
		server,
		{ name: 'obot-mcp-tester', version: 'test' },
		vi.fn<typeof fetch>(async () => new Response(null, { status: 202 }))
	);
	session.status = status;
	session.error = error;
	seed?.(session);

	const onretry = vi.fn();
	render(LogsInspector, { session, serverName: 'Tester Server', onretry });

	let card: HTMLElement | undefined;
	if (fixedHeight) {
		// Stand in for the tester page's card: a fixed-height flex column.
		const root = page
			.getByRole('heading', { name: 'MCP Log', level: 2 })
			.element()
			.closest('div') as HTMLElement;
		card = document.createElement('div');
		card.style.cssText = `height:${CARD_HEIGHT}px;display:flex;flex-direction:column;overflow:hidden;padding:1rem;`;
		root.replaceWith(card);
		card.append(root);
	}
	return { session, onretry, card };
}

function seedHandshake(session: MCPTesterSession) {
	session.log.recordLifecycle('connecting');
	session.log.recordMessage('outgoing', request(1, 'initialize'));
	session.log.recordMessage('incoming', response(1, { protocolVersion: '2025-06-18' }));
	session.log.recordMessage('outgoing', request(2, 'tools/call', { name: 'lookup' }));
	session.log.recordMessage('incoming', response(2, { content: [] }));
}

describe('LogsInspector', () => {
	it('renders a row per frame with direction, kind, method and rpc id', async () => {
		renderLogs({ seed: seedHandshake });

		await expect.element(page.getByRole('list', { name: 'MCP traffic log' })).toBeVisible();
		await expect
			.element(page.getByRole('button', { name: /Sent request initialize #1/ }))
			.toBeVisible();
		await expect
			.element(page.getByRole('button', { name: /Received response initialize #1/ }))
			.toBeVisible();
		await expect
			.element(page.getByRole('button', { name: /Sent request tools\/call #2/ }))
			.toBeVisible();
	});

	it('reveals the raw JSON payload only after the row is expanded', async () => {
		renderLogs({ seed: seedHandshake });

		const row = page.getByRole('button', { name: /Sent request tools\/call #2/ });
		await expect.element(row).toHaveAttribute('aria-expanded', 'false');
		// JsonPreview mounts a CodeMirror editor, so collapsed rows must not render one.
		await expect.element(page.getByLabelText('tools/call payload')).not.toBeInTheDocument();

		await row.click();
		await expect.element(row).toHaveAttribute('aria-expanded', 'true');
		await expect.element(page.getByLabelText('tools/call payload')).toBeVisible();

		await row.click();
		await expect.element(page.getByLabelText('tools/call payload')).not.toBeInTheDocument();
	});

	it('offers an icon-only copy control instead of a fullscreen toggle', async () => {
		renderLogs({ seed: seedHandshake });

		await page.getByRole('button', { name: /Sent request tools\/call #2/ }).click();
		await expect.element(page.getByLabelText('tools/call payload')).toBeVisible();

		const copy = page.getByRole('button', { name: 'Copy entry JSON' });
		await expect.element(copy).toBeVisible();
		expect(copy.element().className).toContain('absolute');
		await expect
			.element(page.getByRole('button', { name: /maximize|fullscreen/i }))
			.not.toBeInTheDocument();
	});

	it('keeps the copy control clear of the scrollbar on a scrolling payload', async () => {
		renderLogs({
			seed: (session) => {
				session.log.recordMessage(
					'incoming',
					response(1, { lines: Array.from({ length: 200 }, (_, i) => `line ${i}`) })
				);
			}
		});

		await page.getByRole('button', { name: /Received response/ }).click();
		const copy = page.getByRole('button', { name: 'Copy entry JSON' });
		await expect.element(copy).toBeVisible();

		const scroller = document.querySelector('.cm-scroller') as HTMLElement;
		expect(scroller.scrollHeight).toBeGreaterThan(scroller.clientHeight);

		// The button must end before the scrollbar track begins.
		await vi.waitFor(() => {
			const contentRight = scroller.getBoundingClientRect().left + scroller.clientWidth;
			expect(copy.element().getBoundingClientRect().right).toBeLessThanOrEqual(contentRight + 1);
		});
	});

	it('does not announce every entry to screen readers', async () => {
		renderLogs({ seed: seedHandshake });

		const list = page.getByRole('list', { name: 'MCP traffic log' }).element();
		expect(list.getAttribute('aria-live')).toBeNull();
		expect(list.getAttribute('role')).toBeNull();
		// A throttled count is announced instead of the entries themselves.
		expect(document.querySelectorAll('[aria-live="polite"]').length).toBeGreaterThan(0);
	});

	async function selectKind(label: string) {
		await page.getByRole('combobox', { name: 'Filter by kind' }).click();
		// The options live in a popover that stays in the DOM while closed, so wait for the
		// option to actually become visible instead of racing the click against the opening.
		const option = page.getByRole('button', { name: label, exact: true });
		await expect.element(option).toBeVisible();
		await option.click();
	}

	it('filters by kind and by method search', async () => {
		renderLogs({ seed: seedHandshake });

		await selectKind('Requests');
		await expect
			.element(page.getByRole('button', { name: /Sent request initialize/ }))
			.toBeVisible();
		await expect
			.element(page.getByRole('button', { name: /Received response initialize/ }))
			.not.toBeInTheDocument();

		await selectKind('All kinds');
		await page.getByLabelText('Search log by method or request id').fill('tools/call');
		await expect.element(page.getByRole('button', { name: /tools\/call/ }).first()).toBeVisible();
		await expect
			.element(page.getByRole('button', { name: /request initialize/ }))
			.not.toBeInTheDocument();
	});

	it('sizes the kind filter to match the search box', async () => {
		renderLogs({ seed: seedHandshake });

		const search = page.getByLabelText('Search log by method or request id').element();
		const kind = page.getByRole('combobox', { name: 'Filter by kind' }).element();
		expect(kind.getBoundingClientRect().height).toBe(search.getBoundingClientRect().height);
	});

	it('offers a way back when no entries match the filters', async () => {
		renderLogs({ seed: seedHandshake });

		await page.getByLabelText('Search log by method or request id').fill('nonexistent-method');
		await expect.element(page.getByText('No entries match these filters.')).toBeVisible();

		await page.getByRole('button', { name: 'Clear filters' }).click();
		await expect
			.element(page.getByRole('button', { name: /Sent request initialize/ }))
			.toBeVisible();
	});

	it('clears the buffer', async () => {
		const { session } = renderLogs({ seed: seedHandshake });

		await page.getByRole('button', { name: 'Clear' }).click();
		await expect.element(page.getByText('No MCP traffic yet.', { exact: false })).toBeVisible();
		expect(session.log.length).toBe(0);
	});

	it('copies the listed entries as NDJSON', async () => {
		const writeText = vi.spyOn(navigator.clipboard, 'writeText').mockResolvedValue();
		renderLogs({ seed: seedHandshake });

		await page.getByRole('button', { name: 'Copy All', exact: true }).click();
		expect(writeText).toHaveBeenCalled();

		const lines = String(writeText.mock.calls[0][0]).split('\n');
		expect(lines).toHaveLength(5);
		expect(() => lines.map((line) => JSON.parse(line))).not.toThrow();
		writeText.mockRestore();
	});

	it('replaces the inline preview with a size warning for very large payloads', async () => {
		renderLogs({
			seed: (session) => {
				session.log.recordMessage(
					'incoming',
					response(1, { text: 'x'.repeat(300 * 1024) }) as JSONRPCMessage
				);
			}
		});

		await page.getByRole('button', { name: /Received response/ }).click();
		await expect.element(page.getByRole('button', { name: 'Render anyway' })).toBeVisible();

		await page.getByRole('button', { name: 'Render anyway' }).click();
		await expect
			.element(page.getByRole('button', { name: 'Render anyway' }))
			.not.toBeInTheDocument();
	});

	it('shows a connection failure banner with the session error and a Retry action', async () => {
		const { onretry } = renderLogs({
			status: 'error',
			error: 'fetch failed',
			seed: (session) => session.log.recordError(new Error('fetch failed'))
		});

		const alert = page.getByRole('alert');
		await expect.element(alert).toBeVisible();
		await expect.element(alert).toHaveTextContent('Connection failed');
		await expect.element(alert).toHaveTextContent('fetch failed');
		// The failed handshake is still inspectable even though the session never connected.
		await expect
			.element(page.getByRole('button', { name: /transport .*fetch failed/ }))
			.toBeVisible();

		await page.getByRole('button', { name: 'Retry' }).click();
		expect(onretry).toHaveBeenCalled();
	});

	it('explains why the log is empty for a server that needs setup', async () => {
		renderLogs({ status: 'setup-required' });

		await expect.element(page.getByText('this server still needs', { exact: false })).toBeVisible();
	});

	it('sticks to the newest entry until the user scrolls away', async () => {
		const { session } = renderLogs({ seed: seedHandshake, fixedHeight: true });
		const list = page.getByRole('list', { name: 'MCP traffic log' }).element();

		const append = async (method: string) => {
			for (let index = 0; index < 12; index++) {
				session.log.recordMessage('outgoing', request(100 + index, method));
			}
			await expect
				.element(page.getByRole('button', { name: new RegExp(method) }).last())
				.toBeVisible();
		};

		await append('tools/list');
		await vi.waitFor(() =>
			expect(list.scrollHeight - list.clientHeight - list.scrollTop).toBeLessThan(16)
		);

		// Scrolling up hands control back to the user; later entries must not yank the view.
		list.scrollTop = 0;
		list.dispatchEvent(new Event('scroll'));
		await append('prompts/list');
		expect(list.scrollTop).toBe(0);
	});
});
