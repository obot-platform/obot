import { createMCPCatalogEntry, createVMCPComponent } from '../../../tests/helpers/mcp';
import { preparePageData } from '../../../tests/helpers/pageData';
import { worker } from '../../../tests/mocks/worker';
import VMcpToolsSetup from './VMcpToolsSetup.svelte';
import { http, HttpResponse } from 'msw';
import { untrack } from 'svelte';
import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

const entry = createMCPCatalogEntry({
	id: 'preview-entry',
	name: 'Preview server',
	manifest: {
		config: [
			{
				key: 'TOKEN',
				name: 'API token',
				description: 'Discovery credential',
				required: true,
				sensitive: true,
				value: '',
				usage: 'header'
			},
			{
				key: 'REGION',
				name: 'Region',
				description: '',
				required: true,
				sensitive: false,
				value: '',
				usage: 'env',
				options: [{ name: 'West', value: 'west' }]
			},
			{
				key: 'FIXED',
				name: 'Fixed credential',
				description: '',
				required: true,
				sensitive: true,
				value: '',
				usage: 'env'
			}
		]
	}
});
const component = createVMCPComponent(entry, {
	configuration: [
		{ key: 'TOKEN', policy: 'userAllowed' },
		{ key: 'REGION', policy: 'userAllowed' },
		{ key: 'FIXED', policy: 'fixed' }
	]
});
const previewURL = `/api/vmcps/vmcp-preview/components/${component.id}/generate-tool-previews`;

async function openSetup() {
	await preparePageData();
	const result = await render(VMcpToolsSetup, { component, vmcpID: 'vmcp-preview', refresh: true });
	untrack(() => result.component.open());
	await expect.element(page.getByLabelText('API token', { exact: false })).toBeVisible();
	return result;
}

async function fillConfiguration() {
	await page.getByLabelText('API token', { exact: false }).fill('preview-secret');
	await page.getByRole('combobox', { name: 'Region', exact: false }).selectOptions('West');
}

