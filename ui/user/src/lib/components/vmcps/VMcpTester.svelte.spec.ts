import { type VMCPInstance } from '$lib/services';
import { profile, vmcpInstances } from '$lib/stores';
import { createVMCP } from '../../../tests/helpers/mcp';
import { preparePageData } from '../../../tests/helpers/pageData';
import { worker } from '../../../tests/mocks/worker';
import VMcpTester from './VMcpTester.svelte';
import { http, HttpResponse } from 'msw';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

const vmcp = createVMCP({ id: 'vmcp-1', displayName: 'Issue Tracker vMCP' });

interface MCPRequest {
	id?: string | number;
	method: string;
}

function mockMCPInitialization(connectID: string) {
	worker.use(
		http.post(`/mcp-connect/${connectID}`, async ({ request }) => {
			const body = (await request.json()) as MCPRequest;
			if (body.method === 'initialize') {
				return HttpResponse.json(
					{
						jsonrpc: '2.0',
						id: body.id,
						result: {
							protocolVersion: '2025-06-18',
							capabilities: {},
							serverInfo: { name: 'vmcp-tester', version: '1.0.0' }
						}
					},
					{ headers: { 'Mcp-Session-Id': 'vmcp-tester-session' } }
				);
			}
			return new HttpResponse(null, { status: 202 });
		}),
		http.get(`/mcp-connect/${connectID}`, () => new HttpResponse(null, { status: 405 }))
	);
}

async function renderVMcpTester(overrides: Record<string, unknown>) {
	await preparePageData(overrides);

	const instance = {
		id: 'vmcpi-1',
		vmcpID: vmcp.id,
		userID: profile.current.id,
		created: '2026-01-01T00:00:00.000Z',
		type: 'vmcpinstance',
		status: { configured: true }
	} as unknown as VMCPInstance;

	vmcpInstances.current = { items: [instance], loading: false };
	mockMCPInitialization(instance.id);

	return render(VMcpTester, { vmcp, onLaunch: vi.fn() });
}

afterEach(() => {
	vmcpInstances.current = { items: [], loading: false };
});

describe('VMcpTester', () => {
	it('offers Chat through the model service when no model provider is configured', async () => {
		await renderVMcpTester({
			models: [],
			defaultModelAliases: [],
			version: {
				hasModelProvider: false,
				hasValidLicense: true,
				mcpTesterModelProxyAvailable: true
			}
		});

		await expect.element(page.getByRole('region', { name: 'Chat composer' })).toBeVisible();
	});

	it('requires a valid license when no model provider is configured', async () => {
		await renderVMcpTester({
			models: [],
			defaultModelAliases: [],
			version: { hasModelProvider: false, hasValidLicense: false }
		});

		await expect.element(page.getByText('Chat unavailable', { exact: true })).toBeVisible();
		await expect
			.element(
				page.getByText('Register a valid Obot license to use Chat without a model provider.')
			)
			.toBeVisible();
	});

	it('keeps the default model alias message when a model provider is configured', async () => {
		await renderVMcpTester({
			models: [],
			defaultModelAliases: [],
			version: { hasModelProvider: true, hasValidLicense: true }
		});

		await expect.element(page.getByText('Chat unavailable', { exact: true })).toBeVisible();
		await expect
			.element(page.getByText('No default llm model is configured. Configure one to use Chat.'))
			.toBeVisible();
	});
});
