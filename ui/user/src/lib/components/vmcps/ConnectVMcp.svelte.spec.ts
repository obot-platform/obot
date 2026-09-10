import type { VMCP, VMCPConfiguration, VMCPInstance } from '$lib/services';
import { vmcpInstances } from '$lib/stores';
import { createMCPCatalogEntry, createVMCP } from '../../../tests/helpers/mcp';
import { preparePageData } from '../../../tests/helpers/pageData';
import { getProfileResponse } from '../../../tests/mocks/data';
import { worker } from '../../../tests/mocks/worker';
import ConnectVMcp from './ConnectVMcp.svelte';
import { http, HttpResponse } from 'msw';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

function configurableVMcp(): VMCP {
	const vmcp = createVMCP({ id: 'vmcp1configurable', displayName: 'Configured vMCP' });
	vmcp.components![0].configuration = [{ key: 'API_TOKEN', policy: 'userAllowed' }];
	vmcp.components![0].catalogEntry.manifest.config = [
		{
			key: 'API_TOKEN',
			name: 'API token',
			description: 'Token used by this server',
			required: true,
			sensitive: true,
			value: '',
			usage: 'env'
		}
	];
	return vmcp;
}

async function renderDialog(vmcp: VMCP, instance?: VMCPInstance) {
	await preparePageData();
	await vmcpInstances.refresh();
	const result = await render(ConnectVMcp);
	result.component.open(vmcp, instance);
	return result;
}

async function continueFromIntro() {
	await expect
		.element(page.getByText('This will begin the initial setup process for this server.'))
		.toBeVisible();
	await page.getByRole('button', { name: 'Continue' }).click();
}

