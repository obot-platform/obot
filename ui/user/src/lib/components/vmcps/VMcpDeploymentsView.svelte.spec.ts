import { Group, type VMCPInstance } from '$lib/services';
import { errors } from '$lib/stores';
import { createMCPCatalogEntry, createVMCP } from '../../../tests/helpers/mcp';
import { createMockProfile, preparePageData } from '../../../tests/helpers/pageData';
import { getProfileResponse } from '../../../tests/mocks/data';
import { worker } from '../../../tests/mocks/worker';
import VMcpDeploymentsView from './VMcpDeploymentsView.svelte';
import { http, HttpResponse } from 'msw';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

function configurableVMcp() {
	const entry = createMCPCatalogEntry({ id: 'entry-default', name: 'Everything' });
	const vmcp = createVMCP({ id: 'vmcp-1', displayName: 'Everything vMCP' }, [entry]);
	vmcp.components![0].configuration = [{ key: 'API_TOKEN', policy: 'userAllowed' }];
	vmcp.components![0].catalogEntry.manifest.config = [
		{
			key: 'API_TOKEN',
			name: 'API token',
			description: 'Token',
			required: true,
			sensitive: true,
			value: '',
			usage: 'env'
		}
	];
	return vmcp;
}

describe('VMcpDeploymentsView.svelte', () => {
	afterEach(() => {
		errors.items = [];
	});

	it('lets the instance owner edit configuration when required fields are missing', async () => {
		const vmcp = configurableVMcp();
		const mine: VMCPInstance = {
			id: 'vmcpi-mine',
			vmcpID: vmcp.id,
			userID: getProfileResponse.id,
			created: '2026-01-01T00:00:00Z',
			status: { missingRequiredConfiguration: ['component-entry-default.API_TOKEN'] }
		};
		const getFullVMcp = vi.fn(() => HttpResponse.json(vmcp));
		const listed = structuredClone(vmcp);
		for (const component of listed.components) {
			delete component.catalogEntry.manifest.config;
		}
		worker.use(
			http.get(`/api/vmcps/${vmcp.id}`, getFullVMcp),
			http.get('/api/vmcp-instances', () => HttpResponse.json({ items: [mine] })),
			http.post('/api/vmcp-instances/vmcpi-mine/reveal', () =>
				HttpResponse.json({ components: {} })
			)
		);
		await preparePageData({ profile: createMockProfile([Group.ADMIN]) });

		render(VMcpDeploymentsView, {
			vmcps: [listed],
			usersMap: new Map()
		});

		await expect.element(page.getByText('Everything vMCP').first()).toBeVisible();
		await page.getByRole('button', { name: 'Actions for Everything vMCP' }).first().click();
		await page.getByRole('button', { name: 'Edit Configuration', exact: true }).click();
		const tokenField = page.getByCSS('input[name="API token"]');
		await expect.element(tokenField).toBeVisible();
		await expect.element(tokenField).toHaveAttribute('type', 'password');
		expect(getFullVMcp).toHaveBeenCalledOnce();
	});

	it('reports a load failure instead of opening configuration without sensitivity metadata', async () => {
		const vmcp = configurableVMcp();
		const mine: VMCPInstance = {
			id: 'vmcpi-mine',
			vmcpID: vmcp.id,
			userID: getProfileResponse.id,
			created: '2026-01-01T00:00:00Z',
			status: { missingRequiredConfiguration: ['component-entry-default.API_TOKEN'] }
		};
		const reveal = vi.fn(() =>
			HttpResponse.json({
				components: { 'component-entry-default': { API_TOKEN: 'saved-token' } }
			})
		);
		const listed = structuredClone(vmcp);
		for (const component of listed.components) {
			delete component.catalogEntry.manifest.config;
		}
		worker.use(
			http.get(`/api/vmcps/${vmcp.id}`, () => new HttpResponse(null, { status: 500 })),
			http.get('/api/vmcp-instances', () => HttpResponse.json({ items: [mine] })),
			http.post('/api/vmcp-instances/vmcpi-mine/reveal', reveal)
		);
		await preparePageData({ profile: createMockProfile([Group.ADMIN]) });

		render(VMcpDeploymentsView, {
			vmcps: [listed],
			usersMap: new Map()
		});

		await page.getByRole('button', { name: 'Actions for Everything vMCP' }).first().click();
		await page.getByRole('button', { name: 'Edit Configuration', exact: true }).click();
		await vi.waitFor(() =>
			expect(errors.items.map((item) => item.message)).toContain('Failed to load vMCP.')
		);
		await expect.element(page.getByCSS('input[name="API token"]')).not.toBeInTheDocument();
		await expect.element(page.getByCSS('input[name="API_TOKEN"]')).not.toBeInTheDocument();
		expect(reveal).not.toHaveBeenCalled();
	});
});
