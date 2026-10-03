import { createMCPCatalogEntry } from '../../../tests/helpers/mcp';
import { preparePageData } from '../../../tests/helpers/pageData';
import { worker } from '../../../tests/mocks/worker';
import McpServerEntryForm from './McpServerEntryForm.svelte';
import { HttpResponse, http } from 'msw';
import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

describe('MCP server entry OAuth management', () => {
	it('shows configured credentials and allows an administrator to clear them', async () => {
		await preparePageData();
		const entry = createMCPCatalogEntry({
			id: 'salesforce',
			name: 'Salesforce',
			runtime: 'remote',
			oauthCredentialConfigured: true,
			manifest: { remoteConfig: { fixedURL: 'https://example.com/mcp', staticOAuthRequired: true } }
		});
		const deleted = vi.fn();
		worker.use(
			http.get('/api/mcp-catalogs/default/entries/salesforce/oauth-credentials', () =>
				HttpResponse.json({ configured: true, clientID: 'salesforce-client' })
			),
			http.delete('/api/mcp-catalogs/default/entries/salesforce/oauth-credentials', () => {
				deleted();
				return new HttpResponse(null, { status: 204 });
			})
		);

		render(McpServerEntryForm, { entry, id: 'default', entity: 'catalog', type: 'remote' });
		await expect.element(page.getByText('OAuth credentials configured')).toBeVisible();
		await page.getByRole('button', { name: 'Manage OAuth Credentials' }).click();
		await expect.element(page.getByRole('button', { name: 'Clear Credentials' })).toBeVisible();
		await page.getByRole('button', { name: 'Clear Credentials' }).click();
		await page.getByRole('button', { name: "Yes, I'm sure" }).click();
		await vi.waitFor(() => expect(deleted).toHaveBeenCalledOnce());
		await expect.element(page.getByText('Requires OAuth Config')).toBeVisible();
	});
});

it('resumes the same temporary tool preview after CLI authentication', async () => {
	await preparePageData();
	const entry = createMCPCatalogEntry({
		id: 'local-preview',
		name: 'Local Preview',
		runtime: 'remote',
		manifest: { remoteConfig: { fixedURL: 'https://mcp.example', localhostCallbackEnabled: true } }
	});
	const preview = vi.fn();
	const oauth = vi.fn();
	const path = '/api/mcp-catalogs/default/entries/local-preview/generate-tool-previews';
	worker.use(
		http.post(path, async ({ request }) => {
			preview(await request.json());
			if (preview.mock.calls.length === 1)
				return HttpResponse.json(
					{ message: 'MCP server requires OAuth authentication' },
					{ status: 400 }
				);
			return HttpResponse.json({
				...entry,
				manifest: {
					...entry.manifest,
					toolPreview: [{ id: 'search', name: 'search', description: 'Search' }]
				}
			});
		}),
		http.post(path + '/oauth-url', async ({ request }) => {
			oauth(await request.json());
			return HttpResponse.json({
				oauthURL: 'https://obot.example/oauth/mcp/login/catalog-preview'
			});
		})
	);
	render(McpServerEntryForm, {
		entry,
		id: 'default',
		entity: 'catalog',
		type: 'remote',
		isDialogView: true
	});
	await page.getByRole('button', { name: 'Tools', exact: true }).click();
	await page.getByRole('button', { name: 'Populate Tool Preview' }).click();
	await expect
		.element(page.getByLabelText('Authentication command'))
		.toHaveTextContent(
			"obot mcp login --url 'https://obot.example/oauth/mcp/login/catalog-preview'"
		);
	await expect
		.element(page.getByRole('link', { name: 'Authenticate', exact: true }))
		.not.toBeInTheDocument();
	await page.getByRole('button', { name: 'Continue', exact: true }).click();
	await expect.element(page.getByText('search', { exact: true }).first()).toBeVisible();
	expect(preview).toHaveBeenCalledTimes(2);
	expect(oauth.mock.calls[0]).toEqual(preview.mock.calls[0]);
	expect(preview.mock.calls[1]).toEqual(preview.mock.calls[0]);
});
