import type { MCPCatalogServer } from '$lib/services';
import { MCPTesterSession } from '$lib/services/mcp/tester.svelte';
import { worker } from '../../../../tests/mocks/worker';
import PromptsInspector from './PromptsInspector.svelte';
import ResourcesInspector from './ResourcesInspector.svelte';
import { ErrorCode } from '@modelcontextprotocol/sdk/types.js';
import { http, HttpResponse } from 'msw';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { cleanup, render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

const server = {
	id: 'ms1capabilities',
	configured: true,
	deploymentStatus: 'Available',
	manifest: { name: 'Tester Server', runtime: 'remote', serverUserType: 'singleUser' }
} as unknown as MCPCatalogServer;

let session: MCPTesterSession;
afterEach(async () => {
	cleanup();
	session?.close();
});

for (const section of ['prompts', 'resources'] as const) {
	describe(`${section} inspector discovery`, () => {
		async function renderInspector(
			reply: Record<string, unknown> | ((cursor?: string) => Record<string, unknown>),
			advertised = true
		) {
			const listRequest = vi.fn();
			worker.use(
				http.get('/mcp-connect/:id', () => new HttpResponse(null, { status: 405 })),
				http.delete('/mcp-connect/:id', () => new HttpResponse(null, { status: 200 })),
				http.post('/mcp-connect/:id', async ({ request }) => {
					const body = (await request.json()) as {
						id?: number;
						method: string;
						params?: { protocolVersion?: string; cursor?: string };
					};
					if (body.method === 'initialize') {
						return HttpResponse.json({
							jsonrpc: '2.0',
							id: body.id,
							result: {
								protocolVersion: body.params?.protocolVersion,
								capabilities: advertised ? { [section]: {} } : {},
								serverInfo: { name: 'test', version: '1' }
							}
						});
					}
					if (body.method === `${section}/list`) {
						listRequest(body.params);
						return HttpResponse.json({
							jsonrpc: '2.0',
							id: body.id,
							...(typeof reply === 'function' ? reply(body.params?.cursor) : reply)
						});
					}
					return new HttpResponse(null, { status: 202 });
				})
			);
			session = new MCPTesterSession(server, { name: 'obot-mcp-tester', version: 'test' });
			await session.initialize();
			render(section === 'prompts' ? PromptsInspector : ResourcesInspector, {
				session,
				onstaged: vi.fn()
			});
			return listRequest;
		}

		async function expectEmpty() {
			await expect
				.element(page.getByText(`This server does not provide ${section}.`))
				.toBeVisible();
			await expect.element(page.getByRole('alert')).not.toBeInTheDocument();
			await expect
				.element(page.getByRole('button', { name: 'Retry', exact: true }))
				.not.toBeInTheDocument();
		}

		it('does not request an unadvertised capability', async () => {
			const listRequest = await renderInspector({ result: {} }, false);
			await expectEmpty();
			expect(listRequest).not.toHaveBeenCalled();
		});

		it.each([
			{ name: 'empty', result: { [section]: [] } },
			{ name: 'missing', result: {} },
			{ name: 'null', result: { [section]: null } }
		])('shows an empty state for a $name list', async ({ result }) => {
			const listRequest = await renderInspector({ result });
			await expectEmpty();
			expect(session.cache[section].loaded).toBe(true);
			expect(session.cache[section].items).toEqual([]);
			expect(listRequest).toHaveBeenCalledTimes(1);
		});

		it('treats method not found as unsupported even when advertised', async () => {
			const listRequest = await renderInspector({
				error: { code: ErrorCode.MethodNotFound, message: 'Method not found' }
			});
			await expectEmpty();
			expect(session.cache[section].unsupported).toBe(true);
			expect(listRequest).toHaveBeenCalledTimes(1);
		});

		it.each([
			{
				name: 'server failure',
				reply: { error: { code: ErrorCode.InternalError, message: 'Temporary failure' } }
			},
			{ name: 'invalid list type', reply: { result: { [section]: 'invalid' } } },
			{ name: 'invalid entry', reply: { result: { [section]: [{}] } } }
		])('preserves errors and Retry for a $name', async ({ reply }) => {
			const listRequest = await renderInspector(reply);
			await expect.element(page.getByRole('alert')).toBeVisible();
			await expect
				.element(page.getByText(`This server does not provide ${section}.`))
				.not.toBeInTheDocument();
			await page.getByRole('button', { name: 'Retry', exact: true }).click();
			await vi.waitFor(() => expect(listRequest).toHaveBeenCalledTimes(2));
			await expect.element(page.getByRole('alert')).toBeVisible();
		});

		it.each([true, false])(
			'retains entries across empty pages (empty first: %s)',
			async (emptyFirst) => {
				const entry = { name: 'Example', uri: 'file:///example' };
				const listRequest = await renderInspector((cursor) => ({
					result: cursor
						? emptyFirst
							? { [section]: [entry] }
							: {}
						: { [section]: emptyFirst ? null : [entry], nextCursor: 'next' }
				}));
				await expect.element(page.getByRole('button', { name: /^Example/ })).toBeVisible();
				await expect.element(page.getByRole('alert')).not.toBeInTheDocument();
				expect(listRequest.mock.calls).toEqual([[{}], [{ cursor: 'next' }]]);
				expect(session.cache[section].pages).toHaveLength(2);
				expect(session.cache[section].items).toHaveLength(1);
			}
		);

		it('still displays valid entries', async () => {
			await renderInspector({
				result: { [section]: [{ name: 'Example', uri: 'file:///example' }] }
			});
			await expect.element(page.getByRole('button', { name: /^Example/ })).toBeVisible();
			await expect.element(page.getByRole('alert')).not.toBeInTheDocument();
		});
	});
}
