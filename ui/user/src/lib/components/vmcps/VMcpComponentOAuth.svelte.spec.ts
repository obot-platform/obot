import { Group } from '$lib/services';
import { createMCPCatalogEntry, createVMCPComponent } from '../../../tests/helpers/mcp';
import { createMockProfile, preparePageData } from '../../../tests/helpers/pageData';
import { worker } from '../../../tests/mocks/worker';
import VMcpComponentOAuth from './VMcpComponentOAuth.svelte';
import { http, HttpResponse } from 'msw';
import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

const entry = createMCPCatalogEntry({
	id: 'remote',
	name: 'Remote server',
	runtime: 'remote',
	manifest: { config: [] }
});
const component = createVMCPComponent(entry);
const url = `/api/vmcps/vmcp-test/components/${component.id}/oauth-credentials`;

describe('component OAuth configuration', () => {
	it('clears only the component override and returns to inherited credentials', async () => {
		await preparePageData();
		const cleared = vi.fn();
		const changed = vi.fn();
		worker.use(
			http.get(url, () =>
				HttpResponse.json({ configured: true, clientID: 'local-client', source: 'component' })
			),
			http.delete(url, () => {
				cleared();
				return HttpResponse.json({
					configured: true,
					clientID: 'catalog-client',
					source: 'catalog'
				});
			})
		);
		render(VMcpComponentOAuth, { vmcpID: 'vmcp-test', component, onChanged: changed });
		await page.getByRole('button', { name: 'Configure Static OAuth', exact: true }).click();
		await expect
			.element(page.getByLabelText('Client ID', { exact: true }))
			.toHaveValue('local-client');
		await page.getByRole('button', { name: 'Clear Credentials', exact: true }).click();
		await expect
			.element(page.getByText('Clear this component’s OAuth override?', { exact: false }))
			.toBeVisible();
		await page.getByRole('button', { name: "Yes, I'm sure", exact: true }).click();
		await vi.waitFor(() => expect(cleared).toHaveBeenCalledOnce());
		await vi.waitFor(() => expect(changed).toHaveBeenCalledOnce());
	});
	it('lets an admin override inherited credentials without editing the catalog', async () => {
		await preparePageData();
		const saved = vi.fn();
		const changed = vi.fn();
		worker.use(
			http.get(url, () =>
				HttpResponse.json({ configured: true, clientID: 'catalog-client', source: 'catalog' })
			),
			http.post(url, async ({ request }) => {
				saved(await request.json());
				return HttpResponse.json({
					configured: true,
					clientID: 'local-client',
					source: 'component'
				});
			})
		);
		render(VMcpComponentOAuth, { vmcpID: 'vmcp-test', component, onChanged: changed });
		await page.getByRole('button', { name: 'Configure Static OAuth', exact: true }).click();
		await expect
			.element(page.getByText('Currently using catalog client catalog-client.', { exact: false }))
			.toBeVisible();
		await page.getByLabelText('Client ID', { exact: true }).fill('local-client');
		await page.getByLabelText('Client Secret (optional)', { exact: true }).fill('local-secret');
		await page.getByRole('button', { name: 'Save', exact: true }).click();
		await vi.waitFor(() =>
			expect(saved).toHaveBeenCalledWith({ clientID: 'local-client', clientSecret: 'local-secret' })
		);
		await vi.waitFor(() => expect(changed).toHaveBeenCalledOnce());
	});

	it('allows public clients and keeps errors visible for retry', async () => {
		await preparePageData();
		worker.use(
			http.get(url, () => HttpResponse.json({ configured: false, source: 'none' })),
			http.post(url, () =>
				HttpResponse.json({ message: 'Credential storage unavailable' }, { status: 500 })
			)
		);
		render(VMcpComponentOAuth, { vmcpID: 'vmcp-test', component });
		await page.getByRole('button', { name: 'Configure Static OAuth', exact: true }).click();
		await page.getByLabelText('Client ID', { exact: true }).fill('public-client');
		await page.getByRole('button', { name: 'Save', exact: true }).click();
		await expect
			.element(page.getByText('Credential storage unavailable', { exact: false }))
			.toBeVisible();
		await expect
			.element(page.getByLabelText('Client ID', { exact: true }))
			.toHaveValue('public-client');
	});

	for (const groups of [[Group.USER], [Group.POWERUSER], [Group.AUDITOR]]) {
		it(`hides credential management from ${groups[0]}`, async () => {
			await preparePageData({ profile: createMockProfile(groups) });
			render(VMcpComponentOAuth, { vmcpID: 'vmcp-test', component });
			await expect
				.element(page.getByRole('button', { name: 'Configure Static OAuth' }))
				.not.toBeInTheDocument();
		});
	}

	it('hides management for Git-managed or read-only definitions', async () => {
		await preparePageData();
		render(VMcpComponentOAuth, { vmcpID: 'vmcp-test', component, disabled: true });
		await expect
			.element(page.getByRole('button', { name: 'Configure Static OAuth' }))
			.not.toBeInTheDocument();
	});
});
