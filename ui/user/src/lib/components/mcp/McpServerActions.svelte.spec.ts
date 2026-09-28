import { createMCPCatalogEntry } from '../../../tests/helpers/mcp';
import { preparePageData } from '../../../tests/helpers/pageData';
import { worker } from '../../../tests/mocks/worker';
import McpServerActions from './McpServerActions.svelte';
import { HttpResponse, http } from 'msw';
import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

vi.mock('$lib/url', async (importOriginal) => ({
	...(await importOriginal<typeof import('$lib/url')>()),
	goto: vi.fn()
}));

describe('MCP server OAuth setup link', () => {
	it('opens configured credentials with a clear action', async () => {
		await preparePageData();
		worker.use(
			http.get('/api/mcp-catalogs/default/entries/salesforce/oauth-credentials', () =>
				HttpResponse.json({ configured: true, clientID: 'salesforce-client' })
			)
		);
		const entry = createMCPCatalogEntry({
			id: 'salesforce',
			name: 'Salesforce',
			runtime: 'remote',
			oauthCredentialConfigured: true,
			manifest: { remoteConfig: { fixedURL: 'https://example.com/mcp', staticOAuthRequired: true } }
		});

		render(McpServerActions, { entry, promptOAuthConfig: true, hideActions: true });
		await expect.element(page.getByRole('button', { name: 'Clear Credentials' })).toBeVisible();
	});
});
