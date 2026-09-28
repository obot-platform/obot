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