describe('VMcpToolsSetup preview credentials', () => {
	it('requests a hostname-constrained server URL for discovery and OAuth', async () => {
		const remoteEntry = createMCPCatalogEntry({
			id: entry.id,
			name: 'Remote server',
			runtime: 'remote',
			manifest: { remoteConfig: { hostname: '*.example.com' } }
		});
		const preview = vi.fn();
		const oauth = vi.fn();
		worker.use(
			http.post(previewURL, async ({ request }) => {
				preview(await request.json());
				return HttpResponse.json(
					{ message: 'MCP server requires OAuth authentication' },
					{ status: 400 }
				);
			}),
			http.post(`${previewURL}/oauth-url`, async ({ request }) => {
				oauth(await request.json());
				return HttpResponse.json({ oauthURL: 'https://oauth.example/authorize' });
			})
		);
		await preparePageData();
		const result = await render(VMcpToolsSetup, {
			component: createVMCPComponent(remoteEntry),
			vmcpID: 'vmcp-preview',
			refresh: true
		});
		untrack(() => result.component.open());
		await expect.element(page.getByLabelText('Server URL', { exact: false })).toBeVisible();
		await expect.element(page.getByText('URL must have hostname *.example.com')).toBeVisible();
		await expect.element(page.getByRole('button', { name: 'Configure Tools' })).toBeDisabled();
		expect(preview).not.toHaveBeenCalled();
		await page
			.getByLabelText('Server URL', { exact: false })
			.fill('https://tenant.example.com/mcp');
		await page.getByRole('button', { name: 'Configure Tools' }).click();
		await expect.element(page.getByRole('link', { name: 'Authenticate' })).toBeVisible();
		expect(preview).toHaveBeenCalledWith({ __url: 'https://tenant.example.com/mcp' });
		expect(oauth).toHaveBeenCalledWith({ __url: 'https://tenant.example.com/mcp' });
	});

	it.each([
		{ usage: 'file' as const, sensitive: true },
		{ usage: 'dynamicFile' as const, sensitive: true },
		{ usage: 'file' as const, sensitive: false },
		{ usage: 'dynamicFile' as const, sensitive: false }
	])(
		'preserves multiline $usage contents (sensitive: $sensitive)',
		async ({ usage, sensitive }) => {
			const fileEntry = createMCPCatalogEntry({
				id: entry.id,
				name: 'File server',
				manifest: {
					config: [{ ...entry.manifest.config![0], usage, sensitive }]
				}
			});
			const preview = vi.fn();
			worker.use(
				http.post(previewURL, async ({ request }) => {
					preview(await request.json());
					return HttpResponse.json(fileEntry);
				})
			);
			await preparePageData();
			const result = await render(VMcpToolsSetup, {
				component: createVMCPComponent(fileEntry, {
					configuration: [{ key: 'TOKEN', policy: 'userAllowed' }]
				}),
				vmcpID: 'vmcp-preview'
			});
			untrack(() => result.component.open());
			const contents =
				'-----BEGIN PRIVATE KEY-----\nfirst-line\nsecond-line\n-----END PRIVATE KEY-----\n';
			await page.getByLabelText('API token', { exact: false }).fill(contents);
			await page.getByRole('button', { name: 'Configure Tools' }).click();
			await vi.waitFor(() => expect(preview).toHaveBeenCalledWith({ TOKEN: contents }));
		}
	);

	it('collects required values before refreshing and sends them through OAuth retries', async () => {
		const preview = vi.fn();
		const oauth = vi.fn();
		worker.use(
			http.post(previewURL, async ({ request }) => {
				preview(await request.json());
				if (preview.mock.calls.length === 1) {
					return HttpResponse.json(
						{ message: 'MCP server requires OAuth authentication' },
						{ status: 400 }
					);
				}
				return HttpResponse.json({
					...entry,
					manifest: {
						...entry.manifest,
						toolPreview: [{ id: 'search', name: 'search', description: 'Search' }]
					}
				});
			}),
			http.post(`${previewURL}/oauth-url`, async ({ request }) => {
				oauth(await request.json());
				return HttpResponse.json({ oauthURL: 'https://oauth.example/authorize' });
			})
		);
		const result = await openSetup();
		await expect.element(page.getByRole('button', { name: 'Configure Tools' })).toBeDisabled();
		await expect.element(page.getByLabelText('Fixed credential')).not.toBeInTheDocument();
		expect(preview).not.toHaveBeenCalled();
		await fillConfiguration();
		await page.getByRole('button', { name: 'Configure Tools' }).click();
		await expect.element(page.getByRole('link', { name: 'Authenticate' })).toBeVisible();
		const payload = { TOKEN: 'preview-secret', REGION: 'west' };
		expect(preview).toHaveBeenCalledWith(payload);
		expect(oauth).toHaveBeenCalledWith(payload);
		document.dispatchEvent(new Event('visibilitychange'));
		await expect.element(page.getByText('search', { exact: true }).first()).toBeVisible();
		expect(preview).toHaveBeenLastCalledWith(payload);
		result.component.close();
		untrack(() => result.component.open());
		await expect.element(page.getByLabelText('API token', { exact: false })).toHaveValue('');
	});

	it('allows retry after a preview error and clears credentials when cancelled', async () => {
		const preview = vi.fn();
		worker.use(
			http.post(previewURL, async ({ request }) => {
				preview(await request.json());
				return HttpResponse.json({ message: 'Invalid credentials' }, { status: 400 });
			})
		);
		const result = await openSetup();
		await fillConfiguration();
		await page.getByRole('button', { name: 'Configure Tools' }).click();
		await expect.element(page.getByRole('alert')).toHaveTextContent('Invalid credentials');
		await expect.element(page.getByRole('button', { name: 'Configure Tools' })).toBeEnabled();
		await page.getByLabelText('API token', { exact: false }).fill('corrected-secret');
		await page.getByRole('button', { name: 'Configure Tools' }).click();
		await vi.waitFor(() => expect(preview).toHaveBeenCalledTimes(2));
		expect(preview).toHaveBeenLastCalledWith({
			TOKEN: 'corrected-secret',
			REGION: 'west'
		});
		result.component.close();
		untrack(() => result.component.open());
		await expect.element(page.getByLabelText('API token', { exact: false })).toHaveValue('');
		await expect.element(page.getByRole('alert')).not.toBeInTheDocument();
	});
});
