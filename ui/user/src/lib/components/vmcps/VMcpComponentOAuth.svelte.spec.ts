import { m } from '$lib/i18n';
import { Group } from '$lib/services';
import { createMCPCatalogEntry, createVMCPComponent } from '../../../tests/helpers/mcp';
import { createMockProfile, preparePageData } from '../../../tests/helpers/pageData';
import { worker } from '../../../tests/mocks/worker';
import VMcpComponentConfigurationDialog from './VMcpComponentConfigurationDialog.svelte';
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

async function openConfiguration(onNext = vi.fn(), readonly = false, gitManaged = false) {
	const result = await render(VMcpComponentConfigurationDialog, {
		vmcpID: 'vmcp-test',
		component,
		onNext,
		readonly,
		gitManaged
	});
	result.component.open(entry, { submitLabel: m.core_save() });
	return result;
}
async function openAdvanced() {
	await page.getByRole('checkbox', { name: 'Advanced', exact: true }).click();
}

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
		await openConfiguration(changed);
		await openAdvanced();
		await expect
			.element(page.getByLabelText('Client ID', { exact: true }))
			.toHaveValue('local-client');
		await page.getByRole('checkbox', { name: 'Static OAuth', exact: true }).click();
		await expect.element(page.getByLabelText('Client ID', { exact: true })).toBeVisible();
		await expect.element(page.getByLabelText('Client ID', { exact: true })).toBeDisabled();
		await expect
			.element(page.getByText('Clear this component’s OAuth override?', { exact: false }))
			.toBeVisible();
		await page.getByRole('button', { name: 'Save', exact: true }).click();
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
		await openConfiguration(changed);
		await openAdvanced();
		await expect
			.element(page.getByText('Currently using catalog client catalog-client.', { exact: false }))
			.toBeVisible();
		await expect.element(page.getByLabelText('Client ID', { exact: true })).toBeVisible();
		await expect.element(page.getByLabelText('Client ID', { exact: true })).toBeDisabled();
		await expect
			.element(page.getByLabelText('Client Secret (optional)', { exact: true }))
			.toBeDisabled();
		await page.getByRole('checkbox', { name: 'Static OAuth', exact: true }).click();
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
		await openConfiguration();
		await openAdvanced();
		await page.getByRole('checkbox', { name: 'Static OAuth', exact: true }).click();
		await page.getByLabelText('Client ID', { exact: true }).fill('public-client');
		await page.getByRole('button', { name: 'Save', exact: true }).click();
		await expect
			.element(page.getByText('Credential storage unavailable', { exact: false }))
			.toBeVisible();
		await expect
			.element(page.getByLabelText('Client ID', { exact: true }))
			.toHaveValue('public-client');
	});

	it.each([
		{ name: 'secret rotation', clientID: 'existing', clientSecret: 'rotated-secret' },
		{ name: 'client replacement', clientID: 'replacement', clientSecret: 'replacement-secret' },
		{ name: 'public client replacement', clientID: 'public-client', clientSecret: '' }
	])('saves $name without deleting the override first', async ({ clientID, clientSecret }) => {
		await preparePageData();
		const saved = vi.fn();
		const deleted = vi.fn();
		const onNext = vi.fn();
		worker.use(
			http.get(url, () =>
				HttpResponse.json({ configured: true, clientID: 'existing', source: 'component' })
			),
			http.post(url, async ({ request }) => {
				saved(await request.json());
				return HttpResponse.json({ configured: true, clientID, source: 'component' });
			}),
			http.delete(url, () => {
				deleted();
				return HttpResponse.json({ configured: false, source: 'none' });
			})
		);
		await openConfiguration(onNext);
		await openAdvanced();
		await expect.element(page.getByLabelText('Client ID', { exact: true })).toHaveValue('existing');
		await expect
			.element(
				page.getByText(
					'When changing the Client ID, also enter its secret, or leave it blank for a public client.',
					{ exact: false }
				)
			)
			.toBeVisible();
		await page.getByLabelText('Client ID', { exact: true }).fill(clientID);
		await page.getByLabelText('Client Secret (optional)', { exact: true }).fill(clientSecret);
		await page.getByRole('button', { name: 'Save', exact: true }).click();
		await vi.waitFor(() => expect(onNext).toHaveBeenCalledOnce());
		expect(saved).toHaveBeenCalledExactlyOnceWith({ clientID, clientSecret });
		expect(deleted).not.toHaveBeenCalled();
	});

	it('keeps existing credentials when saving unchanged', async () => {
		await preparePageData();
		const changed = vi.fn();
		worker.use(
			http.get(url, () =>
				HttpResponse.json({ configured: true, clientID: 'existing', source: 'component' })
			),
			http.post(url, () => {
				changed();
				return HttpResponse.json({});
			}),
			http.delete(url, () => {
				changed();
				return HttpResponse.json({});
			})
		);
		await openConfiguration();
		await openAdvanced();
		await expect.element(page.getByLabelText('Client ID', { exact: true })).toHaveValue('existing');
		await page.getByRole('button', { name: 'Save', exact: true }).click();
		expect(changed).not.toHaveBeenCalled();
	});

	it('does not write credentials when edits are cancelled', async () => {
		await preparePageData();
		const changed = vi.fn();
		worker.use(
			http.get(url, () => HttpResponse.json({ configured: false, source: 'none' })),
			http.post(url, () => {
				changed();
				return HttpResponse.json({});
			})
		);
		await openConfiguration();
		await openAdvanced();
		await expect
			.element(page.getByRole('checkbox', { name: 'Static OAuth', exact: true }))
			.toBeEnabled();
		await page.getByRole('checkbox', { name: 'Static OAuth', exact: true }).click();
		await page.getByLabelText('Client ID', { exact: true }).fill('unsaved');
		await page.getByRole('button', { name: 'Cancel', exact: true }).click();
		expect(changed).not.toHaveBeenCalled();
	});

	for (const groups of [[Group.USER], [Group.POWERUSER], [Group.AUDITOR]]) {
		it(`hides credential management from ${groups[0]}`, async () => {
			await preparePageData({ profile: createMockProfile(groups) });
			await openConfiguration();
			await expect
				.element(page.getByRole('checkbox', { name: 'Static OAuth', exact: true }))
				.not.toBeInTheDocument();
		});
	}

	it.each(['readonly', 'gitManaged'])('hides management for %s definitions', async (mode) => {
		await preparePageData();
		await openConfiguration(vi.fn(), mode === 'readonly', mode === 'gitManaged');
		await expect
			.element(page.getByRole('checkbox', { name: 'Static OAuth', exact: true }))
			.not.toBeInTheDocument();
	});
});