describe('ConnectVMcp.svelte', () => {
	beforeEach(() => {
		vmcpInstances.current = { items: [], loading: false };
	});

	it('opens the connect dialog and starts setup from Preconfigure when there is no instance', async () => {
		await renderDialog(configurableVMcp());
		await expect.element(page.getByCSS('#connect-to-vmcp-dialog')).toBeVisible();
		await expect
			.element(page.getByText('This will begin the initial setup process for this server.'))
			.not.toBeVisible();
		await page.getByRole('button', { name: 'Preconfigure server' }).click();
		await expect
			.element(page.getByText('This will begin the initial setup process for this server.'))
			.toBeVisible();
		await expect
			.element(
				page.getByText('Additional configuration details may also be required', { exact: false })
			)
			.toBeVisible();
	});

	function mockConfigureAndLaunch(
		vmcp: VMCP,
		opts?: {
			launchError?: string;
			launchDelayMs?: number;
			oauthURL?: string | ((check: number) => string);
		}
	) {
		const createInstance = vi.fn();
		const configureInstance = vi.fn();
		const launch = vi.fn();
		let oauthChecks = 0;
		worker.use(
			http.get('/api/vmcp-instances', () => HttpResponse.json({ items: [] })),
			http.post('/api/vmcp-instances', async ({ request }) => {
				createInstance(await request.json());
				return HttpResponse.json({
					id: 'vmcpi-created',
					vmcpID: vmcp.id,
					userID: getProfileResponse.id,
					created: '2026-01-01T00:00:00Z'
				});
			}),
			http.post('/api/vmcp-instances/vmcpi-created/configure', async ({ request }) => {
				configureInstance(await request.json());
				return HttpResponse.json({
					id: 'vmcpi-created',
					vmcpID: vmcp.id,
					userID: getProfileResponse.id,
					created: '2026-01-01T00:00:00Z'
				});
			}),
			http.post(`/api/vmcps/${vmcp.id}/launch`, async () => {
				launch();
				if (opts?.launchDelayMs) {
					await new Promise((resolve) => setTimeout(resolve, opts.launchDelayMs));
				}
				if (opts?.launchError) {
					return HttpResponse.text(opts.launchError, { status: 503 });
				}
				return HttpResponse.json({});
			}),
			http.get(`/api/vmcps/${vmcp.id}/oauth-url`, () => {
				oauthChecks += 1;
				const oauthURL =
					typeof opts?.oauthURL === 'function'
						? opts.oauthURL(oauthChecks)
						: (opts?.oauthURL ?? '');
				return HttpResponse.json({ oauthURL });
			})
		);
		return { createInstance, configureInstance, launch, oauthChecks: () => oauthChecks };
	}

	async function configureFromPreconfigure() {
		await page.getByRole('button', { name: 'Preconfigure server' }).click();
		await continueFromIntro();
		const tokenField = page.getByCSS('input[name="API token"]');
		await expect.element(tokenField).toBeVisible();
		await tokenField.click();
		await tokenField.fill('secret-token');
		await page.getByRole('button', { name: 'Configure', exact: true }).click();
	}

	it('launches immediately from Preconfigure when there is no user configuration', async () => {
		const vmcp = createVMCP({ id: 'vmcp1noconfig', displayName: 'Plain vMCP' });
		const { launch, createInstance, configureInstance } = mockConfigureAndLaunch(vmcp, {
			launchDelayMs: 80
		});

		await renderDialog(vmcp);
		await page.getByRole('button', { name: 'Preconfigure server' }).click();
		await continueFromIntro();

		await expect.element(page.getByText('Launching vMCP...')).toBeVisible();
		await vi.waitFor(() => expect(launch).toHaveBeenCalledOnce());
		expect(createInstance).toHaveBeenCalledWith({ vmcpID: vmcp.id });
		expect(configureInstance).not.toHaveBeenCalled();
		await expect.element(page.getByCSS('#connect-to-vmcp-dialog')).toBeVisible();
	});

	it('creates and configures an instance from Preconfigure', async () => {
		const vmcp = configurableVMcp();
		const { createInstance, configureInstance, launch } = mockConfigureAndLaunch(vmcp);

		await renderDialog(vmcp);
		await configureFromPreconfigure();

		await vi.waitFor(() => expect(launch).toHaveBeenCalledOnce());
		expect(createInstance).toHaveBeenCalledWith({ vmcpID: vmcp.id });
		expect(configureInstance).toHaveBeenCalledWith({
			components: { 'component-entry-default': { API_TOKEN: 'secret-token' } }
		} satisfies VMCPConfiguration);
		await expect.element(page.getByText('This server has already been configured.')).toBeVisible();
	});

	it('shows launch progress and an error when launch fails', async () => {
		const vmcp = configurableVMcp();
		const { configureInstance, launch } = mockConfigureAndLaunch(vmcp, {
			launchError: 'Component MCP server is not healthy'
		});

		await renderDialog(vmcp);
		await configureFromPreconfigure();

		await vi.waitFor(() => expect(launch).toHaveBeenCalledOnce());
		expect(configureInstance).toHaveBeenCalledOnce();
		await expect.element(page.getByText('vMCP Launch Failed')).toBeVisible();
		await expect
			.element(page.getByText('Component MCP server is not healthy', { exact: false }))
			.toBeVisible();
		await expect
			.element(page.getByRole('button', { name: 'Update Configuration and Try Again' }))
			.toBeVisible();
	});

	it('shows configured state and allows editing an existing instance', async () => {
		const vmcp = configurableVMcp();
		const existing: VMCPInstance = {
			id: 'vmcpi-existing',
			vmcpID: vmcp.id,
			userID: getProfileResponse.id,
			created: '2026-01-01T00:00:00Z',
			status: { configured: true }
		};
		worker.use(
			http.get('/api/vmcp-instances', () =>
				HttpResponse.json({
					items: [existing]
				})
			)
		);

		await renderDialog(vmcp, existing);
		await expect
			.element(page.getByText('This will begin the initial setup process for this server.'))
			.not.toBeVisible();
		await expect.element(page.getByText('This server has already been configured.')).toBeVisible();
		await page.getByRole('button', { name: 'Edit configuration' }).click();
		await expect.element(page.getByCSS('input[name="API token"]')).toBeVisible();
	});

	it('prompts for OAuth after launch and continues when authentication completes', async () => {
		const vmcp = createVMCP({ id: 'vmcp1oauth', displayName: 'OAuth vMCP' });
		const { launch, oauthChecks } = mockConfigureAndLaunch(vmcp, {
			oauthURL: (check) => (check === 1 ? 'https://auth.example.com/authorize' : '')
		});

		await renderDialog(vmcp);
		await page.getByRole('button', { name: 'Preconfigure server' }).click();
		await continueFromIntro();

		await vi.waitFor(() => expect(launch).toHaveBeenCalledOnce());
		await expect.element(page.getByRole('link', { name: 'Authenticate' })).toBeVisible();
		await expect.element(page.getByCSS('#connect-to-vmcp-dialog')).not.toBeVisible();

		document.dispatchEvent(new Event('visibilitychange'));

		await vi.waitFor(() => {
			expect(oauthChecks()).toBe(2);
		});
		await expect.element(page.getByCSS('#connect-to-vmcp-dialog')).toBeVisible();
	});

	it('clears OAuth and prompts to authenticate from Reauthenticate', async () => {
		const vmcp = createVMCP({ id: 'vmcp1reauth', displayName: 'Remote vMCP' }, [
			createMCPCatalogEntry({ id: 'entry-remote', name: 'Remote', runtime: 'remote' })
		]);
		const existing: VMCPInstance = {
			id: 'vmcpi-existing',
			vmcpID: vmcp.id,
			userID: getProfileResponse.id,
			created: '2026-01-01T00:00:00Z',
			status: { configured: true }
		};
		const cleared = vi.fn();
		worker.use(
			http.get('/api/vmcp-instances', () => HttpResponse.json({ items: [existing] })),
			http.delete(`/api/vmcps/${vmcp.id}/oauth`, () => {
				cleared();
				return HttpResponse.json({});
			}),
			http.get(`/api/vmcps/${vmcp.id}/oauth-url`, () =>
				HttpResponse.json({ oauthURL: 'https://auth.example.com/authorize' })
			)
		);

		await renderDialog(vmcp, existing);
		await expect.element(page.getByRole('button', { name: 'Reauthenticate' })).toBeVisible();
		await expect
			.element(page.getByRole('button', { name: 'Edit configuration' }))
			.not.toBeInTheDocument();
		await page.getByRole('button', { name: 'Reauthenticate' }).click();

		await vi.waitFor(() => expect(cleared).toHaveBeenCalledOnce());
		await expect.element(page.getByRole('link', { name: 'Authenticate' })).toBeVisible();
	});
});
