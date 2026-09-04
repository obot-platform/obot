import type { VMCP, VMCPConfiguration, VMCPInstance } from '$lib/services';
import { vmcpInstances } from '$lib/stores';
import { createVMCP } from '../../../tests/helpers/mcp';
import { preparePageData } from '../../../tests/helpers/pageData';
import { getProfileResponse } from '../../../tests/mocks/data';
import { worker } from '../../../tests/mocks/worker';
import ConnectVMcp from './ConnectVMcp.svelte';
import { http, HttpResponse } from 'msw';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

function configurableVMcp(): VMCP {
	const vmcp = createVMCP({ id: 'vmcp-configurable', displayName: 'Configured vMCP' });
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

	it('shows an intro dialog when opened without an instance', async () => {
		await renderDialog(configurableVMcp());
		await expect
			.element(page.getByText('This will begin the initial setup process for this server.'))
			.toBeVisible();
		await expect
			.element(
				page.getByText('Additional configuration details may also be required', { exact: false })
			)
			.toBeVisible();
		await expect.element(page.getByCSS('#connect-to-vmcp-dialog')).not.toBeVisible();
	});

	it('creates and configures an instance from Preconfigure', async () => {
		const vmcp = configurableVMcp();
		const createInstance = vi.fn();
		const configureInstance = vi.fn();
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
			})
		);

		await renderDialog(vmcp);
		await continueFromIntro();
		await page.getByRole('button', { name: 'Preconfigure server' }).click();
		const tokenField = page.getByCSS('input[name="API token"]');
		await expect.element(tokenField).toBeVisible();
		await tokenField.click();
		await tokenField.fill('secret-token');
		await page.getByRole('button', { name: 'Configure', exact: true }).click();

		await vi.waitFor(() => expect(configureInstance).toHaveBeenCalledOnce());
		expect(createInstance).toHaveBeenCalledWith({ vmcpID: vmcp.id });
		expect(configureInstance).toHaveBeenCalledWith({
			components: { 'component-entry-default': { API_TOKEN: 'secret-token' } }
		} satisfies VMCPConfiguration);
		await expect.element(page.getByText('This server has already been configured.')).toBeVisible();
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
});
