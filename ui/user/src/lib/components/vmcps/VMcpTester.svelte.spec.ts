import { COMMUNITY_ENTITLEMENT, SETUP_COMMUNITY_SIGNUP_BANNER_COPY } from '$lib/constants';
import { type VMCP, type VMCPInstance } from '$lib/services';
import { profile, vmcpInstances } from '$lib/stores';
import { createVMCP } from '../../../tests/helpers/mcp';
import { preparePageData } from '../../../tests/helpers/pageData';
import { getLicenseResponse } from '../../../tests/mocks/data';
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

function createInstance(status?: VMCPInstance['status']): VMCPInstance {
	return {
		id: 'vmcpi-1',
		vmcpID: vmcp.id,
		userID: profile.current.id,
		created: '2026-01-01T00:00:00.000Z',
		type: 'vmcpinstance',
		status
	} as VMCPInstance;
}

async function renderVMcpTester(
	overrides: Record<string, unknown>,
	options?: {
		instance?: VMCPInstance | null;
		openEditInstanceConfiguration?: (target: VMCP, instance: VMCPInstance) => void;
	}
) {
	await preparePageData(overrides);

	const instance =
		options && 'instance' in options ? options.instance : createInstance({ configured: true });

	vmcpInstances.current = { items: instance ? [instance] : [], loading: false };
	if (instance) {
		mockMCPInitialization(instance.id);
	}

	return render(VMcpTester, {
		vmcp,
		onLaunch: vi.fn(),
		openEditInstanceConfiguration: options?.openEditInstanceConfiguration
	});
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

		await expect
			.element(page.getByRole('heading', { name: 'Unlock Chat & More!', exact: true }))
			.toBeVisible();
		await expect.element(page.getByText(SETUP_COMMUNITY_SIGNUP_BANNER_COPY)).toBeVisible();
		await expect.element(page.getByRole('button', { name: 'Register' })).toBeVisible();
	});

	it('keeps the default model alias message when a model provider is configured', async () => {
		await renderVMcpTester({
			models: [],
			defaultModelAliases: [],
			license: {
				...getLicenseResponse,
				licenseKey: 'community-license-key',
				enterprise: true,
				entitlements: [COMMUNITY_ENTITLEMENT]
			},
			version: { hasModelProvider: true, hasValidLicense: true }
		});

		await expect.element(page.getByText('Chat unavailable', { exact: true })).toBeVisible();
		await expect
			.element(page.getByText('No default llm model is configured. Configure one to use Chat.'))
			.toBeVisible();
	});

	it('opens edit configuration for an unconfigured instance', async () => {
		const instance = createInstance({
			configured: false,
			missingRequiredConfiguration: ['component-entry-default.API_TOKEN']
		});
		const openEditInstanceConfiguration = vi.fn();
		await renderVMcpTester(
			{},
			{
				instance,
				openEditInstanceConfiguration
			}
		);

		await expect
			.element(page.getByText('An update is required before being able to test this vMCP.'))
			.toBeVisible();
		await page.getByRole('button', { name: 'Update Configuration' }).click();
		expect(openEditInstanceConfiguration).toHaveBeenCalledWith(vmcp, instance);
	});

	it('starts a session when the vMCP has no instance', async () => {
		const onLaunch = vi.fn();
		await preparePageData({});
		vmcpInstances.current = { items: [], loading: false };
		await render(VMcpTester, { vmcp, onLaunch });

		await expect
			.element(page.getByText('Start your vMCP to use chat and inspect tools.'))
			.toBeVisible();
		await page.getByRole('button', { name: 'Start Session' }).click();
		expect(onLaunch).toHaveBeenCalledOnce();
	});
});
