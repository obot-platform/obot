import { COMMUNITY_ENTITLEMENT, SETUP_COMMUNITY_SIGNUP_BANNER_COPY } from '$lib/constants';
import { type VMCP, type VMCPInstance } from '$lib/services';
import { vmcpInstances } from '$lib/stores';
import { createVMCP } from '../../../tests/helpers/mcp';
import { preparePageData } from '../../../tests/helpers/pageData';
import { getProfileResponse, getLicenseResponse } from '../../../tests/mocks/data';
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
		userID: getProfileResponse.id,
		created: '2026-01-01T00:00:00.000Z',
		type: 'vmcpinstance',
		status
	} as VMCPInstance;
}

function mockMCPInitializationFailure(connectID: string, status = 500) {
	worker.use(
		http.post(`/mcp-connect/${connectID}`, () => new HttpResponse(null, { status })),
		http.get(`/mcp-connect/${connectID}`, () => new HttpResponse(null, { status: 405 }))
	);
}

function createConfigurableVMcp(): VMCP {
	const target = createVMCP({ id: vmcp.id, displayName: vmcp.displayName });
	const component = target.components![0];
	return {
		...target,
		components: [
			{
				...component,
				configuration: [{ key: 'API_TOKEN', policy: 'userAllowed' }],
				catalogEntry: {
					...component.catalogEntry,
					manifest: {
						...component.catalogEntry!.manifest,
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
				}
			}
		]
	};
}

async function renderVMcpTester(
	overrides: Record<string, unknown>,
	options?: {
		vmcp?: VMCP;
		instance?: VMCPInstance | null;
		onLaunch?: () => void;
		openEditInstanceConfiguration?: (target: VMCP, instance: VMCPInstance) => void;
		connectFailureStatus?: number;
	}
) {
	await preparePageData(overrides);

	const target = options?.vmcp ?? vmcp;
	const instance =
		options && 'instance' in options ? options.instance : createInstance({ configured: true });

	vmcpInstances.current = { items: instance ? [instance] : [], loading: false };
	if (instance) {
		if (options?.connectFailureStatus !== undefined) {
			mockMCPInitializationFailure(instance.id, options.connectFailureStatus);
		} else {
			mockMCPInitialization(instance.id);
		}
	}

	return render(VMcpTester, {
		vmcp: target,
		onLaunch: options?.onLaunch ?? vi.fn(),
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
			.element(
				page.getByText('Before you can continue inspecting this vMCP, an update is required.')
			)
			.toBeVisible();
		await page.getByRole('button', { name: 'Update Configuration' }).click();
		expect(openEditInstanceConfiguration).toHaveBeenCalledWith(vmcp, instance);
	});

	it('starts a session when the vMCP has no instance', async () => {
		const onLaunch = vi.fn();
		await renderVMcpTester({}, { instance: null, onLaunch });

		await expect
			.element(page.getByText('Start your vMCP to use chat and inspect tools.'))
			.toBeVisible();
		await page.getByRole('button', { name: 'Start Session' }).click();
		expect(onLaunch).toHaveBeenCalledOnce();
	});

	it('keeps reauthentication on the tester when credentials expire', async () => {
		const instance = createInstance({ configured: true });
		const onLaunch = vi.fn();
		await renderVMcpTester(
			{},
			{
				vmcp: createConfigurableVMcp(),
				instance,
				onLaunch,
				openEditInstanceConfiguration: vi.fn(),
				connectFailureStatus: 401
			}
		);

		await expect
			.element(page.getByRole('heading', { name: 'Reauthentication required' }))
			.toBeVisible();
		await page.getByRole('button', { name: 'Manage authentication' }).click();
		expect(onLaunch).toHaveBeenCalledOnce();
		await expect
			.element(page.getByRole('button', { name: 'Update Configuration' }))
			.not.toBeInTheDocument();
	});

	it('keeps Retry when a connection fails without user-provided configuration', async () => {
		await renderVMcpTester({}, { connectFailureStatus: 500 });

		await expect.element(page.getByRole('heading', { name: 'Server unavailable' })).toBeVisible();
		await expect.element(page.getByRole('button', { name: 'Retry' })).toBeVisible();
		await expect
			.element(page.getByRole('button', { name: 'Update Configuration' }))
			.not.toBeInTheDocument();
	});

	it.each([500, 503])(
		'opens edit configuration when a configured vMCP with user-provided fields cannot start (%s)',
		async (connectFailureStatus) => {
			const instance = createInstance({ configured: true });
			const target = createConfigurableVMcp();
			const openEditInstanceConfiguration = vi.fn();
			await renderVMcpTester(
				{},
				{
					vmcp: target,
					instance,
					openEditInstanceConfiguration,
					connectFailureStatus
				}
			);

			await expect
				.element(
					page.getByText(
						'There was an issue starting the session. Please verify configuration or contact support if the issue persists.'
					)
				)
				.toBeVisible();
			await expect
				.element(page.getByRole('heading', { name: 'Server unavailable' }))
				.not.toBeInTheDocument();
			await expect.element(page.getByRole('button', { name: 'Retry' })).not.toBeInTheDocument();
			await page.getByRole('button', { name: 'Update Configuration' }).click();
			expect(openEditInstanceConfiguration).toHaveBeenCalledWith(target, instance);
		}
	);
});
